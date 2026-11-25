package traffic

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"golang.org/x/net/proxy"
)

func TestPatchedCoreKeepsOriginalInboundRoute(t *testing.T) {
	core := os.Getenv("NETPROXY_TEST_CAPTURE_CORE")
	if core == "" {
		t.Skip("set NETPROXY_TEST_CAPTURE_CORE to the patched Linux sing-box")
	}
	layout := paths.New(t.TempDir())
	controller := New(layout)
	if err := controller.ReplaceRules([]Rule{{"^http://", "response", `return {body: input.body.toUpperCase()};`}}); err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.Stop)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Capture-Response", "http")
		_, _ = io.WriteString(w, "selected")
	}))
	t.Cleanup(origin.Close)
	dial := startCaptureTestCore(t, core, layout)
	client := &http.Client{Transport: &http.Transport{DialContext: dial, DisableKeepAlives: true}, Timeout: 5 * time.Second}
	request := func(want string) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, origin.URL+"/path?q=fixture", strings.NewReader("hello"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Capture-Request", "http")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if content, err := io.ReadAll(res.Body); err != nil || string(content) != want {
			t.Fatalf("response = %q, %v; want %q", content, err, want)
		}
	}
	request("SELECTED")
	got := controller.Entries()
	if len(got) != 1 || got[0].Outbound != "match-original-inbound" || got[0].Rewrite != "response" || got[0].Target != "/path?q=fixture" {
		t.Fatalf("unexpected capture: %+v", got)
	}
	record, ok := controller.Record(got[0].ID)
	if !ok || record.HTTP == nil || record.HTTP.URL != origin.URL+"/path?q=fixture" ||
		record.HTTP.Request.Headers.Get("X-Capture-Request") != "http" || record.HTTP.Request.Body.Data != "hello" ||
		record.HTTP.Response.Headers.Get("X-Capture-Response") != "http" || record.HTTP.Response.Body.Data != "SELECTED" {
		t.Fatalf("HTTP capture detail mismatch: %+v", record.HTTP)
	}
	controller.Stop()
	request("selected")
}

func startCaptureTestCore(t *testing.T, core string, layout paths.Layout) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	return startCaptureTestCoreWithVLESS(t, core, layout, false)
}

func TestPatchedCorePreservesServerFirstHandshake(t *testing.T) {
	core := os.Getenv("NETPROXY_TEST_CAPTURE_CORE")
	if core == "" {
		t.Skip("set NETPROXY_TEST_CAPTURE_CORE to the patched Linux sing-box")
	}
	layout := paths.New(t.TempDir())
	controller := New(layout)
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.Stop)
	origin, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = origin.Close() })
	go func() {
		if conn, err := origin.Accept(); err == nil {
			defer conn.Close()
			_, _ = io.WriteString(conn, "server-first")
		}
	}()
	dial := startCaptureTestCoreWithVLESS(t, core, layout, true)
	conn, err := dial(t.Context(), "tcp", origin.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	data, err := io.ReadAll(conn)
	if err != nil || string(data) != "server-first" {
		t.Fatalf("server-first data = %q, %v", data, err)
	}
	if entries := controller.Entries(); len(entries) != 1 || entries[0].Outbound != "match-original-inbound" {
		t.Fatalf("unexpected capture: %+v", entries)
	}
}

func startCaptureTestCoreWithVLESS(t *testing.T, core string, layout paths.Layout, vless bool) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	address := startCaptureTestSOCKS(t, core, layout, vless)
	dialer, err := proxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return dialer.(proxy.ContextDialer).DialContext
}

func startCaptureTestSOCKS(t *testing.T, core string, layout paths.Layout, vless bool) string {
	t.Helper()
	address, port := reserveCaptureTestPort(t)
	upstreamInbound, upstreamOutbound := "", `{"type":"direct","tag":"match-original-inbound"}`
	if vless {
		_, relayPort := reserveCaptureTestPort(t)
		const uuid = "50ddddd4-e3a9-4f33-af4c-e989629769d3"
		upstreamInbound = fmt.Sprintf(`,{"type":"vless","tag":"test-relay","listen":"127.0.0.1","listen_port":%d,"users":[{"uuid":%q}]}`, relayPort, uuid)
		upstreamOutbound = fmt.Sprintf(`{"type":"vless","tag":"match-original-inbound","server":"127.0.0.1","server_port":%d,"uuid":%q}`, relayPort, uuid)
	}
	config := fmt.Sprintf(`{"inbounds":[{"type":"mixed","tag":"netproxy-in","listen":"127.0.0.1","listen_port":%d}%s],"outbounds":[%s,{"type":"direct","tag":"direct"},{"type":"block","tag":"block"}],"route":{"netproxy_capture_socket":%q,"final":"block","rules":[{"inbound":"netproxy-in","action":"route","outbound":"match-original-inbound"},{"inbound":"test-relay","action":"route","outbound":"direct"}]}}`, port, upstreamInbound, upstreamOutbound, layout.TrafficSocket())
	configPath := filepath.Join(layout.Root(), "test-core.json")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(layout.Root(), "test-core.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), core, "run", "-c", configPath)
	cmd.Stdout, cmd.Stderr = log, log
	err = cmd.Start()
	log.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			content, _ := os.ReadFile(logPath)
			t.Log(strings.TrimSpace(string(content)))
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond); err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("core did not listen")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return address
}

func reserveCaptureTestPort(t *testing.T) (string, int) {
	t.Helper()
	reservation, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer reservation.Close()
	return reservation.Addr().String(), reservation.Addr().(*net.TCPAddr).Port
}
