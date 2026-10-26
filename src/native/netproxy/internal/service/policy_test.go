package service

import (
	"context"
	"encoding/binary"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"google.golang.org/protobuf/encoding/protowire"
)

const policyTestConfig = `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"Configured","outbounds":["Auto","direct"],"default":"direct"},{"type":"urltest","tag":"Auto","providers":["provider-a"]}],"route":{"final":"Configured"}}`

func policyWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func policyRequireCode(t *testing.T, err error, code string) {
	t.Helper()
	value, ok := errors.AsType[*Error](err)
	if !ok || value.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func TestPolicyConfiguredSummaryHasNoRuntimeOrCatalogState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	policyWriteFile(t, path, policyTestConfig)
	result, err := ReadPolicyGroups(t.Context(), Options{SingBoxConfig: path, ModuleConfig: filepath.Join(root, "missing-module.json"), CatalogRoot: filepath.Join(root, "missing-catalog")})
	if err != nil {
		t.Fatal(err)
	}
	want := PolicySnapshot{DefaultOutbound: "Configured", Source: "configuration", Groups: []serviceapi.Group{
		{Tag: "Configured", Type: "selector", Selectable: true, ConfiguredDefault: "direct", Items: []serviceapi.GroupItem{{Tag: "Auto", Type: "urltest"}, {Tag: "direct", Type: "direct"}}},
		{Tag: "Auto", Type: "urltest", UnavailableProviders: []string{"provider-a"}, Items: []serviceapi.GroupItem{}},
	}}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("summary = %+v, want %+v", result, want)
	}
	content, err := json.Marshal(result, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"groups":`, `"live":false`, `"core_started_at_millis":0`, `"default_outbound":"Configured"`} {
		if !strings.Contains(string(content), key) {
			t.Fatalf("missing %s: %s", key, content)
		}
	}
	for _, key := range []string{`"selected"`, `url_test_time`, `url_test_delay`, `selection`, `provider-a/`} {
		if strings.Contains(string(content), key) {
			t.Fatalf("invented runtime state %s: %s", key, content)
		}
	}
}

func TestPolicyConfiguredSummaryStrictJSONAndNativeFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, content := range []string{
		`null`, `[]`, `{"outbounds":[],"outbounds":[]}`, `{"route":{"final":"a","final":"b"}}`,
		`{"outbounds":[null]}`, `{"outbounds":[{"type":"selector","tag":""}]}`,
		`{"outbounds":[{"type":"selector","tag":"a","outbounds":7}]}`,
		`{"outbounds":[{"type":"urltest","tag":"a","interval":"invalid"}]}`,
		`{"outbounds":[{"type":"selector","tag":"a"},{"type":"urltest","tag":"a"}]}`,
		"{\"outbounds\":[{\"type\":\"selector\",\"tag\":\"\xff\"}]}",
	} {
		policyWriteFile(t, path, content)
		_, err := ReadConfiguredPolicyGroups(t.Context(), Options{SingBoxConfig: path})
		policyRequireCode(t, err, "policy.config_invalid")
	}
	policyWriteFile(t, path, `{"log":{"level":"info"},"outbounds":[{"type":"selector","tag":"Custom / 组","use_all_providers":true,"include":"same","interrupt_exist_connections":true,"custom_future_field":{}}],"route":{"final":"Custom / 组"}}`)
	result, err := ReadConfiguredPolicyGroups(t.Context(), Options{SingBoxConfig: path})
	if err != nil || len(result.Groups) != 1 || result.Groups[0].Tag != "Custom / 组" {
		t.Fatalf("native extensions blocked: %+v %v", result, err)
	}
}

type policyCoreFixture struct {
	mu            sync.Mutex
	options       Options
	groups        []serviceapi.Group
	started       int64
	selects       int
	delays        int
	reads         int
	failRead      bool
	failOperation bool
	ignoreSelect  bool
	onRead        func()
}

func policyString(payload []byte, field protowire.Number, value string) []byte {
	return protowire.AppendString(protowire.AppendTag(payload, field, protowire.BytesType), value)
}

func policyVarint(payload []byte, field protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(payload, field, protowire.VarintType), value)
}

func policyGroupsWire(groups []serviceapi.Group) []byte {
	var payload []byte
	for _, group := range groups {
		encoded := policyString(nil, 1, group.Tag)
		encoded = policyString(encoded, 2, group.Type)
		if group.Selectable {
			encoded = policyVarint(encoded, 3, 1)
		}
		encoded = policyString(encoded, 4, group.Selected)
		for _, item := range group.Items {
			member := policyString(nil, 1, item.Tag)
			member = policyString(member, 2, item.Type)
			member = policyVarint(member, 3, uint64(item.URLTestTime))
			member = policyVarint(member, 4, uint64(item.URLTestDelay))
			encoded = protowire.AppendBytes(protowire.AppendTag(encoded, 6, protowire.BytesType), member)
		}
		payload = protowire.AppendBytes(protowire.AppendTag(payload, 1, protowire.BytesType), encoded)
	}
	return payload
}

func newPolicyCoreFixture(t *testing.T) *policyCoreFixture {
	t.Helper()
	root := t.TempDir()
	f := &policyCoreFixture{started: 1700000000123, options: Options{
		StateFile: filepath.Join(root, "state.json"), SingBoxConfig: filepath.Join(root, "config.json"),
		ModuleConfig: filepath.Join(root, "module.json"), CatalogRoot: filepath.Join(root, "catalog"), RequestTimeout: time.Second,
	}, groups: []serviceapi.Group{
		{Tag: "Custom / 组", Type: "selector", Selectable: true, Selected: "provider-a/same", Items: []serviceapi.GroupItem{
			{Tag: "provider-a/same", Type: "socks"}, {Tag: "provider-b/same", Type: "socks", URLTestTime: 12, URLTestDelay: 34}}},
		{Tag: "NativeAuto", Type: "urltest", Selected: "provider-a/same", Items: []serviceapi.GroupItem{{Tag: "provider-a/same", Type: "socks"}}},
	}}
	policyWriteFile(t, f.options.SingBoxConfig, policyTestConfig)
	policyWriteFile(t, f.options.ModuleConfig, `{"do_not_read_or_write":"module sentinel"}`)
	f.writeState(t, f.started)
	original := serviceFindProcess
	serviceFindProcess = func(string, int) int { return 6543 }
	t.Cleanup(func() { serviceFindProcess = original })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var payload []byte
		switch r.URL.Path {
		case "/daemon.StartedService/GetStartedAt":
			payload = policyVarint(nil, 1, uint64(f.started))
		case "/daemon.StartedService/SubscribeGroups":
			f.reads++
			if f.failRead && f.selects+f.delays > 0 {
				http.Error(w, "private core exception", http.StatusServiceUnavailable)
				return
			}
			payload = policyGroupsWire(f.groups)
			if f.onRead != nil {
				f.onRead()
			}
		case "/daemon.StartedService/SelectOutbound", "/daemon.StartedService/URLTest":
			body, err := io.ReadAll(r.Body)
			if err != nil || len(body) < 5 || int(binary.BigEndian.Uint32(body[1:5])) != len(body)-5 {
				t.Error("invalid Service API request")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			values := map[protowire.Number]string{}
			for data := body[5:]; len(data) > 0; {
				key, kind, size := protowire.ConsumeTag(data)
				if size < 0 || kind != protowire.BytesType {
					t.Error("invalid protobuf request")
					return
				}
				value, count := protowire.ConsumeString(data[size:])
				if count < 0 {
					t.Error("invalid string request")
					return
				}
				values[key], data = value, data[size+count:]
			}
			if strings.HasSuffix(r.URL.Path, "/SelectOutbound") {
				f.selects++
				if values[1] != f.groups[0].Tag || values[2] != "provider-b/same" {
					t.Errorf("rewritten native tags: %v", values)
				}
				if !f.ignoreSelect {
					f.groups[0].Selected = values[2]
				}
			} else {
				f.delays++
				if values[1] != "NativeAuto" {
					t.Errorf("rewritten delay group: %v", values)
				}
			}
			if f.failOperation {
				http.Error(w, "private core exception", http.StatusServiceUnavailable)
				return
			}
		default:
			t.Errorf("unexpected RPC %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/grpc-web+proto")
		frame := make([]byte, 5)
		binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
		_, _ = w.Write(append(frame, payload...))
	}))
	t.Cleanup(server.Close)
	f.options.ServiceAddress = server.URL
	return f
}

func (f *policyCoreFixture) writeState(t *testing.T, identity int64) {
	t.Helper()
	content, err := json.Marshal(stateFile{State: "ready", PID: 6543, CoreStartedAtMillis: identity}, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	policyWriteFile(t, f.options.StateFile, string(content))
}

func TestPolicyLiveSnapshotUsesOnlyRealGroups(t *testing.T) {
	f := newPolicyCoreFixture(t)
	policyWriteFile(t, f.options.SingBoxConfig, `{"outbounds":[{"type":"selector","tag":"DoNotUseConfig","outbounds":false}],"route":{"final":"Configured"}}`)
	result, err := ReadPolicyGroups(t.Context(), f.options)
	if err != nil || !result.Live || result.CoreStartedAtMillis != f.started || result.DefaultOutbound != "Configured" || !reflect.DeepEqual(result.Groups, f.groups) {
		t.Fatalf("live result = %+v, %v", result, err)
	}
	result, err = SelectPolicyGroup(t.Context(), f.options, "Custom / 组", "provider-b/same")
	if err != nil || !result.Live || result.Groups[0].Selected != "provider-b/same" || f.selects != 1 {
		t.Fatalf("select result = %+v, %v, calls=%d", result, err, f.selects)
	}
	result, err = DelayPolicyGroup(t.Context(), f.options, "NativeAuto")
	if err != nil || !result.Live || !reflect.DeepEqual(result.Groups, f.groups) || f.delays != 1 {
		t.Fatalf("delay result = %+v, %v, calls=%d", result, err, f.delays)
	}
	content, err := os.ReadFile(f.options.ModuleConfig)
	if err != nil || string(content) != `{"do_not_read_or_write":"module sentinel"}` {
		t.Fatalf("module changed: %s %v", content, err)
	}
}

func TestPolicySelectValidatesRealGroupAndMembership(t *testing.T) {
	f := newPolicyCoreFixture(t)
	for _, test := range []struct{ group, member, code string }{
		{"Configured", "provider-b/same", "policy.group_not_found"},
		{"NativeAuto", "provider-a/same", "policy.not_selectable"},
		{"Custom / 组", "same", "policy.member_not_found"},
		{"Custom / 组", "provider-c/same", "policy.member_not_found"},
		{"", "provider-b/same", "policy.invalid"},
	} {
		_, err := SelectPolicyGroup(t.Context(), f.options, test.group, test.member)
		policyRequireCode(t, err, test.code)
	}
	_, err := DelayPolicyGroup(t.Context(), f.options, "provider-a/same")
	policyRequireCode(t, err, "policy.group_not_found")
	if f.selects != 0 || f.delays != 0 {
		t.Fatal("invalid request reached mutating RPC")
	}
}

func TestPolicyStoppedRejectsOperationsWithoutCoreCalls(t *testing.T) {
	f := newPolicyCoreFixture(t)
	policyWriteFile(t, f.options.StateFile, `{"state":"stopped"}`)
	_, err := SelectPolicyGroup(t.Context(), f.options, "Custom / 组", "provider-b/same")
	policyRequireCode(t, err, "policy.core_not_ready")
	_, err = DelayPolicyGroup(t.Context(), f.options, "NativeAuto")
	policyRequireCode(t, err, "policy.core_not_ready")
	if f.reads != 0 || f.selects != 0 || f.delays != 0 {
		t.Fatal("stopped operation contacted core")
	}
}

func TestPolicyRejectsStaleIdentityAndDropsOldGroups(t *testing.T) {
	for _, change := range []string{"initial", "during-groups", "state-change", "pid", "zero-identity"} {
		t.Run(change, func(t *testing.T) {
			f := newPolicyCoreFixture(t)
			switch change {
			case "initial":
				f.started++
			case "during-groups":
				f.onRead = func() { f.started++ }
			case "state-change":
				f.onRead = func() { f.writeState(t, f.started+1) }
			case "pid":
				serviceFindProcess = func(string, int) int { return 4567 }
			case "zero-identity":
				f.writeState(t, 0)
			}
			result, err := ReadPolicyGroups(t.Context(), f.options)
			if err != nil {
				t.Fatal(err)
			}
			if result.Live || result.CoreStartedAtMillis != 0 || result.Source != "configuration" {
				t.Fatalf("old groups leaked: %+v", result)
			}
			for _, group := range result.Groups {
				if group.Selected != "" || group.Tag == "Custom / 组" {
					t.Fatal("old runtime group or selected member survived")
				}
			}
			_, err = SelectPolicyGroup(t.Context(), f.options, "Custom / 组", "provider-b/same")
			policyRequireCode(t, err, "policy.core_not_ready")
		})
	}
}

func TestPolicyAcceptedOperationIsNeverRetried(t *testing.T) {
	for _, failure := range []string{"read", "confirm", "operation"} {
		t.Run(failure, func(t *testing.T) {
			f := newPolicyCoreFixture(t)
			f.failRead, f.ignoreSelect, f.failOperation = failure == "read", failure == "confirm", failure == "operation"
			_, err := SelectPolicyGroup(t.Context(), f.options, "Custom / 组", "provider-b/same")
			policyRequireCode(t, err, "policy.runtime_sync_failed")
			if f.selects != 1 || strings.Contains(err.Error(), "private core") {
				t.Fatalf("operation retried or leaked exception: calls=%d %v", f.selects, err)
			}
			if failure != "operation" {
				value, _ := errors.AsType[*Error](err)
				if value.Data.(map[string]any)["accepted"] != true {
					t.Fatal("accepted write not reported")
				}
			}
		})
	}
}

func TestPolicyHonorsCallerCancellation(t *testing.T) {
	f := newPolicyCoreFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := ReadPolicyGroups(ctx, f.options)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("read cancellation = %v", err)
	}
	_, err = SelectPolicyGroup(ctx, f.options, "Custom / 组", "provider-b/same")
	if !errors.Is(err, context.Canceled) || f.selects != 0 {
		t.Fatalf("select cancellation = %v, calls=%d", err, f.selects)
	}
}
