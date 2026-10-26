package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestConfiguredPolicyReadsUpdatedSubscriptionWithoutFetching(t *testing.T) {
	root := t.TempDir()
	catalogRoot := filepath.Join(root, "catalog")
	var downloads, version atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		fmt.Fprintf(w, `{"outbounds":[{"type":"socks","tag":"version-%d","server":"127.0.0.1","server_port":1080}]}`, version.Load())
	}))
	defer server.Close()
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: catalogRoot, GroupID: "remote", Name: "Anonymous", Type: "subscription", URL: server.URL, UpdateInterval: 86400, UpdateViaProxy: "never", Timeout: 5}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.json")
	config := `{"outbounds":[{"type":"urltest","tag":"Auto","providers":["np/remote"]}]}`
	policyWriteFile(t, path, config)
	for _, n := range []int32{1, 2} {
		version.Store(n)
		if _, err := subscription.Update(t.Context(), subscription.UpdateOptions{Root: catalogRoot, GroupID: "remote", ProgressDir: filepath.Join(root, "progress")}); err != nil {
			t.Fatal(err)
		}
		before := downloads.Load()
		data, err := ReadConfiguredPolicyGroups(t.Context(), Options{SingBoxConfig: path, CatalogRoot: catalogRoot})
		if err != nil || len(data.Groups) != 1 || len(data.Groups[0].Items) != 1 || data.Groups[0].Items[0].Tag != fmt.Sprintf("np/remote/version-%d", n) {
			t.Fatalf("updated members %+v: %v", data, err)
		}
		if downloads.Load() != before {
			t.Fatal("offline snapshot performed network IO")
		}
		if content, _ := os.ReadFile(path); string(content) != config {
			t.Fatal("read rewrote config")
		}
	}
}

