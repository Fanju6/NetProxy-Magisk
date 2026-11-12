package module

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestEffectiveConfigPreservesNativeFieldsAndOrdering(t *testing.T) {
	dir := t.TempDir()
	source := paths.SingBoxConfig(dir)
	original := []byte(`{"route":{"final":"custom","rules":[{"outbound":"direct"}]},"experimental":{"custom":{"value":9007199254740993}},"inbounds":[{"tag":"user"}],"outbounds":[{"tag":"custom"}],"providers":[{"tag":"external"}]}`)
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	projection := []jsontext.Value{jsontext.Value(`{"tag":"managed-provider"}`)}
	dest := filepath.Join(dir, "runtime", "config.json")
	inbounds := []jsontext.Value{jsontext.Value(`{"tag":"netproxy-in"}`)}
	if err := writeEffectiveConfig(t.Context(), dir, dest, inbounds, projection); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"route":{"final":"custom","rules":[{"outbound":"direct"}]},"experimental":{"custom":{"value":9007199254740993}},"inbounds":[{"tag":"user"},{"tag":"netproxy-in"}],"outbounds":[{"tag":"custom"}],"providers":[{"tag":"external"},{"tag":"managed-provider"}]}`)
	var gotDocument, wantDocument jsontext.Value = bytes.Clone(raw), want
	if err := gotDocument.Canonicalize(jsontext.CanonicalizeRawInts(false)); err != nil {
		t.Fatal(err)
	}
	if err := wantDocument.Canonicalize(jsontext.CanonicalizeRawInts(false)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotDocument, wantDocument) {
		t.Fatalf("unexpected assembled configuration: %s", raw)
	}
	if current, err := os.ReadFile(source); err != nil || !bytes.Equal(current, original) {
		t.Fatalf("user configuration changed: %v", err)
	}
	if err := os.WriteFile(source, []byte(`{"providers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var invalid *json.SemanticError
	if err := writeEffectiveConfig(t.Context(), dir, dest, inbounds, projection); !errors.As(err, &invalid) {
		t.Fatalf("invalid provider array not rejected: %v", err)
	}
	if current, err := os.ReadFile(dest); err != nil || !bytes.Equal(current, raw) {
		t.Fatalf("failed assembly replaced the previous configuration: %v", err)
	}
}
