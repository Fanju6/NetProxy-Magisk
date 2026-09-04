package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigApplyReturnsStructuredConflict(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config", "singbox")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"dns":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "dns.json")
	if err := os.WriteFile(source, []byte(`{"dns":{"final":"new"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runModuleConfig(context.Background(), []string{
		"apply", "--module-dir", root, "--state-file", filepath.Join(root, "state", "service.json"),
		"--revision", "stale", "singbox/dns", source,
	})
	structured, ok := errors.AsType[*resultError](err)
	if !ok || structured.Code != "config.conflict" {
		t.Fatalf("未返回配置冲突契约: %v", err)
	}
}
