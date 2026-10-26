package module

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

func TestCatalogPublicCandidateCollisionDoesNotCommit(t *testing.T) {
	for _, collision := range []struct{ name, config, code string }{
		{"outbound", `{"outbounds":[{"type":"direct","tag":"np/default/NEW"}]}`, "catalog.outbound_conflict"},
		{"endpoint", `{"endpoints":[{"type":"wireguard","tag":"np/default/NEW"}]}`, "catalog.outbound_conflict"},
		{"provider", `{"providers":[{"type":"local","tag":"np/default","path":"user.json"}]}`, "catalog.provider_conflict"},
	} {
		for _, operation := range []string{"append", "edit", "import", "append-content", "edit-content", "import-content"} {
			t.Run(collision.name+"/"+operation, func(t *testing.T) {
				options := catalogFixture(t)
				writeNativePolicyFixture(t, options, collision.config)
				providerPath := filepath.Join(options.CatalogRoot, "default", "provider.json")
				metaPath := filepath.Join(options.CatalogRoot, "default", "meta.json")
				beforeProvider, beforeMeta := targetCoreRead(t, providerPath), targetCoreRead(t, metaPath)
				input := "socks://127.0.0.1:1080#NEW"
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				var result catalog.MutationResult
				var err error
				switch operation {
				case "append":
					result, err = NodeAppend(ctx, options, "default", input, false)
				case "edit":
					result, err = NodeEdit(ctx, options, "default/NODE", input, false)
				case "import":
					result, err = NodeImport(ctx, options, writeSectionSource(t, input), false)
				case "append-content":
					result, err = NodeAppendContent(ctx, options, "default", []byte(input), false)
				case "edit-content":
					result, err = NodeEditContent(ctx, options, "default/NODE", []byte(input), false)
				case "import-content":
					result, err = NodeImportContent(ctx, options, []byte(input), false)
				}
				requireNativePolicyError(t, err, collision.code, "np/default")
				if result.Revision != 0 || !bytes.Equal(beforeProvider, targetCoreRead(t, providerPath)) || !bytes.Equal(beforeMeta, targetCoreRead(t, metaPath)) {
					t.Fatal("rejected public candidate changed Provider or revision")
				}
				if string(targetCoreRead(t, paths.SingBoxConfig(options.SingBoxDir))) != collision.config {
					t.Fatal("candidate validation rewrote native config")
				}
			})
		}
	}
}

func TestCatalogWorkerCandidateGuardKeepsProviderAndRevision(t *testing.T) {
	for _, test := range []struct{ name, config, cause string }{
		{"outbound", `{"outbounds":[{"type":"direct","tag":"np/feed/NEW"}]}`, "outbounds[0].tag"},
		{"endpoint", `{"endpoints":[{"type":"wireguard","tag":"np/feed/NEW"}]}`, "endpoints[0].tag"},
		{"provider", `{"providers":[{"type":"local","tag":"np/feed","path":"user.json"}]}`, "providers"},
		{"removed-reference", `{"route":{"final":"np/feed/OLD"}}`, "np/feed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := catalogFixture(t)
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				_, _ = w.Write([]byte("socks://127.0.0.1:1080#NEW"))
			}))
			defer server.Close()
			if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: "feed", Name: "Feed", Type: "subscription", URL: server.URL, UpdateViaProxy: "never"}); err != nil {
				t.Fatal(err)
			}
			if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, "feed"), GroupID: "feed", Type: "subscription", Input: "socks://127.0.0.1:1080#OLD"}); err != nil {
				t.Fatal(err)
			}
			writeNativePolicyFixture(t, options, test.config)
			providerPath := filepath.Join(options.CatalogRoot, "feed", "provider.json")
			before := targetCoreRead(t, providerPath)
			metadata, err := catalog.PrivateMetadata(t.Context(), options.CatalogRoot, "feed")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := worker.UpdateGroup(ctx, workerOptions(options), "feed", time.Now(), nil)
			failure, ok := err.(*subscription.Error)
			if !ok || failure.Code != "provider.invalid" || !strings.Contains(fmt.Sprint(failure.Data), test.cause) || result.Persisted || requests != 1 {
				t.Fatalf("Worker did not reject actual candidate: %+v %v requests=%d", result, err, requests)
			}
			after, err := catalog.PrivateMetadata(t.Context(), options.CatalogRoot, "feed")
			if err != nil || after.Revision != metadata.Revision || after.NodeCount != metadata.NodeCount || !bytes.Equal(before, targetCoreRead(t, providerPath)) {
				t.Fatalf("rejected Worker update changed Provider or revision: %+v %v", after, err)
			}
			if string(targetCoreRead(t, paths.SingBoxConfig(options.SingBoxDir))) != test.config {
				t.Fatal("Worker validation rewrote native config")
			}
		})
	}
}

