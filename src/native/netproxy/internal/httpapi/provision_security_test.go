package httpapi

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/androidplatform"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
)

func TestProvisionUnauthenticatedRequestBoundary(t *testing.T) {
	s := resourceFixture(t)
	t.Cleanup(s.stopEvents)
	calls := 0
	s.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
		calls++
		return nil, nil
	})
	for _, tc := range []struct {
		name, method, query, code string
		headers                   http.Header
		status                    int
	}{
		{"valid_bearer", "POST", "", "api.provision_invalid", http.Header{"Authorization": {"Bearer " + s.config.Secret}}, 403},
		{"wrong_bearer", "POST", "", "api.provision_invalid", http.Header{"Authorization": {"Bearer wrong"}}, 403},
		{"empty_authorization", "POST", "", "api.provision_invalid", http.Header{"Authorization": {""}}, 403},
		{"duplicate_authorization", "POST", "", "api.provision_invalid", http.Header{"Authorization": {"", ""}}, 403},
		{"same_origin", "POST", "", "api.provision_invalid", http.Header{"Origin": {"https://" + s.config.Listen}}, 403},
		{"foreign_origin", "POST", "", "api.origin_invalid", http.Header{"Origin": {"https://evil.test"}}, 403},
		{"opaque_origin", "POST", "", "api.origin_invalid", http.Header{"Origin": {"null"}}, 403},
		{"duplicate_origin", "POST", "", "api.origin_invalid", http.Header{"Origin": {"", ""}}, 403},
		{"target_query", "POST", "?package=com.evil.app", "api.provision_invalid", nil, 403},
		{"encoded_secret_query", "POST", "?%53eCrEt=fixture", "api.query_invalid", nil, 400},
		{"duplicate_query", "POST", "?user_id=0&user_id=0", "api.query_invalid", nil, 400},
		{"malformed_query", "POST", "?bad=%zz", "api.query_invalid", nil, 400},
		{"get", "GET", "", "api.unauthorized", nil, 401},
		{"head", "HEAD", "", "api.unauthorized", nil, 401},
		{"preflight", "OPTIONS", "", "api.unauthorized", nil, 401},
		{"put", "PUT", "", "api.unauthorized", nil, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/api/v1/auth/provision"+tc.query, nil)
			r.Host = s.config.Listen
			r.Header = tc.headers.Clone()
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			var result resourceResponse
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != tc.status || result.Schema != 1 || result.OK || result.Code != tc.code {
				t.Fatalf("unexpected response: %d %s (%v)", w.Code, w.Body.String(), err)
			}
			if calls != 0 || !s.provisionAfter.IsZero() || s.provisionBackoff != 0 {
				t.Fatal("rejected request invoked platform or consumed cooldown")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") == "*" {
				t.Fatal("unsafe response caching or CORS")
			}
		})
	}
}

func TestProvisionBodyValidationReleasesAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
		unknownLength           bool
		status                  int
	}{
		{"empty", "", "", false, 200},
		{"empty_object", `{}`, "application/json", false, 200},
		{"chunked_object", `{}`, "application/json", true, 200},
		{"charset", `{}`, "application/json; charset=utf-8", false, 200},
		{"no_content_type", `{}`, "", false, 400},
		{"form", `package=com.evil.app`, "application/x-www-form-urlencoded", false, 400},
		{"target_override", `{"connection":{"secret":"fixture"}}`, "application/json", false, 400},
		{"trailing_document", `{} {}`, "application/json", false, 400},
		{"malformed", `{`, "application/json", false, 400},
		{"whitespace", " ", "application/json", true, 400},
		{"limit", `{}` + strings.Repeat(" ", 1022), "application/json", true, 200},
		{"too_large_known", `{}` + strings.Repeat(" ", 1023), "application/json", false, 413},
		{"too_large_chunked", `{}` + strings.Repeat(" ", 1023), "application/json", true, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := resourceFixture(t)
			t.Cleanup(s.stopEvents)
			calls := 0
			s.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
				calls++
				return nil, nil
			})
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/provision", strings.NewReader(tc.body))
			r.Host = s.config.Listen
			r.Header.Set("Content-Type", tc.contentType)
			if tc.unknownLength {
				r.ContentLength = -1
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
			if tc.status == 200 {
				if calls != 1 {
					t.Fatal("valid request did not invoke platform once")
				}
				return
			}
			if calls != 0 || !s.provisionAfter.IsZero() || s.provisionBackoff != 0 {
				t.Fatal("invalid body invoked platform or consumed cooldown")
			}
			if next := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", ""); next.Code != 200 || calls != 1 {
				t.Fatal("invalid body retained provision admission", next.Code)
			}
		})
	}
}