func TestStoppedPolicyExpandsPersistedCatalog(t *testing.T) {
	root := t.TempDir()
	catalogRoot := filepath.Join(root, "catalog")
	for _, id := range []string{"default", "second", "unreferenced"} {
		_, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
			GroupDir: filepath.Join(catalogRoot, id), GroupID: id, Type: "local",
			Input: "socks://127.0.0.1:1080#香港",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "config.json")
	config := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"Main","outbounds":["direct"],"providers":["np/default","np/second"],"default":"direct"}],"route":{"final":"Main"}}`
	policyWriteFile(t, path, config)
	options := Options{SingBoxConfig: path, CatalogRoot: catalogRoot, StateFile: filepath.Join(root, "missing-state"), SingBoxPath: filepath.Join(root, "never-start")}
	snapshot, err := ReadPolicyGroups(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Live || snapshot.CoreStartedAtMillis != 0 || len(snapshot.Groups) != 1 {
		t.Fatalf("invalid configured snapshot: %+v", snapshot)
	}
	items := snapshot.Groups[0].Items
	if len(items) != 3 || items[0].Tag != "direct" || items[1].Tag != "np/default/香港" || items[2].Tag != "np/second/香港" {
		t.Fatalf("Provider members missing or mixed: %+v", items)
	}
	if snapshot.Groups[0].Selected != "" || items[1].URLTestTime != 0 || items[1].URLTestDelay != 0 {
		t.Fatal("configured snapshot invented live data")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != config {
		t.Fatal("offline read changed native configuration")
	}
}

func TestConfiguredPolicyNativeMembershipRules(t *testing.T) {
	root := t.TempDir()
	catalogRoot := filepath.Join(root, "catalog")
	for _, id := range []string{"first", "second", "unused"} {
		for _, name := range []string{"same", "skip"} {
			if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(catalogRoot, id), GroupID: id, Type: "local", Input: "socks://127.0.0.1:1080#" + name}); err != nil {
				t.Fatal(err)
			}
		}
	}
	path := filepath.Join(root, "config.json")
	for _, test := range []struct {
		name, group       string
		want, unavailable []string
	}{
		{"explicit", `"type":"selector","outbounds":["direct","Nested"],"default":"direct"`, []string{"direct", "Nested"}, nil},
		{"provider", `"type":"selector","providers":["np/first"]`, []string{"np/first/same", "np/first/skip"}, nil},
		{"multiple", `"type":"urltest","providers":["np/second","np/first"]`, []string{"np/second/same", "np/second/skip", "np/first/same", "np/first/skip"}, nil},
		{"mixed-filter", `"type":"selector","outbounds":["direct"],"providers":["np/first","np/second"],"include":"np/first/","exclude":"skip"`, []string{"direct", "np/first/same"}, nil},
		{"all-replaces-list", `"type":"loadbalance","providers":["missing"],"use_all_providers":true,"include":"/same$"`, []string{"np/first/same", "np/second/same", "np/unused/same"}, []string{"external"}},
		{"same-identity-once", `"type":"selector","outbounds":["np/first/same"],"providers":["np/first","np/first"],"exclude":"skip"`, []string{"np/first/same"}, nil},
		{"external", `"type":"selector","outbounds":["direct"],"providers":["external","missing"]`, []string{"direct"}, []string{"external", "missing"}},
		{"filtered-compatible", `"type":"urltest","providers":["np/first"],"exclude":".*"`, []string{"Compatible"}, nil},
		{"empty-no-placeholder", `"type":"selector","providers":[]`, []string{}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 外部 Provider 路径不应被打开，更不能按 url 下载。
			config := `{"providers":[{"type":"http","tag":"external","url":"https://example.invalid/private"}],"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"Nested","outbounds":["direct"]},{"tag":"Test",` + test.group + `}],"custom":{"keep":true}}`
			policyWriteFile(t, path, config)
			snapshot, err := ReadConfiguredPolicyGroups(t.Context(), Options{SingBoxConfig: path, CatalogRoot: catalogRoot})
			if err != nil {
				t.Fatal(err)
			}
			group := snapshot.Groups[len(snapshot.Groups)-1]
			tags := make([]string, 0, len(group.Items))
			for _, item := range group.Items {
				tags = append(tags, item.Tag)
				if item.URLTestTime != 0 || item.URLTestDelay != 0 {
					t.Fatal("runtime delay invented")
				}
			}
			if !reflect.DeepEqual(tags, test.want) || !reflect.DeepEqual(group.UnavailableProviders, test.unavailable) {
				t.Fatalf("members=%v unresolved=%v", tags, group.UnavailableProviders)
			}
			if group.Selected != "" || snapshot.Live || snapshot.CoreStartedAtMillis != 0 || snapshot.Source != "configuration" {
				t.Fatal("live state invented")
			}
			if content, _ := os.ReadFile(path); string(content) != config {
				t.Fatal("config mutated")
			}
		})
	}
}

func TestConfiguredPolicyRereadsCatalogAfterChanges(t *testing.T) {
	root := t.TempDir()
	catalogRoot := filepath.Join(root, "catalog")
	path := filepath.Join(root, "config.json")
	policyWriteFile(t, path, `{"outbounds":[{"type":"selector","tag":"Main","providers":["np/default"]}]}`)
	options := Options{SingBoxConfig: path, CatalogRoot: catalogRoot}
	read := func(want int) {
		t.Helper()
		data, err := ReadConfiguredPolicyGroups(t.Context(), options)
		if err != nil || len(data.Groups) != 1 || len(data.Groups[0].Items) != want {
			t.Fatalf("members=%+v err=%v", data, err)
		}
	}
	read(0)
	mutation := catalog.MutationOptions{GroupDir: filepath.Join(catalogRoot, "default"), GroupID: "default", Type: "local", Input: "socks://127.0.0.1:1080#first"}
	if _, err := catalog.AppendNode(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	read(1)
	mutation.Input = "socks://127.0.0.1:1080#second"
	if _, err := catalog.AppendNode(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	read(2)
	mutation.Tag = "first"
	if _, err := catalog.RemoveNode(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	read(1)
	mutation.Tag = "second"
	if _, err := catalog.RemoveNode(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	read(0)
}
