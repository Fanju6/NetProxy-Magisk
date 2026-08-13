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

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
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
	if err := os.WriteFile(filepath.Join(logDir, "service.log"), []byte("Authorization: Bearer secret-bearer\npayload={\"uuid\":\"secret-log-uuid\",\"auth_str\":\"secret-log-auth\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(moduleConfig, []byte("SUB_URL=https://example.test/sub?token=secret-token\nWIFI_SSID_LIST=\"secret-office,secret-home\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ebpfConfig, []byte("HWID=secret-hwid\nPROXY_APPS_LIST=\"secret.app.one,secret.app.two\"\nBYPASS_APPS_LIST=\"secret.app.three\"\n"), 0o600); err != nil {
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
	if err := os.WriteFile(filepath.Join(catalogRoot, "provider.json"), []byte(`{"outbounds":[{"type":"hysteria","tag":"hy","auth_str":"secret-hysteria-auth"}],"endpoints":[{"type":"wireguard","tag":"wg","private_key":"secret-private-key","pre_shared_key":"secret-wireguard-psk"}]}`), 0o600); err != nil {
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
		for _, secret := range []string{
			"secret-bearer", "secret-token", "secret-hwid", "secret-config",
			"secret-log-uuid", "secret-log-auth", "secret-hysteria-auth", "secret-private-key",
			"secret-wireguard-psk", "secret-office", "secret-home", "secret.app.one",
			"secret.app.two", "secret.app.three",
		} {
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
	content := logfile.RedactText(`[{
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

func TestReadLogRedactsURLsForAllLogKinds(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"service", "core"} {
		path, err := LogFile(Options{LogDir: logDir}, kind)
		if err != nil {
			t.Fatal(err)
		}
		content := "retry HTTP://example.test/api/subscription?id=123\n" +
			"update https://user:password@example.test/sub?token=secret-token\n" +
			`payload={"uuid":"secret-uuid","auth_str":"secret-auth","pre_shared_key":"secret-psk"}` + "\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := ReadLog(Options{LogDir: logDir}, kind, 20)
		if err != nil {
			t.Fatal(err)
		}
		shown := snapshot.Content
		for _, secret := range []string{"example.test", "password", "secret-token", "secret-uuid", "secret-auth", "secret-psk"} {
			if strings.Contains(shown, secret) {
				t.Fatalf("%s 日志泄露 %q: %s", kind, secret, shown)
			}
		}
		if strings.Count(shown, "[订阅链接已隐藏]") != 2 {
			t.Fatalf("%s 日志 URL 脱敏结果异常: %s", kind, shown)
		}
	}
}

func TestReadLogReturnsStructuredNativeEntriesOnly(t *testing.T) {
	logDir := t.TempDir()
	content := strings.Join([]string{
		"[2026-08-13 12:00:00] [INFO] [service] [service.start] [success] [-] 服务启动完成",
		"not-a-native-event",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(logDir, "service.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadLog(Options{LogDir: logDir}, "service", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("结构化日志未忽略非契约行: %+v", snapshot.Entries)
	}
	entry := snapshot.Entries[0]
	if entry.Component != "service" || entry.Event != "service.start" || entry.Result != "success" || entry.ErrorCode != "" {
		t.Fatalf("结构化日志字段异常: %+v", entry)
	}
}

func TestReadLogKeepsCoreOutsideNativeEntryContract(t *testing.T) {
	logDir := t.TempDir()
	content := "[2026-08-13 12:00:00] [INFO] [service] [service.start] [success] [-] 服务启动完成\n"
	if err := os.WriteFile(filepath.Join(logDir, "sing-box.log"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadLog(Options{LogDir: logDir}, "core", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 0 {
		t.Fatalf("内核日志不应进入 Native 结构化契约: %+v", snapshot.Entries)
	}
	if snapshot.Content != content {
		t.Fatalf("内核日志文本异常: %q", snapshot.Content)
	}
}
