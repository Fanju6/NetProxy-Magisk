package traffic

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"golang.org/x/net/websocket"
)

func TestCaptureWebSocketThroughCoreBridge(t *testing.T) {
	for _, scheme := range []string{"ws", "wss"} {
		t.Run(scheme, func(t *testing.T) {
			handler := websocket.Handler(func(conn *websocket.Conn) {
				var message string
				if websocket.Message.Receive(conn, &message) == nil {
					_ = websocket.Message.Send(conn, "echo:"+message)
				}
			})
			var server *httptest.Server
			if scheme == "wss" {
				server = httptest.NewTLSServer(handler)
			} else {
				server = httptest.NewServer(handler)
			}
			defer server.Close()
			layout := paths.New(t.TempDir())
			controller := New(layout)
			if scheme == "wss" {
				controller.upstreamRoots = server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
				if _, err := controller.SetMITMPolicy(MITMPolicy{Enabled: true}); err != nil {
					t.Fatal(err)
				}
			}
			if err := controller.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(controller.Stop)
			dial := bridgeFixture(layout)
			if core := os.Getenv("NETPROXY_TEST_CAPTURE_CORE"); core != "" {
				dial = startCaptureTestCore(t, core, layout)
			}
			url := scheme + server.URL[strings.IndexByte(server.URL, ':'):]
			config, err := websocket.NewConfig(url, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			conn, err := dial(t.Context(), "tcp", config.Location.Host)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			if scheme == "wss" {
				ca, err := controller.Certificate(false)
				roots := x509.NewCertPool()
				if err != nil || !roots.AppendCertsFromPEM([]byte(ca.PEM)) {
					t.Fatal("missing capture root", err)
				}
				conn = tls.Client(conn, &tls.Config{RootCAs: roots, ServerName: config.Location.Hostname()})
			}
			socket, err := websocket.NewClient(config, conn)
			if err != nil {
				t.Fatal(err)
			}
			message := strings.Repeat("x", wsPreviewLimit+1)
			if err := websocket.Message.Send(socket, message); err != nil {
				t.Fatal(err)
			}
			var echoed string
			if err := websocket.Message.Receive(socket, &echoed); err != nil || echoed != "echo:"+message {
				t.Fatalf("echo mismatch: %q, %v", echoed, err)
			}
			_ = socket.Close()
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				entries := controller.Entries()
				if len(entries) == 0 {
					time.Sleep(time.Millisecond)
					continue
				}
				entry := entries[0]
				record, ok := controller.Record(entry.ID)
				if !ok || record.HTTP == nil || record.HTTP.WebSocket == nil || record.Status != 101 ||
					!strings.HasPrefix(record.HTTP.URL, scheme+"://") || len(record.HTTP.WebSocket.Frames) < 2 {
					t.Fatalf("missing WebSocket detail: %+v", record)
				}
				frames := record.HTTP.WebSocket.Frames
				if frames[0].Direction != "send" || frames[0].Opcode != 1 || frames[0].Size != uint64(len(message)) ||
					len(frames[0].Text) != wsPreviewLimit || !frames[0].Truncated ||
					frames[1].Direction != "receive" || !strings.HasPrefix(frames[1].Text, "echo:") {
					t.Fatalf("unexpected WebSocket frames: %+v", frames)
				}
				return
			}
			t.Fatal("capture not recorded")
		})
	}
}
