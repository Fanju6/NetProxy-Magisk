package module

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func writeNativePolicyFixture(t *testing.T, options Options, content string) {
	t.Helper()
	if err := os.WriteFile(paths.SingBoxConfig(options.SingBoxDir), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func requireNativePolicyError(t *testing.T, err error, code, field string) {
	t.Helper()
	if code == "" {
		if err == nil {
			t.Fatal("expected invalid JSON to be rejected")
		}
		return
	}
	var failure *service.Error
	if !errors.As(err, &failure) || failure.Code != code || !strings.Contains(failure.Message, field) {
		t.Fatalf("expected %s at %s, got %v", code, field, err)
	}
}

func TestNativePolicyReferenceValidation(t *testing.T) {
	for _, test := range []struct {
		name, config, code, field string
	}{
		{"selector-provider", `{"outbounds":[{"type":"selector","tag":"Main","providers":["np/missing"]}]}`, "policy.provider_not_found", "outbounds[0].providers"},
		{"urltest-provider", `{"outbounds":[{"type":"urltest","tag":"Main","providers":["np/missing"]}]}`, "policy.provider_not_found", "outbounds[0].providers"},
		{"selector-outbound", `{"outbounds":[{"type":"selector","tag":"Main","outbounds":["missing"]}]}`, "policy.outbound_not_found", "outbounds[0].outbounds"},
		{"urltest-outbound", `{"outbounds":[{"type":"urltest","tag":"Main","outbounds":["missing"]}]}`, "policy.outbound_not_found", "outbounds[0].outbounds"},
		{"loadbalance-outbound", `{"outbounds":[{"type":"loadbalance","tag":"Main","outbounds":["missing"]}]}`, "policy.outbound_not_found", "outbounds[0].outbounds"},
		{"selector-default", `{"outbounds":[{"type":"selector","tag":"Main","outbounds":["Compatible"],"default":"missing"}]}`, "policy.outbound_not_found", "outbounds[0].default"},
		{"managed-member", `{"outbounds":[{"type":"selector","tag":"Main","outbounds":["np/default/missing"]}]}`, "policy.outbound_not_found", "outbounds[0].outbounds"},
		{"empty-outbound-reference", `{"outbounds":[{"type":"selector","tag":"Main","outbounds":[""]}]}`, "policy.outbound_not_found", "outbounds[0].outbounds"},
		{"route-final", `{"route":{"final":"missing"}}`, "policy.outbound_not_found", "route.final"},
		{"route-rule", `{"route":{"rules":[{"domain_suffix":["example.test"],"outbound":"missing"}]}}`, "policy.outbound_not_found", "route.rules[0].outbound"},
		{"logical-route", `{"route":{"rules":[{"type":"logical","mode":"and","rules":[{"type":"logical","mode":"or","rules":[{"domain_suffix":["example.test"]}]}],"outbound":"missing"}]}}`, "policy.outbound_not_found", "route.rules[0].outbound"},
		{"dns-match", `{"dns":{"rules":[{"outbound":"missing","server":"resolver"}]}}`, "policy.outbound_not_found", "dns.rules[0].outbound"},
		{"nested-dns-match", `{"dns":{"rules":[{"type":"logical","mode":"and","rules":[{"type":"logical","mode":"or","rules":[{"outbound":["missing"]}]}],"server":"resolver"}]}}`, "policy.outbound_not_found", "dns.rules[0].rules[0].rules[0].outbound"},
		{"outbound-detour", `{"outbounds":[{"type":"socks","tag":"SOCKS","server":"127.0.0.1","server_port":1080,"detour":"missing"}]}`, "policy.outbound_not_found", "outbounds[0].detour"},
		{"endpoint-detour", `{"endpoints":[{"type":"wireguard","tag":"WG","detour":"missing"}]}`, "policy.outbound_not_found", "endpoints[0].detour"},
		{"dns-detour", `{"dns":{"servers":[{"type":"https","tag":"dns","detour":"missing"}]}}`, "policy.outbound_not_found", "dns.servers[0].detour"},
		{"http-detour", `{"http_clients":[{"tag":"http","detour":"missing"}]}`, "policy.outbound_not_found", "http_clients[0].detour"},
		{"ntp-detour", `{"ntp":{"enabled":true,"detour":"missing"}}`, "policy.outbound_not_found", "ntp[0].detour"},
		{"duplicate-outbound", `{"outbounds":[{"type":"direct","tag":"same"},{"type":"direct","tag":"same"}]}`, "policy.outbound_duplicate", "outbounds[1].tag"},
		{"duplicate-endpoint", `{"endpoints":[{"type":"wireguard","tag":"same"},{"type":"wireguard","tag":"same"}]}`, "policy.outbound_duplicate", "endpoints[1].tag"},
		{"cross-kind-duplicate", `{"outbounds":[{"type":"direct","tag":"same"}],"endpoints":[{"type":"wireguard","tag":"same"}]}`, "policy.outbound_duplicate", "endpoints[0].tag"},
		{"implicit-tag-duplicate", `{"outbounds":[{"type":"direct"},{"type":"direct","tag":"0"}]}`, "policy.outbound_duplicate", "outbounds[1].tag"},
		{"core-fallback-duplicate", `{"outbounds":[{"type":"direct","tag":"Compatible"}]}`, "policy.outbound_duplicate", "outbounds[0].tag"},
		{"duplicate-provider", `{"providers":[{"type":"local","tag":"same","path":"a"},{"type":"remote","tag":"same","url":"https://example.invalid"}]}`, "policy.provider_duplicate", "providers[1].tag"},
		{"managed-provider-conflict", `{"providers":[{"type":"local","tag":"np/default","path":"user.json"}]}`, "catalog.provider_conflict", "providers"},
		{"managed-outbound-conflict", `{"outbounds":[{"type":"direct","tag":"np/default/NODE"}]}`, "catalog.outbound_conflict", "outbounds[0].tag"},
		{"managed-endpoint-conflict", `{"endpoints":[{"type":"wireguard","tag":"np/default/NODE"}]}`, "catalog.outbound_conflict", "endpoints[0].tag"},
		{"malformed-providers", `{"outbounds":[{"type":"selector","tag":"Main","providers":3}]}`, "policy.config_invalid", "outbounds[0]"},
		{"null-document", `null`, "", ""},
		{"duplicate-json-key", `{"route":{"final":"Compatible","final":"missing"}}`, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := catalogFixture(t)
			writeNativePolicyFixture(t, options, test.config)
			// No check, core start, or network is needed to reject the graph.
			options.SingBoxPath = filepath.Join(t.TempDir(), "unavailable-core")
			before := targetCoreRead(t, paths.SingBoxConfig(options.SingBoxDir))
			_, err := Prepare(t.Context(), options)
			requireNativePolicyError(t, err, test.code, test.field)
			if after := targetCoreRead(t, paths.SingBoxConfig(options.SingBoxDir)); !bytes.Equal(before, after) {
				t.Fatal("validation rewrote native config")
			}
			if _, err := os.Stat(filepath.Join(options.RuntimeDir, "providers.json")); !os.IsNotExist(err) {
				t.Fatal("invalid graph generated runtime Providers")
			}
		})
	}
}

func TestNativePolicyReferenceValidationPreservesNativeExtensions(t *testing.T) {
	options := catalogFixture(t)
	for _, config := range []string{
		`{"outbounds":[{"type":"selector","tag":"Main","providers":["np/default"],"outbounds":["Compatible"]}],"route":{"final":"Main"}}`,
		`{"outbounds":[{"type":"selector","tag":"Main","use_all_providers":true,"providers":["ignored-missing"]}],"route":{"final":"Main"}}`,
		`{"outbounds":[{"type":"selector","tag":"Main","outbounds":["np/default/NODE"],"default":"np/default/NODE"}],"route":{"final":"np/default/NODE"}}`,
		`{"outbounds":[{"type":"direct"},{"type":"selector","tag":"Main","outbounds":["0"]}]}`,
		`{"providers":[{"type":"remote","tag":"user","url":"https://example.invalid","path":"missing.json"}],"outbounds":[{"type":"selector","tag":"Main","providers":["user"],"outbounds":["user/dynamic"]}]}`,
		`{"providers":[{"type":"local","tag":"user","path":"missing.json"}],"route":{"final":"user/dynamic"}}`,
		`{"endpoints":[{"type":"wireguard","tag":"WG"}],"outbounds":[{"type":"selector","tag":"Main","outbounds":["WG"]}],"route":{"final":"Main"}}`,
		`{"outbounds":[{"type":"direct","tag":"Direct","vendor":{"outbounds":["missing"],"providers":["np/missing"],"detour":"missing"}}],"route":{"final":"Direct","vendor":{"final":"missing"}},"vendor":{"outbound":"missing","default":"missing","precision":9007199254740993}}`,
		`{"dns":{"servers":[{"type":"local","tag":"np/default/missing"}],"rules":[{"outbound":"any","server":"np/default/missing"}],"final":"np/default/missing"}}`,
		`{"route":{"rules":[{"type":"logical","mode":"and","rules":[{"type":"logical","mode":"or","rules":[{"domain_suffix":["example.test"]}]}],"outbound":"np/default/NODE"} ]},"dns":{"rules":[{"type":"logical","mode":"and","rules":[{"outbound":["np/default/NODE"]}],"server":"resolver"}]}}`,
	} {
		writeNativePolicyFixture(t, options, config)
		if err := validateNativePolicyReferences(t.Context(), options); err != nil {
			t.Fatalf("valid native projection rejected: %s: %v", config, err)
		}
		if after := targetCoreRead(t, paths.SingBoxConfig(options.SingBoxDir)); string(after) != config {
			t.Fatal("validation changed unknown fields or number spelling")
		}
	}
}

func TestNativePolicyValidationSkipsEmptyResourcesAndUsesStableTags(t *testing.T) {
	options := catalogFixture(t)
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: "empty", Type: "local", Name: "Empty"}); err != nil {
		t.Fatal(err)
	}
	writeNativePolicyFixture(t, options, `{"outbounds":[{"type":"selector","tag":"Main","providers":["np/default"]}],"providers":[{"type":"local","tag":"np/empty","path":"user.json"}]}`)
	if err := catalog.SetGroupName(t.Context(), options.CatalogRoot, "default", "Renamed", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := validateNativePolicyReferences(t.Context(), options); err != nil {
		t.Fatalf("empty resource generated Provider or rename changed tag: %v", err)
	}
	writeNativePolicyFixture(t, options, `{"outbounds":[{"type":"selector","tag":"Main","providers":["np/empty"]}]}`)
	requireNativePolicyError(t, validateNativePolicyReferences(t.Context(), options), "policy.provider_not_found", "providers")
}

