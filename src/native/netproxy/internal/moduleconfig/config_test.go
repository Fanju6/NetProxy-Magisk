package moduleconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdatePreservesCommentsAndReadsQuotedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	content := "# 注释\nACTIVE_GROUP_ID=\"default\"\nSELECTOR_MODE=urltest\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Update(path, map[string]string{
		"ACTIVE_GROUP_ID":   Quote("remote"),
		"SELECTOR_MODE":     "urltest",
		"SELECTED_NODE_REF": Quote(""),
	}); err != nil {
		t.Fatal(err)
	}
	value, err := ReadValue(path, "ACTIVE_GROUP_ID", "")
	if err != nil || value != "remote" {
		t.Fatalf("active group = %q, err = %v", value, err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "# 注释\nACTIVE_GROUP_ID=\"remote\"\nSELECTOR_MODE=urltest\nSELECTED_NODE_REF=\"\"\n" {
		t.Fatalf("unexpected config:\n%s", updated)
	}
}
