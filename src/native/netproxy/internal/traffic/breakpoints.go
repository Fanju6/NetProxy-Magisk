package traffic

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const maxPaused = 16

var (
	ErrBreakpointGone = errors.New("断点已结束或不存在")
	ErrBreakpointEdit = errors.New("断点修改无效")
)

type BreakpointRule struct {
	Domains []string `json:"domains"`
	Pattern string   `json:"pattern"`
	Phase   string   `json:"phase"`
	Enabled *bool    `json:"enabled,omitempty"`
	Name    string   `json:"name,omitempty"`
}

type BreakpointConfig struct {
	TimeoutSeconds int              `json:"timeout_seconds"`
	Rules          []BreakpointRule `json:"rules"`
}

type PausedStream struct {
	ID       string      `json:"id"`
	Rule     string      `json:"rule,omitempty"`
	Phase    string      `json:"phase"`
	URL      string      `json:"url"`
	Method   string      `json:"method"`
	Status   int         `json:"status,omitempty"`
	Headers  http.Header `json:"headers"`
	Body     *string     `json:"body,omitempty"`
	Deadline time.Time   `json:"deadline"`
}

type PausedSummary struct {
	ID       string    `json:"id"`
	Phase    string    `json:"phase"`
	URL      string    `json:"url"`
	Method   string    `json:"method"`
	Deadline time.Time `json:"deadline"`
}