func TestProvisionResponseNeverForwardsPlatformPayload(t *testing.T) {
	s := resourceFixture(t)
	t.Cleanup(s.stopEvents)
	for _, failing := range []bool{false, true} {
		s.provisionAfter = time.Time{}
		s.platform = platformFunc(func(_ context.Context, request androidplatform.Request) (jsontext.Value, error) {
			if request.Operation != "provision" || request.Connection == nil || *request.Connection != s.config.Connection() {
				t.Fatal("provision did not use server-owned connection")
			}
			payload, err := json.Marshal(request.Connection, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			if failing {
				return payload, &contract.Error{Code: "android.query_failed", Message: "Platform unavailable"}
			}
			return payload, nil
		})
		w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", "")
		if (!failing && w.Code != 200) || (failing && w.Code != 500) {
			t.Fatal("unexpected status", w.Code)
		}
		var result resourceResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Schema != 1 || result.OK == failing {
			t.Fatal("invalid envelope", err)
		}
		if !failing && (result.Code != "api.provisioned" || strings.TrimSpace(string(result.Data)) != `{"delivered":true}`) {
			t.Fatal("success returned more than delivery acknowledgement", w.Body.String())
		}
		for _, secret := range []string{s.config.Secret, s.config.Connection().Address, "certificate_sha256", "PRIVATE KEY"} {
			if strings.Contains(w.Body.String(), secret) || strings.Contains(w.Header().Get("Location"), secret) {
				t.Fatal("response exposed private connection material")
			}
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("provision response can be cached")
		}
	}
}

func TestProvisionStalledInvalidBodyDoesNotBlockDeliveryOrChangeCooldown(t *testing.T) {
	s := resourceFixture(t)
	t.Cleanup(s.stopEvents)
	calls := 0
	s.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
		calls++
		return nil, nil
	})
	body := &provisionBlockedBody{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(body.release) }) }
	defer release()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/provision", body)
	request.Host = s.config.Listen
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.ServeHTTP(response, request)
	}()
	t.Cleanup(func() { release(); <-done })
	select {
	case <-body.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provision did not begin reading body")
	}
	if valid := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", ""); valid.Code != 200 || calls != 1 {
		t.Fatal("stalled invalid body blocked legitimate delivery", valid.Code)
	}
	after, backoff := s.provisionAfter, s.provisionBackoff
	if remaining := time.Until(after); remaining <= 0 || remaining > provisionCooldown || backoff != 0 {
		t.Fatal("successful delivery did not establish normal cooldown")
	}
	release()
	<-done
	if response.Code != 400 || calls != 1 || !s.provisionAfter.Equal(after) || s.provisionBackoff != backoff {
		t.Fatal("invalid body changed delivery or cooldown state", response.Code)
	}
	if !s.provisionMu.TryLock() {
		t.Fatal("body read failure retained provision lock")
	}
	s.provisionMu.Unlock()
	if next := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", ""); next.Code != 409 || calls != 1 || !s.provisionAfter.Equal(after) || s.provisionBackoff != backoff {
		t.Fatal("subsequent request bypassed or extended cooldown", next.Code)
	}
}

type provisionBlockedBody struct {
	entered chan struct{}
	release chan struct{}
}

func (b *provisionBlockedBody) Read([]byte) (int, error) {
	close(b.entered)
	<-b.release
	return 0, io.ErrUnexpectedEOF
}
