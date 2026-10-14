package serviceapi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestIsRetryable(t *testing.T) {
	for _, test := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{context.Canceled, false},
		{context.DeadlineExceeded, true},
		{io.EOF, true},
		{io.ErrUnexpectedEOF, true},
		{&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, true},
		{&net.OpError{Op: "read", Net: "tcp", Err: context.Canceled}, false},
		{errors.New("invalid protobuf payload"), false},
	} {
		if got := IsRetryable(test.err); got != test.want {
			t.Errorf("IsRetryable(%v)=%v，预期 %v", test.err, got, test.want)
		}
	}
	for _, code := range []int{0, 3, 4, 5, 7, 8, 10, 12, 14, 16} {
		err := parseTrailer([]byte(fmt.Sprintf("grpc-status: %d\r\ngrpc-message: fixture\r\n", code)))
		want := code == 4 || code == 8 || code == 10 || code == 14
		if got := IsRetryable(fmt.Errorf("wrapped: %w", err)); got != want {
			t.Errorf("gRPC status %d 重试分类错误: %v", code, err)
		}
	}
}

func TestHTTPRetryableStatus(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 408, 429, 500, 501, 502, 503, 504} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "fixture", code)
			}))
			defer server.Close()
			client, err := New(server.URL, "")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			err = client.Select(t.Context(), "Proxy", "Auto/fixture")
			want := code == 408 || code == 429 || code == 500 || code == 502 || code == 503 || code == 504
			if err == nil || IsRetryable(err) != want || !strings.HasPrefix(err.Error(), fmt.Sprintf("Service API HTTP %d:", code)) {
				t.Fatalf("HTTP %d 重试分类或错误消息异常: %v", code, err)
			}
		})
	}
}

func TestReadyChecksServiceAPIVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload []byte
		switch request.URL.Path {
		case methodGetVersion:
			payload = appendString(payload, 1, "1.14.0-beta.17-reF1nd")
			payload = appendVarint(payload, 2, minimumAPIVersion+3)
		case methodGetStartedAt:
			payload = appendVarint(payload, 1, 123456)
		default:
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		writeTestFrame(t, writer, 0, payload)
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	startedAt, err := client.Ready(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if startedAt.UnixMilli != 123456 {
		t.Fatalf("unexpected startedAt: %d", startedAt.UnixMilli)
	}
}

func TestReadyRejectsMissingServiceAPIVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodGetVersion {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		payload := appendString(nil, 1, "legacy")
		writeTestFrame(t, writer, 0, payload)
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Ready(t.Context()); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("最低要求=%d", minimumAPIVersion)) {
		t.Fatalf("未拒绝缺少版本号的 Service API: %v", err)
	}
}

func TestStartedAtOverGRPCWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodGetStartedAt {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-secret" {
			http.Error(writer, "missing authorization", http.StatusUnauthorized)
			return
		}
		requestFrame, err := io.ReadAll(request.Body)
		if err != nil || len(requestFrame) != 5 || binary.BigEndian.Uint32(requestFrame[1:]) != 0 {
			http.Error(writer, "invalid request frame", http.StatusBadRequest)
			return
		}
		payload := protowire.AppendTag(nil, 1, protowire.VarintType)
		payload = protowire.AppendVarint(payload, 123456)
		writeTestFrame(t, writer, 0, payload)
		writeTestFrame(t, writer, 0x80, []byte("grpc-status: 0\r\n"))
	}))
	defer server.Close()

	client, err := New(server.URL, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	startedAt, err := client.StartedAt(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if startedAt.UnixMilli != 123456 {
		t.Fatalf("unexpected startedAt: %d", startedAt.UnixMilli)
	}
}

func TestStartedAtWithoutGRPCStatusFrame(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodGetStartedAt {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		payload := protowire.AppendTag(nil, 1, protowire.VarintType)
		payload = protowire.AppendVarint(payload, 654321)
		writeTestFrame(t, writer, 0, payload)
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	startedAt, err := client.StartedAt(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if startedAt.UnixMilli != 654321 {
		t.Fatalf("unexpected startedAt: %d", startedAt.UnixMilli)
	}
}

func TestStatusOverGRPCWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodSubscribeStatus {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		requestFrame, err := io.ReadAll(request.Body)
		if err != nil || len(requestFrame) <= 5 {
			http.Error(writer, "invalid request frame", http.StatusBadRequest)
			return
		}
		interval, length := protowire.ConsumeVarint(requestFrame[6:])
		if requestFrame[5] != byte(protowire.EncodeTag(1, protowire.VarintType)) || length < 0 || interval != uint64(time.Second) {
			http.Error(writer, "invalid status interval", http.StatusBadRequest)
			return
		}

		var payload []byte
		for number, value := range map[protowire.Number]uint64{
			1: 4096,
			2: 12,
			3: 3,
			4: 4,
			5: 1,
			6: 128,
			7: 256,
			8: 1024,
			9: 2048,
		} {
			payload = appendVarint(payload, number, value)
		}
		writeTestFrame(t, writer, 0, payload)
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	status, err := client.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Memory != 4096 || status.Goroutines != 12 || status.ConnectionsIn != 3 ||
		status.ConnectionsOut != 4 || !status.TrafficAvailable || status.Uplink != 128 ||
		status.Downlink != 256 || status.UplinkTotal != 1024 || status.DownlinkTotal != 2048 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestCloseAllConnectionsOverGRPCWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodCloseAllConnections {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		requestFrame, err := io.ReadAll(request.Body)
		if err != nil || len(requestFrame) != 5 || binary.BigEndian.Uint32(requestFrame[1:]) != 0 {
			http.Error(writer, "invalid request frame", http.StatusBadRequest)
			return
		}
		writeTestFrame(t, writer, 0, nil)
		writeTestFrame(t, writer, 0x80, []byte("grpc-status: 0\r\n"))
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err = client.CloseAllConnections(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func writeTestFrame(t *testing.T, writer io.Writer, flag byte, payload []byte) {
	t.Helper()
	header := []byte{flag, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
}

func TestWireCodecMarshalSelectOutbound(t *testing.T) {
	content, err := (wireCodec{}).Marshal(&selectOutboundRequest{
		Group:    "Proxy",
		Outbound: "Auto/default",
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := appendString(nil, 1, "Proxy")
	expected = appendString(expected, 2, "Auto/default")
	if !reflect.DeepEqual(content, expected) {
		t.Fatalf("unexpected protobuf payload: %x", content)
	}
}

func TestWireCodecUnmarshalGroups(t *testing.T) {
	item := appendString(nil, 1, "default/node")
	item = appendString(item, 2, "vless")
	item = protowire.AppendTag(item, 4, protowire.VarintType)
	item = protowire.AppendVarint(item, 88)

	group := appendString(nil, 1, "Select/default")
	group = appendString(group, 2, "selector")
	group = protowire.AppendTag(group, 3, protowire.VarintType)
	group = protowire.AppendVarint(group, 1)
	group = appendString(group, 4, "default/node")
	group = protowire.AppendTag(group, 6, protowire.BytesType)
	group = protowire.AppendBytes(group, item)

	content := protowire.AppendTag(nil, 1, protowire.BytesType)
	content = protowire.AppendBytes(content, group)
	var response groupsResponse
	if err := (wireCodec{}).Unmarshal(content, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Items) != 1 {
		t.Fatalf("unexpected groups: %#v", response.Groups)
	}
	if response.Groups[0].Selected != "default/node" || response.Groups[0].Items[0].URLTestDelay != 88 {
		t.Fatalf("unexpected decoded group: %#v", response.Groups[0])
	}
}

func TestOutboundsOverGRPCWeb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != methodSubscribeOutbounds {
			http.Error(writer, "unexpected method", http.StatusNotFound)
			return
		}
		item := appendString(nil, 1, "fixture/node")
		item = appendString(item, 2, "vless")
		payload := protowire.AppendTag(nil, 1, protowire.BytesType)
		payload = protowire.AppendBytes(payload, item)
		writeTestFrame(t, writer, 0, payload)
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	outbounds, err := client.Outbounds(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(outbounds) != 1 || outbounds[0].Tag != "fixture/node" || outbounds[0].Type != "vless" {
		t.Fatalf("unexpected outbounds: %#v", outbounds)
	}
}

func TestSubscribeClashModeUsesNativeFieldAndStreamsUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != methodSubscribeClashMode || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !reflect.DeepEqual(body, []byte{0, 0, 0, 0, 0}) {
			t.Error("模式订阅请求不是原生 Empty", err, body)
			return
		}
		for _, mode := range []string{"Rule", "Direct", "Proxy", ""} {
			payload := appendString(nil, 2, "not-current-mode")
			payload = appendString(payload, 3, mode)
			writeTestFrame(t, w, 0, payload)
		}
		writeTestFrame(t, w, 0x80, []byte("grpc-status: 0\r\n"))
	}))
	defer server.Close()
	client, err := New(server.URL, "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var modes []string
	err = client.SubscribeClashMode(t.Context(), func(mode string) error {
		modes = append(modes, mode)
		return nil
	})
	if err != nil || !reflect.DeepEqual(modes, []string{"Rule", "Direct", "Proxy", ""}) {
		t.Fatal("模式流未按字段 3 解码全部更新", modes, err)
	}
	if err := client.SubscribeClashMode(t.Context(), nil); err == nil {
		t.Fatal("未拒绝空模式回调")
	}
}

func TestSubscribeClashModeRejectsMalformedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestFrame(t, w, 0, appendVarint(nil, 3, 1))
	}))
	defer server.Close()
	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	called := false
	err = client.SubscribeClashMode(t.Context(), func(string) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("错误 wire type 被作为有效模式发送", err)
	}
}

func TestSubscribeClashModeCancellationAndConsumerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		writeTestFrame(t, w, 0, appendString(nil, 3, "Rule"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- client.SubscribeClashMode(ctx, func(string) error { cancel(); return nil })
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("模式流未传播取消", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消未关闭模式流")
	}
	consumerError := errors.New("fixture consumer stopped")
	if err := client.SubscribeClashMode(t.Context(), func(string) error { return consumerError }); !errors.Is(err, consumerError) {
		t.Fatal("未传播消费方退出", err)
	}
}
