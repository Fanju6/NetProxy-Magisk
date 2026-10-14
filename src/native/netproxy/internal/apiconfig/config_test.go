package apiconfig

import (
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInitializeIsAtomicAndKeepsIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "api.json")
	var tasks sync.WaitGroup
	results := make(chan Config, 16)
	for range 16 {
		tasks.Go(func() {
			config, err := Initialize(t.Context(), path)
			if err != nil {
				t.Error(err)
				return
			}
			results <- config
		})
	}
	tasks.Wait()
	close(results)
	var previous Config
	for config := range results {
		if previous != (Config{}) && previous != config {
			t.Fatal("并发初始化轮换了凭据")
		}
		if err := config.Validate(); err != nil {
			t.Fatal(err)
		}
		previous = config
	}
	if previous == (Config{}) {
		t.Fatal("没有生成凭据")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("凭据权限不是0600", info.Mode())
	}
	next, err := Initialize(t.Context(), filepath.Join(t.TempDir(), "api.json"))
	if err != nil || next.Secret == previous.Secret {
		t.Fatal("不同安装复用了随机凭据", err)
	}
}

func TestInvalidConfigIsNeverReplaced(t *testing.T) {
	for _, content := range []string{`{"listen":"0.0.0.0:9091","secret":"` + strings.Repeat("a", 64) + `"}`,
		`{"listen":"127.0.0.1:9091","secret":null}`, `{"listen":"127.0.0.1:9091","secret":"leak-marker","extra":true}`,
		`{"listen":"127.0.0.1:9091","listen":"127.0.0.1:9092","secret":"` + strings.Repeat("a", 64) + `"}`, "null", "[]"} {
		path := filepath.Join(t.TempDir(), "api.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Initialize(t.Context(), path)
		if err == nil || strings.Contains(err.Error(), "leak-marker") {
			t.Fatal("无效配置被接受或泄漏", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != content {
			t.Fatal("损坏凭据被覆盖", err)
		}
	}
}

func TestListenOnlyAllowsLoopback(t *testing.T) {
	for _, listen := range []string{"localhost:9091", "0.0.0.0:9091", "[::]:9091", "192.0.2.1:9091", "127.0.0.1:0"} {
		if err := (Config{Listen: listen, Secret: strings.Repeat("a", 64)}).Validate(); err == nil {
			t.Fatal(listen)
		}
	}
	config, err := Initialize(t.Context(), filepath.Join(t.TempDir(), "api.json"))
	if err != nil {
		t.Fatal(err)
	}
	config.Listen = "[::1]:9091"
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTLSRejectsOtherInstallationAndLegacyIdentity(t *testing.T) {
	first, err := Initialize(t.Context(), filepath.Join(t.TempDir(), "api.json"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Initialize(t.Context(), filepath.Join(t.TempDir(), "api.json"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Connection().CertificateSHA256 == second.Connection().CertificateSHA256 {
		t.Fatal("不同安装复用了 TLS 身份")
	}
	broken := first
	broken.PrivateKey = second.PrivateKey
	if broken.Validate() == nil {
		t.Fatal("不匹配的证书和私钥被接受")
	}
	first.Certificate, first.PrivateKey = "", ""
	if first.Validate() == nil {
		t.Fatal("旧明文配置被接受")
	}
}

func TestPinnedTLSIgnoresCertificateDatesButRejectsWrongIdentityAndHostname(t *testing.T) {
	for _, years := range []int{-20, 20} {
		t.Run(time.Now().AddDate(years, 0, 0).Format("2006"), func(t *testing.T) {
			config, err := Initialize(t.Context(), filepath.Join(t.TempDir(), "api.json"))
			if err != nil {
				t.Fatal(err)
			}
			certificateBlock, _ := pem.Decode([]byte(config.Certificate))
			certificate, err := x509.ParseCertificate(certificateBlock.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			keyBlock, _ := pem.Decode([]byte(config.PrivateKey))
			key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			certificate.NotBefore = time.Now().AddDate(years, 0, 0)
			certificate.NotAfter = certificate.NotBefore.AddDate(1, 0, 0)
			der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, key.(crypto.Signer).Public(), key)
			if err != nil {
				t.Fatal(err)
			}
			config.Certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
			server.TLS, err = config.ServerTLS()
			if err != nil {
				t.Fatal(err)
			}
			server.StartTLS()
			defer server.Close()
			transport := &http.Transport{TLSClientConfig: config.ClientTLS()}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: time.Second}
			response, err := client.Get(server.URL)
			if err != nil {
				t.Fatal("正确安装身份不应因设备时钟或证书日期而失效", err)
			}
			response.Body.Close()
			if response.StatusCode != 204 {
				t.Fatal(response.StatusCode)
			}
			other, err := Initialize(t.Context(), filepath.Join(t.TempDir(), "api.json"))
			if err != nil {
				t.Fatal(err)
			}
			wrongHost := config.ClientTLS()
			wrongHost.ServerName = "wrong.example"
			for _, tlsConfig := range []*tls.Config{other.ClientTLS(), wrongHost} {
				transport := &http.Transport{TLSClientConfig: tlsConfig}
				defer transport.CloseIdleConnections()
				if response, err := (&http.Client{Transport: transport, Timeout: time.Second}).Get(server.URL); err == nil {
					response.Body.Close()
					t.Fatal("错误安装身份或主机名被接受")
				}
			}
		})
	}
}
