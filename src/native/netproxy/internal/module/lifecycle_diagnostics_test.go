package module

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReadinessDiagnosticsKeepStatusNotResponseBody(t *testing.T) {
	if !serviceProcessAlive(os.Getpid()) {
		t.Skip("readiness probe requires procfs")
	}
	for _, test := range []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, "service.api_auth"},
		{http.StatusServiceUnavailable, "service.api_unavailable"},
	} {
		t.Run(test.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "private-response-marker", test.status)
			}))
			defer server.Close()
			_, err := waitForServiceReady(t.Context(), Options{ServiceAddress: server.URL}, os.Getpid(), 200*time.Millisecond, 0)
			if err == nil {
				t.Fatal("unusable endpoint reported ready")
			}
			code, reason := operationFailure(err)
			if code != test.code || !strings.Contains(reason, "HTTP "+strconv.Itoa(test.status)) || strings.Contains(reason, "private-response-marker") {
				t.Fatalf("unexpected probe diagnostics: %s %s", code, reason)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := waitForServiceReady(ctx, Options{}, os.Getpid(), time.Second, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
}

func TestCoreLogEvidenceAndFailureState(t *testing.T) {
	options := newTestOptions(t.TempDir())
	if err := ensureLifecycleStateDir(options); err != nil {
		t.Fatal(err)
	}
	path, _ := LogFile(options, "core")
	if err := os.WriteFile(path, []byte("FATAL[0000] older-attempt-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkpoint := checkpointCoreLog(path)
	cause := errors.New("sing-box exited before ready")
	if err := checkpoint.failure(path, cause); err != cause {
		t.Fatalf("previous failure included: %v", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("+0800 2026-11-25 12:00:00 FATAL invalid outbound; url=https://example.invalid/diagnostic-secret-marker\n")
	if err = errors.Join(err, file.Close()); err != nil {
		t.Fatal(err)
	}
	failure := checkpoint.failure(path, cause)
	err = failServiceStart(options, 0, 0, "service.ready_failed", "", failure)
	if !errors.Is(err, cause) {
		t.Fatalf("original error lost: %v", err)
	}
	state, err := ReadServiceState(options.StateFile)
	if err != nil || state.State != "failed" || !strings.Contains(state.Error, "invalid outbound") {
		t.Fatalf("failure summary not persisted: %+v, %v", state, err)
	}
	log, err := ReadLog(options, "service", 10)
	if err != nil || len(log.Entries) != 1 || log.Entries[0].ErrorCode != "service.core_fatal" || !strings.Contains(log.Content, "invalid outbound") {
		t.Fatalf("diagnostic not shown in service log: %+v, %v", log, err)
	}
	for _, marker := range []string{"older-attempt-marker", "diagnostic-secret-marker"} {
		if strings.Contains(state.Error+log.Content, marker) {
			t.Fatalf("unexpected marker in service diagnostics: %s", marker)
		}
	}
	if err := os.WriteFile(path, []byte("FATAL[0000] different-attempt-after-clear\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.failure(path, cause); err != cause {
		t.Fatalf("log was replaced by a truncation: %v", err)
	}
}

func TestCheckOutputIsBounded(t *testing.T) {
	var capture configCheckOutput
	data := []byte(strings.Repeat("x", lifecycleEvidenceLimit+1024))
	if n, err := io.Copy(&capture, io.LimitReader(bytes.NewReader(data), int64(len(data)))); err != nil || n != int64(len(data)) || capture.buffer.Len() != lifecycleEvidenceLimit {
		t.Fatalf("bounded capture len=%d, consumed=%d, err=%v", capture.buffer.Len(), n, err)
	}
}
