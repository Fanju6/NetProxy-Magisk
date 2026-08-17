package ebpf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildRuntimeUsesNewLocalAndSharedSchema(t *testing.T) {
	config := loadFixture(t, `EBPF_MODE="hybrid"
EBPF_NETWORK="tcp,udp"
EBPF_LOCAL_IPV6_MODE="auto"
EBPF_BYPASS_PRIVATE_ADDRESS=0
APP_PROXY_MODE="blacklist"
BYPASS_APPS_LIST="0:com.android.chrome,10:org.telegram.messenger"
EBPF_SHARED_INTERFACES="wlan2,wlan0"
EBPF_SHARED_INCLUDE_SOURCE_CIDR="192.168.43.0/24,fd00::/64"
EBPF_SHARED_INCLUDE_MAC_ADDRESS="02:11:22:33:44:55,AA:BB:CC:DD:EE:FF"
EBPF_SHARED_STATE_CAPACITY=512
`)
	inbound := runtimeInbound(t, config, func(refs []PackageRef) ([]uint32, error) {
		return []uint32{10123, 10124}, nil
	})
	if inbound["mode"] != "hybrid" || inbound["bypass_private_address"] != false {
		t.Fatalf("unexpected base inbound: %#v", inbound)
	}
	local := inbound["local"].(map[string]any)
	if local["ipv6_mode"] != "auto" || local["include_uid"] != nil {
		t.Fatalf("unexpected local fields: %#v", local)
	}
	if got := local["exclude_uid"].([]any); !reflect.DeepEqual(got, []any{float64(10123), float64(10124)}) {
		t.Fatalf("unexpected resolved app UIDs: %#v", got)
	}
	shared := inbound["shared"].(map[string]any)
	if got := len(shared["interface"].([]any)); got != 2 {
		t.Fatalf("unexpected shared interfaces: %d", got)
	}
	if shared["state_capacity"] != float64(512) {
		t.Fatalf("unexpected shared state capacity: %#v", shared["state_capacity"])
	}
	advanced := shared["advanced"].(map[string]any)
	if advanced["tc_priority"] != float64(1) {
		t.Fatalf("unexpected shared advanced fields: %#v", advanced)
	}
	for _, key := range []string{"cgroup_enabled", "cgroup_ipv6_mode", "shared_network", "redirect_address", "map_capacity"} {
		if _, ok := inbound[key]; ok {
			t.Fatalf("legacy eBPF field %q is still emitted: %#v", key, inbound)
		}
	}
}

func TestWhitelistAlwaysIncludesRootUID(t *testing.T) {
	config := loadFixture(t, `APP_PROXY_MODE="whitelist"
PROXY_APPS_LIST="10:com.example.app"
`)
	inbound := runtimeInbound(t, config, func([]PackageRef) ([]uint32, error) {
		return []uint32{10123}, nil
	})
	local := inbound["local"].(map[string]any)
	if got := local["include_uid"].([]any); !reflect.DeepEqual(got, []any{float64(0), float64(10123)}) {
		t.Fatalf("whitelist must include root UID: %#v", got)
	}
	if got, ok := local["include_package"]; ok && len(got.([]any)) != 0 {
		t.Fatalf("application policy should be resolved to UID: %#v", got)
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
		{value: "direct, ChinaIP", want: []string{"direct", "ChinaIP"}},
		{value: "direct ChinaIP", want: []string{"direct ChinaIP"}},
		{value: "wlan2，wlan0", want: []string{"wlan2", "wlan0"}},
		{value: "direct,, ChinaIP,", want: []string{"direct", "ChinaIP"}},
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
		"EBPF_CGROUP_ENABLED=1\n",
		"EBPF_SHARED_NETWORK=1\n",
		"APP_ANDROID_USERS=0\n",
		"PROXY_APPS_LIST=\"com.example.app\"\n",
	} {
		if _, err := Load(writeFixture(t, content)); err == nil {
			t.Fatalf("removed or unscoped configuration unexpectedly loaded: %q", content)
		}
	}
	if _, err := Load(writeFixture(t, "EBPF_MODE=shared\nEBPF_NETWORK=tcp\nEBPF_SHARED_INTERFACES=ap0\n")); err == nil {
		t.Fatal("shared DNS interception without UDP should fail")
	}
}

func runtimeInbound(t *testing.T, config Config, resolve PackageUIDResolver) map[string]any {
	t.Helper()
	if resolve == nil {
		resolve = func([]PackageRef) ([]uint32, error) { return []uint32{}, nil }
	}
	runtime, err := config.BuildWithResolver(resolve)
	if err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(runtime)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	return document["inbounds"].([]any)[0].(map[string]any)
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
