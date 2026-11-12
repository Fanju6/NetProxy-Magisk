package module

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestCatalogCandidatePreservesExplicitReferences(t *testing.T) {
	for _, test := range []struct {
		name, config, code string
	}{
		{"explicit provider", `{"outbounds":[{"type":"selector","tag":"Work","providers":["np/sub"]}]}`, "catalog.provider_in_use"},
		{"explicit member", `{"outbounds":[{"type":"urltest","tag":"Test","outbounds":["np/sub/NODE"]}]}`, "catalog.provider_in_use"},
		{"default member", `{"outbounds":[{"type":"selector","tag":"Work","default":"np/sub/NODE"}]}`, "catalog.provider_in_use"},
		{"route member", `{"route":{"final":"np/sub/NODE"}}`, "catalog.provider_in_use"},
		{"dialer member", `{"dns":{"servers":[{"type":"tls","detour":"np/sub/NODE"}]}}`, "catalog.provider_in_use"},
		{"use all", `{"outbounds":[{"type":"selector","tag":"Work","providers":["np/sub"],"use_all_providers":true,"outbounds":["direct"]}]}`, ""},
		{"similar id", `{"route":{"final":"np/other/NODE"}}`, ""},
		{"namespace conflict", `{"providers":[{"type":"local","tag":"np/sub","path":"unused"}]}`, "catalog.provider_conflict"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := catalogFixture(t)
			targetCoreWrite(t, paths.SingBoxConfig(options.SingBoxDir), []byte(test.config))
			err := validateResourceCandidate(options, "sub", provider.Document{})
			if test.code == "" && err == nil {
				return
			}
			var problem *service.Error
			if test.code == "" || !errors.As(err, &problem) || problem.Code != test.code {
				t.Fatalf("want %q, got %v", test.code, err)
			}
		})
	}
}

func TestCatalogRemovalReloadFailureRestoresProviderAndRuntime(t *testing.T) {
	options := catalogFixture(t)
	groupID := "sub"
	groupDir := filepath.Join(options.CatalogRoot, groupID)
	if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
		GroupDir: groupDir, GroupID: groupID, Type: "subscription", Input: "socks://127.0.0.1:1081#NODE",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.InboundConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	targetCoreWrite(t, options.InboundConfig, []byte(testInboundConfig))
	if err := os.MkdirAll(options.RuntimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := paths.RuntimeConfig(options.RuntimeDir)
	targetCoreWrite(t, runtime, []byte(`{"providers":[{"tag":"np/sub"}]}`))
	before := map[string][]byte{
		filepath.Join(groupDir, "meta.json"):     targetCoreRead(t, filepath.Join(groupDir, "meta.json")),
		filepath.Join(groupDir, "provider.json"): targetCoreRead(t, filepath.Join(groupDir, "provider.json")),
		runtime:                                  targetCoreRead(t, runtime),
	}
	isolateConfigApplyHooks(t, true)
	reloadCalls, restores := 0, 0
	configReload = func(_ context.Context, _ Options) error {
		reloadCalls++
		if _, err := os.Stat(groupDir); !os.IsNotExist(err) {
			t.Fatalf("candidate removal was not installed: %v", err)
		}
		targetCoreWrite(t, runtime, []byte(`{"providers":[]}`))
		return errors.New("injected core reload failure")
	}
	configRestoreReload = func(_ context.Context, _ Options, _ configApplyJournal) error {
		restores++
		for path, expected := range before {
			if !bytes.Equal(targetCoreRead(t, path), expected) {
				t.Fatalf("restore did not publish old %s before core start", filepath.Base(path))
			}
		}
		return nil
	}
	err := RemoveSubscription(t.Context(), options, groupID)
	var problem *service.Error
	if !errors.As(err, &problem) || problem.Code != "subscription.runtime_sync_failed" || reloadCalls != 1 || restores != 1 {
		t.Fatalf("rollback not confirmed: %v reloads=%d restores=%d", err, reloadCalls, restores)
	}
	if data, ok := problem.Data.(map[string]any); !ok || data["persisted"] != false || data["rolled_back"] != true {
		t.Fatalf("unexpected rollback result: %+v", problem.Data)
	}
	if _, err := os.Stat(configTransactionPath(options)); !os.IsNotExist(err) {
		t.Fatalf("rolled-back journal left behind: %v", err)
	}
}

func TestSubscriptionCommitGuardRejectsBusyLifecycle(t *testing.T) {
	options := catalogFixture(t)
	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	release, err := ProviderCommitGuard(options)("default", provider.Document{})
	var problem *subscription.Error
	if release != nil || !errors.As(err, &problem) || problem.Code != "subscription.busy" {
		t.Fatalf("conflicting lifecycle operation not rejected: %v", err)
	}
}

func TestRemovingLastProviderRejectsEmptyNativeGroup(t *testing.T) {
	options := catalogFixture(t)
	targetCoreWrite(t, paths.SingBoxConfig(options.SingBoxDir), []byte(`{"outbounds":[{"type":"urltest","tag":"Auto","use_all_providers":true}]}`))
	if _, err := NodeRemove(t.Context(), options, "default/NODE"); err != nil {
		t.Fatal(err)
	}
	_, err := NodeRemove(t.Context(), options, "default/节点 / 02")
	var problem *service.Error
	if !errors.As(err, &problem) || problem.Code != "catalog.provider_in_use" {
		t.Fatalf("empty configured URLTest was accepted: %v", err)
	}
	present, err := catalog.GroupContainsTag(t.Context(), options.CatalogRoot, "default", "节点 / 02")
	if err != nil || !present {
		t.Fatalf("last node changed on rejection: %v", err)
	}
}

func TestSubscriptionCandidateCannotRemoveConfiguredMember(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("socks://127.0.0.1:1081#NEW\n"))
	}))
	defer source.Close()
	options := catalogFixture(t)
	const id = "sub"
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: id, Type: "subscription", Name: "fixture",
		URL: source.URL, UpdateViaProxy: "never",
	}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(options.CatalogRoot, id)
	old, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
		GroupDir: dir, GroupID: id, Type: "subscription", Input: "socks://127.0.0.1:1081#OLD",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "provider.json")
	before := targetCoreRead(t, path)
	targetCoreWrite(t, paths.SingBoxConfig(options.SingBoxDir), []byte(`{"outbounds":[{"type":"selector","tag":"Work","providers":["np/sub"],"default":"np/sub/OLD"}]}`))
	_, err = UpdateSubscription(t.Context(), options, id)
	var problem *subscription.Error
	if !errors.As(err, &problem) || problem.Code != "catalog.provider_in_use" {
		t.Fatalf("subscription candidate was not rejected: %v", err)
	}
	metadata, err := catalog.LoadMetadata(t.Context(), filepath.Join(dir, "meta.json"), id)
	if err != nil || metadata.Revision != old.Revision || !bytes.Equal(targetCoreRead(t, path), before) {
		t.Fatalf("rejected candidate changed the persisted provider: %v", err)
	}
}