func TestNativeResourceCollisionsUseCandidateWithoutCatalogLocks(t *testing.T) {
	for _, test := range []struct {
		name, config, code string
	}{
		{"provider", `{"providers":[{"type":"local","tag":"np/default","path":"user.json"}]}`, "catalog.provider_conflict"},
		{"outbound", `{"outbounds":[{"type":"direct","tag":"np/default/NEW"}]}`, "catalog.outbound_conflict"},
		{"endpoint", `{"endpoints":[{"type":"wireguard","tag":"np/default/NEW"}]}`, "catalog.outbound_conflict"},
		{"unrelated-extension", `{"vendor":{"providers":["np/default"],"tag":"np/default/NEW"}}`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := catalogFixture(t)
			writeNativePolicyFixture(t, options, test.config)
			path := filepath.Join(options.CatalogRoot, "default", "provider.json")
			before := targetCoreRead(t, path)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			_, err := catalog.AppendNode(ctx, catalog.MutationOptions{
				GroupDir: filepath.Dir(path), GroupID: "default", Type: "local", Input: "socks://127.0.0.1:1080#NEW",
				Validate: func(candidate provider.Document) error {
					return validateNativeResourceCollisions(options, "default", candidate)
				},
			})
			if test.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			requireNativePolicyError(t, err, test.code, "np/default")
			if after := targetCoreRead(t, path); !bytes.Equal(before, after) {
				t.Fatal("conflicting candidate committed while stopped")
			}
			if err := validateNativeResourceCollisions(options, "default", provider.Document{}); err != nil {
				t.Fatalf("empty candidate generated a managed Provider: %v", err)
			}
		})
	}
}

