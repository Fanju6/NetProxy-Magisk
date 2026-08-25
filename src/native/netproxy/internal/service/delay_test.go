package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"google.golang.org/protobuf/encoding/protowire"
)

type delayServerState struct {
	mu             sync.Mutex
	current        []serviceapi.GroupItem
	updated        []serviceapi.GroupItem
	updateAfter    int
	outboundsCalls int
	urlTestCalls   int
	failURLTest    bool
}

func delayServer(t *testing.T, state *delayServerState) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/daemon.StartedService/URLTest":
			state.mu.Lock()
			state.urlTestCalls++
			fail := state.failURLTest
			state.mu.Unlock()
			if fail {
				http.Error(writer, "urltest failed", http.StatusBadGateway)
				return
			}
			writeServiceAPIFrame(t, writer, nil)
		case "/daemon.StartedService/SubscribeOutbounds":
			state.mu.Lock()
			state.outboundsCalls++
			items := state.current
			if state.urlTestCalls > 0 && state.updateAfter > 0 && state.outboundsCalls >= state.updateAfter {
				items = state.updated
			}
			state.mu.Unlock()
			writeServiceAPIFrame(t, writer, delayOutboundsPayload(items))
		default:
			http.NotFound(writer, request)
		}
	}))
}

func delayOutboundsPayload(items []serviceapi.GroupItem) []byte {
	var payload []byte
	for _, item := range items {
		payload = protowire.AppendTag(payload, 1, protowire.BytesType)
		payload = protowire.AppendBytes(payload, delayItemPayload(item))
	}
	return payload
}

func delayItemPayload(item serviceapi.GroupItem) []byte {
	var payload []byte
	payload = appendDelayString(payload, 1, item.Tag)
	payload = appendDelayString(payload, 2, item.Type)
	if item.URLTestTime != 0 {
		payload = protowire.AppendTag(payload, 3, protowire.VarintType)
		payload = protowire.AppendVarint(payload, uint64(item.URLTestTime))
	}
	if item.URLTestDelay != 0 {
		payload = protowire.AppendTag(payload, 4, protowire.VarintType)
		payload = protowire.AppendVarint(payload, uint64(item.URLTestDelay))
	}
	return payload
}

func delayItemByTag(items []serviceapi.GroupItem, tag string) (serviceapi.GroupItem, bool) {
	for _, item := range items {
		if item.Tag == tag {
			return item, true
		}
	}
	return serviceapi.GroupItem{}, false
}

func appendDelayString(payload []byte, number protowire.Number, value string) []byte {
	if value == "" {
		return payload
	}
	payload = protowire.AppendTag(payload, number, protowire.BytesType)
	return protowire.AppendString(payload, value)
}

func delayOutbounds(first, second serviceapi.GroupItem) []serviceapi.GroupItem {
	return []serviceapi.GroupItem{
		{Tag: "Auto/本地配置", Type: "urltest"},
		{Tag: "Select/本地配置", Type: "selector"},
		first,
		second,
	}
}

func delayOptions(t *testing.T, catalogRoot, moduleConfig, address string) Options {
	t.Helper()
	return Options{
		CatalogRoot:    catalogRoot,
		ModuleConfig:   moduleConfig,
		ServiceAddress: address,
		RequestTimeout: time.Second,
	}
}

func delayFixtureFiles(t *testing.T) (string, string) {
	t.Helper()
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	writeCatalogFixture(t, catalogRoot)
	if err := os.WriteFile(moduleConfig, []byte("SELECTOR_MODE=urltest\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return catalogRoot, moduleConfig
}

func TestDelayGroupReturnsFreshResultsAndPerNodeTimeouts(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	current := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 100, URLTestDelay: 40},
		serviceapi.GroupItem{Tag: "本地配置/DROP", Type: "socks", URLTestTime: 100, URLTestDelay: 50},
	)
	updated := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 200, URLTestDelay: 88},
		serviceapi.GroupItem{Tag: "本地配置/DROP", Type: "socks"},
	)
	state := &delayServerState{current: current, updated: updated, updateAfter: 1}
	server := delayServer(t, state)
	defer server.Close()

	result, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "auto", "default")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "Auto/本地配置" || len(result.Groups) != 1 || len(result.Groups[0].Items) != 2 {
		t.Fatalf("分组测速结果异常: %#v", result)
	}
	node, nodeFound := delayItemByTag(result.Groups[0].Items, "本地配置/NODE")
	if !nodeFound || node.URLTestDelay != 88 {
		t.Fatalf("新结果未返回: %#v", result.Groups[0].Items)
	}
	drop, dropFound := delayItemByTag(result.Groups[0].Items, "本地配置/DROP")
	if !dropFound || drop.URLTestDelay != 0 {
		t.Fatalf("失败节点未保留为 timeout: %#v", result.Groups[0].Items)
	}
}

