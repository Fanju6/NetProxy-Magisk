package module

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExportLogsIncludesRuntimeConfigAndRedactsSecrets(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.prop"), []byte("version=v8.0.0-test\nversionCode=5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(root, "logs")
	moduleConfig := filepath.Join(root, "config", "module.conf")
	ebpfConfig := filepath.Join(root, "config", "ebpf", "ebpf.conf")
	singboxDir := filepath.Join(root, "config", "singbox")
	runtimeDir := filepath.Join(root, "runtime")
	catalogRoot := filepath.Join(root, "data", "catalog", "group")
	for _, dir := range []string{logDir, filepath.Dir(moduleConfig), filepath.Dir(ebpfConfig), filepath.Join(singboxDir, "confdir"), runtimeDir, catalogRoot} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(logDir, "service.log"), []byte("Authorization: Bearer secret-bearer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(moduleConfig, []byte("SUB_URL=https://example.test/sub?token=secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ebpfConfig, []byte("HWID=secret-hwid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "ebpf.json"), []byte(`{"type":"ebpf","tag":"runtime"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(root, "dev", "netproxy", "service.json")
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, []byte(`{"state":"ready"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(singboxDir, "confdir", "08_services.json"), []byte(`{"secret":"secret-config"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogRoot, "meta.json"), []byte(`{"url":"https://example.test/sub?token=secret-token","hwid":"secret-hwid","custom_headers":{"Authorization":"Bearer secret-bearer"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(root, "export", "diagnostics.tar.gz")
	options := Options{
		ModuleDir:          root,
		ManagerVersion:     "8.0.0-manager-test",
		ManagerVersionCode: "29",
		CatalogRoot:        catalogRoot,
		ModuleConfig:       moduleConfig,
		EBPFConfig:         ebpfConfig,
		SingBoxDir:         singboxDir,
		RuntimeDir:         runtimeDir,
		StateFile:          stateFile,
		LogDir:             logDir,
	}
	if err := ExportLogs(options, destination); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("诊断包权限应为 0600，实际为 %o", info.Mode().Perm())
	}

	file, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	seenRuntime := false
	seenState := false
	seenReadme := false
	for {
		header, readErr := reader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(header.Name, "runtime/ebpf.json") {
			seenRuntime = true
		}
		if header.Name == "state/service.json" {
			seenState = true
		}
		if header.Name == "runtime/service.json" {
			t.Fatal("服务状态不应归档为运行时配置")
		}
		if header.Name == "README.txt" {
			seenReadme = true
			readme := string(content)
			for _, version := range []string{
				"管理器版本: 8.0.0-manager-test",
				"管理器版本号: 29",
				"模块版本: v8.0.0-test",
				"模块版本号: 5",
			} {
				if !strings.Contains(readme, version) {
					t.Fatalf("README.txt 缺少版本信息 %q: %s", version, readme)
				}
			}
		}
		for _, secret := range []string{"secret-bearer", "secret-token", "secret-hwid", "secret-config"} {
			if strings.Contains(string(content), secret) {
				t.Fatalf("诊断包泄露敏感值 %q，文件 %s，内容 %s", secret, header.Name, content)
			}
		}
	}
	if !seenRuntime {
		t.Fatal("诊断包未包含运行时配置")
	}
	if !seenState {
		t.Fatal("诊断包未包含服务状态")
	}
	if !seenReadme {
		t.Fatal("诊断包未包含 README.txt")
	}
}

func TestRedactTextRedactsTopLevelJSONArray(t *testing.T) {
	content := redactText(`[{
  "url": "https://example.test/sub?token=secret-token",
  "nested": {"password": "secret-password"}
}, {"authorization": "Bearer secret-bearer"}]`)
	for _, secret := range []string{"secret-token", "secret-password", "secret-bearer"} {
		if strings.Contains(content, secret) {
			t.Fatalf("top-level JSON array leaked %q: %s", secret, content)
		}
	}
	if !strings.Contains(content, "***") {
		t.Fatalf("redacted JSON array did not contain a replacement: %s", content)
	}
}

func TestLogFileRejectsRemovedSubscriptionLog(t *testing.T) {
	_, err := LogFile(Options{LogDir: t.TempDir()}, "sub")
	if err == nil {
		t.Fatal("已删除的独立订阅日志类型不应继续可用")
	}
}
