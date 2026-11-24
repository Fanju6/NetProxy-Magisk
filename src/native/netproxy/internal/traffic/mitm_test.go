//go:build linux

package traffic

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func testTransparentHTTPS(t *testing.T, core string) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("抓包代理凭据到达 HTTPS 上游")
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "upstream")
	}))
	defer origin.Close()
	layout := paths.New(t.TempDir())
	c := New(layout)
	originRoots := origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	c.upstreamRoots = originRoots
	if err := c.ReplaceRules([]Rule{{"^https://127[.]0[.]0[.]1:", "response", `return {body: input.body.toUpperCase()};`}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetMITMPolicy(MITMPolicy{Enabled: true, Hosts: []string{"127.0.0.1"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	dial := bridgeFixture(layout)
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
	request := func(trust *x509.CertPool) (string, error) {
		t.Helper()
		transport := &http.Transport{DialContext: dial, TLSClientConfig: &tls.Config{RootCAs: trust}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, origin.URL+"/debug?token=synthetic", nil)
		if err != nil {
			return "", err
		}
		if trust == roots {
			request.Header.Set("Proxy-Authorization", "synthetic")
		}
		res, err := client.Do(request)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		return string(data), err
	}
	if body, err := request(roots); err != nil || body != "UPSTREAM" {
		t.Fatalf("未重写 HTTPS 正文: %q, %v", body, err)
	}
	// 只信任上游证书的客户端拒绝抓包证书。
	if _, err := request(originRoots); err == nil {
		t.Fatal("客户端意外接受抓包证书")
	}
	state := c.currentMITM()
	deadline := time.Now().Add(time.Second)
	for !state.bypass("127.0.0.1", time.Now()) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if body, err := request(originRoots); err != nil || body != "upstream" {
		t.Fatalf("下次 TCP 连接未透传: %q, %v", body, err)
	}
	entries := c.Entries()
	if len(entries) != 3 || entries[0].TLS != "bypass" || entries[1].TLS != "handshake_failed" ||
		entries[2].TLS != "decrypted" || entries[2].Rewrite != "response" || strings.Contains(entries[2].Host, "synthetic") {
		t.Fatalf("意外抓包记录: %+v", entries)
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
