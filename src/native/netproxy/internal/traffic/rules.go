package traffic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"regexp/syntax"
	"strings"
	"time"

	"github.com/dop251/goja"
	"golang.org/x/net/http/httpguts"
)

const (
	MaxRulesBytes = 256 << 10
	maxBody       = 256 << 10
)

type Rule struct {
	Pattern string   `json:"pattern"`
	Phase   string   `json:"phase"`
	Script  string   `json:"script"`
	Domains []string `json:"domains"`
	Enabled *bool    `json:"enabled,omitempty"`
}

func (rule Rule) enabled() bool { return rule.Enabled == nil || *rule.Enabled }

func (rule Rule) copyEnabled() Rule {
	enabled := rule.enabled()
	rule.Enabled = &enabled
	return rule
}

type compiledRule struct {
	Rule
	id      string
	pattern *regexp.Regexp
	program *goja.Program
}

type scriptInput struct {
	URL    string  `json:"url"`
	Method string  `json:"method"`
	Status int     `json:"status,omitzero"`
	Body   *string `json:"body,omitempty"`
}

type rewrite struct {
	SetHeaders    map[string]string `json:"set_headers"`
	RemoveHeaders []string          `json:"remove_headers"`
	Body          *string           `json:"body"`
	Redirect      string            `json:"redirect"`
	Reject        bool              `json:"reject"`
}

func compileRules(rules []Rule, legacy bool) ([]compiledRule, error) {
	if len(rules) > 32 {
		return nil, fmt.Errorf("最多允许 32 条重写规则")
	}
	compiled := make([]compiledRule, 0, len(rules))
	for i, rule := range rules {
		if (rule.Phase != "request" && rule.Phase != "response") || len(rule.Pattern) == 0 ||
			len(rule.Pattern) > 1024 || len(rule.Script) == 0 || len(rule.Script) > 8192 {
			return nil, fmt.Errorf("第 %d 条重写规则的阶段、表达式或脚本长度无效", i+1)
		}
		if legacy && rule.Domains == nil {
			rule.Domains = legacyDomains(rule.Pattern)
		}
		if (!legacy && len(rule.Domains) == 0) || len(rule.Domains) > 32 {
			return nil, fmt.Errorf("第 %d 条重写规则需要 1–32 个域名", i+1)
		}
		domains := make([]string, 0, len(rule.Domains))
		seen := make(map[string]bool)
		for _, value := range rule.Domains {
			host := canonicalHost(strings.TrimSpace(value))
			if !validMITMHost(host) {
				return nil, fmt.Errorf("第 %d 条重写规则的域名无效", i+1)
			}
			if !seen[host] {
				seen[host] = true
				domains = append(domains, host)
			}
		}
		rule.Domains = domains
		var pattern *regexp.Regexp
		var program *goja.Program
		if rule.enabled() {
			var err error
			pattern, err = regexp.Compile(rule.Pattern)
			if err != nil {
				return nil, fmt.Errorf("第 %d 条重写规则的 URL 正则表达式无效", i+1)
			}
			program, err = goja.Compile("rewrite.js", "(function (input) {\n"+rule.Script+"\n})", true)
			if err != nil {
				return nil, fmt.Errorf("第 %d 条重写规则的 JavaScript 语法无效", i+1)
			}
		}
		sum := sha256.Sum256([]byte(rule.Phase + "\x00" + rule.Pattern + "\x00" + rule.Script + "\x00" + strings.Join(domains, "\x00")))
		compiled = append(compiled, compiledRule{rule.copyEnabled(), hex.EncodeToString(sum[:]), pattern, program})
	}
	return compiled, nil
}