type BreakpointEdit struct {
	Method  *string     `json:"method,omitempty"`
	URL     *string     `json:"url,omitempty"`
	Status  *int        `json:"status,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	Body    *string     `json:"body,omitempty"`
}

type BreakpointAudit struct {
	Rule          string   `json:"rule,omitempty"`
	Phase         string   `json:"phase"`
	Outcome       string   `json:"outcome"`
	Actor         string   `json:"actor,omitempty"`
	Changes       []string `json:"changes,omitempty"`
	OriginChanged bool     `json:"origin_changed,omitempty"`
	OriginalExit  *Exit    `json:"original_exit,omitempty"`
	ActualExit    *Exit    `json:"actual_exit,omitempty"`
	RouteChanged  bool     `json:"route_changed,omitempty"`
	Unattributed  bool     `json:"unattributed,omitempty"`
	RouteFailure  string   `json:"route_failure,omitempty"`
}

type breakpointRule struct {
	BreakpointRule
	regex *regexp.Regexp
}

type breakpointDecision struct {
	outcome string
	edit    *BreakpointEdit
}

type breakpointPause struct {
	PausedStream
	owner  *session
	result chan breakpointDecision
	done   <-chan struct{}
}

type breakpointState struct {
	mu       sync.Mutex
	loaded   bool
	loadErr  error
	config   BreakpointConfig
	rules    []breakpointRule
	paused   map[string]*breakpointPause
	reserved int
	changed  chan struct{}
}

func compileBreakpoints(config BreakpointConfig) (BreakpointConfig, []breakpointRule, error) {
	if config.TimeoutSeconds == 0 {
		config.TimeoutSeconds = 60
	}
	if config.TimeoutSeconds < 1 || config.TimeoutSeconds > 120 || len(config.Rules) > 32 {
		return BreakpointConfig{}, nil, errors.New("断点配置最多允许 32 条规则，超时范围为 1–120 秒")
	}
	rules := make([]BreakpointRule, 0, len(config.Rules))
	compiled := make([]breakpointRule, 0, len(config.Rules))
	for i, rule := range config.Rules {
		rule.Name = strings.TrimSpace(rule.Name)
		if utf8.RuneCountInString(rule.Name) > 64 || len(rule.Pattern) == 0 || len(rule.Pattern) > 1024 ||
			(rule.Phase != "request" && rule.Phase != "response" && rule.Phase != "both") ||
			len(rule.Domains) < 1 || len(rule.Domains) > 32 {
			return BreakpointConfig{}, nil, fmt.Errorf("第 %d 条断点规则的标题、域名、阶段或表达式无效", i+1)
		}
		var domains []string
		for _, item := range rule.Domains {
			host := canonicalHost(strings.TrimSpace(item))
			if !validMITMHost(host) {
				return BreakpointConfig{}, nil, fmt.Errorf("第 %d 条断点规则的域名无效", i+1)
			}
			if !slices.Contains(domains, host) {
				domains = append(domains, host)
			}
		}
		rule.Domains = domains
		enabled := rule.Enabled == nil || *rule.Enabled
		rule.Enabled = &enabled
		regex, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return BreakpointConfig{}, nil, fmt.Errorf("第 %d 条断点规则的 URL 正则表达式无效", i+1)
		}
		rules = append(rules, rule)
		compiled = append(compiled, breakpointRule{rule, regex})
	}
	config.Rules = rules
	return config, compiled, nil
}

func (c *Controller) loadBreakpoints() {
	state := &c.breakpoints
	if state.loaded {
		return
	}
	state.loaded = true
	var config BreakpointConfig
	file, err := os.Open(c.layout.TrafficBreakpoints())
	if err == nil {
		content, readErr := io.ReadAll(io.LimitReader(file, MaxRulesBytes+1))
		_ = file.Close()
		if readErr != nil || len(content) > MaxRulesBytes || json.Unmarshal(content, &config) != nil {
			state.loadErr = errors.New("无法读取断点规则")
			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		state.loadErr = errors.New("无法读取断点规则")
		return
	}
	state.config, state.rules, state.loadErr = compileBreakpoints(config)
}

func (c *Controller) Breakpoints() (BreakpointConfig, error) {
	state := &c.breakpoints
	state.mu.Lock()
	defer state.mu.Unlock()
	c.loadBreakpoints()
	return state.config, state.loadErr
}

func (c *Controller) ReplaceBreakpoints(config BreakpointConfig) error {
	config, rules, err := compileBreakpoints(config)
	if err != nil {
		return err
	}
	data, err := json.Marshal(config)
	if err != nil || len(data) > MaxRulesBytes {
		return errors.New("断点规则超过大小限制")
	}
	state := &c.breakpoints
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := provider.WriteAtomic(c.layout.TrafficBreakpoints(), data, 0o600); err != nil {
		return errors.New("无法保存断点规则")
	}
	state.loaded, state.loadErr, state.config, state.rules = true, nil, config, rules
	return nil
}

func (s *session) reserveBreakpoint(phase, host, target string) (string, time.Duration, bool) {
	state := &s.controller.breakpoints
	state.mu.Lock()
	defer state.mu.Unlock()
	if s.drainingBreakpoints || state.reserved == maxPaused || s.ctx.Err() != nil {
		return "", 0, false
	}
	s.controller.loadBreakpoints()
	for _, rule := range state.rules {
		if *rule.Enabled && (rule.Phase == phase || rule.Phase == "both") &&
			matchesHostPatterns(rule.Domains, canonicalHost(host)) && rule.regex.MatchString(target) {
			state.reserved++
			name := rule.Name
			if name == "" {
				name = rule.Domains[0]
			}
			return name, time.Duration(state.config.TimeoutSeconds) * time.Second, true
		}
	}
	return "", 0, false
}

func (s *session) breakpointBudget() time.Duration {
	state := &s.controller.breakpoints
	state.mu.Lock()
	defer state.mu.Unlock()
	s.controller.loadBreakpoints()
	for _, rule := range state.rules {
		if *rule.Enabled {
			return time.Duration(state.config.TimeoutSeconds*2) * time.Second
		}
	}
	return 0
}

func (c *Controller) Paused() ([]PausedSummary, <-chan struct{}) {
	state := &c.breakpoints
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.changed == nil {
		state.changed = make(chan struct{})
	}
	result := make([]PausedSummary, 0, len(state.paused))
	for _, pause := range state.paused {
		result = append(result, PausedSummary{pause.ID, pause.Phase, pause.URL, pause.Method, pause.Deadline})
	}
	slices.SortFunc(result, func(a, b PausedSummary) int {
		if cmp := a.Deadline.Compare(b.Deadline); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	return result, state.changed
}

func (state *breakpointState) notify() {
	if state.changed != nil {
		close(state.changed)
	}
	state.changed = make(chan struct{})
}

func (c *Controller) PausedDetail(id string) (PausedStream, bool) {
	state := &c.breakpoints
	state.mu.Lock()
	defer state.mu.Unlock()
	if pause := state.paused[id]; pause != nil {
		return pause.PausedStream, true
	}
	return PausedStream{}, false
}

func (state *breakpointState) resolve(pause *breakpointPause, decision breakpointDecision) {
	delete(state.paused, pause.ID)
	pause.result <- decision
	state.notify()
}

func (c *Controller) ResolveBreakpoint(id string, edit *BreakpointEdit, abort bool) error {
	state := &c.breakpoints
	state.mu.Lock()
	defer state.mu.Unlock()
	pause := state.paused[id]
	if pause == nil {
		return ErrBreakpointGone
	}
	outcome := "continued"
	if abort {
		outcome = "aborted"
	} else if edit != nil {
		validated, err := validateBreakpointEdit(pause.PausedStream, *edit)
		if err != nil {
			return err
		}
		edit = &validated
		outcome = "edited"
	}
	state.resolve(pause, breakpointDecision{outcome, edit})
	return nil
}

func (s *session) pauseBreakpoint(ctx context.Context, view PausedStream, timeout time.Duration, done <-chan struct{}) breakpointDecision {
	state := &s.controller.breakpoints
	state.mu.Lock()
	if s.drainingBreakpoints || s.ctx.Err() != nil {
		state.reserved--
		state.mu.Unlock()
		return breakpointDecision{outcome: "shutdown"}
	}
	if state.paused == nil {
		state.paused = make(map[string]*breakpointPause)
	}
	view.ID, view.Deadline = rand.Text(), time.Now().Add(timeout).UTC()
	pause := &breakpointPause{view, s, make(chan breakpointDecision, 1), done}
	state.paused[view.ID] = pause
	state.notify()
	state.mu.Unlock()
	timer := time.NewTimer(time.Until(view.Deadline))
	defer timer.Stop()
	reason := "canceled"
	select {
	case decision := <-pause.result:
		state.mu.Lock()
		state.reserved--
		state.mu.Unlock()
		return decision
	case <-timer.C:
		reason = "timed_out"
	case <-ctx.Done():
	case <-s.ctx.Done():
		reason = "shutdown"
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.paused[view.ID] == pause {
		state.resolve(pause, breakpointDecision{outcome: reason})
	}
	state.reserved--
	return <-pause.result
}

func (c *Controller) DrainBreakpoints(ctx context.Context) {
	c.mu.Lock()
	s := c.session
	c.mu.Unlock()
	if s == nil {
		return
	}
	state := &c.breakpoints
	state.mu.Lock()
	s.drainingBreakpoints = true
	var pending []<-chan struct{}
	for _, pause := range state.paused {
		if pause.owner == s {
			pending = append(pending, pause.done)
			state.resolve(pause, breakpointDecision{outcome: "shutdown"})
		}
	}
	state.mu.Unlock()
	for _, done := range pending {
		select {
		case <-ctx.Done():
			return
		case <-done:
		}
	}
}