func TestCatalogPublicRemovalUsesEffectiveNativeReferences(t *testing.T) {
	type removalCase struct {
		name, config string
		inUse        bool
	}
	tests := []removalCase{
		{"selector-default-use-all", `{"outbounds":[{"type":"selector","tag":"Main","use_all_providers":true,"outbounds":["Compatible"],"default":"np/default/NODE"}]}`, true},
		{"nested-group", `{"outbounds":[{"type":"selector","tag":"Main","outbounds":["Auto"]},{"type":"urltest","tag":"Auto","outbounds":["np/default/NODE"]}]}`, true},
		{"route-final", `{"route":{"final":"np/default/NODE"}}`, true},
		{"logical-route", `{"route":{"rules":[{"type":"logical","mode":"and","rules":[{"type":"logical","mode":"or","rules":[{"domain_suffix":["example.test"]}]}],"action":"route","outbound":"np/default/NODE"}]}}`, true},
		{"bypass", `{"route":{"rules":[{"domain_suffix":["example.test"],"action":"bypass","outbound":"np/default/NODE"}]}}`, true},
		{"dns-match", `{"dns":{"rules":[{"outbound":"np/default/NODE","server":"resolver"}]}}`, true},
		{"nested-dns-match", `{"dns":{"rules":[{"type":"logical","mode":"and","rules":[{"type":"logical","mode":"or","rules":[{"outbound":["np/default/NODE"]}]}],"server":"resolver"}]}}`, true},
		{"outbound-detour", `{"outbounds":[{"type":"socks","tag":"Proxy","server":"127.0.0.1","server_port":1080,"detour":"np/default/NODE"}]}`, true},
		{"endpoint-detour", `{"endpoints":[{"type":"wireguard","tag":"WG","detour":"np/default/NODE"}]}`, true},
		{"dns-detour", `{"dns":{"servers":[{"type":"https","tag":"dns","detour":"np/default/NODE"}]}}`, true},
		{"http-detour", `{"http_clients":[{"tag":"http","detour":"np/default/NODE"}]}`, true},
		{"ntp-detour", `{"ntp":{"enabled":true,"detour":"np/default/NODE"}}`, true},
		{"dns-server-not-outbound", `{"dns":{"servers":[{"type":"local","tag":"np/default/NODE"}],"rules":[{"domain_suffix":["example.test"],"server":"np/default/NODE"}],"final":"np/default/NODE"}}`, false},
		{"unknown-fields", `{"outbounds":[{"type":"direct","tag":"Direct","providers":["np/default"],"default":"np/default/NODE","vendor":{"detour":"np/default/NODE"}}],"route":{"vendor":{"outbound":"np/default/NODE"}},"vendor":{"providers":["np/default"],"outbounds":["np/default/NODE"],"default":"np/default/NODE","final":"np/default/NODE","precision":9007199254740993}}`, false},
	}
	for _, groupType := range []string{"selector", "urltest", "loadbalance"} {
		tests = append(tests,
			removalCase{groupType + "-provider", fmt.Sprintf(`{"outbounds":[{"type":%q,"tag":"Main","providers":["np/default"]}]}`, groupType), true},
			removalCase{groupType + "-ignored-provider", fmt.Sprintf(`{"outbounds":[{"type":%q,"tag":"Main","outbounds":["Compatible"],"use_all_providers":true,"providers":["np/default"]}]}`, groupType), false},
			removalCase{groupType + "-member-use-all", fmt.Sprintf(`{"outbounds":[{"type":%q,"tag":"Main","use_all_providers":true,"outbounds":["np/default/NODE"]}]}`, groupType), true},
		)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := catalogFixture(t)
			// Leave one member so removing NODE also removes its Local Provider.
			document, err := catalog.GroupProvider(t.Context(), options.CatalogRoot, "default")
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range document.Outbounds {
				if node.Tag != "NODE" {
					if _, err := NodeRemove(t.Context(), options, "default/"+node.Tag); err != nil {
						t.Fatal(err)
					}
				}
			}
			writeNativePolicyFixture(t, options, test.config)
			providerPath := filepath.Join(options.CatalogRoot, "default", "provider.json")
			metaPath := filepath.Join(options.CatalogRoot, "default", "meta.json")
			beforeProvider, beforeMeta := targetCoreRead(t, providerPath), targetCoreRead(t, metaPath)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := NodeRemove(ctx, options, "default/NODE")
			if test.inUse {
				requireNativePolicyError(t, err, "catalog.provider_in_use", "np/default")
				if result.Revision != 0 || !bytes.Equal(beforeProvider, targetCoreRead(t, providerPath)) || !bytes.Equal(beforeMeta, targetCoreRead(t, metaPath)) {
					t.Fatal("rejected removal changed Provider or revision")
				}
			} else if err != nil || result.NodeCount != 0 || result.Revision != 4 {
				t.Fatalf("ignored fields blocked last-member removal: %+v %v", result, err)
			}
			if string(targetCoreRead(t, paths.SingBoxConfig(options.SingBoxDir))) != test.config {
				t.Fatal("removal changed native config or unknown fields")
			}
		})
	}
}
