package httpapi

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/androidplatform"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestUntrustedPortOwnerNeverReceivesBearer(t *testing.T) {
	trusted := resourceFixture(t).config
	other := resourceFixture(t).config
	var requests atomic.Int32
	impostor := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	var err error
	impostor.TLS, err = other.ServerTLS()
	if err != nil {
		t.Fatal(err)
	}
	impostor.StartTLS()
	defer impostor.Close()
	trusted.Listen = impostor.Listener.Addr().String()
	if _, err := Probe(t.Context(), trusted); err == nil {
		t.Fatal("接受了其他安装的服务身份")
	}
	if requests.Load() != 0 {
		t.Fatal("证书校验前发送了认证请求")
	}
}

func TestHTTPBoundaryRejectsUntrustedRequests(t *testing.T) {
	server := resourceFixture(t)
	for _, test := range []struct {
		name, method, path, host, origin, auth string
		status                                 int
	}{
		{"auth", "GET", "/api/v1/info", server.config.Listen, "", "", 401},
		{"host", "GET", "/api/v1/info", "localhost:9091", "", "Bearer " + server.config.Secret, 403},
		{"origin", "GET", "/api/v1/info", server.config.Listen, "https://example.test", "Bearer " + server.config.Secret, 403},
		{"null-origin", "GET", "/api/v1/info", server.config.Listen, "null", "Bearer " + server.config.Secret, 403},
		{"query-token", "GET", "/api/v1/info?token=redacted", server.config.Listen, "", "Bearer " + server.config.Secret, 400},
		{"method", "PATCH", "/api/v1/info", server.config.Listen, "", "Bearer " + server.config.Secret, 405},
		{"path", "GET", "/api/v1/exec", server.config.Listen, "", "Bearer " + server.config.Secret, 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.auth != "" {
				request.Header.Set("Authorization", test.auth)
			}
			writer := httptest.NewRecorder()
			server.ServeHTTP(writer, request)
			var result contract.Result
			if err := json.Unmarshal(writer.Body.Bytes(), &result); err != nil || writer.Code != test.status || result.Schema != 1 || result.OK {
				t.Fatal(writer.Code, writer.Body.String(), err)
			}
			if writer.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("私有响应可被缓存")
			}
		})
	}
}

func TestUnauthorizedSlowBodyCannotHoldConnection(t *testing.T) {
	server, cancel, done := serveFixture(t)
	connection, err := tls.Dial("tcp", server.config.Listen, server.config.ClientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(connection, "POST /api/v1/service/start HTTP/1.1\r\nHost: "+server.config.Listen+"\r\nContent-Length: 10\r\nContent-Type: application/json\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatal("未鉴权慢正文阻塞了拒绝响应", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 401 || !response.Close {
		t.Fatal("未立即关闭未鉴权连接", response.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("慢正文阻塞了停服")
	}
}

func TestHeadNeverEntersResourcesOrEvents(t *testing.T) {
	server, cancel, done := serveFixture(t)
	for range cap(server.writeSlots) {
		server.writeSlots <- struct{}{}
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: server.config.ClientTLS()}}
	defer client.CloseIdleConnections()
	for _, test := range []struct{ path, allowed string }{
		{"/api/v1/info", "GET"}, {"/api/v1/config?target=module", "GET, PUT"}, {"/api/v1/events", "GET"},
	} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodHead, server.config.Connection().Address+test.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+server.config.Secret)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("HEAD 进入业务处理或持续订阅", err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusMethodNotAllowed || response.Header.Get("Allow") != test.allowed {
			t.Fatal("HEAD 未在业务准入前拒绝或允许方法错误", response.StatusCode, response.Header.Get("Allow"))
		}
	}
	server.eventMu.Lock()
	hub := server.events
	server.eventMu.Unlock()
	if hub != nil {
		t.Fatal("HEAD 创建了事件订阅")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HEAD 阻塞了服务退出")
	}
}

func TestDecodeRejectsDuplicateUnknownAndMalformedJSON(t *testing.T) {
	for _, content := range []string{"null", `{"value":null}`, `{"value":"a","value":"b"}`, `{"extra":"a"}`, `{"value":1}`, `{"value":"a"} {}`, "{\"value\":\"\xff\"}"} {
		var value struct {
			Value string `json:"value"`
		}
		request := httptest.NewRequest("PUT", "/", strings.NewReader(content))
		request.Header.Set("Content-Type", "application/json")
		if err := decodeRequest(request, &value); err == nil {
			t.Fatal("无效请求被接受", content)
		}
	}
	var value struct {
		Value string `json:"value"`
	}
	request := httptest.NewRequest("PUT", "/", strings.NewReader(`{"value":"long"}`))
	request.Header.Set("Content-Type", "application/json")
	if err := decodeRequestLimit(request, &value, 8); err == nil {
		t.Fatal("未限制正文大小")
	}
}

