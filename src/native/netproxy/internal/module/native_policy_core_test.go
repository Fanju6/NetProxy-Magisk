package module

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestNativePolicyTargetCoreSelectionPersistsByGroup(t *testing.T) {
	core := targetCoreBinary(t)
	dir := t.TempDir()
	options := newTestOptions(dir)
	for _, id := range []string{"a", "b"} {
		if err := catalog.EnsureGroup(t.Context(), catalog.GroupOptions{
			Root: options.CatalogRoot, GroupID: id, Name: "Same name", Type: "subscription",
			URL: "https://invalid.invalid/never", UpdateInterval: 3600, UpdateViaProxy: "never", Timeout: 60,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
			GroupDir: filepath.Join(options.CatalogRoot, id), GroupID: id,
			Type: "subscription", Input: "socks://127.0.0.1:1080#shared",
		}); err != nil {
			t.Fatal(err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	_ = listener.Close()
	config := []byte(fmt.Sprintf(`{
		"log":{"disabled":true},
		"experimental":{"cache_file":{"enabled":true,"path":"cache.db"}},
		"services":[{"type":"api","listen":"127.0.0.1","listen_port":%d,"secret":"fixture"}],
		"outbounds":[
			{"type":"selector","tag":"Main","outbounds":["direct","Work"],"use_all_providers":true,"default":"direct"},
			{"type":"selector","tag":"Work","outbounds":["direct"],"providers":["np/b"],"default":"direct"},
			{"type":"direct","tag":"direct"}
		],
		"route":{"final":"Main"}
	}`, address.Port))
	targetCoreWrite(t, filepath.Join(dir, "config.json"), config)
	configPath := filepath.Join(dir, "effective.json")
	options.SingBoxDir = dir
	options.SingBoxPath = core
	options.ServiceAddress = address.String()
	options.ServiceSecret = "fixture"
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	client, err := serviceapi.New(address.String(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	run := func(check func(int64)) {
		t.Helper()
		projection, err := catalog.BuildRuntime(ctx, options.CatalogRoot)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeEffectiveConfig(ctx, dir, configPath, nil, projection.Providers); err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, core, "run", "-c", configPath)
		command.Dir = dir
		log, err := os.CreateTemp(dir, "core-")
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		command.Stderr = log
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		defer func() {
			_ = command.Process.Kill()
			<-done
		}()
		var identity int64
		for {
			attempt, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			started, err := client.Ready(attempt)
			cancel()
			if err == nil {
				identity = started.UnixMilli
				break
			}
			select {
			case <-ctx.Done():
				t.Fatalf("isolated core did not become ready: %s", targetCoreRead(t, log.Name()))
			case <-time.After(25 * time.Millisecond):
			}
		}
		if err := os.MkdirAll(filepath.Dir(options.StateFile), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := WriteServiceState(options.StateFile, "ready", int64(command.Process.Pid), identity/1000, time.Now().Unix(), "", ServiceIdentity{Backend: "tun", StartedAtMillis: identity}); err != nil {
			t.Fatal(err)
		}
		check(identity)
	}
	checkGroups := func(identity int64, main, work string) {
		t.Helper()
		snapshot, err := ReadPolicyGroups(ctx, options)
		if err != nil || !snapshot.Live || snapshot.CoreStartedAtMillis != identity {
			t.Fatalf("expected live policy snapshot, got %+v: %v", snapshot, err)
		}
		groups := snapshot.Groups
		if len(groups) != 2 {
			t.Fatalf("Catalog added or removed native policy groups: %+v", groups)
		}
		for _, expected := range []struct {
			tag, selected string
			members       []string
		}{
			{"Main", main, []string{"direct", "Work", "np/a/shared", "np/b/shared"}},
			{"Work", work, []string{"direct", "np/b/shared"}},
		} {
			index := slices.IndexFunc(groups, func(group serviceapi.Group) bool { return group.Tag == expected.tag })
			if index < 0 {
				t.Fatalf("missing group %s", expected.tag)
			}
			group := groups[index]
			actual := make([]string, 0, len(group.Items))
			for _, item := range group.Items {
				actual = append(actual, item.Tag)
			}
			slices.Sort(actual)
			slices.Sort(expected.members)
			if !group.Selectable || group.Selected != expected.selected || !slices.Equal(actual, expected.members) {
				t.Fatalf("unexpected native group: %+v", group)
			}
		}
	}
	var previousIdentity int64
	run(func(identity int64) {
		previousIdentity = identity
		checkGroups(identity, "direct", "direct")
		work := service.PolicyTarget{CoreStartedAtMillis: identity, GroupTag: "Work"}
		if _, err := SelectPolicyGroup(ctx, options, work, "np/b/shared"); err != nil {
			t.Fatal(err)
		}
		checkGroups(identity, "direct", "np/b/shared")
		main := service.PolicyTarget{CoreStartedAtMillis: identity, GroupTag: "Main"}
		if _, err := SelectPolicyGroup(ctx, options, main, "np/a/shared"); err != nil {
			t.Fatal(err)
		}
		checkGroups(identity, "np/a/shared", "np/b/shared")
		name := "Renamed subscription"
		edited, err := EditSubscription(ctx, options, "a", subscription.EditOptions{Name: &name})
		if err != nil || !edited.NameChanged || !edited.Persisted || edited.RequiresUpdate {
			t.Fatalf("rename must be metadata-only: %+v: %v", edited, err)
		}
		checkGroups(identity, "np/a/shared", "np/b/shared")
		if _, err := NodeAppend(ctx, options, "a", "socks://127.0.0.1:1081#added", false); err != nil {
			t.Fatal(err)
		}
		watch, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		for {
			snapshot, err := ReadPolicyGroups(watch, options)
			if err != nil {
				t.Fatalf("read after Provider edit: %v", err)
			}
			if !snapshot.Live || snapshot.CoreStartedAtMillis != identity {
				t.Fatal("content-only Provider edit replaced the core")
			}
			if slices.ContainsFunc(snapshot.Groups, func(group serviceapi.Group) bool {
				return group.Tag == "Main" && group.Selected == "np/a/shared" &&
					slices.ContainsFunc(group.Items, func(item serviceapi.GroupItem) bool { return item.Tag == "np/a/added" })
			}) {
				break
			}
			select {
			case <-watch.Done():
				t.Fatal("Provider watcher did not publish the new member")
			case <-time.After(50 * time.Millisecond):
			}
		}
		if _, err := NodeRemove(ctx, options, "a/added"); err != nil {
			t.Fatal(err)
		}
	})
	run(func(identity int64) {
		stale := service.PolicyTarget{CoreStartedAtMillis: previousIdentity, GroupTag: "Main"}
		_, err := SelectPolicyGroup(ctx, options, stale, "direct")
		var rejected *service.Error
		if !errors.As(err, &rejected) || rejected.Code != "policy.core_changed" {
			t.Fatalf("stale selection was not rejected: %v", err)
		}
		checkGroups(identity, "np/a/shared", "np/b/shared")
	})
	emptyConfig := strings.NewReplacer(
		`"providers":["np/b"]`, `"use_all_providers":true`,
		`"default":"direct"`, `"default":"block"`,
		`"outbounds":["direct"`, `"outbounds":["block"`,
		`{"type":"direct","tag":"direct"}`, `{"type":"direct","tag":"direct"},{"type":"block","tag":"block"}`,
	).Replace(string(config))
	targetCoreWrite(t, filepath.Join(dir, "config.json"), []byte(emptyConfig))
	for _, id := range []string{"a", "b"} {
		if err := catalog.DeleteGroup(ctx, options.CatalogRoot, id); err != nil {
			t.Fatal(err)
		}
	}
	run(func(identity int64) {
		snapshot, err := ReadPolicyGroups(ctx, options)
		if err != nil || !snapshot.Live || snapshot.CoreStartedAtMillis != identity || len(snapshot.Groups) != 2 {
			t.Fatalf("empty Catalog did not start: %+v: %v", snapshot, err)
		}
		for _, group := range snapshot.Groups {
			if group.Selected != "block" || slices.ContainsFunc(group.Items, func(item serviceapi.GroupItem) bool {
				return strings.HasPrefix(item.Tag, "np/")
			}) {
				t.Fatalf("empty Catalog did not fall back to configured member: %+v", group)
			}
		}
	})
}
