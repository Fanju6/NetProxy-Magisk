//go:build linux

package traffic

import (
	"bufio"
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

func TestCaptureRawUpgradeWithRequestBodyAndBufferedPayload(t *testing.T) {
	layout := paths.New(t.TempDir())
	c := New(layout)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "body" {
			t.Errorf("origin request body = %q, %v", body, err)
			return
		}
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: fixture\r\n\r\n")
		if err := rw.Flush(); err != nil {
			t.Error(err)
			return
		}
		payload := make([]byte, len("earlylate"))
		if _, err := io.ReadFull(rw, payload); err != nil {
			t.Error(err)
			return
		}
		_, _ = conn.Write(payload)
	}))
	defer origin.Close()
	conn, err := bridgeFixture(layout)(t.Context(), "tcp", strings.TrimPrefix(origin.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(conn, "POST / HTTP/1.1\r\nHost: fixture.test\r\nConnection: Upgrade\r\nUpgrade: fixture\r\nContent-Length: 4\r\n\r\nbodyearly"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("unexpected upgrade status: %d", response.StatusCode)
	}
	if _, err := io.WriteString(conn, "late"); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, len("earlylate"))
	if _, err := io.ReadFull(reader, payload); err != nil || string(payload) != "earlylate" {
		t.Fatalf("relayed payload = %q, %v", payload, err)
	}
}

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
		{"/blocked", "request", "return {reject: true}", []string{"127.0.0.1"}, nil, ""},
		{"/redirect", "request", `return {redirect: "https://example.org/new"}`, []string{"127.0.0.1"}, nil, ""},
		{"/rewrite", "request", `return {set_headers: {"X-Edited": "yes"}}`, []string{"127.0.0.1"}, nil, ""},
		{"/rewrite", "response", `return {body: input.body.toUpperCase(), set_headers: {"X-Rewritten": "yes"}}`, []string{"127.0.0.1"}, nil, ""},
		{"/timeout", "response", `while (true) {}`, []string{"127.0.0.1"}, nil, ""},
	}
	if err := config.ReplaceRules(rules); err != nil {
		t.Fatal(err)
	}
	if err := config.ReplaceRules([]Rule{{"[", "request", "return {}", []string{"127.0.0.1"}, nil, ""}}); err == nil {
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
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	rules = append(rules, Rule{Pattern: "/rewrite/", Phase: "request", Script: `return {set_headers: {"X-Edited": "fallback"}}`, Domains: []string{"127.0.0.1"}})
	rules[2].Enabled = new(bool)
	if err := controller.ReplaceRules(rules); err != nil {
		t.Fatal(err)
	}
	headers = do(plain.URL+"/rewrite/test", http.StatusOK, "ORIGINAL")
	if headers.Get("X-Request") != "fallback" {
		t.Fatalf("disabled rule shadowed enabled fallback: %v", headers)
	}
	entries = controller.Entries()
	if len(entries) != 1 {
		t.Fatalf("unexpected records with disabled rule: %+v", entries)
	}
	record, ok := controller.Record(entries[0].ID)
	if !ok || record.HTTP == nil || len(record.HTTP.Rewrites) != 2 || record.HTTP.Rewrites[0].RuleIndex != 3 || record.HTTP.Rewrites[1].RuleIndex != 5 {
		t.Fatalf("unexpected audits with disabled rule: %+v", record.HTTP)
	}
	*rules[2].Enabled = true
	if err := controller.ReplaceRules(rules); err != nil {
		t.Fatal(err)
	}
	headers = do(plain.URL+"/rewrite/test", http.StatusOK, "ORIGINAL")
	if headers.Get("X-Request") != "yes" {
		t.Fatalf("re-enabled rule did not regain priority: %v", headers)
	}
}

func TestRequestBodyRewritePreservesFramingAndInput(t *testing.T) {
	type received struct {
		body, encoding, digest string
		length                 int64
		chunked                bool
	}
	upstream := make(chan received, 1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		upstream <- received{string(body), r.Header.Get("Content-Encoding"), r.Header.Get("Content-Digest"), r.ContentLength, len(r.TransferEncoding) != 0}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()
	layout := paths.New(t.TempDir())
	controller := New(layout)
	if err := controller.ReplaceRules([]Rule{
		{Pattern: "/edit$", Phase: "request", Domains: []string{"127.0.0.1"},
			Script: `return {body: input.body === "empty" ? "" : (input.body || "") + "!", set_headers: {"X-Rewrite": "yes"}};`},
	}); err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	transport := &http.Transport{DialContext: bridgeFixture(layout), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	for _, test := range []struct {
		name, body, encoding, contentType string
		chunked, changed                  bool
		want                              string
	}{
		{"fixed", "sample", "", "text/plain", false, true, "sample!"},
		{"chunked", "sample", "", "application/json", true, true, "sample!"},
		{"empty result", "empty", "", "text/plain", true, true, ""},
		{"oversized chunked", strings.Repeat("a", maxBody+1), "", "text/plain", true, false, ""},
		{"oversized fixed", strings.Repeat("a", maxBody+1), "", "text/plain", false, false, ""},
		{"encoded", "compressed-bytes", "gzip", "text/plain", false, false, ""},
		{"non-UTF-8", "\xff", "", "text/plain", false, false, ""},
		{"binary", "sample", "", "application/octet-stream", false, false, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, origin.URL+"/edit", strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			if test.chunked {
				req.ContentLength = -1
			}
			req.Header.Set("Content-Type", test.contentType)
			req.Header.Set("Content-Digest", "fixture")
			if test.encoding != "" {
				req.Header.Set("Content-Encoding", test.encoding)
			}
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("unexpected status %d", res.StatusCode)
			}
			got := <-upstream
			record, ok := controller.Record(controller.Entries()[0].ID)
			if !ok || record.HTTP == nil || len(record.HTTP.Rewrites) != 1 {
				t.Fatalf("missing rewrite audit: %+v", record.HTTP)
			}
			audit := record.HTTP.Rewrites[0]
			if test.changed {
				if got.body != test.want || got.length != int64(len(test.want)) || got.chunked || got.digest != "" || got.encoding != "" {
					t.Fatalf("wrong rewritten body/framing: %+v", got)
				}
				if audit.Body == nil || *audit.Body != (BodyChange{len(test.body), len(test.want)}) || record.HTTP.Request.Body.Data != test.body {
					t.Fatalf("wrong body audit or captured original: %+v", record.HTTP)
				}
			} else if got.body != test.body || got.encoding != test.encoding || got.digest != "fixture" || audit.Error != "body_unavailable" {
				t.Fatalf("unavailable body changed (length=%d, encoding=%q, digest=%q): %+v", len(got.body), got.encoding, got.digest, audit)
			}
		})
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
	preview := c.Preview()
	if !preview.Running || preview.Count != maxEntries+7 || len(preview.Entries) != 3 ||
		preview.Entries[0].ID != entries[0].ID || preview.Entries[2].ID != entries[2].ID {
		t.Fatalf("unexpected live preview: %+v", preview)
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
