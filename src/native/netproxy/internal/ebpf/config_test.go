package ebpf

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	json "encoding/json/v2"

	"github.com/sagernet/sing-box/option"
)

func TestBuildRuntimeUsesNewLocalAndSharedSchema(t *testing.T) {
	config := loadFixture(t, `EBPF_MODE="hybrid"
EBPF_NETWORK="tcp,udp"
EBPF_LOCAL_DNS_MODE="respect_policy"
EBPF_LOCAL_IPV6=0
EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS=0
EBPF_SHARED_DNS_MODE="off"
EBPF_SHARED_IPV6=1
EBPF_SHARED_BYPASS_PRIVATE_ADDRESS=0
APP_PROXY_MODE="blacklist"
BYPASS_APPS_LIST="0:com.android.chrome,10:org.telegram.messenger"
EBPF_SHARED_INTERFACES="wlan2,wlan0"
EBPF_SHARED_INCLUDE_SOURCE_CIDR="192.168.43.0/24,fd00::/64"
EBPF_SHARED_INCLUDE_MAC_ADDRESS="02:11:22:33:44:55,AA:BB:CC:DD:EE:FF"
EBPF_SHARED_TC_PRIORITY=7
`)
	inbound := runtimeInbound(t, config, func(refs []PackageRef) (PackageUIDResolution, error) {
		return PackageUIDResolution{UIDs: []uint32{10123, 10124}}, nil
	})
	if inbound["mode"] != "hybrid" {
		t.Fatalf("unexpected base inbound: %#v", inbound)
	}
	if _, exists := inbound["bypass_private_address"]; exists {
		t.Fatalf("top-level bypass_private_address is no longer supported: %#v", inbound)
	}
	if _, exists := inbound["dns_mode"]; exists {
		t.Fatalf("top-level dns_mode is no longer supported: %#v", inbound)
	}
	local := inbound["local"].(map[string]any)
	assertMatchesSingBoxOptions[option.EBPFLocalOptions](t, local)
	if local["dns_mode"] != "respect_policy" || local["ipv6"] != false || local["bypass_private_address"] != false || local["include_uid"] != nil {
		t.Fatalf("unexpected local fields: %#v", local)
	}
	if got := local["exclude_uid"].([]any); !reflect.DeepEqual(got, []any{float64(10123), float64(10124)}) {
		t.Fatalf("unexpected resolved app UIDs: %#v", got)
	}
	shared := inbound["shared"].(map[string]any)
	assertMatchesSingBoxOptions[option.EBPFSharedOptions](t, shared)
	if shared["bypass_private_address"] != false {
		t.Fatalf("unexpected shared private address policy: %#v", shared)
	}
	if shared["dns_mode"] != "off" {
		t.Fatalf("unexpected shared DNS policy: %#v", shared)
	}
	if shared["ipv6"] != true {
		t.Fatalf("unexpected shared IPv6 policy: %#v", shared)
	}
	if got := len(shared["interface"].([]any)); got != 2 {
		t.Fatalf("unexpected shared interfaces: %d", got)
	}
	advanced := shared["advanced"].(map[string]any)
	if advanced["tc_priority"] != float64(7) {
		t.Fatalf("unexpected shared advanced fields: %#v", advanced)
	}
	for _, key := range []string{"tcp_splice", "cgroup_enabled", "cgroup_ipv6_mode", "shared_network", "redirect_address", "map_capacity"} {
		if _, ok := inbound[key]; ok {
			t.Fatalf("legacy eBPF field %q is still emitted: %#v", key, inbound)
		}
	}
}

func TestWhitelistAlwaysIncludesRootUID(t *testing.T) {
	config := loadFixture(t, `APP_PROXY_MODE="whitelist"
PROXY_APPS_LIST="10:com.example.app"
`)
	inbound := runtimeInbound(t, config, func([]PackageRef) (PackageUIDResolution, error) {
		return PackageUIDResolution{UIDs: []uint32{10123}}, nil
	})
	local := inbound["local"].(map[string]any)
	if got := local["include_uid"].([]any); !reflect.DeepEqual(got, []any{float64(0), float64(10123)}) {
		t.Fatalf("whitelist must include root UID: %#v", got)
	}
	if got, ok := local["include_package"]; ok && len(got.([]any)) != 0 {
		t.Fatalf("application policy should be resolved to UID: %#v", got)
	}
}

