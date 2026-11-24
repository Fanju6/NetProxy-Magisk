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
		_, _ = io.WriteString(w, "selected")
	}))
	t.Cleanup(origin.Close)
	dial := startCaptureTestCore(t, core, layout)
	client := &http.Client{Transport: &http.Transport{DialContext: dial, DisableKeepAlives: true}, Timeout: 5 * time.Second}
	request := func(want string) {
		t.Helper()
		res, err := client.Get(origin.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if content, err := io.ReadAll(res.Body); err != nil || string(content) != want {
			t.Fatalf("response = %q, %v; want %q", content, err, want)
		}
	}
	request("SELECTED")
	if got := controller.Entries(); len(got) != 1 || got[0].Outbound != "match-original-inbound" || got[0].Rewrite != "response" {
		t.Fatalf("unexpected capture: %+v", got)
	}
	controller.Stop()
	request("selected")
}

func startCaptureTestCore(t *testing.T, core string, layout paths.Layout) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	reservation, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	port := reservation.Addr().(*net.TCPAddr).Port
	reservation.Close()
	config := fmt.Sprintf(`{"inbounds":[{"type":"mixed","tag":"netproxy-in","listen":"127.0.0.1","listen_port":%d}],"outbounds":[{"type":"direct","tag":"match-original-inbound"},{"type":"block","tag":"block"}],"route":{"netproxy_capture_socket":%q,"final":"block","rules":[{"inbound":"netproxy-in","action":"route","outbound":"match-original-inbound"}]}}`, port, layout.TrafficSocket())
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
	dialer, err := proxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return dialer.(proxy.ContextDialer).DialContext
}