func TestDelayGroupSettlesAfterFreshResultsWithoutWaitingForUnknownFailures(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	current := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 100, URLTestDelay: 40},
		serviceapi.GroupItem{Tag: "本地配置/DROP", Type: "socks"},
	)
	updated := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 200, URLTestDelay: 88},
		serviceapi.GroupItem{Tag: "本地配置/DROP", Type: "socks"},
	)
	state := &delayServerState{current: current, updated: updated, updateAfter: 1}
	server := delayServer(t, state)
	defer server.Close()

	started := time.Now()
	result, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "auto", "default")
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) >= 500*time.Millisecond {
		t.Fatalf("部分失败时未快速收敛: %s", time.Since(started))
	}
	node, nodeFound := delayItemByTag(result.Groups[0].Items, "本地配置/NODE")
	drop, dropFound := delayItemByTag(result.Groups[0].Items, "本地配置/DROP")
	if !nodeFound || !dropFound || node.URLTestDelay != 88 || drop.URLTestDelay != 0 {
		t.Fatalf("快速收敛结果异常: %#v", result.Groups[0].Items)
	}
}

func TestDelaySupportsSingleNodeAutoGroupWithoutGroupsSnapshot(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	current := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 100, URLTestDelay: 40},
		serviceapi.GroupItem{},
	)
	updated := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 200, URLTestDelay: 91},
		serviceapi.GroupItem{},
	)
	state := &delayServerState{current: current, updated: updated, updateAfter: 1}
	server := delayServer(t, state)
	defer server.Close()

	result, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "Auto/default", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "Auto/本地配置" || len(result.Groups[0].Items) != 1 || result.Groups[0].Items[0].URLTestDelay != 91 {
		t.Fatalf("单节点 Auto 分组测速异常: %#v", result)
	}
}

func TestDelaySingleNodeReturnsFreshResult(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	current := delayOutbounds(serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 100, URLTestDelay: 40}, serviceapi.GroupItem{})
	updated := delayOutbounds(serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 200, URLTestDelay: 73}, serviceapi.GroupItem{})
	state := &delayServerState{current: current, updated: updated, updateAfter: 1}
	server := delayServer(t, state)
	defer server.Close()

	result, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "default/NODE", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups[0].Items) != 1 || result.Groups[0].Items[0].URLTestDelay != 73 {
		t.Fatalf("单节点测速异常: %#v", result)
	}
}

func TestOfflineDelayWaitsForResults(t *testing.T) {
	current := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks"},
		serviceapi.GroupItem{Tag: "本地配置/DROP", Type: "socks"},
	)
	updated := delayOutbounds(
		serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 200, URLTestDelay: 73},
		serviceapi.GroupItem{Tag: "本地配置/DROP", Type: "socks", URLTestTime: 200, URLTestDelay: 91},
	)
	state := &delayServerState{current: current, updated: updated, updateAfter: 1}
	server := delayServer(t, state)
	defer server.Close()
	client, err := serviceapi.New(strings.TrimPrefix(server.URL, "http://"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := delayWithClient(ctx, client, "Auto/本地配置", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].Items) != 2 {
		t.Fatalf("离线测速结果异常: %#v", result)
	}
	for _, item := range result.Groups[0].Items {
		if item.URLTestTime != 200 || item.URLTestDelay <= 0 {
			t.Fatalf("离线测速未等待临时会话结果: %#v", result.Groups[0].Items)
		}
	}
}

func TestDelayReturnsCachedResultWhenBatchIsStillRunning(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	current := delayOutbounds(serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks", URLTestTime: 100, URLTestDelay: 40}, serviceapi.GroupItem{})
	state := &delayServerState{current: current}
	server := delayServer(t, state)
	defer server.Close()
	result, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "default/NODE", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].Items[0].URLTestDelay != 40 {
		t.Fatalf("测速批次尚未完成时不应把缓存结果改成超时: %#v", result)
	}
}

func TestDelayAllFailedIsSuccessfulRequestWithTimeoutItems(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	current := delayOutbounds(serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks"}, serviceapi.GroupItem{})
	state := &delayServerState{current: current}
	server := delayServer(t, state)
	defer server.Close()
	result, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "auto", "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups[0].Items) != 1 || result.Groups[0].Items[0].URLTestDelay != 0 {
		t.Fatalf("全失败结果语义异常: %#v", result)
	}
}

