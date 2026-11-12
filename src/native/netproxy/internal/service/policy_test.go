package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestPolicyOperationsDoNotReplayUnconfirmedWrites(t *testing.T) {
	const identity = 1700000000123
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"state":"ready","pid":123,"core_started_at_millis":1700000000123}`), 0o600); err != nil {
		t.Fatal(err)
	}
	withServiceProcess(t, 123)
	var selects, delays atomic.Int32
	appendField := func(data []byte, field protowire.Number, value string) []byte {
		return protowire.AppendString(protowire.AppendTag(data, field, protowire.BytesType), value)
	}
	member := appendField(nil, 1, "np/example/node")
	group := appendField(nil, 1, "Custom")
	group = appendField(group, 2, "selector")
	group = protowire.AppendVarint(protowire.AppendTag(group, 3, protowire.VarintType), 1)
	group = appendField(group, 4, "direct")
	group = protowire.AppendBytes(protowire.AppendTag(group, 6, protowire.BytesType), member)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload []byte
		switch r.URL.Path {
		case "/daemon.StartedService/GetStartedAt":
			payload = protowire.AppendVarint(protowire.AppendTag(nil, 1, protowire.VarintType), identity)
		case "/daemon.StartedService/SubscribeGroups":
			if selects.Load() != 0 {
				http.Error(w, "private fixture error", http.StatusServiceUnavailable)
				return
			}
			payload = protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), group)
		case "/daemon.StartedService/SelectOutbound":
			selects.Add(1)
		case "/daemon.StartedService/URLTest":
			delays.Add(1)
		default:
			t.Errorf("unexpected RPC %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeServiceAPIFrame(t, w, payload)
	}))
	defer server.Close()
	options := Options{StateFile: path, ServiceAddress: server.URL, RequestTimeout: time.Second}
	target := PolicyTarget{GroupTag: "Custom", CoreStartedAtMillis: identity}
	if err := RequestPolicyDelay(t.Context(), options, target); err != nil || delays.Load() != 1 {
		t.Fatalf("expected one accepted async delay request, got %d: %v", delays.Load(), err)
	}
	if _, err := SelectPolicyGroup(t.Context(), options, target, "not-a-member"); err == nil || selects.Load() != 0 {
		t.Fatalf("invalid member reached write: %v, calls %d", err, selects.Load())
	}
	_, err := SelectPolicyGroup(t.Context(), options, target, "np/example/node")
	var operation *Error
	if !errors.As(err, &operation) || operation.Code != "policy.outcome_unknown" || strings.Contains(err.Error(), "private fixture") || selects.Load() != 1 {
		t.Fatalf("unconfirmed selection was retried, leaked, or reported as confirmed: %v, calls %d", err, selects.Load())
	}
}
