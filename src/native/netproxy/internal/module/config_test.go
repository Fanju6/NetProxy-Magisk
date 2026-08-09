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
	const content = "{\"type\":\"ebpf\"}\n"
	if err := os.WriteFile(filepath.Join(options.RuntimeDir, "ebpf.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	documents, err := ListConfigs(options)
	if err != nil {
		t.Fatal(err)
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

	read, err := ReadConfig(options, runtimeDocument.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read["content"] != content {
		t.Fatalf("读取到的运行时配置不一致: %q", read["content"])
	}
}
