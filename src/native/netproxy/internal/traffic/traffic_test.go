//go:build linux

package traffic

import (
	"context"
	json "encoding/json/v2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"golang.org/x/sys/unix"
)

func TestCaptureAndRewritingThroughCoreBridge(t *testing.T) {
	const body = "original"
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" || (r.TLS == nil && r.Header.Get("X-Hop") != "") {
			t.Error("代理凭据或逐跳请求头到达上游")
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("ETag", `"before"`)
		w.Header().Set("X-Request", r.Header.Get("X-Edited"))
		_, _ = io.WriteString(w, body)
	})
	plain := httptest.NewServer(origin)
	defer plain.Close()
	encrypted := httptest.NewTLSServer(origin)
	defer encrypted.Close()
	layout := paths.New(t.TempDir())
	config := New(layout)
	rules := []Rule{
		{"/blocked", "request", "return {reject: true}"},
		{"/redirect", "request", `return {redirect: "https://example.org/new"}`},
		{"/rewrite", "request", `return {set_headers: {"X-Edited": "yes"}}`},
		{"/rewrite", "response", `return {body: input.body.toUpperCase(), set_headers: {"X-Rewritten": "yes"}}`},
		{"/timeout", "response", `while (true) {}`},
	}
	if err := config.ReplaceRules(rules); err != nil {
		t.Fatal(err)
	}
	if err := config.ReplaceRules([]Rule{{"[", "request", "return {}"}}); err == nil {
		t.Fatal("接受了无效正则")
	}
	controller := New(layout)
	if loaded, err := controller.Rules(); err != nil || len(loaded) != len(rules) {
		t.Fatalf("规则未保留: %v, %v", loaded, err)
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	if info, err := os.Stat(layout.TrafficSocket()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket is not private: %v", err)
	}
	transport := encrypted.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = bridgeFixture(layout)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	do := func(target string, status int, wantBody string) http.Header {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Connection", "X-Hop")
		request.Header.Set("X-Hop", "do not forward")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		content, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != status || string(content) != wantBody {
			t.Fatalf("response %d %q (%v); want %d %q", response.StatusCode, content, err, status, wantBody)
		}
		return response.Header
	}
	headers := do(plain.URL+"/rewrite/private?token=synthetic", http.StatusOK, "ORIGINAL")
	if headers.Get("X-Request") != "yes" || headers.Get("X-Rewritten") != "yes" || headers.Get("ETag") != "" {
		t.Fatalf("unexpected headers: %v", headers)
	}
	do(plain.URL+"/blocked", http.StatusForbidden, "Rejected by rewrite rule\n")
	if headers := do(plain.URL+"/redirect", http.StatusFound, ""); headers.Get("Location") != "https://example.org/new" {
		t.Fatalf("unexpected redirect: %v", headers)
	}
	do(plain.URL+"/timeout", http.StatusOK, body)
	// TLS 保持端到端；抓包只记录 TCP 元数据。
	do(encrypted.URL+"/rewrite", http.StatusOK, body)
	entries := controller.Entries()
	if len(entries) != 5 || entries[0].Method != "TCP" || entries[1].Rewrite != "response_skipped" ||
		entries[4].Outbound != "selected-by-inbound" ||
		entries[4].Rewrite != "request+response" || strings.Contains(entries[4].Host, "synthetic") {
		t.Fatalf("unexpected capture: %+v", entries)
	}
	controller.Stop()
	if status := controller.Status(); status.Running || status.Count != 5 {
		t.Fatalf("unexpected stopped state: %+v", status)
	}
	controller.Clear()
	if entries := controller.Entries(); len(entries) != 0 {
		t.Fatalf("clear left entries: %+v", entries)
	}
}

func TestRewritePanicSkipsScriptAndLogsOnlyFunctions(t *testing.T) {
	layout := paths.New(t.TempDir())
	s := &session{controller: New(layout), scriptSlots: make(chan struct{}, 1)}
	var match RewriteMatch
	result := s.runScript(t.Context(), nil, scriptInput{URL: "http://private.example/"}, &match)
	if match.Error != "script_error" || result.Reject || result.Redirect != "" || len(s.scriptSlots) != 0 {
		t.Fatalf("panic did not skip the rewrite: %+v, %+v", result, match)
	}
	data, err := os.ReadFile(layout.ServiceLog())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "[traffic.rewrite] [failed] [traffic.panic]") ||
		!strings.Contains(got, "compiledRule).run") || strings.Contains(got, "private.example") {
		t.Fatalf("unexpected panic diagnostic: %s", got)
	}
}