func serveFixture(t *testing.T) (*Server, context.CancelFunc, <-chan error) {
	t.Helper()
	server := resourceFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.config.Listen = listener.Addr().String()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- server.serveListener(ctx, listener) }()
	t.Cleanup(cancel)
	return server, cancel, done
}

func TestServerStartsWithoutInvokingAndroidPlatform(t *testing.T) {
	server := resourceFixture(t)
	var calls atomic.Int32
	server.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
		calls.Add(1)
		return nil, nil
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.config.Listen = listener.Addr().String()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- server.serveListener(ctx, listener) }()
	t.Cleanup(func() { cancel(); <-done })
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: server.config.ClientTLS()}}
	defer client.CloseIdleConnections()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.config.Connection().Address+"/api/v1/info", nil)
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || calls.Load() != 0 {
		t.Fatal("API 启动或普通读取触发平台进程", response.StatusCode, calls.Load())
	}
}

func TestAcceptedWriteSurvivesDisconnectAndShutdownDrains(t *testing.T) {
	server, cancelServer, serverDone := serveFixture(t)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	server.route("POST /api/v1/test-write", time.Second, false, func(ctx context.Context, _ *http.Request) (contract.Result, error) {
		close(started)
		<-release
		finished <- ctx.Err()
		return contract.Success("test.saved", "已保存", nil), nil
	})
	requestContext, cancelRequest := context.WithCancel(t.Context())
	request, err := http.NewRequestWithContext(requestContext, "POST", server.config.Connection().Address+"/api/v1/test-write", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: server.config.ClientTLS()}}
		defer client.CloseIdleConnections()
		response, err := client.Do(request)
		if response != nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	<-started
	cancelRequest()
	<-requestDone
	cancelServer()
	select {
	case <-serverDone:
		t.Fatal("关闭丢弃了已接受写入")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal("断线取消了领域事务", err)
	}
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("管理服务收尾未完成")
	}
}

func TestInfoAndWholeConfigReadSurviveBrokenModule(t *testing.T) {
	server := resourceFixture(t)
	for _, path := range []string{server.options.ModuleConfig, server.options.InboundConfig} {
		if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/api/v1/info", "/api/v1/config?target=module", "/api/v1/config?target=inbound"} {
		request := httptest.NewRequest("GET", path, nil)
		request.Host = server.config.Listen
		request.Header.Set("Authorization", "Bearer "+server.config.Secret)
		writer := httptest.NewRecorder()
		server.ServeHTTP(writer, request)
		if writer.Code != 200 {
			t.Fatal(path, writer.Code, writer.Body.String())
		}
	}
}

