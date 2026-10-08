package module

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLocalRuleSetUsesCoreMatchers(t *testing.T) {
	options := Options{SingBoxPath: targetCoreBinary(t)}
	for _, test := range []struct {
		name, content, message string
	}{
		{"empty", `{"version":1,"rules":[]}`, ""},
		{"common", `{"version":1,"rules":[{"domain_suffix":["example.com"]},{"ip_cidr":["192.0.2.1","2001:db8::/32"]},{"port":[443]},{"port_range":["8000:9000"]}]}`, ""},
		{"logical", `{"version":5,"rules":[{"type":"logical","mode":"and","rules":[{"domain_regex":["^example\\.com$"]},{"network":"tcp"}]}]}`, ""},
		{"invalid-regex", `{"version":1,"rules":[{"domain_regex":["["]}]}`, "domain_regex"},
		{"invalid-cidr", `{"version":1,"rules":[{"ip_cidr":["192.0.2.0/99"]}]}`, "规则集"},
		{"invalid-port", `{"version":1,"rules":[{"port":[65536]}]}`, "规则集"},
		{"invalid-range", `{"version":1,"rules":[{"port_range":["not-a-range"]}]}`, "port_range"},
		{"unconditional", `{"version":1,"rules":[{}]}`, ""},
		{"invalid-mode", `{"version":1,"rules":[{"type":"logical","mode":"invalid","rules":[{"domain":"example.com"}]}]}`, "logical mode"},
		{"unknown-field", `{"version":1,"rules":[{"outbound":"Proxy"}]}`, "规则集"},
		{"duplicate-key", `{"version":1,"version":2,"rules":[]}`, "JSON"},
		{"invalid-version", `{"version":99,"rules":[]}`, "version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := filepath.Join(t.TempDir(), "rules.json")
			if err := os.WriteFile(candidate, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			err := validateConfig(context.Background(), options, "singbox/rules/local/proxy.json", candidate, []byte(test.content))
			if test.message == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("预期 %q，实际 %v", test.message, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateConfig(ctx, options, "singbox/rules/local/proxy.json", "", []byte(`{"version":1,"rules":[{"domain":"example.com"}]}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("规则校验未遵循取消: %v", err)
	}
}

func TestApplyLocalRulesStoppedValidatesBeforeCommit(t *testing.T) {
	core := targetCoreBinary(t)
	options, _, source, runtimeContent := configApplyOptions(t)
	options.SingBoxPath = core
	isolateConfigApplyHooks(t, false)
	target := "singbox/rules/local/direct.json"
	destination, err := ResolveConfig(options, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	old := []byte(`{"version":1,"rules":[{"domain":"example.com"}]}`)
	if err := os.WriteFile(destination, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"version":1,"rules":[{"domain_regex":"["}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfig(context.Background(), options, target, source, false, configRevision(old)); err == nil {
		t.Fatal("不应保存无效规则")
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != string(old) {
		t.Fatalf("无效规则覆盖了原文件: %q, %v", content, err)
	}
	valid := []byte(`{"version":1,"rules":[{"ip_cidr":"192.0.2.0/24"}]}`)
	if err := os.WriteFile(source, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfig(context.Background(), options, target, source, false, configRevision(old)); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfig(context.Background(), options, target, source, false, configRevision(old)); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("旧 revision 未被拒绝: %v", err)
	}
	assertRuntimeContent(t, options, runtimeContent)
}

func TestListConfigsUsesReadableRuntimeID(t *testing.T) {
	options := newTestOptions(t.TempDir())
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
	if err := os.WriteFile(filepath.Join(remoteRules, "cn-ip.srs"), []byte{0x00, 0xff, 0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	const content = "{\"type\":\"ebpf\"}\n"
	if err := os.WriteFile(filepath.Join(options.RuntimeDir, "inbound.json"), []byte(content), 0o600); err != nil {
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
		if documents[index].ID == "runtime/inbound.json" {
			runtimeDocument = &documents[index]
			break
		}
	}
	if runtimeDocument == nil {
		t.Fatal("运行时配置未出现在配置列表")
	}
	if runtimeDocument.ID != "runtime/inbound.json" {
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
	if _, err := ReadConfig(options, "singbox/rules/remote/cn-ip.srs"); err == nil {
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
