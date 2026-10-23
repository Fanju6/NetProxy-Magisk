package module

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestExportLogsRedactsTLSPrivateKeysInCompleteArchive(t *testing.T) {
	options := newTestOptions(t.TempDir())
	secrets := []string{}
	secret := func() string {
		marker := fmt.Sprintf("F3_FAKE_PRIVATE_%03d", len(secrets))
		secrets = append(secrets, marker)
		return marker
	}
	serverTLS := map[string]any{
		"enabled":      true,
		"key":          []string{"-----BEGIN PRIVATE KEY-----", secret(), "-----END PRIVATE KEY-----"},
		"ech":          map[string]any{"key": []string{secret()}},
		"control_wrap": map[string]any{"key": []string{secret()}},
	}
	clientTLS := map[string]any{
		"enabled": true, "client_key": []string{"-----BEGIN RSA PRIVATE KEY-----", secret(), "-----END RSA PRIVATE KEY-----"},
	}
	configuration := map[string]any{
		"key":         "ordinary-business-key",
		"inbounds":    []any{map[string]any{"type": "trojan", "tag": "tls-in", "tls": serverTLS}},
		"outbounds":   []any{map[string]any{"type": "trojan", "tag": "tls-out", "tls": clientTLS}},
		"nested":      map[string]any{"tls": map[string]any{"key": secret(), "client_key": secret()}, "key": "ordinary-nested-key"},
		"private_key": secret(), "token": secret(),
		"unknown_pem_lines": []string{"-----BEGIN EC PRIVATE KEY-----", secret(), "-----END EC PRIVATE KEY-----"},
		"providers": []any{map[string]any{
			"type": "local", "tag": "user", "path": "user.json", "key": "ordinary-provider-definition-key",
			"override_tls": map[string]any{
				"client_key": []string{"-----BEGIN PRIVATE KEY-----", secret(), "-----END PRIVATE KEY-----"},
			},
		}},
	}
	configContent, err := json.Marshal(configuration, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	providerContent, err := json.Marshal(map[string]any{
		"key": "ordinary-provider-key",
		"outbounds": []any{map[string]any{
			"type": "trojan", "tag": "node", "tls": map[string]any{"client_key": []string{secret()}},
		}},
	}, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	logContent := "diagnostic-start\n"
	for _, label := range []string{"PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY", "ENCRYPTED PRIVATE KEY", "OPENSSH PRIVATE KEY"} {
		logContent += "-----BEGIN " + label + "-----\n" + secret() + "\n-----END " + label + "-----\n"
	}
	logContent += "diagnostic-end\n-----BEGIN PRIVATE KEY-----\n" + secret()
	malformedContent := []byte(`{"tls":{"key":["` + secret())
	fixtures := map[string][]byte{
		paths.SingBoxConfig(options.SingBoxDir):                        configContent,
		filepath.Join(options.CatalogRoot, "default", "provider.json"): providerContent,
		filepath.Join(options.RuntimeDir, "inbound.json"):              configContent,
		options.InboundConfig:                                          configContent,
		options.ModuleConfig:                                           malformedContent,
		filepath.Join(options.LogDir, "service.log"):                   []byte(logContent),
		filepath.Join(options.LogDir, "sing-box.log.1"):                []byte(logContent),
	}
	for path, content := range fixtures {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	destination := filepath.Join(options.ModuleDir, "export", "diagnostics.tar.gz")
	if err := ExportLogs(options, destination); err != nil {
		t.Fatal(err)
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
	uncompressed, err := io.ReadAll(compressed)
	if err != nil {
		t.Fatal("诊断 gzip 内容或尾部校验失败")
	}
	for index, marker := range secrets {
		if bytes.Contains(uncompressed, []byte(marker)) {
			t.Fatalf("完整归档包含秘密 marker #%d", index)
		}
	}
	entries := map[string][]byte{}
	reader := tar.NewReader(bytes.NewReader(uncompressed))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal("诊断 tar 结构不完整")
		}
		if header.Mode != 0o600 {
			t.Fatal("归档成员权限不符合私有诊断契约")
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal("归档成员未完整读取")
		}
		entries[header.Name] = content
	}
	for name, keep := range map[string]string{
		"config/singbox/config.json":         "ordinary-business-key",
		"config/inbound.json":                "ordinary-nested-key",
		"runtime/inbound.json":               "ordinary-nested-key",
		"data/catalog/default/provider.json": "ordinary-provider-key",
		"logs/service.log":                   "diagnostic-end",
		"logs/sing-box.log.1":                "diagnostic-end",
		"config/module.json":                 "JSON 配置损坏",
		"README.txt":                         "敏感信息已脱敏",
	} {
		if !bytes.Contains(entries[name], []byte(keep)) {
			t.Fatalf("归档成员缺失或普通诊断字段被清除: %s", name)
		}
	}
	for path, before := range fixtures {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("诊断导出修改了源文件")
		}
	}
	if strings.Contains(string(entries["config/module.json"]), "tls") {
		t.Fatal("损坏 JSON 原文未完整省略")
	}
}
