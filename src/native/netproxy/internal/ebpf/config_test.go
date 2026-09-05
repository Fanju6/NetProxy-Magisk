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

func TestDefaultBypassRuleSet(t *testing.T) {
	config := loadFixture(t, "")
	if !reflect.DeepEqual(config.BypassRuleSets, []string{"geoip/cn"}) {
		t.Fatalf("默认绕过规则必须引用已声明的 GeoIP 标签: %v", config.BypassRuleSets)
	}
}

func TestDefaultRuntimeUsesLocalCgroupOnly(t *testing.T) {
	config := loadFixture(t, "")
	inbound := runtimeInbound(t, config, nil)
	local := inbound["local"].(map[string]any)
	assertMatchesSingBoxOptions[option.EBPFLocalOptions](t, local)
	if local["enabled"] != true || local["data_plane"] != "cgroup" {
		t.Fatalf("unexpected default local data path: %#v", local)
	}
	shared := inbound["shared"].(map[string]any)
	assertMatchesSingBoxOptions[option.EBPFSharedOptions](t, shared)
	if !reflect.DeepEqual(shared, map[string]any{"enabled": false}) {
		t.Fatalf("unexpected default shared data path: %#v", shared)
	}
}

func TestBuildRuntimeUsesEnabledLocalAndSharedDataPlanes(t *testing.T) {
	config := loadFixture(t, `EBPF_LOCAL_ENABLED=1
EBPF_LOCAL_DATA_PLANE="cgroup"
EBPF_LOCAL_CGROUP_PATH="/sys/fs/cgroup"
EBPF_SHARED_ENABLED=1
EBPF_SHARED_DATA_PLANE="socket_assign"
EBPF_NETWORK="tcp,udp"
EBPF_TC_PRIORITY=7
EBPF_LOCAL_DNS_MODE="respect_policy"
EBPF_LOCAL_IPV6=0
EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS=0
EBPF_LOCAL_BYPASS_PORT="53,853"
EBPF_LOCAL_BYPASS_PORT_RANGE="8000:8080"
EBPF_SHARED_DNS_MODE="off"
EBPF_SHARED_IPV6=1
EBPF_SHARED_BYPASS_PRIVATE_ADDRESS=0
EBPF_SHARED_BYPASS_PORT="67,68"
EBPF_SHARED_BYPASS_PORT_RANGE="10000:10100"
APP_PROXY_MODE="blacklist"
BYPASS_APPS_LIST="0:com.android.chrome,10:org.telegram.messenger"
EBPF_SHARED_INTERFACES="wlan2,wlan0"
EBPF_SHARED_INCLUDE_SOURCE_CIDR="192.168.43.0/24,fd00::/64"
EBPF_SHARED_INCLUDE_MAC_ADDRESS="02:11:22:33:44:55,AA:BB:CC:DD:EE:FF"
`)
	inbound := runtimeInbound(t, config, func(refs []PackageRef) (PackageUIDResolution, error) {
		return PackageUIDResolution{UIDs: []uint32{10123, 10124}}, nil
	})
	if _, exists := inbound["mode"]; exists {
		t.Fatalf("removed mode field is still emitted: %#v", inbound)
	}
	if _, exists := inbound["bypass_private_address"]; exists {
		t.Fatalf("top-level bypass_private_address is no longer supported: %#v", inbound)
	}
	if _, exists := inbound["dns_mode"]; exists {
		t.Fatalf("top-level dns_mode is no longer supported: %#v", inbound)
	}
	if inbound["tc_priority"] != float64(7) {
		t.Fatalf("unexpected top-level TC priority: %#v", inbound)
	}
	local := inbound["local"].(map[string]any)
	assertMatchesSingBoxOptions[option.EBPFLocalOptions](t, local)
	if local["enabled"] != true || local["data_plane"] != "cgroup" || local["cgroup_path"] != "/sys/fs/cgroup" {
		t.Fatalf("unexpected local data plane: %#v", local)
	}
	if local["dns_mode"] != "respect_policy" || local["ipv6"] != false || local["bypass_private_address"] != false || local["include_uid"] != nil {
		t.Fatalf("unexpected local fields: %#v", local)
	}
	if got := local["exclude_uid"].([]any); !reflect.DeepEqual(got, []any{float64(10123), float64(10124)}) {
		t.Fatalf("unexpected resolved app UIDs: %#v", got)
	}
	if got := local["bypass_port"].([]any); !reflect.DeepEqual(got, []any{float64(53), float64(853)}) {
		t.Fatalf("unexpected local bypass ports: %#v", got)
	}
	if got := local["bypass_port_range"].([]any); !reflect.DeepEqual(got, []any{"8000:8080"}) {
		t.Fatalf("unexpected local bypass port ranges: %#v", got)
	}
	shared := inbound["shared"].(map[string]any)
	assertMatchesSingBoxOptions[option.EBPFSharedOptions](t, shared)
	if shared["enabled"] != true || shared["data_plane"] != "socket_assign" {
		t.Fatalf("unexpected shared data plane: %#v", shared)
	}
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
	if _, exists := shared["advanced"]; exists {
		t.Fatalf("removed shared advanced object is still emitted: %#v", shared)
	}
	if got := shared["bypass_port"].([]any); !reflect.DeepEqual(got, []any{float64(67), float64(68)}) {
		t.Fatalf("unexpected shared bypass ports: %#v", got)
	}
	if got := shared["bypass_port_range"].([]any); !reflect.DeepEqual(got, []any{"10000:10100"}) {
		t.Fatalf("unexpected shared bypass port ranges: %#v", got)
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

func TestDisabledLocalPathOnlyEmitsEnablement(t *testing.T) {
	config := loadFixture(t, `EBPF_LOCAL_ENABLED=0
EBPF_SHARED_ENABLED=1
EBPF_SHARED_INTERFACES="ap0"
APP_PROXY_ENABLE=0
EBPF_LOCAL_INCLUDE_UID=1234
`)
	inbound := runtimeInbound(t, config, nil)
	local := inbound["local"].(map[string]any)
	assertMatchesSingBoxOptions[option.EBPFLocalOptions](t, local)
	if !reflect.DeepEqual(local, map[string]any{"enabled": false}) {
		t.Fatalf("disabled local path emitted settings: %#v", local)
	}
	if shared := inbound["shared"].(map[string]any); shared["interface"].([]any)[0] != "ap0" {
		t.Fatalf("unexpected shared data path: %#v", shared)
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
		"EBPF_MODE=local\n",
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
		"EBPF_SHARED_ROUTING_MARK=1\n",
		"EBPF_SHARED_ROUTING_TABLE=2026\n",
		"EBPF_SHARED_TC_PRIORITY=1\n",
	} {
		if _, err := Load(writeFixture(t, content)); err == nil {
			t.Fatalf("removed or unscoped configuration unexpectedly loaded: %q", content)
		}
	}
	if _, err := Load(writeFixture(t, "EBPF_LOCAL_ENABLED=0\nEBPF_SHARED_ENABLED=1\nEBPF_NETWORK=tcp\nEBPF_SHARED_DNS_MODE=hijack\nEBPF_SHARED_INTERFACES=ap0\n")); err != nil {
		t.Fatalf("shared TCP-only DNS interception should be accepted: %v", err)
	}
}

func TestTopLevelPriorityDefaultMatchesSingBox(t *testing.T) {
	config := loadFixture(t, `EBPF_LOCAL_ENABLED=0
EBPF_SHARED_ENABLED=1
EBPF_SHARED_INTERFACES="ap0"
APP_PROXY_ENABLE=0
`)
	inbound := runtimeInbound(t, config, nil)
	if inbound["tc_priority"] != float64(1) {
		t.Fatalf("unexpected top-level TC priority: %#v", inbound)
	}
	shared := inbound["shared"].(map[string]any)
	if shared["dns_mode"] != "hijack" {
		t.Fatalf("unexpected default shared DNS mode: %#v", shared)
	}
	if _, exists := shared["advanced"]; exists {
		t.Fatalf("removed shared advanced object is still emitted: %#v", shared)
	}
}

func TestLoadRejectsInvalidEnablementAndDataPlanes(t *testing.T) {
	for _, content := range []string{
		"EBPF_LOCAL_ENABLED=0\nEBPF_SHARED_ENABLED=0\n",
		"EBPF_LOCAL_DATA_PLANE=auto\n",
		"EBPF_LOCAL_DATA_PLANE=tc\nEBPF_LOCAL_CGROUP_PATH=/sys/fs/cgroup\n",
		"EBPF_LOCAL_CGROUP_PATH=relative\n",
		"EBPF_SHARED_ENABLED=1\nEBPF_SHARED_DATA_PLANE=auto\n",
		"EBPF_SHARED_ENABLED=1\nEBPF_SHARED_INTERFACES=\n",
	} {
		if _, err := Load(writeFixture(t, content)); err == nil {
			t.Fatalf("invalid enablement or data plane unexpectedly loaded: %q", content)
		}
	}
}

func TestLoadRejectsInvalidBypassPorts(t *testing.T) {
	for _, content := range []string{
		"EBPF_LOCAL_BYPASS_PORT=0\n",
		"EBPF_LOCAL_BYPASS_PORT=65536\n",
		"EBPF_SHARED_BYPASS_PORT_RANGE=8000\n",
		"EBPF_SHARED_BYPASS_PORT_RANGE=9000:8000\n",
	} {
		if _, err := Load(writeFixture(t, content)); err == nil {
			t.Fatalf("invalid bypass port configuration unexpectedly loaded: %q", content)
		}
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
