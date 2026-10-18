package subscription

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func TestCandidateGuardRetainsPreviousProvider(t *testing.T) {
	root := t.TempDir()
	content := `{"outbounds":[{"type":"socks","tag":"one","server":"127.0.0.1","server_port":1080}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(content)) }))
	defer server.Close()
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: root, GroupID: "test", Type: "subscription", URL: server.URL, Timeout: 5}); err != nil {
		t.Fatal(err)
	}
	options := UpdateOptions{Root: root, GroupID: "test", ProgressDir: filepath.Join(root, "progress"), Now: time.Now()}
	initial, err := Update(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "test", "provider.json")
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = `{"outbounds":[{"type":"socks","tag":"two","server":"127.0.0.2","server_port":1080}]}`
	options.ValidateProvider = func(document provider.Document) error {
		if len(document.Outbounds) != 1 || document.Outbounds[0].Tag != "two" {
			t.Fatal("guard did not see candidate")
		}
		return errors.New("native group still references one")
	}
	result, err := Update(t.Context(), options)
	if err == nil || result.Persisted {
		t.Fatalf("rejected candidate was committed: %+v %v", result, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(previous, after) {
		t.Fatal("candidate failure replaced the last valid Provider")
	}
	metadata, err := catalog.PrivateMetadata(t.Context(), root, "test")
	if err != nil || metadata.Revision != initial.Revision {
		t.Fatalf("rejected candidate changed revision: %+v %v", metadata, err)
	}
	options.ValidateProvider = nil
	content = `{"outbounds":[{"type":"socks","tag":"one","server":"127.0.0.2","server_port":1080}]}`
	result, err = Update(t.Context(), options)
	if err != nil || !result.ReloadRequired {
		t.Fatalf("same-tag replacement needs acknowledged reload: %+v %v", result, err)
	}
}