func TestNativePolicyInvalidStoppedConfigDoesNotPersist(t *testing.T) {
	options, destination, source, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	before := targetCoreRead(t, destination)
	if err := os.WriteFile(source, []byte(`{"outbounds":[{"type":"selector","tag":"Main","providers":["np/missing"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ApplyConfig(t.Context(), options, "singbox/config.json", source, false, "")
	requireNativePolicyError(t, err, "policy.provider_not_found", "providers")
	if after := targetCoreRead(t, destination); !bytes.Equal(before, after) {
		t.Fatal("invalid stopped config replaced native config")
	}
	assertRuntimeContent(t, options, runtimeContent)
}

func TestSelectorDefaultApplyConfigContent(t *testing.T) {
	for _, test := range []struct{ name, selector, code string }{
		{"provider-only", `"providers":["np/default"],"default":"np/default/node-a"`, "policy.selector_default_invalid"},
		{"provider-use-all", `"use_all_providers":true,"default":"np/default/node-a"`, "policy.selector_default_invalid"},
		{"unknown-default", `"providers":["np/default"],"default":"missing"`, "policy.outbound_not_found"},
		{"static-nonmember", `"outbounds":["direct"],"default":"other"`, "policy.selector_default_invalid"},
		{"static-not-explicit", `"providers":["np/default"],"default":"direct"`, "policy.selector_default_invalid"},
		{"static-member", `"outbounds":["direct"],"default":"direct"`, ""},
		{"slash-member", `"outbounds":["np/custom/node-a"],"default":"np/custom/node-a"`, ""},
		{"compatible-implicit", `"providers":["np/default"],"default":"Compatible"`, ""},
		{"compatible-empty-members", `"outbounds":[],"providers":["np/default"],"default":"Compatible"`, ""},
		{"compatible-explicit", `"outbounds":["Compatible"],"default":"Compatible"`, ""},
		{"compatible-not-member", `"outbounds":["direct"],"default":"Compatible"`, "policy.selector_default_invalid"},
		{"without-default", `"providers":["np/default"]`, ""},
		{"empty-default", `"providers":["np/default"],"default":""`, ""},
	} {
		for _, target := range []string{"singbox/config.json", "singbox/outbounds"} {
			for _, validateOnly := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/validate=%t", test.name, target, validateOnly), func(t *testing.T) {
					options, destination, _, runtimeContent := configApplyOptions(t)
					isolateConfigApplyHooks(t, false)
					if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
						GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default", Type: "local",
						Input: "socks://127.0.0.1:1080#node-a",
					}); err != nil {
						t.Fatal(err)
					}
					before := targetCoreRead(t, destination)
					read, err := ReadConfig(options, target)
					if err != nil {
						t.Fatal(err)
					}
					providerPath := filepath.Join(options.CatalogRoot, "default", "provider.json")
					metaPath := filepath.Join(options.CatalogRoot, "default", "meta.json")
					providerBefore, metaBefore := targetCoreRead(t, providerPath), targetCoreRead(t, metaPath)
					candidate := `{"outbounds":[{"type":"selector","tag":"Main",` + test.selector + `},{"type":"direct","tag":"direct"},{"type":"direct","tag":"other"},{"type":"direct","tag":"np/custom/node-a"}]}`
					revision, err := ApplyConfigContent(t.Context(), options, target, []byte(candidate), validateOnly, read["revision"])
					if test.code != "" {
						requireNativePolicyError(t, err, test.code, "outbounds[0].default")
						if revision != "" {
							t.Fatal("rejected default returned a saved revision")
						}
					} else if err != nil || revision == "" {
						t.Fatalf("valid default rejected: %s %v", revision, err)
					}
					after, readErr := ReadConfig(options, target)
					if readErr != nil {
						t.Fatal(readErr)
					}
					if test.code != "" || validateOnly {
						if !bytes.Equal(before, targetCoreRead(t, destination)) || after["revision"] != read["revision"] {
							t.Fatal("default preflight changed config or revision")
						}
					} else {
						var saved, expected struct {
							Outbounds []map[string]any `json:"outbounds"`
						}
						if err := json.Unmarshal([]byte(after["content"]), &saved); err != nil {
							t.Fatal(err)
						}
						if err := json.Unmarshal([]byte(candidate), &expected); err != nil {
							t.Fatal(err)
						}
						if after["revision"] != revision || !reflect.DeepEqual(saved, expected) {
							t.Fatal("saved default or revision differs from candidate")
						}
					}
					assertRuntimeContent(t, options, runtimeContent)
					assertInboundJournalAbsent(t, options)
					if !bytes.Equal(providerBefore, targetCoreRead(t, providerPath)) || !bytes.Equal(metaBefore, targetCoreRead(t, metaPath)) {
						t.Fatal("default preflight changed Catalog or revision")
					}
				})
			}
		}
	}
}
