package module

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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
			Root: options.CatalogRoot, GroupID: id, Name: "同名订阅", Type: "subscription",
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
			{"type":"selector","tag":"Work","outbounds":["direct"],"providers":["同名订阅 (2)"],"default":"direct"},
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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
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
			{"Main", main, []string{"direct", "Work", "同名订阅/shared", "同名订阅 (2)/shared"}},
			{"Work", work, []string{"direct", "同名订阅 (2)/shared"}},
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
		if _, err := SelectPolicyGroup(ctx, options, work, "同名订阅 (2)/shared"); err != nil {
			t.Fatal(err)
		}
		checkGroups(identity, "direct", "同名订阅 (2)/shared")
		main := service.PolicyTarget{CoreStartedAtMillis: identity, GroupTag: "Main"}
		if _, err := SelectPolicyGroup(ctx, options, main, "同名订阅/shared"); err != nil {
			t.Fatal(err)
		}
		checkGroups(identity, "同名订阅/shared", "同名订阅 (2)/shared")
		name := "Renamed subscription"
		edited, err := EditSubscription(ctx, options, "b", subscription.EditOptions{Name: &name})
		var problem *service.Error
		if !errors.As(err, &problem) || problem.Code != "catalog.provider_in_use" || edited.Persisted {
			t.Fatalf("explicit Provider reference must block rename: %+v: %v", edited, err)
		}
		checkGroups(identity, "同名订阅/shared", "同名订阅 (2)/shared")
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
				return group.Tag == "Main" && group.Selected == "同名订阅/shared" &&
					slices.ContainsFunc(group.Items, func(item serviceapi.GroupItem) bool { return item.Tag == "同名订阅/added" })
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
		checkGroups(identity, "同名订阅/shared", "同名订阅 (2)/shared")
	})
	emptyConfig := strings.NewReplacer(
		`"providers":["同名订阅 (2)"]`, `"use_all_providers":true`,
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
				return strings.Contains(item.Tag, "/")
			}) {
				t.Fatalf("empty Catalog did not fall back to configured member: %+v", group)
			}
		}
	})

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer probe.Close()
	socks, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	socksPort := socks.Addr().(*net.TCPAddr).Port
	_ = socks.Close()
	targetCoreWrite(t, filepath.Join(dir, "config.json"), []byte(fmt.Sprintf(`{
		"log":{"disabled":true},
		"inbounds":[{"type":"socks","tag":"fixture-socks","listen":"127.0.0.1","listen_port":%d}],
		"services":[{"type":"api","listen":"127.0.0.1","listen_port":%d,"secret":"fixture"}],
		"outbounds":[
			{"type":"selector","tag":"Proxy","outbounds":["Auto","block","direct"],"use_all_providers":true,"default":"block"},
			{"type":"urltest","tag":"Auto","outbounds":["block"],"use_all_providers":true,"url":%q},
			{"type":"block","tag":"block"},
			{"type":"direct","tag":"direct"}
		],
		"route":{"rules":[{"inbound":["fixture-socks"],"action":"route","outbound":"direct"}],"final":"Proxy"}
	}`, socksPort, address.Port, probe.URL)))
	waitAuto := func(expected string, failedMember string) {
		t.Helper()
		watch, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var last []serviceapi.Group
		for {
			groups, err := client.Groups(watch)
			if err == nil {
				last = groups
				if slices.ContainsFunc(groups, func(group serviceapi.Group) bool {
					if group.Tag != "Auto" || group.Selectable || group.Selected != expected {
						return false
					}
					if failedMember != "" {
						return slices.ContainsFunc(group.Items, func(item serviceapi.GroupItem) bool {
							return item.Tag == failedMember && item.URLTestTime == 0 && item.URLTestDelay == 0
						})
					}
					return true
				}) {
					return
				}
			}
			select {
			case <-watch.Done():
				t.Fatalf("Auto did not select %q: %+v (%v)", expected, last, err)
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	selectAuto := func(identity int64) {
		t.Helper()
		if _, err := SelectPolicyGroup(ctx, options, service.PolicyTarget{
			CoreStartedAtMillis: identity, GroupTag: "Proxy",
		}, "Auto"); err != nil {
			t.Fatal(err)
		}
	}
	run(func(identity int64) {
		selectAuto(identity)
		waitAuto("block", "")
	})
	if err := catalog.InitializeGroup(ctx, catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "auto-test", Name: "Auto test", Type: "local",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(ctx, catalog.MutationOptions{
		GroupDir: filepath.Join(options.CatalogRoot, "auto-test"), GroupID: "auto-test", Type: "local",
		Input: fmt.Sprintf("socks://127.0.0.1:%d#healthy", socksPort),
	}); err != nil {
		t.Fatal(err)
	}
	run(func(identity int64) {
		selectAuto(identity)
		if err := client.URLTest(ctx, "Auto"); err != nil {
			t.Fatal(err)
		}
		waitAuto("Auto test/healthy", "")
		probe.Close()
		if err := client.URLTest(ctx, "Auto"); err != nil {
			t.Fatal(err)
		}
		// Alpha.13 clears a failed result but retains the previous selection.
		waitAuto("Auto test/healthy", "Auto test/healthy")
	})
	if err := catalog.DeleteGroup(ctx, options.CatalogRoot, "auto-test"); err != nil {
		t.Fatal(err)
	}
	run(func(identity int64) {
		selectAuto(identity)
		waitAuto("block", "")
	})
}
