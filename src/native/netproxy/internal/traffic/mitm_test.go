//go:build linux

package traffic

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestHTTPSRewriteAndNextConnectionBypass(t *testing.T) {
	t.Run("bridge", func(t *testing.T) { testTransparentHTTPS(t, "") })
	if core := os.Getenv("NETPROXY_TEST_CAPTURE_CORE"); core != "" {
		t.Run("patched core", func(t *testing.T) { testTransparentHTTPS(t, core) })
	}
}

func TestTLSExclusionAndUpstreamFailureDoNotPin(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "upstream")
	}))
	defer origin.Close()
	layout := paths.New(t.TempDir())
	c := New(layout)
	if _, err := c.SetMITMPolicy(MITMPolicy{Enabled: true, BypassHosts: []string{"127.0.0.1"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	uid := uint32(10123)
	get := func(trust *x509.CertPool, status int) {
		t.Helper()
		transport := &http.Transport{
			DialContext:     bridgeFixtureFor(layout, &Exit{UID: &uid, Package: "com.example.capture"}),
			TLSClientConfig: &tls.Config{RootCAs: trust},
		}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		response, err := client.Get(origin.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		if response.StatusCode != status {
			t.Fatalf("unexpected HTTP status %d, wanted %d", response.StatusCode, status)
		}
	}
	get(origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, http.StatusOK)
	if entries := c.Entries(); len(entries) != 1 || entries[0].PassthroughReason != "excluded_host" {
		t.Fatalf("expected explicit exclusion reason, got %+v", entries)
	}
	if _, err := c.SetMITMPolicy(MITMPolicy{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	public, err := c.Certificate(false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(public.PEM)) {
		t.Fatal("invalid debug CA")
	}
	get(roots, http.StatusBadGateway)
	if entries := c.Entries(); len(entries) != 2 || entries[0].PassthroughReason != "upstream_handshake_failed" {
		t.Fatalf("expected upstream TLS failure reason, got %+v", entries)
	}
	if pinned, err := c.PinnedBypasses(); err != nil || len(pinned) != 0 {
		t.Fatalf("upstream verification failure pinned client: %+v, %v", pinned, err)
	}
}

func TestClientTLSCloseAfterCertificateDoesNotPin(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer origin.Close()
	layout := paths.New(t.TempDir())
	c := New(layout)
	c.upstreamRoots = origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	if _, err := c.SetMITMPolicy(MITMPolicy{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	public, err := c.Certificate(false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(public.PEM)) {
		t.Fatal("invalid debug CA")
	}
	uid := uint32(10123)
	dial := bridgeFixtureFor(layout, &Exit{UID: &uid, Package: "com.example.capture"})
	for i := range 4 {
		raw, err := dial(t.Context(), "tcp", origin.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		gotCertificate := false
		client := tls.Client(raw, &tls.Config{
			ServerName: "127.0.0.1", RootCAs: roots,
			VerifyConnection: func(tls.ConnectionState) error {
				gotCertificate = true
				_ = raw.Close()
				return io.EOF
			},
		})
		err = client.HandshakeContext(t.Context())
		_ = raw.Close()
		if err == nil || !gotCertificate {
			t.Fatalf("connection %d did not close after receiving the certificate: %v", i, err)
		}
		deadline := time.Now().Add(time.Second)
		for len(c.Entries()) <= i && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		entries := c.Entries()
		if len(entries) != i+1 || entries[0].TLS != "handshake_failed" || entries[0].PassthroughReason != "client_handshake_closed" {
			t.Fatalf("unexpected close record: %+v", entries)
		}
	}
	if pinned, err := c.PinnedBypasses(); err != nil || len(pinned) != 0 {
		t.Fatalf("client close created a bypass: %+v, %v", pinned, err)
	}
}

func TestTLSHandshakeFailureClassification(t *testing.T) {
	for alert, reason := range map[tls.AlertError]string{
		42: "certificate_rejected", 46: "certificate_rejected", 48: "client_ca_untrusted", 49: "certificate_rejected",
	} {
		if got := remoteCertificateReason(&net.OpError{Op: "remote error", Err: alert}); got != reason {
			t.Errorf("remote alert %v: got %q, want %q", alert, got, reason)
		}
	}
	for _, tc := range []struct {
		err      error
		upstream bool
		reason   string
	}{
		{io.EOF, false, "client_handshake_closed"},
		{&net.OpError{Op: "read", Err: syscall.ECONNRESET}, false, "client_handshake_closed"},
		{&net.OpError{Op: "local error", Err: tls.AlertError(42)}, false, "client_handshake_failed"},
		{&net.OpError{Op: "remote error", Err: tls.AlertError(40)}, false, "client_handshake_failed"},
		{os.ErrDeadlineExceeded, false, "client_handshake_timeout"},
		{context.DeadlineExceeded, true, "upstream_handshake_timeout"},
		{&net.DNSError{Err: "fixture", Name: "example.invalid"}, true, "upstream_handshake_failed"},
	} {
		if remoteCertificateReason(tc.err) != "" {
			t.Errorf("%v incorrectly counted as a remote certificate alert", tc.err)
		}
		if got := tlsHandshakeReason(tc.err, tc.upstream); got != tc.reason {
			t.Errorf("reason for %v: got %q, want %q", tc.err, got, tc.reason)
		}
	}
}

func TestPatchedCoreCaptureDetailWithCurl(t *testing.T) {
	core := os.Getenv("NETPROXY_TEST_CAPTURE_CORE")
	if core == "" {
		t.Skip("set NETPROXY_TEST_CAPTURE_CORE to the patched Linux sing-box")
	}
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl unavailable")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Capture-Response", "synthetic")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.Copy(w, r.Body)
	})
	plain := httptest.NewServer(handler)
	defer plain.Close()
	encrypted := httptest.NewTLSServer(handler)
	defer encrypted.Close()
	layout := paths.New(t.TempDir())
	controller := New(layout)
	controller.upstreamRoots = encrypted.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	if _, err := controller.SetMITMPolicy(MITMPolicy{Enabled: true, Hosts: []string{"127.0.0.1"}}); err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	defer controller.Stop()
	cert, err := controller.Certificate(false)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(layout.Root(), "ca-test.pem")
	if err := os.WriteFile(caPath, []byte(cert.PEM), 0o600); err != nil {
		t.Fatal(err)
	}
	proxyURL := "socks5h://" + startCaptureTestSOCKS(t, core, layout, false)
	for _, origin := range []string{plain.URL, encrypted.URL} {
		url := origin + "/sample/form?value=fixture"
		out, err := exec.CommandContext(t.Context(), curl, "--silent", "--show-error", "--fail", "--max-time", "5",
			"--noproxy", "", "--proxy", proxyURL, "--cacert", caPath,
			"-H", "X-Capture-Request: synthetic", "-H", "Content-Type: text/plain", "--data-binary", "echo-body", url).CombinedOutput()
		if err != nil || string(out) != "echo-body" {
			t.Fatalf("curl capture fixture failed: %q %v", out, err)
		}
		entries := controller.Entries()
		if len(entries) == 0 {
			t.Fatal("no capture entry")
		}
		record, found := controller.Record(entries[0].ID)
		if !found || record.HTTP == nil || record.HTTP.URL != url ||
			record.HTTP.Request.Headers.Get("X-Capture-Request") != "synthetic" ||
			record.HTTP.Response.Headers.Get("X-Capture-Response") != "synthetic" ||
			record.HTTP.Request.Body.Data != "echo-body" || record.HTTP.Response.Body.Data != "echo-body" ||
			(strings.HasPrefix(origin, "https:") && record.TLS != "decrypted") {
			t.Fatal("missing URL, headers or bodies in curl capture detail")
		}
	}
}

func testTransparentHTTPS(t *testing.T, core string) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("抓包代理凭据到达 HTTPS 上游")
		}
		if body, err := io.ReadAll(r.Body); err != nil || string(body) != "request-body" {
			t.Errorf("unexpected upstream request body: %q, %v", body, err)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Capture-Response", "https")
		_, _ = io.WriteString(w, "upstream")
	}))
	defer origin.Close()
	layout := paths.New(t.TempDir())
	c := New(layout)
	originRoots := origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	c.upstreamRoots = originRoots
	if err := c.ReplaceRules([]Rule{{"^https://127[.]0[.]0[.]1:", "response", `return {body: input.body.toUpperCase()};`, []string{"127.0.0.1"}, nil, ""}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetMITMPolicy(MITMPolicy{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	ownerUID, otherUID := uint32(10123), uint32(10124)
	const ownerPackage = "com.example.pinned"
	dial := bridgeFixtureFor(layout, &Exit{UID: &ownerUID, Package: ownerPackage})
	if core != "" {
		dial = startCaptureTestCore(t, core, layout)
	}
	public, err := c.Certificate(false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(public.PEM)) {
		t.Fatal("无效公开 CA")
	}
	requestWithDial := func(trust *x509.CertPool, dial func(context.Context, string, string) (net.Conn, error)) (string, error) {
		t.Helper()
		transport := &http.Transport{DialContext: dial, TLSClientConfig: &tls.Config{RootCAs: trust}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, origin.URL+"/debug?token=synthetic", strings.NewReader("request-body"))
		if err != nil {
			return "", err
		}
		if trust == roots {
			request.Header.Set("Proxy-Authorization", "synthetic")
		}
		request.Header.Set("X-Capture-Request", "https")
		request.Header.Set("Content-Type", "text/plain")
		res, err := client.Do(request)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		return string(data), err
	}
	request := func(trust *x509.CertPool) (string, error) { return requestWithDial(trust, dial) }
	if body, err := request(roots); err != nil || body != "UPSTREAM" {
		t.Fatalf("未重写 HTTPS 正文: %q, %v", body, err)
	}
	// 只信任上游证书的客户端拒绝抓包证书。
	if _, err := request(originRoots); err == nil {
		t.Fatal("客户端意外接受抓包证书")
	}
	state := c.currentMITM()
	deadline := time.Now().Add(time.Second)
	for len(c.Entries()) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	failureTLS := "handshake_failed"
	if core == "" {
		failureTLS = "pinned_skipped"
		pinned := New(layout)
		if !pinned.isPinned("127.0.0.1", &SourceApp{UID: ownerUID, Package: ownerPackage}) ||
			pinned.isPinned("127.0.0.1", &SourceApp{UID: otherUID, Package: "com.example.other"}) ||
			pinned.isPinned("127.0.0.1", &SourceApp{UID: 110123, UserID: 1, Package: ownerPackage}) {
			t.Fatal("持久化证书跳过没有按应用和用户隔离")
		}
		if body, err := request(originRoots); err != nil || body != "upstream" {
			t.Fatalf("下次 TCP 连接未透传: %q, %v", body, err)
		}
		if body, err := requestWithDial(roots, bridgeFixtureFor(layout, &Exit{UID: &otherUID, Package: "com.example.other"})); err != nil || body != "UPSTREAM" {
			t.Fatalf("其他应用不应被跳过: %q, %v", body, err)
		}
		if _, err := c.ClearPinnedBypasses(); err != nil {
			t.Fatal(err)
		}
		if entries, err := New(layout).PinnedBypasses(); err != nil || len(entries) != 0 {
			t.Fatalf("未持久清空证书跳过记录: %+v, %v", entries, err)
		}
	}
	entries := c.Entries()
	if core == "" {
		if len(entries) != 4 || entries[0].TLS != "decrypted" || entries[1].TLS != "pinned_skipped" ||
			entries[1].PassthroughReason != "certificate_bypassed" {
			t.Fatalf("意外抓包记录: %+v", entries)
		}
		entries = entries[2:]
	}
	if len(entries) != 2 || entries[0].TLS != failureTLS || entries[0].Status != 0 || entries[0].PassthroughReason != "certificate_rejected" ||
		entries[1].TLS != "decrypted" || entries[1].Rewrite != "response" || strings.Contains(entries[1].Host, "synthetic") {
		t.Fatalf("意外抓包记录: %+v", entries)
	}
	record, ok := c.Record(entries[1].ID)
	if !ok || record.HTTP == nil || record.HTTP.URL != origin.URL+"/debug?token=synthetic" ||
		record.HTTP.Request.Headers.Get("X-Capture-Request") != "https" || record.HTTP.Request.Body.Data != "request-body" ||
		record.HTTP.Response.Headers.Get("X-Capture-Response") != "https" || record.HTTP.Response.Body.Data != "UPSTREAM" {
		t.Fatalf("HTTPS capture detail mismatch: %+v", record.HTTP)
	}
	for _, entry := range entries[:1] {
		if record, ok := c.Record(entry.ID); !ok || record.HTTP != nil {
			t.Fatal("TLS fallback exposed HTTP detail")
		}
	}
	if _, err := c.SetMITMPolicy(MITMPolicy{}); err != nil || c.currentMITM() != nil || state.ctx.Err() == nil {
		t.Fatalf("未撤销当前 HTTPS 抓包策略: %v", err)
	}
}

func TestCAIdentityIsPrivateAndNeverOverwritten(t *testing.T) {
	layout := paths.New(t.TempDir())
	c := New(layout)
	if _, err := c.Certificate(false); err == nil {
		t.Fatal("读取操作隐式创建了 CA")
	}
	first, err := c.Certificate(true)
	if err != nil || strings.Contains(first.PEM, "PRIVATE KEY") || !androidCAFilename.MatchString(first.Filename) {
		t.Fatalf("公开 CA 无效: %v", err)
	}
	again, err := New(layout).Certificate(true)
	if err != nil || again != first {
		t.Fatalf("CA 初始化覆盖了已有身份: %v", err)
	}
	info, err := os.Stat(layout.TrafficCA())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("CA 私钥文件权限不正确: %v", err)
	}
	if openssl, err := exec.LookPath("openssl"); err == nil {
		cmd := exec.CommandContext(t.Context(), openssl, "x509", "-noout", "-subject_hash_old")
		cmd.Stdin = strings.NewReader(first.PEM)
		got, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(got))+".0" != first.Filename {
			t.Fatalf("Android subject_hash_old 不匹配: %q %s %v", got, first.Filename, err)
		}
	}
	if err := os.WriteFile(layout.TrafficCA(), []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Certificate(true); err == nil {
		t.Fatal("损坏的 CA 被覆盖")
	}
	if got, _ := os.ReadFile(layout.TrafficCA()); string(got) != "damaged" {
		t.Fatal("损坏的 CA 原件未保留")
	}
}

func TestMITMHostScope(t *testing.T) {
	policy, err := normalizePolicy(MITMPolicy{Enabled: true, Hosts: []string{"Example.ORG.", "*.internal.example", "127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	scope := newMITM(policy, nil)
	defer scope.cancel()
	for _, host := range []string{"example.org", "dev.internal.example", "x.y.internal.example", "127.0.0.1"} {
		if !scope.matches(host) {
			t.Errorf("未匹配 %s", host)
		}
	}
	for _, host := range []string{"internal.example", "badinternal.example", "example.org.evil", "10.0.0.1"} {
		if scope.matches(host) {
			t.Errorf("错误匹配 %s", host)
		}
	}
}

func TestMITMDefaultsToAllHostsExceptBypass(t *testing.T) {
	policy, err := normalizePolicy(MITMPolicy{Enabled: true, Hosts: []string{}, BypassHosts: []string{"*.internal.example", "example.org"}})
	if err != nil {
		t.Fatal(err)
	}
	scope := newMITM(policy, nil)
	defer scope.cancel()
	if !scope.matches("other.example") || !scope.matches("internal.example") ||
		scope.matches("sub.internal.example") || scope.matches("example.org") {
		t.Fatal("默认范围或排除名单未生效")
	}
}

func TestSystemTrustCandidatePreservesExistingRoots(t *testing.T) {
	base, staged := t.TempDir(), t.TempDir()
	root := []byte("existing trust root")
	if err := os.WriteFile(filepath.Join(base, "abcdef01.0"), root, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := populateStore(base, staged, Certificate{Filename: "abcdef02.0", PEM: "public test cert"}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(staged, "abcdef01.0")); err != nil || !bytes.Equal(got, root) {
		t.Fatalf("原有 CA 未保留: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(staged, "abcdef02.0")); err != nil || string(got) != "public test cert" {
		t.Fatalf("公开 CA 未写入: %q %v", got, err)
	}
	if err := populateStore(base, t.TempDir(), Certificate{Filename: "abcdef01.0", PEM: "conflict"}); err == nil {
		t.Fatal("允许覆盖现有系统证书")
	}
}
