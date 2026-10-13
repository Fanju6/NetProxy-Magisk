package httpapi

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	json "encoding/json/v2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
)

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
	connection, err := net.Dial("tcp", server.config.Listen)
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
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for _, test := range []struct{ path, allowed string }{
		{"/api/v1/info", "GET"}, {"/api/v1/config?target=module", "GET, PUT"}, {"/api/v1/events", "GET"},
	} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodHead, "http://"+server.config.Listen+test.path, nil)
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
	request, err := http.NewRequestWithContext(requestContext, "POST", "http://"+server.config.Listen+"/api/v1/test-write", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	requestDone := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
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

func TestEventsSendSnapshotAndCloseWithServer(t *testing.T) {
	server, cancel, done := serveFixture(t)
	ctx, stop := context.WithTimeout(t.Context(), 3*time.Second)
	defer stop()
	request, err := http.NewRequestWithContext(ctx, "GET", "http://"+server.config.Listen+"/api/v1/events?topics=status", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	found := false
	for !found {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal("事件流没有初始快照", err)
		}
		found = strings.HasPrefix(line, "data: ")
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
