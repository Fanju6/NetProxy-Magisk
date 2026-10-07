package inbound

import (
	"bytes"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAnonymousNativeFixtures(t *testing.T) {
	for _, name := range []string{"ebpf-local", "ebpf-shared", "ebpf-both", "tun"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name+".json")
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(content, ""); err != nil {
				t.Fatal(err)
			}
			config, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			original := encodeConfig(t, config)
			built, err := config.BuildWithResolver(func([]PackageRef) (PackageUIDResolution, error) {
				t.Fatal("fixture 不应查询设备")
				return PackageUIDResolution{}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(built.Runtime.Inbounds) != 1 || built.Backend != config.Backend {
				t.Fatal(built)
			}
			actual, err := json.Marshal(built.Runtime, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join("testdata", name+".golden.json"))
			if err != nil {
				t.Fatal(err)
			}
			canonical := func(content []byte) []byte {
				var value any
				if err := json.Unmarshal(content, &value); err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(value, json.Deterministic(true))
				if err != nil {
					t.Fatal(err)
				}
				return encoded
			}
			if !bytes.Equal(canonical(actual), canonical(expected)) {
				t.Fatalf("fixture 生成物变化:\n实际 %s\n期望 %s", actual, expected)
			}
			if !bytes.Equal(original, encodeConfig(t, config)) {
				t.Fatal("fixture 模板被生成流程修改")
			}
			again, err := config.BuildWithResolver(nil)
			if err != nil {
				t.Fatal(err)
			}
			second, _ := json.Marshal(again.Runtime, json.Deterministic(true))
			if !bytes.Equal(actual, second) {
				t.Fatal("运行时生成不确定")
			}
		})
	}
}

func TestDefaultEBPFFixtureMatchesPreviousAnonymousSemantics(t *testing.T) {
	config, err := Load(filepath.Join("testdata", "ebpf-local.json"))
	if err != nil {
		t.Fatal(err)
	}
	native, err := config.EBPFOptions()
	if err != nil {
		t.Fatal(err)
	}
	local, shared := native.EffectiveEnablement()
	if !local || shared || native.Local.DataPlane != "cgroup" || native.Local.DNSMode != "respect_policy" || !enabledByDefault(native.Local.IPv6) || !enabledByDefault(native.Local.BypassPrivateAddress) {
		t.Fatal(native)
	}
	if time.Duration(native.UDPTimeout) != 5*time.Minute || native.TCPriority != 1 || !reflect.DeepEqual(native.Network.Build(), []string{"tcp", "udp"}) || !reflect.DeepEqual([]string(native.Local.BypassRuleSet), []string{"geoip/cn"}) {
		t.Fatal(native)
	}
	if !config.App.Enabled || config.App.Mode != "blacklist" || len(config.App.ProxyApps)+len(config.App.BypassApps) != 0 {
		t.Fatal(config.App)
	}
	if native.Shared.DataPlane != "packet_rewrite" || native.Shared.DNSMode != "hijack" || !reflect.DeepEqual([]string(native.Shared.Interface), []string{"wlan2"}) {
		t.Fatal(native.Shared)
	}
	built, err := config.BuildWithResolver(func([]PackageRef) (PackageUIDResolution, error) {
		t.Fatal("空黑名单不应查询 UID")
		return PackageUIDResolution{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(built.Runtime.Inbounds[0], &output); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(output["shared"], map[string]any{"enabled": false}) || output["tag"] != Tag {
		t.Fatal(output)
	}
}
