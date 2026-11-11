package policy

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

func TestSaveLoadPreservesSelectionAndAddsNewDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "policies.json")
	items := Defaults()
	items[0].Mode = "manual"
	items[0].NodeTag = "Tokyo"
	if err := Save(context.Background(), path, items); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(Defaults()) || loaded[0].Mode != "manual" || loaded[0].NodeTag != "Tokyo" {
		t.Fatalf("selection was not preserved: %#v", loaded)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	items, err := Load(context.Background(), filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7 {
		t.Fatalf("want seven defaults, got %d", len(items))
	}
}

func TestSaveLoadPreservesCustomPolicies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.json")
	custom := Policy{
		ID:             "custom-example",
		Name:           "Work AI",
		Type:           "custom",
		PackageNames:   []string{"com.example.work"},
		DomainSuffixes: []string{"example.com"},
		GroupID:        "default",
		Mode:           "auto",
		RuntimeTag:     "Proxy",
	}
	items := append(Defaults(), custom)
	if err := Save(context.Background(), path, items); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := Find(loaded, custom.ID)
	if !ok || got.Name != custom.Name || !slices.Contains(got.PackageNames, "com.example.work") {
		t.Fatalf("custom policy was not preserved: %#v", loaded)
	}
	if err := Validate(custom); err != nil {
		t.Fatalf("valid custom policy rejected: %v", err)
	}
}

func TestValidateRejectsEmptyCustomPolicy(t *testing.T) {
	if err := Validate(Policy{ID: "custom-empty", Name: "Empty", Type: "custom", Mode: "proxy"}); err == nil {
		t.Fatal("empty custom policy should require a match condition")
	}
}

func TestValidateRejectsInvalidPolicyPorts(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		if err := Validate(Policy{ID: "custom-port", Name: "Ports", Type: "custom", PackageNames: []string{"com.example"}, Ports: []int{port}, Mode: "proxy"}); err == nil {
			t.Fatalf("invalid port %d was accepted", port)
		}
	}
}

func TestDefaultsUseServiceSpecificRuleSets(t *testing.T) {
	defaults := Defaults()
	want := map[string]string{
		"youtube": "geosite/youtube",
		"tiktok":  "geosite/tiktok",
		"netflix": "geosite/netflix",
		"github":  "geosite/github",
	}
	for id, ruleSet := range want {
		item, ok := Find(defaults, id)
		if !ok || len(item.RuleSets) != 1 || item.RuleSets[0] != ruleSet {
			t.Fatalf("%s rule sets = %#v, want [%s]", id, item.RuleSets, ruleSet)
		}
	}
	fcm, ok := Find(defaults, "fcm")
	if !ok || fcm.Mode != "proxy" || len(fcm.RuleSets) != 1 || fcm.RuleSets[0] != "fcm" || !strings.Contains(fmt.Sprint(fcm.Ports), "5228") {
		t.Fatalf("FCM default = %#v", fcm)
	}
	ai, ok := Find(defaults, "ai")
	if !ok || !slices.Contains(ai.PackageNames, "com.openai.chatgpt") || !slices.Contains(ai.DomainSuffixes, "openai.com") {
		t.Fatalf("AI match set = %#v", ai)
	}
}

func TestLoadUpgradesOlderAIPolicyMatchSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.json")
	old := `{"schema":1,"policies":[{"id":"ai","name":"AI","icon":"AI","type":"domain","rule_sets":["geosite/category-ai-!cn"],"group_id":"default","node_tag":"","mode":"manual","runtime_tag":"Proxy"}]}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ai, ok := Find(items, "ai")
	if !ok || !slices.Contains(ai.PackageNames, "com.openai.chatgpt") || !slices.Contains(ai.DomainSuffixes, "chatgpt.com") || ai.Mode != "manual" {
		t.Fatalf("older AI policy was not upgraded without losing selection: %#v", ai)
	}
}

func TestLoadPreservesDisabledBuiltinPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.json")
	content := `{"schema":1,"policies":[{"id":"youtube","name":"YouTube","icon":"YT","type":"domain","rule_sets":["geosite/youtube"],"group_id":"default","mode":"auto","runtime_tag":"Proxy","disabled":true}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	youtube, ok := Find(items, "youtube")
	if !ok || !youtube.Disabled {
		t.Fatalf("disabled built-in policy was not preserved: %#v", youtube)
	}
}

func TestSaveUsesPrivateAtomicFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policies.json")
	if err := Save(context.Background(), path, Defaults()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("want private policy file, got %o", info.Mode().Perm())
	}
}

