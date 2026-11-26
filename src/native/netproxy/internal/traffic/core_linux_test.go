package traffic

import (
	"context"
	"errors"
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
	if err := controller.ReplaceRules([]Rule{
		{"^http://", "request", `return {set_headers: {"X-Capture-Request":"changed"}};`, []string{"127.0.0.1"}, nil, ""},
		{"^http://", "request", `return {reject: true};`, []string{"127.0.0.1"}, nil, ""},
		{"^http://", "response", `return {body: input.body.toUpperCase()};`, []string{"127.0.0.1"}, nil, ""},
	}); err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.Stop)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, err := io.ReadAll(r.Body); err != nil || string(body) != "hello" {
			t.Errorf("unexpected upstream request body: %q, %v", body, err)
		}
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
	if len(got) != 1 || got[0].Outbound != "match-original-inbound" || got[0].Rewrite != "request+response" || !got[0].Rewritten || got[0].Target != "/path?q=fixture" {
		t.Fatalf("unexpected capture: %+v", got)
	}
	record, ok := controller.Record(got[0].ID)
	if !ok || record.HTTP == nil || record.HTTP.URL != origin.URL+"/path?q=fixture" ||
		record.HTTP.Request.Headers.Get("X-Capture-Request") != "http" || record.HTTP.Request.Body.Data != "hello" ||
		record.HTTP.Response.Headers.Get("X-Capture-Response") != "http" || record.HTTP.Response.Body.Data != "SELECTED" {
		t.Fatalf("HTTP capture detail mismatch: %+v", record.HTTP)
	}
	exit := record.Exit
	if exit == nil || exit.Inbound != "netproxy-in" || exit.Remote != strings.TrimPrefix(origin.URL, "http://") ||
		len(exit.Chain) != 2 || exit.Chain[0] != (ExitHop{"match-original-inbound", "selector"}) ||
		exit.Chain[1] != (ExitHop{"capture-node", "direct"}) {
		t.Fatalf("unexpected capture exit: %+v", exit)
	}
	audit := record.HTTP.Rewrites
	if len(audit) != 3 || audit[0].State != "applied" || len(audit[0].Headers) != 1 ||
		strings.Join(audit[0].Headers[0].Before, ",") != "http" || strings.Join(audit[0].Headers[0].After, ",") != "changed" ||
		audit[1].State != "shadowed" || audit[2].State != "applied" ||
		audit[2].Body == nil || *audit[2].Body != (BodyChange{8, 8}) || audit[2].OriginalStatus != http.StatusOK {
		t.Fatalf("unexpected rewrite audit: %+v", audit)
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

func TestPatchedCoreReplayRoutesEditedOriginWithOriginalApp(t *testing.T) {
	core := os.Getenv("NETPROXY_TEST_CAPTURE_CORE")
	if core == "" {
		t.Skip("set NETPROXY_TEST_CAPTURE_CORE to the patched Linux sing-box")
	}
	var received = make(chan string, 4)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- r.Method + " " + r.URL.Path + " " + r.Header.Get("X-Replay") + " " + string(body)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "origin response")
	})
	first, second, untrusted := httptest.NewServer(handler), httptest.NewServer(handler), httptest.NewTLSServer(handler)
	defer first.Close()
	defer second.Close()
	defer untrusted.Close()
	layout := paths.New(t.TempDir())
	controller := New(layout)
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	owner := &SourceApp{UID: 1010123, UserID: 10, Package: "com.example.replay"}
	exit := &Exit{Inbound: "netproxy-in", Chain: []ExitHop{{Tag: "match-original-inbound", Type: "selector"}, {Tag: "capture-node", Type: "direct"}}}
	saved := func(app *SourceApp) string {
		controller.recordDetail(controller.session, Entry{Method: http.MethodPost, Exit: exit, SourceApp: app}, &HTTPDetail{
			URL: first.URL + "/original",
			Request: CapturedMessage{
				Headers: http.Header{"X-Replay": {"before"}},
				Body:    CapturedBody{Data: "before body", Format: "text"},
			},
		})
		return controller.Entries()[0].ID
	}
	original := saved(owner)
	req := ReplayInput{ID: original}
	if _, err := controller.Replay(t.Context(), req); !errors.Is(err, ErrReplayCore) {
		t.Fatalf("replay without a running core = %v", err)
	}
	port := second.Listener.Addr().(*net.TCPAddr).Port
	startCaptureTestSOCKS(t, core, layout, false,
		fmt.Sprintf(`{"inbound":"netproxy-in","port":%d,"user_id":%d,"package_name":%q,"action":"route","outbound":"direct"}`, port, owner.UID, owner.Package))
	replay := func(input ReplayInput) (string, error) {
		t.Helper()
		for range 100 {
			controller.session.replayMu.Lock()
			available := controller.session.replayOffer != nil
			controller.session.replayMu.Unlock()
			if available {
				return controller.Replay(t.Context(), input)
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("core did not offer a replay connection")
		return "", nil
	}
	id, err := replay(req)
	if err != nil {
		t.Fatal(err)
	}
	quick, _ := controller.Record(id)
	if quick.ReplayOf != original || quick.ReplayRouteChanged || quick.Outbound != "match-original-inbound" || quick.HTTP.Response.Body.Data != "origin response" {
		t.Fatalf("quick resend = %+v", quick)
	}
	if got := <-received; got != "POST /original before before body" {
		t.Fatalf("quick request = %q", got)
	}
	req.Edit = &ReplayEdit{Method: http.MethodPut, URL: second.URL + "/edited", Headers: http.Header{"X-Replay": {"after"}}, Body: "after body"}
	id, err = replay(req)
	if err != nil {
		t.Fatal(err)
	}
	edited, _ := controller.Record(id)
	if edited.ReplayOf != original || !edited.ReplayRouteChanged || edited.Outbound != "direct" ||
		edited.ReplayOriginal.Chain[1].Tag != "capture-node" || edited.Exit.Package != owner.Package ||
		edited.SourceApp == nil || *edited.SourceApp != *owner {
		t.Fatalf("edited replay = %+v", edited)
	}
	if got := <-received; got != "PUT /edited after after body" {
		t.Fatalf("edited request = %q", got)
	}
	req.ID = saved(nil)
	id, err = replay(req)
	if err != nil {
		t.Fatal(err)
	}
	unattributed, _ := controller.Record(id)
	if !unattributed.ReplayUnattributed || unattributed.SourceApp != nil || unattributed.Outbound != "match-original-inbound" {
		t.Fatalf("unattributed replay = %+v", unattributed)
	}
	req.Edit.URL = untrusted.URL
	if _, err := replay(req); !errors.Is(err, ErrReplayResponse) {
		t.Fatalf("self-signed replay = %v", err)
	}
}

func startCaptureTestSOCKS(t *testing.T, core string, layout paths.Layout, vless bool, replayRules ...string) string {
	t.Helper()
	address, port := reserveCaptureTestPort(t)
	upstreamInbound, upstreamOutbound := "", `{"type":"direct","tag":"capture-node"}`
	if vless {
		_, relayPort := reserveCaptureTestPort(t)
		const uuid = "50ddddd4-e3a9-4f33-af4c-e989629769d3"
		upstreamInbound = fmt.Sprintf(`,{"type":"vless","tag":"test-relay","listen":"127.0.0.1","listen_port":%d,"users":[{"uuid":%q}]}`, relayPort, uuid)
		upstreamOutbound = fmt.Sprintf(`{"type":"vless","tag":"capture-node","server":"127.0.0.1","server_port":%d,"uuid":%q}`, relayPort, uuid)
	}
	rules := append(replayRules, `{"inbound":"netproxy-in","action":"route","outbound":"match-original-inbound"}`, `{"inbound":"test-relay","action":"route","outbound":"direct"}`)
	config := fmt.Sprintf(`{"inbounds":[{"type":"mixed","tag":"netproxy-in","listen":"127.0.0.1","listen_port":%d}%s],"outbounds":[%s,{"type":"selector","tag":"match-original-inbound","outbounds":["capture-node"]},{"type":"direct","tag":"direct"},{"type":"block","tag":"block"}],"route":{"netproxy_capture_socket":%q,"final":"block","rules":[%s]}}`, port, upstreamInbound, upstreamOutbound, layout.TrafficSocket(), strings.Join(rules, ","))
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
