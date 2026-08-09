package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverRollsBackIncompletePair(t *testing.T) {
	root := t.TempDir()
	groupDir := filepath.Join(root, "group")
	stagingDir := filepath.Join(root, "staging", "catalog-crashed")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "provider.json"), []byte("new-provider"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "meta.json"), []byte("new-meta"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"provider.json.bak", "meta.json.bak"} {
		content := "old-provider"
		if strings.HasPrefix(name, "meta") {
			content = "old-meta"
		}
		if err := os.WriteFile(filepath.Join(stagingDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "target"), []byte(groupDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "journal"), []byte("begin\nprovider\nmeta\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Recover(root); err != nil {
		t.Fatalf("recover: %v", err)
	}
	providerContent, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(providerContent) != "old-provider" {
		t.Fatalf("provider was not rolled back: %q", providerContent)
	}
	metadataContent, err := os.ReadFile(filepath.Join(groupDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(metadataContent) != "old-meta" {
		t.Fatalf("metadata was not rolled back: %q", metadataContent)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("staging transaction was not removed: %v", err)
	}
}

func TestRecoverRemovesCommittedPairJournal(t *testing.T) {
	root := t.TempDir()
	txDir := filepath.Join(root, "staging", "catalog-committed")
	if err := os.MkdirAll(txDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(txDir, "journal"), []byte("begin\nprovider\nmeta\ncommit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(root); err != nil {
		t.Fatalf("recover committed journal: %v", err)
	}
	if _, err := os.Stat(txDir); !os.IsNotExist(err) {
		t.Fatalf("committed transaction was not cleaned: %v", err)
	}
}

func TestRecoverRemovesIncompleteJournal(t *testing.T) {
	root := t.TempDir()
	txDir := filepath.Join(root, "staging", "catalog-incomplete")
	if err := os.MkdirAll(txDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(txDir, "journal"), []byte("begin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(root); err != nil {
		t.Fatalf("recover incomplete journal: %v", err)
	}
	if _, err := os.Stat(txDir); !os.IsNotExist(err) {
		t.Fatalf("incomplete transaction was not removed: %v", err)
	}
}
