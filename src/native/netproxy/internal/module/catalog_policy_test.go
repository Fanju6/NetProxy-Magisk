package module

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func TestCatalogNativeReferenceGuardBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name, native            string
		removeLast, wantFailure bool
	}{
		{"provider", `{"outbounds":[{"type":"selector","tag":"Custom","providers":["np/default"]}],"route":{"final":"Custom"}}`, true, true},
		{"explicit-member", `{"outbounds":[{"type":"selector","tag":"Custom","outbounds":["np/default/NODE"]}]}`, false, true},
		{"use-all", `{"outbounds":[{"type":"selector","tag":"Custom","outbounds":["DIRECT"],"use_all_providers":true}]}`, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := newTestOptions(t.TempDir())
			path := paths.SingBoxConfig(options.SingBoxDir)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(test.native), 0600); err != nil {
				t.Fatal(err)
			}
			mutation := catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default", Type: "local", Input: "socks://127.0.0.1:1080#NODE"}
			if _, err := catalog.AppendNode(t.Context(), mutation); err != nil {
				t.Fatal(err)
			}
			if !test.removeLast {
				mutation.Input = "socks://127.0.0.1:1080#OTHER"
				if _, err := catalog.AppendNode(t.Context(), mutation); err != nil {
					t.Fatal(err)
				}
			}
			providerPath := filepath.Join(mutation.GroupDir, "provider.json")
			before, err := os.ReadFile(providerPath)
			if err != nil {
				t.Fatal(err)
			}
			mutation.Tag = "NODE"
			mutation.Validate = resourceValidator(options, "default")
			_, err = catalog.RemoveNode(t.Context(), mutation)
			if (err != nil) != test.wantFailure {
				t.Fatalf("reference validation: %v", err)
			}
			if test.wantFailure {
				var failure *service.Error
				if !errors.As(err, &failure) || failure.Code != "catalog.provider_in_use" {
					t.Fatalf("missing clear diagnostic: %v", err)
				}
				after, _ := os.ReadFile(providerPath)
				if !bytes.Equal(before, after) {
					t.Fatal("rejected mutation changed Provider")
				}
			}
			unchanged, _ := os.ReadFile(path)
			if !bytes.Equal(unchanged, []byte(test.native)) {
				t.Fatal("Catalog rewrote user native config")
			}
		})
	}
}

func TestCatalogProviderConflictDoesNotCommitNode(t *testing.T) {
	options := newTestOptions(t.TempDir())
	path := paths.SingBoxConfig(options.SingBoxDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"providers":[{"type":"local","tag":"np/default","path":"user.json"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	groupDir := filepath.Join(options.CatalogRoot, "default")
	_, err := catalog.AppendNode(context.Background(), catalog.MutationOptions{GroupDir: groupDir, GroupID: "default", Type: "local", Input: "socks://127.0.0.1:1080#NODE", Validate: resourceValidator(options, "default")})
	var failure *service.Error
	if !errors.As(err, &failure) || failure.Code != "catalog.provider_conflict" {
		t.Fatalf("expected Provider tag conflict: %v", err)
	}
	if _, err := os.Stat(filepath.Join(groupDir, "provider.json")); !os.IsNotExist(err) {
		t.Fatal("conflicting mutation wrote a Provider")
	}
}
