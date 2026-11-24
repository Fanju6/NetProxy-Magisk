package traffic

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCaptureAndRewritingThroughMixedInbound(t *testing.T) {
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
	path := filepath.Join(t.TempDir(), "traffic-rules.json")
	config := New(path)
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
	controller := New(path)
	if loaded, err := controller.Rules(); err != nil || len(loaded) != len(rules) {
		t.Fatalf("规则未保留: %v, %v", loaded, err)
	}
	if err := controller.Start(socksFixture(t)); err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	access, err := controller.Access()
	if err != nil {
		t.Fatal(err)
	}
	proxyURL := &url.URL{Scheme: "http", Host: access.Listen}
	transport := encrypted.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
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
	do(plain.URL+"/rewrite", http.StatusProxyAuthRequired, "Proxy Authentication Required\n")
	proxyURL.User = url.UserPassword(access.Username, access.Password)
	headers := do(plain.URL+"/rewrite/private?token=synthetic", http.StatusOK, "ORIGINAL")
	if headers.Get("X-Request") != "yes" || headers.Get("X-Rewritten") != "yes" || headers.Get("ETag") != "" {
		t.Fatalf("unexpected headers: %v", headers)
	}
	do(plain.URL+"/blocked", http.StatusForbidden, "Rejected by rewrite rule\n")
	if headers := do(plain.URL+"/redirect", http.StatusFound, ""); headers.Get("Location") != "https://example.org/new" {
		t.Fatalf("unexpected redirect: %v", headers)
	}
	do(plain.URL+"/timeout", http.StatusOK, body)
	// TLS 保持端到端；代理只能看到 CONNECT。
	do(encrypted.URL+"/rewrite", http.StatusOK, body)
	entries := controller.Entries()
	if len(entries) != 5 || entries[0].Method != http.MethodConnect || entries[1].Rewrite != "response_skipped" ||
		entries[4].Rewrite != "request+response" || strings.Contains(entries[4].Host, "synthetic") {
		t.Fatalf("unexpected capture: %+v", entries)
	}
	controller.Stop()
	if status := controller.Status(); status.Running || status.Listen != "" || status.Count != 5 {
		t.Fatalf("unexpected stopped state: %+v", status)
	}
	controller.Clear()
	if entries := controller.Entries(); len(entries) != 0 {
		t.Fatalf("clear left entries: %+v", entries)
	}
}

func TestCaptureRetainsNewestEntries(t *testing.T) {
	c := New("")
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

func socksFixture(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSOCKSFixture(conn)
		}
	}()
	return listener.Addr().String()
}

func serveSOCKSFixture(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	var greeting [3]byte
	if _, err := io.ReadFull(client, greeting[:]); err != nil || greeting != [3]byte{5, 1, 0} {
		return
	}
	_, _ = client.Write([]byte{5, 0})
	var request [10]byte
	if _, err := io.ReadFull(client, request[:]); err != nil || request[0] != 5 || request[1] != 1 ||
		request[3] != 1 || request[4] != 127 {
		return
	}
	address := net.JoinHostPort(net.IP(request[4:8]).String(), strconv.Itoa(int(binary.BigEndian.Uint16(request[8:]))))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp4", address)
	if err != nil {
		return
	}
	defer upstream.Close()
	_, _ = client.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	done := make(chan struct{})
	go func() { defer close(done); _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	<-done
}