func TestCaptureRetainsNewestEntries(t *testing.T) {
	c := New(paths.New(t.TempDir()))
	current := &session{}
	c.session = current
	for i := range maxEntries + 7 {
		c.record(current, Entry{Host: strconv.Itoa(i)})
	}
	c.record(&session{}, Entry{Host: "old-session"})
	entries := c.Entries()
	if len(entries) != maxEntries || entries[0].Host != strconv.Itoa(maxEntries+6) || entries[maxEntries-1].Host != "7" {
		t.Fatalf("unexpected ring length or ordering: %v", entries)
	}
}

func bridgeFixture(layout paths.Layout) func(context.Context, string, string) (net.Conn, error) {
	return bridgeFixtureFor(layout, nil)
}

func bridgeFixtureFor(layout paths.Layout, exit *Exit) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: time.Second}
		origin, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		conn, err := dialer.DialContext(ctx, "unix", layout.TrafficSocket())
		if err != nil {
			origin.Close()
			return nil, err
		}
		bridge := conn.(*net.UnixConn)
		defer func() {
			if err != nil {
				origin.Close()
				bridge.Close()
			}
		}()
		var fds [2]int
		fds, err = unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		core, peer := os.NewFile(uintptr(fds[0]), "core"), os.NewFile(uintptr(fds[1]), "worker")
		defer core.Close()
		defer peer.Close()
		var relay net.Conn
		relay, err = net.FileConn(core)
		if err != nil {
			return nil, err
		}
		metadata, _ := json.Marshal(flow{Destination: address, Outbound: "selected-by-inbound", Exit: exit})
		_ = bridge.SetDeadline(time.Now().Add(time.Second))
		_, _, err = bridge.WriteMsgUnix(append(metadata, '\n'), unix.UnixRights(int(peer.Fd())), nil)
		var ready [1]byte
		if err == nil {
			_, err = io.ReadFull(bridge, ready[:])
		}
		if err != nil || ready != [1]byte{1} {
			relay.Close()
			err = errBridge
			return nil, err
		}
		_ = bridge.SetDeadline(time.Time{})
		go func() {
			defer relay.Close()
			defer origin.Close()
			(&session{}).relayStreams(relay, origin)
		}()
		return bridge, nil
	}
}

func TestCaptureAppsFilterBeforeAcknowledgingBridge(t *testing.T) {
	layout := paths.New(t.TempDir())
	c := New(layout)
	if _, err := c.SetAppsPolicy(AppsPolicy{Apps: []string{"10:app.same"}}); err != nil {
		t.Fatal(err)
	}
	c = New(layout)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "original")
	}))
	defer origin.Close()
	addr := strings.TrimPrefix(origin.URL, "http://")
	sameProfile, otherProfile := uint32(1012034), uint32(12034)
	for _, exit := range []*Exit{nil, {UID: &otherProfile, Package: "app.same"}, {UID: &sameProfile, Package: "app.other"}} {
		conn, err := bridgeFixtureFor(layout, exit)(t.Context(), "tcp", addr)
		if err == nil {
			conn.Close()
			t.Fatal("不应接管未选中的来源")
		}
	}
	client := &http.Client{Transport: &http.Transport{DialContext: bridgeFixtureFor(layout, &Exit{UID: &sameProfile, Package: "app.same"})}, Timeout: time.Second}
	resp, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	entries := c.Entries()
	if len(entries) != 1 || entries[0].SourceApp == nil || *entries[0].SourceApp != (SourceApp{UID: sameProfile, UserID: 10, Package: "app.same"}) {
		t.Fatalf("抓包来源不匹配: %+v", entries)
	}
	if _, err := c.SetAppsPolicy(AppsPolicy{Apps: []string{}}); err != nil || !c.capturesApp(nil) {
		t.Fatal("空列表应允许全部连接", err)
	}
}