func legacyDomains(pattern string) []string {
	expression, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil || expression.Op != syntax.OpConcat || len(expression.Sub) < 2 ||
		expression.Sub[0].Op != syntax.OpBeginText || expression.Sub[1].Op != syntax.OpLiteral {
		return nil
	}
	prefix := string(expression.Sub[1].Rune)
	_, address, ok := strings.Cut(prefix, "://")
	if !ok || !(strings.HasPrefix(prefix, "http://") || strings.HasPrefix(prefix, "https://") ||
		strings.HasPrefix(prefix, "ws://") || strings.HasPrefix(prefix, "wss://")) {
		return nil
	}
	end := strings.IndexAny(address, ":/?")
	if end < 0 {
		return nil
	}
	host := canonicalHost(address[:end])
	if !validMITMHost(host) {
		return nil
	}
	return []string{host}
}

func indexRules(rules []compiledRule) map[string][]int {
	index := make(map[string][]int)
	for i, rule := range rules {
		if !rule.enabled() {
			continue
		}
		for _, domain := range rule.Domains {
			index[domain] = append(index[domain], i)
		}
	}
	return index
}

func (rule *compiledRule) run(ctx context.Context, input scriptInput) (rewrite, string) {
	vm := goja.New()
	vm.SetMaxCallStackSize(256)
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	defer context.AfterFunc(ctx, func() { vm.Interrupt("rewrite deadline") })()
	raw, err := json.Marshal(input)
	if err != nil {
		return rewrite{}, "script_error"
	}
	jsonObject := vm.Get("JSON").ToObject(vm)
	parse, _ := goja.AssertFunction(jsonObject.Get("parse"))
	stringify, _ := goja.AssertFunction(jsonObject.Get("stringify"))
	arg, err := parse(jsonObject, vm.ToValue(string(raw)))
	if err != nil {
		return rewrite{}, "script_error"
	}
	fn, err := vm.RunProgram(rule.program)
	if err != nil {
		return rewrite{}, "script_error"
	}
	call, ok := goja.AssertFunction(fn)
	if !ok {
		return rewrite{}, "script_error"
	}
	value, err := call(goja.Undefined(), arg)
	if err != nil {
		if ctx.Err() != nil {
			return rewrite{}, "deadline"
		}
		return rewrite{}, "script_error"
	}
	if goja.IsUndefined(value) || goja.IsNull(value) {
		return rewrite{}, ""
	}
	encoded, err := stringify(jsonObject, value)
	if err != nil || goja.IsUndefined(encoded) {
		if ctx.Err() != nil {
			return rewrite{}, "deadline"
		}
		return rewrite{}, "invalid_result"
	}
	text := encoded.String()
	var result *rewrite
	if len(text) > 2*maxBody || json.Unmarshal([]byte(text), &result, json.RejectUnknownMembers(true)) != nil ||
		result == nil || !result.valid(rule.Phase) {
		return rewrite{}, "invalid_result"
	}
	return *result, ""
}

func (result rewrite) valid(phase string) bool {
	if len(result.SetHeaders)+len(result.RemoveHeaders) > 32 ||
		(result.Body != nil && len(*result.Body) > maxBody) ||
		(phase != "request" && (result.Redirect != "" || result.Reject)) ||
		(result.Redirect != "" && result.Reject) {
		return false
	}
	if result.Redirect != "" {
		target, err := url.Parse(result.Redirect)
		if err != nil || !validTarget(target) || len(result.Redirect) > 4096 {
			return false
		}
	}
	for name, value := range result.SetHeaders {
		if !editableHeader(name) || !httpguts.ValidHeaderFieldValue(value) || len(value) > 4096 {
			return false
		}
	}
	for _, name := range result.RemoveHeaders {
		if !editableHeader(name) {
			return false
		}
	}
	return true
}

func editableHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "connection", "content-length", "content-encoding", "expect", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
		return false
	}
	return len(name) <= 128 && httpguts.ValidHeaderFieldName(name)
}

func applyHeaders(headers http.Header, result rewrite) {
	for _, name := range result.RemoveHeaders {
		headers.Del(name)
	}
	for name, value := range result.SetHeaders {
		headers.Set(name, value)
	}
}
