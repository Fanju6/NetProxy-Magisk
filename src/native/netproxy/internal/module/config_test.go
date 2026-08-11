package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListConfigsUsesReadableRuntimeID(t *testing.T) {
	options := NewOptions(t.TempDir())
	if err := os.MkdirAll(options.RuntimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	localRules := filepath.Join(options.SingBoxDir, "rules", "local")
	remoteRules := filepath.Join(options.SingBoxDir, "rules", "remote")
	if err := os.MkdirAll(localRules, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(remoteRules, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localRules, "direct.json"), []byte(`{"version":1,"rules":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteRules, "Proxy.srs"), []byte{0x00, 0xff, 0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	const content = "{\"type\":\"ebpf\"}\n"
	if err := os.WriteFile(filepath.Join(options.RuntimeDir, "ebpf.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(options.RuntimeDir, "service.json"), []byte(`{"state":"ready"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(options.RuntimeDir, "internal.json"), []byte("internal"), 0o600); err != nil {
		t.Fatal(err)
	}

	documents, err := ListConfigs(options)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range documents {
		if document.Filename == "service.json" || document.Filename == "internal.json" {
			t.Fatalf("内部状态文件不应出现在配置列表: %q", document.ID)
		}
	}
	if _, err := ReadConfig(options, "runtime/service.json"); err == nil {
		t.Fatal("内部服务状态不应作为运行时配置读取")
	}
	var runtimeDocument *ConfigDocument
	for index := range documents {
		if documents[index].Filename == "ebpf.json" {
			runtimeDocument = &documents[index]
			break
		}
	}
	if runtimeDocument == nil {
		t.Fatal("运行时配置未出现在配置列表")
	}
	if runtimeDocument.ID != "runtime/ebpf.json" {
		t.Fatalf("运行时配置 ID 错误: %q", runtimeDocument.ID)
	}

	var localRuleDocument *ConfigDocument
	for index := range documents {
		if documents[index].Filename == "direct.json" {
			localRuleDocument = &documents[index]
			break
		}
	}
	if localRuleDocument == nil || localRuleDocument.ID != "singbox/rules/local/direct.json" || localRuleDocument.Category != "rules" {
		t.Fatalf("本地规则集文档契约错误: %#v", localRuleDocument)
	}
	if _, err := ReadConfig(options, "singbox/rules/remote/Proxy.srs"); err == nil {
		t.Fatal("远程 SRS 不应作为可编辑配置读取")
	}

	read, err := ReadConfig(options, runtimeDocument.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read["content"] != content {
		t.Fatalf("读取到的运行时配置不一致: %q", read["content"])
	}
}
