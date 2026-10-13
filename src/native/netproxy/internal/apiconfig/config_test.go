package apiconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
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
	if err := (Config{Listen: "[::1]:9091", Secret: strings.Repeat("a", 64)}).Validate(); err != nil {
		t.Fatal(err)
	}
}
