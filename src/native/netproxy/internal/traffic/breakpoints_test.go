package traffic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

type breakpointTransport func(*http.Request) (*http.Response, error)

func (roundTrip breakpointTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return roundTrip(r)
}

func nextPause(t *testing.T, c *Controller, phase string) PausedSummary {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		paused, changed := c.Paused()
		for _, p := range paused {
			if p.Phase == phase {
				return p
			}
		}
		select {
		case <-changed:
		case <-timer.C:
			t.Fatalf("no %s pause", phase)
		}
	}
}

func TestBreakpointActions(t *testing.T) {
	for _, action := range []string{"edited", "aborted", "timed_out", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			c := New(paths.New(t.TempDir()))
			c.session = &session{ctx: t.Context(), controller: c, scriptSlots: make(chan struct{}, 1)}
			err := c.ReplaceBreakpoints(BreakpointConfig{TimeoutSeconds: 1, Rules: []BreakpointRule{
				{Domains: []string{"example.test"}, Pattern: "/old|/new", Phase: "both", Name: "fixture"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "http://example.test/old", strings.NewReader("original request"))
			req.Header.Set("Content-Type", "text/plain")
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				c.session.proxyHTTP(w, req, time.Now(), breakpointTransport(func(r *http.Request) (*http.Response, error) {
					data, _ := io.ReadAll(r.Body)
					if action == "edited" && (r.URL.Path != "/new" || r.Method != "PUT" || string(data) != "changed request") {
						t.Errorf("wrong edited request: %s %s %q", r.Method, r.URL, data)
					}
					if action != "edited" && string(data) != "original request" {
						t.Errorf("original body changed: %q", data)
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/plain"}},
						Body: io.NopCloser(strings.NewReader("origin")), ContentLength: 6}, nil
				}), flow{}, 1, nil)
			}()
			pause := nextPause(t, c, "request")
			detail, ok := c.PausedDetail(pause.ID)
			if !ok || detail.Body == nil || *detail.Body != "original request" {
				t.Fatalf("unexpected pause %+v", detail)
			}
			switch action {
			case "edited":
				method, target, body := "PUT", "http://example.test/new", "changed request"
				if err := c.ResolveBreakpoint(pause.ID, &BreakpointEdit{Method: &method, URL: &target, Body: &body}, false); err != nil {
					t.Fatal(err)
				}
				response := nextPause(t, c, "response")
				status, responseBody := 202, "changed response"
				if err := c.ResolveBreakpoint(response.ID, &BreakpointEdit{Status: &status, Body: &responseBody}, false); err != nil {
					t.Fatal(err)
				}
			case "aborted":
				if err := c.ResolveBreakpoint(pause.ID, nil, true); err != nil {
					t.Fatal(err)
				}
			case "timed_out":
				response := nextPause(t, c, "response")
				if err := c.ResolveBreakpoint(response.ID, nil, false); err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				c.DrainBreakpoints(ctx)
			}
			<-done
			if err := c.ResolveBreakpoint(pause.ID, nil, false); !errors.Is(err, ErrBreakpointGone) {
				t.Fatalf("stale pause accepted: %v", err)
			}
			entries := c.Entries()
			if len(entries) != 1 {
				t.Fatalf("expected one audit record, got %d", len(entries))
			}
			record, ok := c.Record(entries[0].ID)
			if !ok || len(record.HTTP.Breakpoints) == 0 || record.HTTP.Breakpoints[0].Outcome != action {
				t.Fatalf("incorrect audit: %+v", record.HTTP)
			}
			if action == "edited" && (w.Code != http.StatusAccepted || w.Body.String() != "changed response" ||
				len(record.HTTP.Breakpoints) != 2 || record.HTTP.Breakpoints[1].Outcome != "edited") ||
				action == "aborted" && w.Code != http.StatusBadGateway ||
				(action == "timed_out" || action == "shutdown") && (w.Code != 200 || w.Body.String() != "origin") {
				t.Fatalf("incorrect response: %d %q", w.Code, w.Body)
			}
		})
	}
}

func TestBreakpointLimitAndDomainValidation(t *testing.T) {
	c := New(paths.New(t.TempDir()))
	s := &session{ctx: t.Context(), controller: c}
	config := BreakpointConfig{Rules: []BreakpointRule{{Phase: "request", Pattern: "/"}}}
	if c.ReplaceBreakpoints(config) == nil {
		t.Fatal("domain requirement was not enforced")
	}
	config.Rules[0].Domains = []string{"sub.example.test"}
	if err := c.ReplaceBreakpoints(config); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= maxPaused; i++ {
		_, timeout, matched := s.reserveBreakpoint("request", "sub.example.test", "https://sub.example.test/")
		if matched != (i < maxPaused) || matched && timeout != time.Minute {
			t.Fatalf("unexpected match at %d: %t, %s", i, matched, timeout)
		}
	}
}
