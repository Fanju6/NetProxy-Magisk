package subscription

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("{\"code\":\"subscription.updated\"}\n\n{\"ok\":true}\n"), 0o600); err != nil {
		t.Fatalf("write history: %v", err)
	}
	entries, err := LoadHistory(path)
	if err != nil || len(entries) != 2 {
		t.Fatalf("load history: %d, %v", len(entries), err)
	}
	if _, err := LoadHistory(filepath.Join(t.TempDir(), "missing.jsonl")); err != nil {
		t.Fatalf("missing history: %v", err)
	}
}

func TestLoadHistoryRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("write history: %v", err)
	}
	if _, err := LoadHistory(path); err == nil {
		t.Fatal("invalid history unexpectedly accepted")
	}
}
