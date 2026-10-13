package serviceapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

type streamSubscription struct {
	name string
	call func(*Client, context.Context, func() error) error
}

func streamSubscriptions() []streamSubscription {
	return []streamSubscription{
		{"status", func(client *Client, ctx context.Context, consume func() error) error {
			return client.SubscribeStatus(ctx, time.Second, func(Status) error { return consume() })
		}},
		{"groups", func(client *Client, ctx context.Context, consume func() error) error {
			return client.SubscribeGroups(ctx, func([]Group) error { return consume() })
		}},
	}
}

func streamHTTPClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(server.URL, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func streamFrame(flag byte, payload []byte) []byte {
	header := []byte{flag, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	return append(header, payload...)
}

func streamGroupsPayload(tag, selected string) []byte {
	item := appendString(nil, 1, selected)
	item = appendString(item, 2, "vless")
	item = appendVarint(item, 3, 123456)
	item = appendVarint(item, 4, 88)
	group := appendString(nil, 1, tag)
	group = appendString(group, 2, "selector")
	group = appendVarint(group, 3, 1)
	group = appendString(group, 4, selected)
	group = appendVarint(group, 5, 1)
	group = protowire.AppendTag(group, 6, protowire.BytesType)
	group = protowire.AppendBytes(group, item)
	payload := protowire.AppendTag(nil, 1, protowire.BytesType)
	return protowire.AppendBytes(payload, group)
}

func TestSubscriptionsOverContinuousGRPCWeb(t *testing.T) {
	const interval = 375 * time.Millisecond
	var statusPayload []byte
	for index, value := range []uint64{4096, 12, 3, 4, 1, 128, 256, 1024, 2048} {
		statusPayload = appendVarint(statusPayload, protowire.Number(index+1), value)
	}
	wantStatuses := []Status{
		{Memory: 4096, Goroutines: 12, ConnectionsIn: 3, ConnectionsOut: 4, TrafficAvailable: true,
			Uplink: 128, Downlink: 256, UplinkTotal: 1024, DownlinkTotal: 2048},
		{Memory: 8192},
		{},
	}
	wantGroups := [][]Group{
		{{Tag: "Select/fixture", Type: "selector", Selectable: true, Selected: "fixture/first", Expanded: true,
			Items: []GroupItem{{Tag: "fixture/first", Type: "vless", URLTestTime: 123456, URLTestDelay: 88}}}},
		{{Tag: "Select/fixture", Type: "selector", Selectable: true, Selected: "fixture/second", Expanded: true,
			Items: []GroupItem{{Tag: "fixture/second", Type: "vless", URLTestTime: 123456, URLTestDelay: 88}}}},
		nil,
	}
	for _, test := range []struct {
		name           string
		method         string
		requestPayload []byte
		payloads       [][]byte
		call           func(*Client, context.Context, chan<- struct{}) error
	}{
		{"status", methodSubscribeStatus, appendVarint(nil, 1, uint64(interval)),
			[][]byte{statusPayload, appendVarint(nil, 1, 8192), nil},
			func(client *Client, ctx context.Context, consumed chan<- struct{}) error {
				var statuses []Status
				err := client.SubscribeStatus(ctx, interval, func(status Status) error {
					statuses = append(statuses, status)
					select {
					case consumed <- struct{}{}:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				if !reflect.DeepEqual(statuses, wantStatuses) {
					t.Errorf("状态快照不完整或残留上一帧字段: %#v", statuses)
				}
				return err
			}},
		{"groups", methodSubscribeGroups, nil,
			[][]byte{streamGroupsPayload("Select/fixture", "fixture/first"), streamGroupsPayload("Select/fixture", "fixture/second"), nil},
			func(client *Client, ctx context.Context, consumed chan<- struct{}) error {
				var snapshots [][]Group
				err := client.SubscribeGroups(ctx, func(groups []Group) error {
					snapshots = append(snapshots, groups)
					select {
					case consumed <- struct{}{}:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				if !reflect.DeepEqual(snapshots, wantGroups) {
					t.Errorf("分组快照累积或改写了上一帧: %#v", snapshots)
				}
				return err
			}},
	} {
		t.Run(test.name, func(t *testing.T) {
			consumed := make(chan struct{})
			client := streamHTTPClient(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != test.method {
					http.Error(writer, "unexpected method", http.StatusNotFound)
					return
				}
				for key, want := range map[string]string{
					"Authorization": "Bearer test-secret", "Content-Type": "application/grpc-web+proto",
					"Accept": "application/grpc-web+proto", "X-Grpc-Web": "1", "X-User-Agent": "netproxyctl/1",
				} {
					if got := request.Header.Get(key); got != want {
						t.Errorf("请求头 %s=%q，预期 %q", key, got, want)
						http.Error(writer, "invalid headers", http.StatusUnauthorized)
						return
					}
				}
				frame, err := io.ReadAll(request.Body)
				if err != nil || !bytes.Equal(frame, streamFrame(0, test.requestPayload)) {
					t.Errorf("请求帧或 interval 编码错误: %x, %v", frame, err)
					http.Error(writer, "invalid request", http.StatusBadRequest)
					return
				}
				for _, payload := range test.payloads {
					writeTestFrame(t, writer, 0, payload)
					writer.(http.Flusher).Flush()
					select {
					case <-consumed:
					case <-request.Context().Done():
						return
					}
				}
				writeTestFrame(t, writer, 0x80, []byte("grpc-status: 0\r\n"))
			})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			if err := test.call(client, ctx, consumed); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSubscriptionsFrameTermination(t *testing.T) {
	data := streamFrame(0, nil)
	success := streamFrame(0x80, []byte("grpc-status: 0\r\n"))
	type terminationTest struct {
		name          string
		body          []byte
		calls         int
		wantError     string
		unexpectedEOF bool
		retryable     bool
	}
	tests := []terminationTest{
		{name: "empty EOF", wantError: "ended without gRPC status"},
		{name: "complete frames without trailer", body: bytes.Repeat(data, 2), calls: 2},
		{name: "success trailer", body: append(bytes.Repeat(data, 2), success...), calls: 2},
		{name: "trailer only", body: success},
		{name: "authentication trailer", body: streamFrame(0x80, []byte("grpc-status: 16\r\ngrpc-message: invalid%20secret\r\n")), wantError: "gRPC status 16: invalid secret"},
		{name: "error after data", body: append(bytes.Repeat(data, 2), streamFrame(0x80, []byte("grpc-status: 14\r\ngrpc-message: unavailable\r\n"))...), calls: 2, wantError: "gRPC status 14: unavailable", retryable: true},
		{name: "missing status", body: append(bytes.Clone(data), streamFrame(0x80, []byte("grpc-message: fixture\r\n"))...), calls: 1, wantError: "has no grpc-status"},
		{name: "invalid status", body: append(bytes.Clone(data), streamFrame(0x80, []byte("grpc-status: invalid\r\n"))...), calls: 1, wantError: "invalid Service API grpc-status"},
		{name: "negative status", body: streamFrame(0x80, []byte("grpc-status: -1\r\n")), wantError: "has no grpc-status"},
		{name: "malformed protobuf", body: streamFrame(0, []byte{0x0d, 0, 0, 0, 0}), wantError: "invalid protobuf"},
		{name: "later malformed protobuf", body: append(bytes.Clone(data), streamFrame(0, []byte{0x0d, 0, 0, 0, 0})...), calls: 1, wantError: "invalid protobuf"},
		{name: "compressed frame", body: streamFrame(1, nil), wantError: "unsupported Service API frame flag"},
		{name: "later unsupported flag", body: append(bytes.Clone(data), streamFrame(2, nil)...), calls: 1, wantError: "unsupported Service API frame flag"},
	}
	for _, flag := range []byte{0, 0x80} {
		header := []byte{flag, 0, 0, 0, 2}
		for length := 1; length < len(header); length++ {
			tests = append(tests, terminationTest{name: fmt.Sprintf("truncated header %02x/%d", flag, length),
				body: append(bytes.Clone(data), header[:length]...), calls: 1, unexpectedEOF: true, retryable: true})
		}
		for _, suffix := range [][]byte{nil, {0}} {
			tests = append(tests, terminationTest{name: fmt.Sprintf("truncated payload %02x/%d", flag, len(suffix)),
				body: append(append(bytes.Clone(data), header...), suffix...), calls: 1, unexpectedEOF: true, retryable: true})
		}
		binary.BigEndian.PutUint32(header[1:], maxFrameSize+1)
		tests = append(tests, terminationTest{name: fmt.Sprintf("oversized frame %02x", flag),
			body: append(bytes.Clone(data), header...), calls: 1, wantError: "frame is too large"})
	}
	for _, subscription := range streamSubscriptions() {
		t.Run(subscription.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					client := streamHTTPClient(t, func(writer http.ResponseWriter, _ *http.Request) {
						_, _ = writer.Write(test.body)
					})
					calls := 0
					err := subscription.call(client, t.Context(), func() error { calls++; return nil })
					if calls != test.calls {
						t.Fatalf("消费次数=%d，预期 %d", calls, test.calls)
					}
					if test.unexpectedEOF {
						if !errors.Is(err, io.ErrUnexpectedEOF) {
							t.Fatalf("截断帧未返回 ErrUnexpectedEOF: %v", err)
						}
					} else if test.wantError != "" {
						if err == nil || !strings.Contains(err.Error(), test.wantError) {
							t.Fatalf("帧错误未保留: %v，预期 %q", err, test.wantError)
						}
					} else if err != nil {
						t.Fatalf("完整流结束失败: %v", err)
					}
					if IsRetryable(err) != test.retryable {
						t.Fatalf("重试分类变化: %v", err)
					}
				})
			}
		})
	}
}

type streamRoundTripper func(*http.Request) (*http.Response, error)

func (transport streamRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type streamBody struct {
	reader     io.Reader
	closer     io.Closer
	closed     chan struct{}
	closeOnce  sync.Once
	readBytes  atomic.Int64
	beforeRead func()
}

func (body *streamBody) Read(buffer []byte) (int, error) {
	if body.beforeRead != nil {
		body.beforeRead()
	}
	n, err := body.reader.Read(buffer)
	body.readBytes.Add(int64(n))
	return n, err
}

func (body *streamBody) Close() error {
	body.closeOnce.Do(func() {
		if body.closer != nil {
			_ = body.closer.Close()
		}
		close(body.closed)
	})
	return nil
}

func streamBodyClient(t *testing.T, body *streamBody) *Client {
	t.Helper()
	client, err := New("http://service-api.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = streamRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body, Request: request}, nil
	})
	t.Cleanup(func() { _ = body.Close(); _ = client.Close() })
	return client
}

func TestSubscriptionsBackpressureAndCancelDuringConsume(t *testing.T) {
	for _, subscription := range streamSubscriptions() {
		t.Run(subscription.name, func(t *testing.T) {
			frame := streamFrame(0, nil)
			body := &streamBody{reader: bytes.NewReader(bytes.Repeat(frame, 2)), closed: make(chan struct{})}
			client := streamBodyClient(t, body)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			calls := 0
			go func() {
				done <- subscription.call(client, ctx, func() error {
					calls++
					close(entered)
					<-release
					return nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("未同步进入 consumer")
			}
			if got := body.readBytes.Load(); got != int64(len(frame)) {
				t.Fatalf("consumer 返回前预读了下一帧: %d", got)
			}
			cancel()
			select {
			case <-body.closed:
			case <-time.After(3 * time.Second):
				t.Fatal("consumer 阻塞期间取消未关闭 body")
			}
			select {
			case err := <-done:
				t.Fatalf("consumer 尚未返回，订阅却已退出: %v", err)
			default:
			}
			unblock()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) || calls != 1 {
					t.Fatalf("取消后继续消费或错误丢失: calls=%d, err=%v", calls, err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("consumer 返回后订阅未退出")
			}
		})
	}
}

func TestSubscriptionsCancelBlockedRead(t *testing.T) {
	for _, subscription := range streamSubscriptions() {
		t.Run(subscription.name, func(t *testing.T) {
			for _, prefix := range [][]byte{nil, {0, 0, 0, 0, 2}} {
				for _, deadline := range []bool{false, true} {
					t.Run(fmt.Sprintf("prefix=%d/deadline=%v", len(prefix), deadline), func(t *testing.T) {
						reader, writer := io.Pipe()
						defer writer.Close()
						body := &streamBody{reader: io.MultiReader(bytes.NewReader(prefix), reader), closer: reader, closed: make(chan struct{})}
						reading := make(chan struct{})
						var readingOnce sync.Once
						body.beforeRead = func() {
							if body.readBytes.Load() == int64(len(prefix)) {
								readingOnce.Do(func() { close(reading) })
							}
						}
						client := streamBodyClient(t, body)
						ctx, cancel := context.WithCancel(t.Context())
						if deadline {
							cancel()
							ctx, cancel = context.WithTimeout(t.Context(), 200*time.Millisecond)
						}
						defer cancel()
						done := make(chan error, 1)
						go func() {
							done <- subscription.call(client, ctx, func() error {
								t.Error("取消时消费了不完整帧")
								return nil
							})
						}()
						select {
						case <-reading:
						case <-time.After(3 * time.Second):
							t.Fatal("未进入阻塞读取")
						}
						want := context.DeadlineExceeded
						if !deadline {
							want = context.Canceled
							cancel()
						}
						select {
						case err := <-done:
							if !errors.Is(err, want) {
								t.Fatalf("取消错误未保留: %v，预期 %v", err, want)
							}
						case <-time.After(3 * time.Second):
							t.Fatal("取消未解除 body 读取阻塞")
						}
						select {
						case <-body.closed:
						default:
							t.Fatal("订阅退出时未关闭 body")
						}
					})
				}
			}
		})
	}
}

func TestSubscriptionsConsumerError(t *testing.T) {
	for _, subscription := range streamSubscriptions() {
		t.Run(subscription.name, func(t *testing.T) {
			for _, want := range []error{errors.New("consumer stopped"), io.EOF} {
				t.Run(want.Error(), func(t *testing.T) {
					frame := streamFrame(0, nil)
					body := &streamBody{reader: bytes.NewReader(bytes.Repeat(frame, 2)), closed: make(chan struct{})}
					client := streamBodyClient(t, body)
					calls := 0
					err := subscription.call(client, t.Context(), func() error { calls++; return want })
					if err != want || calls != 1 || body.readBytes.Load() != int64(len(frame)) {
						t.Fatalf("consumer 错误被吞掉或未停止读帧: calls=%d, err=%v", calls, err)
					}
					select {
					case <-body.closed:
					default:
						t.Fatal("consumer 错误后未关闭 body")
					}
				})
			}
		})
	}
}

func TestSubscriptionsFragmentedReads(t *testing.T) {
	frames := append(bytes.Repeat(streamFrame(0, nil), 3), streamFrame(0x80, []byte("grpc-status: 0\r\n"))...)
	for _, subscription := range streamSubscriptions() {
		t.Run(subscription.name, func(t *testing.T) {
			body := &streamBody{reader: iotest.OneByteReader(bytes.NewReader(frames)), closed: make(chan struct{})}
			client := streamBodyClient(t, body)
			calls := 0
			err := subscription.call(client, t.Context(), func() error { calls++; return nil })
			if err != nil || calls != 3 {
				t.Fatalf("跨短读拆分的帧未完整消费: calls=%d, err=%v", calls, err)
			}
		})
	}
}

func TestReadFrameRejectsWrappedEOFWithinFrame(t *testing.T) {
	header := []byte{0, 0, 0, 0, 2}
	for length := 1; length <= len(header)+1; length++ {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			content := append(bytes.Clone(header), 0)
			reader := io.MultiReader(bytes.NewReader(content[:length]), iotest.ErrReader(fmt.Errorf("read: %w", io.EOF)))
			if _, _, err := readFrame(reader); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("包装的 EOF 将部分帧当成流结束: %v", err)
			}
		})
	}
}

func TestSubscriptionsHTTPError(t *testing.T) {
	for _, subscription := range streamSubscriptions() {
		for _, code := range []int{http.StatusForbidden, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("%s/%d", subscription.name, code), func(t *testing.T) {
				client := streamHTTPClient(t, func(writer http.ResponseWriter, _ *http.Request) {
					http.Error(writer, "fixture", code)
				})
				err := subscription.call(client, t.Context(), func() error {
					t.Error("HTTP 错误不应调用 consumer")
					return nil
				})
				if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d:", code)) || IsRetryable(err) != (code == http.StatusServiceUnavailable) {
					t.Fatalf("HTTP 错误或重试分类丢失: %v", err)
				}
			})
		}
	}
}

func TestSubscriptionsRejectNilConsumer(t *testing.T) {
	client := streamHTTPClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		t.Error("nil consumer 不应发送请求")
		writer.WriteHeader(http.StatusInternalServerError)
	})
	if err := client.SubscribeStatus(t.Context(), time.Second, nil); err == nil {
		t.Fatal("未拒绝 nil status consumer")
	}
	if err := client.SubscribeGroups(t.Context(), nil); err == nil {
		t.Fatal("未拒绝 nil groups consumer")
	}
}

func TestUnaryFrameTermination(t *testing.T) {
	data := streamFrame(0, appendVarint(nil, 1, 654321))
	for _, test := range []struct {
		name      string
		tail      []byte
		wantError string
	}{
		{"complete frame without trailer", nil, ""},
		{"success trailer", streamFrame(0x80, []byte("grpc-status: 0\r\n")), ""},
		{"error trailer", streamFrame(0x80, []byte("grpc-status: 14\r\n")), "gRPC status 14"},
		{"invalid trailer", streamFrame(0x80, []byte("grpc-status: invalid\r\n")), "invalid Service API grpc-status"},
		{"partial header 1", []byte{0}, "unexpected EOF"},
		{"partial header 2", []byte{0, 0}, "unexpected EOF"},
		{"partial header 3", []byte{0, 0, 0}, "unexpected EOF"},
		{"partial header 4", []byte{0, 0, 0, 0}, "unexpected EOF"},
		{"missing payload", []byte{0, 0, 0, 0, 2}, "unexpected EOF"},
		{"partial payload", []byte{0, 0, 0, 0, 2, 0}, "unexpected EOF"},
		{"missing trailer payload", []byte{0x80, 0, 0, 0, 2}, "unexpected EOF"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := streamHTTPClient(t, func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write(append(bytes.Clone(data), test.tail...))
			})
			startedAt, err := client.StartedAt(t.Context())
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) || startedAt.UnixMilli != 0 {
					t.Fatalf("unary 接受了截断帧或错误 trailer: %#v, %v", startedAt, err)
				}
			} else if err != nil || startedAt.UnixMilli != 654321 {
				t.Fatalf("unary 完整帧兼容回归: %#v, %v", startedAt, err)
			}
		})
	}
	t.Run("empty EOF is still rejected", func(t *testing.T) {
		client := streamHTTPClient(t, func(http.ResponseWriter, *http.Request) {})
		if _, err := client.StartedAt(t.Context()); err == nil || !strings.Contains(err.Error(), "ended without gRPC status") {
			t.Fatalf("unary 空响应被接受: %v", err)
		}
	})
}

func TestSnapshotsReturnBeforeStreamEnds(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload []byte
		call    func(*Client, context.Context) error
	}{
		{"status", appendVarint(nil, 1, 4096), func(client *Client, ctx context.Context) error {
			status, err := client.Status(ctx)
			if status.Memory != 4096 {
				t.Errorf("首帧状态丢失: %#v", status)
			}
			return err
		}},
		{"groups", streamGroupsPayload("Select/fixture", "fixture/first"), func(client *Client, ctx context.Context) error {
			groups, err := client.Groups(ctx)
			if len(groups) != 1 || groups[0].Selected != "fixture/first" {
				t.Errorf("首帧分组丢失: %#v", groups)
			}
			return err
		}},
		{"outbounds", nil, func(client *Client, ctx context.Context) error {
			_, err := client.Outbounds(ctx)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			closed := make(chan struct{})
			client := streamHTTPClient(t, func(writer http.ResponseWriter, request *http.Request) {
				defer close(closed)
				writeTestFrame(t, writer, 0, test.payload)
				writer.(http.Flusher).Flush()
				<-request.Context().Done()
			})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			if err := test.call(client, ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case <-closed:
			case <-ctx.Done():
				t.Fatal("首帧返回后未关闭流")
			}
		})
	}
}
