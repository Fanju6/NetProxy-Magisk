package traffic

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
	Pattern string `json:"pattern"`
	Phase   string `json:"phase"`
	Script  string `json:"script"`
}

type compiledRule struct {
	Rule
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

func compileRules(rules []Rule) ([]compiledRule, error) {
	if len(rules) > 32 {
		return nil, fmt.Errorf("最多允许 32 条重写规则")
	}
	compiled := make([]compiledRule, 0, len(rules))
	for i, rule := range rules {
		if (rule.Phase != "request" && rule.Phase != "response") || len(rule.Pattern) == 0 ||
			len(rule.Pattern) > 1024 || len(rule.Script) == 0 || len(rule.Script) > 8192 {
			return nil, fmt.Errorf("第 %d 条重写规则的阶段、表达式或脚本长度无效", i+1)
		}
		pattern, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("第 %d 条重写规则的 URL 正则表达式无效", i+1)
		}
		program, err := goja.Compile("rewrite.js", "(function (input) {\n"+rule.Script+"\n})", true)
		if err != nil {
			return nil, fmt.Errorf("第 %d 条重写规则的 JavaScript 语法无效", i+1)
		}
		compiled = append(compiled, compiledRule{rule, pattern, program})
	}
	return compiled, nil
}

func findRule(rules []compiledRule, phase, target string) *compiledRule {
	for i := range rules {
		if rules[i].Phase == phase && rules[i].pattern.MatchString(target) {
			return &rules[i]
		}
	}
	return nil
}

func (rule *compiledRule) run(ctx context.Context, input scriptInput) (rewrite, bool) {
	vm := goja.New()
	vm.SetMaxCallStackSize(256)
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	defer context.AfterFunc(ctx, func() { vm.Interrupt("rewrite deadline") })()
	raw, err := json.Marshal(input)
	if err != nil {
		return rewrite{}, false
	}
	jsonObject := vm.Get("JSON").ToObject(vm)
	parse, _ := goja.AssertFunction(jsonObject.Get("parse"))
	stringify, _ := goja.AssertFunction(jsonObject.Get("stringify"))
	arg, err := parse(jsonObject, vm.ToValue(string(raw)))
	if err != nil {
		return rewrite{}, false
	}
	fn, err := vm.RunProgram(rule.program)
	if err != nil {
		return rewrite{}, false
	}
	call, ok := goja.AssertFunction(fn)
	if !ok {
		return rewrite{}, false
	}
	value, err := call(goja.Undefined(), arg)
	if err != nil {
		return rewrite{}, false
	}
	if goja.IsUndefined(value) || goja.IsNull(value) {
		return rewrite{}, true
	}
	encoded, err := stringify(jsonObject, value)
	if err != nil || goja.IsUndefined(encoded) {
		return rewrite{}, false
	}
	text := encoded.String()
	var result *rewrite
	if len(text) > 2*maxBody || json.Unmarshal([]byte(text), &result, json.RejectUnknownMembers(true)) != nil ||
		result == nil || !result.valid(rule.Phase) {
		return rewrite{}, false
	}
	return *result, true
}

func (result rewrite) valid(phase string) bool {
	if len(result.SetHeaders)+len(result.RemoveHeaders) > 32 ||
		(result.Body != nil && (phase != "response" || len(*result.Body) > maxBody)) ||
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
