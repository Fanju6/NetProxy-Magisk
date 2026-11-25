package traffic

import (
	"net/http"
	"slices"
	"sort"
	"strings"
)

type RewriteMatch struct {
	ID             string         `json:"id"`
	RuleIndex      int            `json:"rule_index"`
	Pattern        string         `json:"pattern"`
	Phase          string         `json:"phase"`
	State          string         `json:"state"`
	Actions        []string       `json:"actions,omitempty"`
	Redirect       string         `json:"redirect,omitempty"`
	OriginalStatus int            `json:"original_status,omitzero"`
	Headers        []HeaderChange `json:"headers,omitempty"`
	Body           *BodyChange    `json:"body,omitempty"`
	Error          string         `json:"error,omitempty"`
	DurationUS     int64          `json:"duration_us"`
}

type HeaderChange struct {
	Name      string   `json:"name"`
	Before    []string `json:"before,omitempty"`
	After     []string `json:"after,omitempty"`
	Truncated bool     `json:"truncated"`
}

type BodyChange struct {
	Before int `json:"before"`
	After  int `json:"after"`
}

func matchingRules(rules []compiledRule, target string) (matches []RewriteMatch, request, response int) {
	request, response = -1, -1
	for i := range rules {
		rule := &rules[i]
		if !rule.pattern.MatchString(target) {
			continue
		}
		first := &request
		if rule.Phase == "response" {
			first = &response
		}
		state := "shadowed"
		if *first < 0 {
			*first = len(matches)
			state = "not_reached"
		}
		matches = append(matches, RewriteMatch{ID: rule.id, RuleIndex: i, Pattern: rule.Pattern, Phase: rule.Phase, State: state})
	}
	return
}

func (m *RewriteMatch) trackHeaders(headers http.Header, result rewrite) func() {
	before := make(http.Header)
	capture := func(name string) {
		name = http.CanonicalHeaderKey(name)
		before[name] = slices.Clone(headers[name])
	}
	for name := range result.SetHeaders {
		capture(name)
	}
	for _, name := range result.RemoveHeaders {
		capture(name)
	}
	if result.Body != nil {
		capture("Content-Length")
		for _, name := range bodyValidators {
			capture(name)
		}
	}
	return func() {
		names := make([]string, 0, len(before))
		for name := range before {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			after := headers[name]
			if slices.Equal(before[name], after) {
				continue
			}
			change := HeaderChange{Name: name}
			change.Before = auditValues(before[name], &change.Truncated)
			change.After = auditValues(after, &change.Truncated)
			m.Headers = append(m.Headers, change)
		}
		if len(m.Headers) > 0 {
			m.Actions = append(m.Actions, "headers")
		}
	}
}

func auditValues(values []string, truncated *bool) []string {
	var result []string
	budget := 384
	for _, value := range values {
		if budget <= 0 {
			*truncated = true
			break
		}
		prefix := validPrefix(value, budget)
		*truncated = *truncated || len(prefix) < len(value)
		result = append(result, strings.Clone(prefix))
		budget -= len(prefix) + 16
	}
	return result
}

func (m *RewriteMatch) finish() bool {
	switch {
	case m.Error != "":
		m.State = "error"
	case len(m.Actions) == 0:
		m.State = "no_change"
	default:
		m.State = "applied"
	}
	return m.State == "applied"
}

func (m RewriteMatch) size() int {
	n := 256 + len(m.ID) + len(m.Pattern) + len(m.Redirect)
	for _, h := range m.Headers {
		n += 128 + len(h.Name)
		for _, value := range h.Before {
			n += 24 + len(value)
		}
		for _, value := range h.After {
			n += 24 + len(value)
		}
	}
	return n
}