func TestBuildRuntimeProjectsAutoManualAndMissingNode(t *testing.T) {
	root := t.TempDir()
	if err := catalog.EnsureGroup(context.Background(), catalog.GroupOptions{Root: root, GroupID: "default", Name: "本地配置", Type: "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(context.Background(), catalog.MutationOptions{
		GroupDir: filepath.Join(root, "default"), GroupID: "default", Type: "local",
		Input: "socks://127.0.0.1:1080#" + url.PathEscape("Tokyo"),
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "runtime", "policies.json")
	items := []Policy{
		{ID: "ai", RuleSets: []string{"geosite/ai"}, GroupID: "default", Mode: "auto", RuntimeTag: "Proxy"},
		{ID: "youtube", RuleSets: []string{"geosite/video"}, GroupID: "default", Mode: "manual", NodeTag: "Tokyo", RuntimeTag: "Proxy"},
		{ID: "google", RuleSets: []string{"geosite/google"}, GroupID: "default", Mode: "manual", NodeTag: "Removed", RuntimeTag: "Proxy"},
		{ID: "regional", RuleSets: []string{"geosite/regional"}, GroupID: "default", Mode: "auto", Region: "other"},
	}
	written, err := BuildRuntime(context.Background(), path, root, items)
	if err != nil || !written {
		t.Fatalf("BuildRuntime() = %v, %v", written, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, `"outbound":"Auto/本地配置"`) || !strings.Contains(text, `"outbound":"本地配置/Tokyo"`) {
		t.Fatalf("runtime overlay omitted auto or manual target: %s", text)
	}
	if !strings.Contains(text, `"action":"hijack-dns"`) || strings.Index(text, `"action":"hijack-dns"`) > strings.Index(text, `"outbound":"Auto/本地配置"`) {
		t.Fatalf("runtime overlay must hijack DNS before policy routes: %s", text)
	}
	if strings.Contains(text, "Removed") {
		t.Fatalf("runtime overlay retained missing manual node: %s", text)
	}
	if !strings.Contains(text, `"outbound":"Auto/本地配置 [other]"`) {
		t.Fatalf("地区策略未引用对应地区自动分组: %s", text)
	}
}

func TestBuildRuntimeProjectsPackageAndDomainMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "policies.json")
	written, err := BuildRuntime(context.Background(), path, t.TempDir(), []Policy{
		{ID: "ai", RuleSets: []string{"geosite/ai"}, PackageNames: []string{"com.openai.chatgpt"}, DomainSuffixes: []string{"openai.com"}, Mode: "proxy"},
	})
	if err != nil || !written {
		t.Fatalf("BuildRuntime() = %v, %v", written, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, `"package_name":["com.openai.chatgpt"]`) || !strings.Contains(text, `"domain_suffix":["openai.com"]`) {
		t.Fatalf("runtime overlay omitted AI app/domain matches: %s", text)
	}
}

func TestBuildRuntimeSkipsDisabledPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "policies.json")
	written, err := BuildRuntime(context.Background(), path, t.TempDir(), []Policy{
		{ID: "youtube", RuleSets: []string{"geosite/youtube"}, Mode: "proxy", Disabled: true},
	})
	if err != nil || !written {
		t.Fatalf("BuildRuntime() = %v, %v", written, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "geosite/youtube") {
		t.Fatalf("disabled policy still produced a route: %s", content)
	}
}

func TestBuildRuntimeKeepsPolicyPortsWithSelectedTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "runtime", "policies.json")
	written, err := BuildRuntime(context.Background(), path, root, []Policy{
		{ID: "fcm", RuleSets: []string{"fcm"}, Ports: []int{5228, 5229, 5230}, Mode: "direct"},
	})
	if err != nil || !written {
		t.Fatalf("BuildRuntime() = %v, %v", written, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, `"rule_set":["fcm"]`) || !strings.Contains(text, `"network":"tcp"`) || !strings.Contains(text, `"port":[5228,5229,5230]`) || !strings.Contains(text, `"outbound":"direct"`) {
		t.Fatalf("FCM route overlay lost domain/port target: %s", text)
	}
}

func TestBuildRuntimeProjectsIndependentPolicyTargets(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "runtime", "policies.json")
	items := []Policy{
		{ID: "direct", RuleSets: []string{"fcm"}, Mode: "direct"},
		{ID: "proxy", RuleSets: []string{"geosite/google"}, Mode: "proxy"},
	}
	written, err := BuildRuntime(context.Background(), path, root, items)
	if err != nil || !written {
		t.Fatalf("BuildRuntime() = %v, %v", written, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, `"outbound":"direct"`) || !strings.Contains(text, `"outbound":"Proxy"`) {
		t.Fatalf("independent targets missing: %s", text)
	}
}
