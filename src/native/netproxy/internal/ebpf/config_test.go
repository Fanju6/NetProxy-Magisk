package ebpf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRuntimeIncludesTypedCgroupAndSharedFields(t *testing.T) {
	config := loadFixture(t, `EBPF_NETWORK=""
EBPF_UDP_TIMEOUT="5m"
EBPF_DNS_MODE="hijack"
EBPF_CGROUP_ENABLED=1
EBPF_IPV6_MODE="auto"
APP_PROXY_ENABLE=1
APP_PROXY_MODE="blacklist"
APP_ANDROID_USERS="0 999"
BYPASS_APPS_LIST="com.android.chrome org.telegram.messenger"
EBPF_SHARED_NETWORK=1
EBPF_SHARED_INTERFACES="wlan2"
EBPF_SHARED_INCLUDE_SOURCE_CIDRS="192.168.43.0/24"
EBPF_SHARED_INCLUDE_MAC_ADDRESSES="02:11:22:33:44:55"
`)

	inbound := runtimeInbound(t, config)
	for _, key := range []string{"cgroup_enabled", "cgroup_ipv6_mode", "include_android_user", "exclude_package", "map_capacity", "shared_network"} {
		if _, ok := inbound[key]; !ok {
			t.Fatalf("runtime inbound does not contain %q: %#v", key, inbound)
		}
	}
	if inbound["cgroup_ipv6_mode"] != "auto" {
		t.Fatalf("unexpected IPv6 mode: %#v", inbound["cgroup_ipv6_mode"])
	}
	shared := inbound["shared_network"].(map[string]any)
	if shared["tc_priority"] != float64(1) {
		t.Fatalf("unexpected TC priority: %#v", shared["tc_priority"])
	}
	if len(inbound["include_android_user"].([]any)) != 2 {
		t.Fatalf("unexpected Android users: %#v", inbound["include_android_user"])
	}
}

func TestCgroupDisabledOmitsLocalFields(t *testing.T) {
	config := loadFixture(t, `EBPF_CGROUP_ENABLED=0
EBPF_SHARED_NETWORK=1
EBPF_SHARED_INTERFACES="wlan2"
EBPF_TCP_MAP_CAPACITY=not-used
EBPF_UDP_MAP_CAPACITY=not-used
EBPF_SOCKET_MAP_CAPACITY=not-used
`)
	inbound := runtimeInbound(t, config)
	for _, key := range []string{"cgroup_path", "cgroup_ipv6_mode", "include_uid", "include_android_user", "include_package", "exclude_package", "map_capacity"} {
		if _, ok := inbound[key]; ok {
			t.Fatalf("local cgroup field %q must be omitted: %#v", key, inbound)
		}
	}
	if inbound["cgroup_enabled"] != false {
		t.Fatalf("unexpected cgroup_enabled: %#v", inbound["cgroup_enabled"])
	}
	if _, ok := inbound["shared_network"]; !ok {
		t.Fatal("shared network configuration was omitted")
	}
}

func TestWhitelistWithoutPackagesUsesSentinel(t *testing.T) {
	config := loadFixture(t, `APP_PROXY_MODE="whitelist"
PROXY_APPS_LIST=""
`)
	inbound := runtimeInbound(t, config)
	users := inbound["include_uid"].([]any)
	if len(users) != 1 || users[0] != float64(4294967295) {
		t.Fatalf("unexpected whitelist sentinel: %#v", users)
	}
	if packages := inbound["include_package"].([]any); len(packages) != 0 {
		t.Fatalf("unexpected package list: %#v", packages)
	}
}

func TestLoadRejectsUnknownAndInvalidConfiguration(t *testing.T) {
	if _, err := Load(writeFixture(t, "EBPF_NETWROK=tcp\n")); err == nil {
		t.Fatal("expected unknown key to fail")
	}
	if _, err := Load(writeFixture(t, "EBPF_UDP_TIMEOUT=0m\n")); err == nil {
		t.Fatal("expected zero timeout to fail")
	}
	if _, err := Load(writeFixture(t, "EBPF_SHARED_NETWORK=1\nEBPF_NETWORK=tcp\n")); err == nil {
		t.Fatal("expected shared DNS/tcp combination to fail")
	}
}

func TestDiagnoseReturnsChineseStructuredReport(t *testing.T) {
	report := Diagnose(writeFixture(t, "EBPF_SHARED_INCLUDE_MAC_ADDRESSES=02:11:22:33:44:5G\n"))
	if report.Valid || len(report.Diagnostics) == 0 || report.Diagnostics[0].Level != "error" {
		t.Fatalf("unexpected diagnostic report: %#v", report)
	}
}

func runtimeInbound(t *testing.T, config Config) map[string]any {
	t.Helper()
	content, err := json.Marshal(config.Build())
	if err != nil {
		t.Fatal(err)
	}
	var runtime map[string]any
	if err := json.Unmarshal(content, &runtime); err != nil {
		t.Fatal(err)
	}
	inbounds := runtime["inbounds"].([]any)
	return inbounds[0].(map[string]any)
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
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