func TestDelayTargetMissingIsStructured(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	state := &delayServerState{current: []serviceapi.GroupItem{{Tag: "direct", Type: "direct"}}}
	server := delayServer(t, state)
	defer server.Close()

	_, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "Auto/default", "")
	var structured *Error
	if !errors.As(err, &structured) || structured.Code != "node.delay_target_missing" {
		t.Fatalf("目标缺失未返回结构化错误: %v", err)
	}
}

func TestDelayServiceAPIFailureIsStructured(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	current := delayOutbounds(serviceapi.GroupItem{Tag: "本地配置/NODE", Type: "socks"}, serviceapi.GroupItem{})
	state := &delayServerState{current: current, failURLTest: true}
	server := delayServer(t, state)
	defer server.Close()
	originalRunner := offlineDelayRunner
	offlineCalled := false
	offlineDelayRunner = func(context.Context, Options, delayRequest) (DelayResult, error) {
		offlineCalled = true
		return DelayResult{}, nil
	}
	defer func() { offlineDelayRunner = originalRunner }()

	_, err := Delay(context.Background(), delayOptions(t, catalogRoot, moduleConfig, server.URL), "Auto/default", "")
	var structured *Error
	if !errors.As(err, &structured) || structured.Code != "node.delay_api_failed" {
		t.Fatalf("API 失败未返回结构化错误: %v", err)
	}
	if offlineCalled {
		t.Fatal("Service API 已返回 HTTP 错误时不应启动离线测速")
	}
}

func TestDelayFallsBackToOfflineSessionWhenServiceIsStopped(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	originalRunner := offlineDelayRunner
	defer func() { offlineDelayRunner = originalRunner }()
	offlineDelayRunner = func(_ context.Context, _ Options, request delayRequest) (DelayResult, error) {
		if request.Target != "Auto/本地配置" || request.GroupID != "default" || request.NodeTag != "" {
			t.Fatalf("离线测速目标解析异常: %#v", request)
		}
		return DelayResult{Target: request.Target}, nil
	}
	options := delayOptions(t, catalogRoot, moduleConfig, address)
	options.StateFile = filepath.Join(t.TempDir(), "service.json")
	options.SingBoxPath = filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(options.SingBoxPath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Delay(context.Background(), options, "auto", "default")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "Auto/本地配置" {
		t.Fatalf("离线测速结果异常: %#v", result)
	}
}

func TestDelayDoesNotStartOfflineSessionWhileServiceIsStarting(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	originalRunner := offlineDelayRunner
	offlineCalled := false
	offlineDelayRunner = func(context.Context, Options, delayRequest) (DelayResult, error) {
		offlineCalled = true
		return DelayResult{}, nil
	}
	defer func() { offlineDelayRunner = originalRunner }()
	stateFile := filepath.Join(t.TempDir(), "service.json")
	if err := os.WriteFile(stateFile, []byte(`{"state":"starting"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	options := delayOptions(t, catalogRoot, moduleConfig, address)
	options.StateFile = stateFile

	_, err = Delay(context.Background(), options, "auto", "default")
	var structured *Error
	if !errors.As(err, &structured) || structured.Code != "node.delay_api_failed" {
		t.Fatalf("启动中 API 不可用应保留在线错误: %v", err)
	}
	if offlineCalled {
		t.Fatal("正式服务启动期间不应创建离线测速会话")
	}
}

func TestDelayTimeoutDoesNotStartOfflineSession(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeServiceAPIFrame(t, writer, nil)
	}))
	defer server.Close()
	originalRunner := offlineDelayRunner
	offlineCalled := false
	offlineDelayRunner = func(context.Context, Options, delayRequest) (DelayResult, error) {
		offlineCalled = true
		return DelayResult{}, nil
	}
	defer func() { offlineDelayRunner = originalRunner }()
	options := delayOptions(t, catalogRoot, moduleConfig, server.URL)
	options.RequestTimeout = 20 * time.Millisecond

	_, err := Delay(context.Background(), options, "auto", "default")
	var structured *Error
	if !errors.As(err, &structured) || structured.Code != "node.delay_timeout" {
		t.Fatalf("在线测速超时错误异常: %v", err)
	}
	if offlineCalled {
		t.Fatal("在线请求超时后不应使用已到期的上下文启动离线测速")
	}
}
