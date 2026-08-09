package ebpf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRuntimeIncludesTypedCgroupAndSharedFields(t *testing.T) {
	config := loadFixture(t, `EBPF_NETWORK=""
EBPF_UDP_TIMEOUT="5m"
EBPF_DNS_MODE="hijack"
EBPF_CGROUP_ENABLED=1
EBPF_CGROUP_IPV6_MODE="auto"
EBPF_BYPASS_PRIVATE_ADDRESS=0
APP_PROXY_ENABLE=1
APP_PROXY_MODE="blacklist"
APP_ANDROID_USERS="0 999"
BYPASS_APPS_LIST="com.android.chrome org.telegram.messenger"
EBPF_SHARED_NETWORK=1
EBPF_SHARED_INTERFACES="wlan2"
EBPF_SHARED_INCLUDE_SOURCE_CIDRS="192.168.43.0/24"
EBPF_SHARED_INCLUDE_MAC_ADDRESSES="02:11:22:33:44:55"
EBPF_SHARED_PROXY_MAP_CAPACITY=128
EBPF_SHARED_BYPASS_MAP_CAPACITY=256
EBPF_SHARED_FRAGMENT_MAP_CAPACITY=512
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
	if inbound["bypass_private_address"] != false {
		t.Fatalf("unexpected private address bypass: %#v", inbound["bypass_private_address"])
	}
	shared := inbound["shared_network"].(map[string]any)
	if shared["tc_priority"] != float64(1) {
		t.Fatalf("unexpected TC priority: %#v", shared["tc_priority"])
	}
	sharedCapacity := shared["map_capacity"].(map[string]any)
	if sharedCapacity["proxy"] != float64(128) ||
		sharedCapacity["bypass"] != float64(256) ||
		sharedCapacity["fragment"] != float64(512) {
		t.Fatalf("unexpected shared map capacity: %#v", sharedCapacity)
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

func TestBuildUsesOnlyActiveApplicationList(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		include     []string
		exclude     []string
		wantInclude int
		wantExclude int
	}{
		{
			name:        "blacklist",
			mode:        "blacklist",
			include:     []string{"com.proxy.app"},
			exclude:     []string{"com.bypass.app"},
			wantInclude: 0,
			wantExclude: 1,
		},
		{
			name:        "whitelist",
			mode:        "whitelist",
			include:     []string{"com.proxy.app"},
			exclude:     []string{"com.bypass.app"},
			wantInclude: 1,
			wantExclude: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := loadFixture(t, "APP_PROXY_ENABLE=1\nAPP_PROXY_MODE=\""+test.mode+"\"\nPROXY_APPS_LIST=\""+strings.Join(test.include, " ")+"\"\nBYPASS_APPS_LIST=\""+strings.Join(test.exclude, " ")+"\"\n")
			inbound := runtimeInbound(t, config)
			if got := len(inbound["include_package"].([]any)); got != test.wantInclude {
				t.Fatalf("unexpected include_package count: got %d, want %d", got, test.wantInclude)
			}
			if got := len(inbound["exclude_package"].([]any)); got != test.wantExclude {
				t.Fatalf("unexpected exclude_package count: got %d, want %d", got, test.wantExclude)
			}
		})
	}
}

func TestLoadRejectsUnknownAndInvalidConfiguration(t *testing.T) {
	if _, err := Load(writeFixture(t, "EBPF_NETWROK=tcp\n")); err == nil {
		t.Fatal("expected unknown key to fail")
	}
	if _, err := Load(writeFixture(t, "EBPF_IPV6_MODE=auto\n")); err == nil {
		t.Fatal("expected removed IPv6 key to fail")
	}
	if _, err := Load(writeFixture(t, "EBPF_UDP_TIMEOUT=0m\n")); err == nil {
		t.Fatal("expected zero timeout to fail")
	}
	if _, err := Load(writeFixture(t, "EBPF_SHARED_NETWORK=1\nEBPF_NETWORK=tcp\n")); err == nil {
		t.Fatal("expected shared DNS/tcp combination to fail")
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
