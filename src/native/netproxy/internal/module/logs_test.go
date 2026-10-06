package module

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

type limitedArchiveWriter struct {
	remaining int
	failure   error
}

func (writer *limitedArchiveWriter) Write(content []byte) (int, error) {
	if len(content) > writer.remaining {
		return 0, writer.failure
	}
	writer.remaining -= len(content)
	return len(content), nil
}

func TestExportLogsReportsArchiveFinalizationFailure(t *testing.T) {
	options := NewOptions(t.TempDir())
	failure := errors.New("模拟压缩尾部写入失败")
	if err := writeLogArchive(options, &limitedArchiveWriter{remaining: 10, failure: failure}); !errors.Is(err, failure) {
		t.Fatalf("压缩结束错误丢失: %v", err)
	}
}

func TestFailedLogExportPreservesDestination(t *testing.T) {
	options := NewOptions(t.TempDir())
	if err := os.MkdirAll(options.ModuleConfig, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(options.ModuleDir, "diagnostic.tar.gz")
	if err := os.WriteFile(destination, []byte("previous export"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ExportLogs(options, destination); err == nil {
		t.Fatal("错误日志源未导致导出失败")
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != "previous export" {
		t.Fatalf("已有导出被破坏: %q %v", content, err)
	}
	matches, err := filepath.Glob(filepath.Join(options.ModuleDir, ".diagnostics-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("导出临时文件未清理: %v %v", matches, err)
	}
}

func TestExportLogsPreservesExistingDestinationPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不提供 POSIX 文件权限语义")
	}
	options := NewOptions(t.TempDir())
	destination := filepath.Join(options.ModuleDir, "diagnostic.tar.gz")
	if err := os.WriteFile(destination, []byte("previous export"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(destination, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExportLogs(options, destination); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("已有目标文件权限未保留: %o", info.Mode().Perm())
	}
}

func TestExportLogsRejectsSymbolicDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 创建符号链接需要额外权限")
	}
	options := NewOptions(t.TempDir())
	target := filepath.Join(options.ModuleDir, "target.tar.gz")
	if err := os.WriteFile(target, []byte("previous export"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(options.ModuleDir, "diagnostic.tar.gz")
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}
	if err := ExportLogs(options, destination); err == nil || !strings.Contains(err.Error(), "不是普通文件") {
		t.Fatalf("符号链接目标未被拒绝: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "previous export" {
		t.Fatalf("符号链接指向的文件被破坏: %q %v", content, err)
	}
}

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
	for _, dir := range []string{logDir, filepath.Dir(moduleConfig), filepath.Dir(ebpfConfig), singboxDir, runtimeDir, catalogRoot} {
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
	if err := os.WriteFile(ebpfConfig, []byte("PROXY_APPS_LIST=\"0:secret.app.one,10:secret.app.two\"\nBYPASS_APPS_LIST=\"0:secret.app.three\"\n"), 0o600); err != nil {
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
	if err := os.WriteFile(filepath.Join(singboxDir, "config.json"), []byte(`{"secret":"secret-config"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogRoot, "meta.json"), []byte(`{"url":"https://example.test/sub?token=secret-token","hwid":"secret-hwid","custom_headers":{"Authorization":"Bearer secret-bearer"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogRoot, "provider.json"), []byte(`{"outbounds":[{"type":"hysteria","tag":"hy","auth_str":"secret-hysteria-auth"}],"endpoints":[{"type":"wireguard","tag":"wg","private_key":"secret-private-key","pre_shared_key":"secret-wireguard-psk"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	telemetryPath := paths.New(root).TelemetryState()
	if err := os.MkdirAll(filepath.Dir(telemetryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(telemetryPath, []byte(`{"pending":[{"uuid":"private-telemetry-event"}]}`), 0o600); err != nil {
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
			"secret.app.two", "secret.app.three", "private-telemetry-event",
		} {
			if strings.Contains(string(content), secret) {
				t.Fatalf("诊断包泄露敏感值 %q，文件 %s，内容 %s", secret, header.Name, content)
			}
		}
		if strings.Contains(header.Name, "telemetry") {
			t.Fatal("诊断包不应包含设备统计状态")
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