func TestMissingPackageRefsAreSkippedWithWarnings(t *testing.T) {
	config := loadFixture(t, `APP_PROXY_MODE="whitelist"
PROXY_APPS_LIST="0:com.example.installed,10:com.example.removed"
`)
	built, err := config.BuildWithResolver(func(refs []PackageRef) (PackageUIDResolution, error) {
		if len(refs) != 2 {
			t.Fatalf("unexpected package refs: %#v", refs)
		}
		return PackageUIDResolution{
			UIDs:    []uint32{10123},
			Missing: []PackageRef{refs[1]},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(built.MissingPackages, []PackageRef{{UserID: 10, Package: "com.example.removed"}}) {
		t.Fatalf("unexpected missing package refs: %#v", built.MissingPackages)
	}
	if got := built.Runtime.Inbounds[0].Local.IncludeUID; !reflect.DeepEqual(got, []uint32{0, 10123}) {
		t.Fatalf("missing package changed whitelist UIDs: %#v", got)
	}
}

func TestBlacklistMissingPackageIsSkipped(t *testing.T) {
	config := loadFixture(t, `APP_PROXY_MODE="blacklist"
BYPASS_APPS_LIST="0:com.example.removed"
`)
	built, err := config.BuildWithResolver(func(refs []PackageRef) (PackageUIDResolution, error) {
		return PackageUIDResolution{Missing: refs}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(built.MissingPackages, []PackageRef{{UserID: 0, Package: "com.example.removed"}}) {
		t.Fatalf("unexpected missing package refs: %#v", built.MissingPackages)
	}
	if len(built.Runtime.Inbounds[0].Local.ExcludeUID) != 0 {
		t.Fatalf("missing blacklist package produced an UID: %#v", built.Runtime.Inbounds[0].Local.ExcludeUID)
	}
}

func TestWhitelistAllMissingKeepsRootUID(t *testing.T) {
	config := loadFixture(t, `APP_PROXY_MODE="whitelist"
PROXY_APPS_LIST="0:com.example.removed,10:com.example.otherremoved"
`)
	built, err := config.BuildWithResolver(func(refs []PackageRef) (PackageUIDResolution, error) {
		return PackageUIDResolution{Missing: refs}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := built.Runtime.Inbounds[0].Local.IncludeUID; !reflect.DeepEqual(got, []uint32{0}) {
		t.Fatalf("whitelist without installed packages lost root UID: %#v", got)
	}
}

func TestSharedModeOmitsLocalFields(t *testing.T) {
	config := loadFixture(t, `EBPF_MODE="shared"
EBPF_SHARED_INTERFACES="ap0"
APP_PROXY_ENABLE=0
EBPF_LOCAL_CGROUP_PATH=not-used
EBPF_LOCAL_INCLUDE_UID=1234
`)
	inbound := runtimeInbound(t, config, nil)
	if _, ok := inbound["local"]; ok {
		t.Fatalf("shared mode emitted local fields: %#v", inbound)
	}
	if shared := inbound["shared"].(map[string]any); shared["interface"].([]any)[0] != "ap0" {
		t.Fatalf("unexpected shared mode: %#v", shared)
	}
}

func TestParsePackageRefsRequiresUserScope(t *testing.T) {
	refs, err := ParsePackageRefs("0:com.example.app,10:com.example.app", "PROXY_APPS_LIST")
	if err != nil || len(refs) != 2 || refs[1].String() != "10:com.example.app" {
		t.Fatalf("unexpected package refs: %#v, %v", refs, err)
	}
	if _, err := ParsePackageRefs("com.example.app", "PROXY_APPS_LIST"); err == nil {
		t.Fatal("package without Android user should fail")
	}
}

func TestCommaSeparatedValuesUseCommaAsTheOnlyListSeparator(t *testing.T) {
	tests := []struct {
		value string
		want  []string
	}{
		{value: "direct, cn-ip", want: []string{"direct", "cn-ip"}},
		{value: "direct cn-ip", want: []string{"direct cn-ip"}},
		{value: "wlan2，wlan0", want: []string{"wlan2", "wlan0"}},
		{value: "direct,, cn-ip,", want: []string{"direct", "cn-ip"}},
		{value: "  ", want: []string{}},
	}
	for _, test := range tests {
		if got := CommaSeparated(test.value); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("CommaSeparated(%q) = %#v, want %#v", test.value, got, test.want)
		}
	}
}

func TestLoadRejectsRemovedConfiguration(t *testing.T) {
	for _, content := range []string{
		"EBPF_DNS_MODE=hijack\n",
		"EBPF_CGROUP_ENABLED=1\n",
		"EBPF_SHARED_NETWORK=1\n",
		"EBPF_BYPASS_PRIVATE_ADDRESS=1\n",
		"APP_ANDROID_USERS=0\n",
		"PROXY_APPS_LIST=\"com.example.app\"\n",
		"EBPF_LOCAL_DNS_MODE=respect_bypass\n",
		"EBPF_SHARED_DNS_MODE=respect_bypass\n",
		"EBPF_TCP_SPLICE=1\n",
		"EBPF_LOCAL_IPV6_MODE=auto\n",
		"EBPF_LOCAL_STATE_CAPACITY=512\n",
		"EBPF_SHARED_IPV6_MODE=always\n",
		"EBPF_SHARED_STATE_CAPACITY=512\n",
		"EBPF_SHARED_DATA_PLANE=auto\n",
		"EBPF_SHARED_ROUTING_MARK=1\n",
		"EBPF_SHARED_ROUTING_TABLE=2026\n",
	} {
		if _, err := Load(writeFixture(t, content)); err == nil {
			t.Fatalf("removed or unscoped configuration unexpectedly loaded: %q", content)
		}
	}
	if _, err := Load(writeFixture(t, "EBPF_MODE=shared\nEBPF_NETWORK=tcp\nEBPF_SHARED_DNS_MODE=hijack\nEBPF_SHARED_INTERFACES=ap0\n")); err != nil {
		t.Fatalf("shared TCP-only DNS interception should be accepted: %v", err)
	}
}

func TestSharedAdvancedDefaultsMatchSingBox(t *testing.T) {
	config := loadFixture(t, `EBPF_MODE="shared"
EBPF_SHARED_INTERFACES="ap0"
APP_PROXY_ENABLE=0
`)
	inbound := runtimeInbound(t, config, nil)
	shared := inbound["shared"].(map[string]any)
	if shared["dns_mode"] != "hijack" {
		t.Fatalf("unexpected default shared DNS mode: %#v", shared)
	}
	advanced := shared["advanced"].(map[string]any)
	if advanced["tc_priority"] != float64(1) {
		t.Fatalf("unexpected shared advanced defaults: %#v", advanced)
	}
	if len(advanced) != 1 {
		t.Fatalf("removed shared advanced fields must not be emitted: %#v", advanced)
	}
}

func runtimeInbound(t *testing.T, config Config, resolve PackageUIDResolver) map[string]any {
	t.Helper()
	if resolve == nil {
		resolve = func([]PackageRef) (PackageUIDResolution, error) {
			return PackageUIDResolution{UIDs: []uint32{}}, nil
		}
	}
	built, err := config.BuildWithResolver(resolve)
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(built.Runtime, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	return document["inbounds"].([]any)[0].(map[string]any)
}

func assertMatchesSingBoxOptions[T any](t *testing.T, value any) {
	t.Helper()
	content, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	var options T
	if err := json.Unmarshal(content, &options, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf("生成字段不符合当前 sing-box eBPF 契约: %v", err)
	}
}

func loadFixture(t *testing.T, content string) Config {
	t.Helper()
	config, err := Load(writeFixture(t, content))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ebpf.conf")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