func TestDiagnosticDownloadIsCompleteAndOmitsCredentials(t *testing.T) {
	server := resourceFixture(t)
	request := httptest.NewRequest("GET", "/api/v1/diagnostics/export", nil)
	request.Host = server.config.Listen
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	writer := httptest.NewRecorder()
	server.ServeHTTP(writer, request)
	if writer.Code != 200 || writer.Header().Get("Content-Type") != "application/gzip" {
		t.Fatal(writer.Code, writer.Body.String())
	}
	archive, err := gzip.NewReader(bytes.NewReader(writer.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(header.Name, "api.json") || bytes.Contains(content, []byte(server.config.Secret)) {
			t.Fatal("诊断包包含管理凭据")
		}
	}
}

func TestAndroidAppReadCanExceedFifteenSeconds(t *testing.T) {
	server, cancel, done := serveFixture(t)
	t.Cleanup(func() { cancel(); <-done })
	server.platform = platformFunc(func(ctx context.Context, _ androidplatform.Request) (jsontext.Value, error) {
		timer := time.NewTimer(16 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			return jsontext.Value(`{"apps":[]}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	ctx, stop := context.WithTimeout(t.Context(), 20*time.Second)
	defer stop()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.config.Connection().Address+"/api/v1/android/apps", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: server.config.ClientTLS()}}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("应用查询被普通读取期限中断", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(response.Body)
		t.Fatal(response.Status, string(content))
	}
}

func TestShutdownCancelsSubscriptionTransferButPreservesProvider(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	server, stopServer, done := serveFixture(t)
	defer stopServer()
	resourceSubscriptionFixture(t, server, "slow-subscription", upstream.URL)
	provider := paths.New(server.options.ModuleDir).Catalog()
	provider = filepath.Join(provider, "slow-subscription", "provider.json")
	before, err := os.ReadFile(provider)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.config.Connection().Address+"/api/v1/subscriptions/slow-subscription/update", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: server.config.ClientTLS()}}
	defer client.CloseIdleConnections()
	finished := make(chan error, 1)
	go func() {
		response, err := client.Do(request)
		if response != nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("订阅下载没有启动")
	}
	stopServer()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("停服被慢订阅下载阻塞")
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(provider)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("取消网络传输破坏了原 Provider", err)
	}
}

func TestRequestBodyLimitHandlesKnownAndUnknownLengths(t *testing.T) {
	for _, test := range []struct {
		content string
		length  int64
		status  int
	}{
		{`{"value":"ok"}`, 14, 0},
		{`{"value":"ok"}`, -1, 0},
		{`{"value":"ok"}`, 15, http.StatusRequestEntityTooLarge},
		{`{"value":"long"}`, -1, http.StatusRequestEntityTooLarge},
	} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.content))
		request.Header.Set("Content-Type", "application/json")
		request.ContentLength = test.length
		var input struct {
			Value string `json:"value"`
		}
		err := decodeRequestLimit(request, &input, 14)
		if test.status == 0 {
			if err != nil || input.Value != "ok" {
				t.Fatal(input, err)
			}
		} else if err == nil || failureStatus("api.body_too_large", err) != test.status {
			t.Fatal("超大正文未返回明确 HTTP 状态", err)
		}
	}
}

func TestHTTPStatusIsExplicitAndDoesNotChangeCLIExitStatus(t *testing.T) {
	for _, test := range []struct {
		code   string
		status int
	}{
		{"subscription.read_only", 422}, {"subscription.interval_too_short", 422},
		{"subscription.invalid_target", 422}, {"subscription.busy", 409}, {"subscription.cancelled", 408},
		{"node.runtime_sync_failed", 502}, {"node.persisted_effect_failed", 500},
		{"node.delay_busy", 409}, {"node.delay_timeout", 504},
		{"unregistered.invalid", 500}, {"unregistered.not_found", 500},
	} {
		err := &subscription.Error{Code: test.code, Message: "匿名错误"}
		if actual := failureStatus(test.code, err); actual != test.status {
			t.Errorf("%s: %d", test.code, actual)
		}
	}
	err := &contract.Error{Status: 2, HTTPStatus: 422, Code: "fixture", Message: "匿名错误"}
	_, exit := contract.Failure(t.Context(), err)
	if exit != 2 || failureStatus(err.Code, err) != 422 {
		t.Fatal("混用了 HTTP 状态与 CLI 退出码", exit)
	}
}

func TestInfoCachesVersionAndPreservesMemoryFields(t *testing.T) {
	fixture := resourceFixture(t)
	file := paths.New(fixture.options.ModuleDir).ModuleProp()
	resourceWriteFile(t, file, "version=v8.5.3\nversionCode=1084\n")
	server, err := New(fixture.options, fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	defer server.stopEvents()
	resourceWriteFile(t, file, "version=changed\nversionCode=0\n")
	_, response := resourceRequest(t, server, http.MethodGet, "/api/v1/info", "")
	var info struct {
		Version string            `json:"version"`
		Build   string            `json:"version_code"`
		Memory  map[string]uint64 `json:"memory"`
	}
	if err := json.Unmarshal(response.Data, &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != "v8.5.3" || info.Build != "1084" {
		t.Fatal("版本没有按运行实例缓存", info)
	}
	if len(info.Memory) != 4 || info.Memory["heap_alloc_bytes"] == 0 || info.Memory["heap_inuse_bytes"] < info.Memory["heap_alloc_bytes"] || info.Memory["stack_inuse_bytes"] == 0 {
		t.Fatal("内存指标字段或语义发生变化", info.Memory)
	}
}

func TestEventsRemainConnectedPastRequestReadDeadline(t *testing.T) {
	server, cancel, done := serveFixture(t)
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("事件流阻止管理服务停止")
		}
	})
	ctx, stop := context.WithTimeout(t.Context(), 25*time.Second)
	defer stop()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.config.Connection().Address+"/api/v1/events?topics=groups", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: server.config.ClientTLS()}}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.Status)
	}
	reader := bufio.NewScanner(response.Body)
	for heartbeats := 0; heartbeats < 2; {
		if !reader.Scan() {
			t.Fatal("SSE 在普通请求读取期限后断开", reader.Err())
		}
		if reader.Text() == ": heartbeat" {
			heartbeats++
		}
	}
}

func TestEventsSendSnapshotAndCloseWithServer(t *testing.T) {
	server, cancel, done := serveFixture(t)
	ctx, stop := context.WithTimeout(t.Context(), 3*time.Second)
	defer stop()
	request, err := http.NewRequestWithContext(ctx, "GET", server.config.Connection().Address+"/api/v1/events?topics=status,subscriptions", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: server.config.ClientTLS()}}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	found := false
	name := ""
	for !found {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal("事件流没有初始快照", err)
		}
		if strings.HasPrefix(line, "event: ") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		}
		if name == "subscriptions" && strings.HasPrefix(line, "data: ") {
			var result resourceResponse
			var groups []catalog.GroupSummary
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &result); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(result.Data, &groups); err != nil || len(groups) != 1 || groups[0].ID != "default" || !groups[0].Active {
				t.Fatalf("订阅事件遗漏本地分组: %+v %v", groups, err)
			}
			found = true
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("事件流阻止管理服务停止")
	}
}
