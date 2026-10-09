package inbound

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/option"
)

const testEBPF = `{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true},"shared":{"enabled":false}}`
const testTUN = `{"type":"tun","tag":"netproxy-in","address":["172.19.0.1/30","fdfe:dcba:9876::1/126"],"auto_route":true,"auto_redirect":true}`

func fixture(t *testing.T, backend, ebpf, tun string) Config {
	t.Helper()
	if ebpf == "" {
		ebpf = testEBPF
	}
	if tun == "" {
		tun = testTUN
	}
	config, err := Parse([]byte(`{"backend":"` + backend + `","root_policy":"default","app":{"enabled":false,"mode":"blacklist","proxy_apps":[],"bypass_apps":[]},"ebpf":` + ebpf + `,"tun":` + tun + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func encodeConfig(t *testing.T, config Config) []byte {
	t.Helper()
	content, err := json.Marshal(config, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestLoadAndV2RoundTrip(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		t.Run(backend, func(t *testing.T) {
			config := fixture(t, backend, "", "")
			content := encodeConfig(t, config)
			path := filepath.Join(t.TempDir(), "inbound.json")
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(content, encodeConfig(t, loaded)) {
				t.Fatal("配置往返改变了模板")
			}
			if err := Validate(content, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestStrictOuterJSON(t *testing.T) {
	base := string(encodeConfig(t, fixture(t, "ebpf", "", "")))
	tests := []string{
		`null`, `[]`, ``, `EBPF_LOCAL_ENABLED=1`, base + ` {}`,
		strings.Replace(base, `"backend":"ebpf"`, `"backend":"ebpf","backend":"tun"`, 1),
		strings.Replace(base, `"backend":"ebpf"`, `"Backend":"ebpf"`, 1),
		strings.Replace(base, `"backend":"ebpf"`, `"backend":"legacy"`, 1),
		strings.Replace(base, `"backend":"ebpf"`, `"backend":null`, 1),
		strings.Replace(base, `"backend":"ebpf",`, ``, 1),
		strings.Replace(base, `"backend":"ebpf"`, `"backend":"ebpf","extra":1`, 1),
		strings.Replace(base, `"root_policy":"default"`, `"root_policy":"auto"`, 1),
		strings.Replace(base, `"root_policy":"default"`, `"root_policy":null`, 1),
		strings.Replace(base, `"root_policy":"default"`, `"root_policy":true`, 1),
		strings.Replace(base, `"app":{`, `"app":null,"removed":{`, 1),
		strings.Replace(base, `"enabled":false`, `"enabled":null`, 1),
		strings.Replace(base, `"enabled":false`, `"enabled":false,"enabled":true`, 1),
		strings.Replace(base, `"mode":"blacklist"`, `"mode":"other"`, 1),
		strings.Replace(base, `"proxy_apps":[]`, `"proxy_apps":null`, 1),
		strings.Replace(base, `"proxy_apps":[]`, `"proxy_apps":"0:com.example.app"`, 1),
		strings.Replace(base, `"proxy_apps":[]`, `"proxy_apps":["com.example.app"]`, 1),
		strings.Replace(base, `"proxy_apps":[]`, `"proxy_apps":[],"extra":1`, 1),
		strings.Replace(base, `"ebpf":`+testEBPF, `"ebpf":null`, 1),
		strings.Replace(base, `"tun":`+testTUN, `"tun":[]`, 1),
		strings.Replace(base, `"tun":`+testTUN, `"tun":null`, 1),
		strings.Replace(base, `"tun":`+testTUN, `"tun":{"unknown":"`+string([]byte{0xff})+`"}`, 1),
		strings.Replace(base, `"tun":`+testTUN, `"tun":{"unknown":1,"unknown":2}`, 1),
	}
	for index, content := range tests {
		if _, err := Parse([]byte(content)); err == nil {
			t.Fatalf("第 %d 个非法配置被接受: %s", index, content)
		}
	}
	for _, field := range []string{"backend", "root_policy", "app", "ebpf", "tun"} {
		var fields map[string]jsontext.Value
		if err := json.Unmarshal([]byte(base), &fields); err != nil {
			t.Fatal(err)
		}
		delete(fields, field)
		content, _ := json.Marshal(fields, json.Deterministic(true))
		if _, err := Parse(content); err == nil {
			t.Fatalf("接受缺少 %s 的配置", field)
		}
	}
}

func TestSelectedSectionIsolation(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, invalid := range []string{`"unknown":true`, `"address":false`, `"local":{"enabled":"bad"}`} {
			config := fixture(t, backend, "", "")
			unselected := "tun"
			if backend == "ebpf" {
				config.TUN = jsontext.Value(`{"type":"tun","tag":"netproxy-in",` + invalid + `}`)
			} else {
				config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in",` + invalid + `}`)
				unselected = "ebpf"
			}
			content := encodeConfig(t, config)
			parsed, err := Parse(content)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(content, backend); err != nil {
				t.Fatal(err)
			}
			if err := Validate(content, unselected); err == nil {
				t.Fatal("未校验指定的未选分区")
			}
			if err := Validate(content, ""); err == nil {
				t.Fatal("完整校验接受非法未选分区")
			}
			if _, err := parsed.BuildWithResolver(nil); err != nil {
				t.Fatal(err)
			}
			parsed.Backend = unselected
			if _, err := Parse(encodeConfig(t, parsed)); err == nil {
				t.Fatal("切换到非法分区未失败")
			}
		}
	}
	if err := Validate(encodeConfig(t, fixture(t, "ebpf", "", "")), "backend"); err == nil {
		t.Fatal("接受未知校验分区")
	}
}

func TestUnselectedSectionRequiresIdentity(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		unselected := "tun"
		if backend == "tun" {
			unselected = "ebpf"
		}
		for _, invalid := range []string{`{}`, `{"type":"legacy","tag":"netproxy-in"}`, `{"type":"` + unselected + `","tag":"wrong"}`, `{"type":null,"tag":"netproxy-in"}`, `{"type":"` + unselected + `","tag":null}`, `{"Type":"` + unselected + `","tag":"netproxy-in"}`} {
			config := fixture(t, backend, "", "")
			if backend == "ebpf" {
				config.TUN = jsontext.Value(invalid)
			} else {
				config.EBPF = jsontext.Value(invalid)
			}
			content := encodeConfig(t, config)
			if _, err := Parse(content); err == nil {
				t.Fatalf("未选分区身份错误未拒绝: %s", content)
			}
			if err := Validate(content, backend); err == nil {
				t.Fatal("分区校验未检查外层身份")
			}
			if _, err := config.BuildWithResolver(nil); err == nil {
				t.Fatal("Build 未检查外层身份")
			}
			if _, err := config.EffectiveContent(); err == nil {
				t.Fatal("EffectiveContent 未检查外层身份")
			}
		}
	}
}

func TestNativeStrictFieldsAndIdentity(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		raw := config.EBPF
		if backend == "tun" {
			raw = config.TUN
		}
		for _, invalid := range []string{
			strings.Replace(string(raw), `"type":"`+backend+`"`, `"type":"legacy"`, 1),
			strings.Replace(string(raw), `"tag":"netproxy-in"`, `"tag":"ebpf-in"`, 1),
			strings.Replace(string(raw), `"tag":"netproxy-in"`, `"tag":null`, 1),
			strings.Replace(string(raw), `"tag":"netproxy-in",`, ``, 1),
			strings.Replace(string(raw), `"type":"`+backend+`"`, `"type":"`+backend+`","extra":true`, 1),
			strings.Replace(string(raw), `"type":"`+backend+`"`, `"Type":"`+backend+`"`, 1),
		} {
			if backend == "ebpf" {
				config.EBPF = jsontext.Value(invalid)
			} else {
				config.TUN = jsontext.Value(invalid)
			}
			if _, err := Parse(encodeConfig(t, config)); err == nil {
				t.Fatalf("接受非法原生对象: %s", invalid)
			}
		}
	}
	for _, invalid := range []string{
		`{"type":"ebpf","tag":"netproxy-in","local":null}`,
		`{"type":"ebpf","tag":"netproxy-in","shared":null}`,
		`{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"unknown":1}}`,
		`{"type":"ebpf","tag":"netproxy-in","shared":{"enabled":false,"unknown":1}}`,
	} {
		config := fixture(t, "ebpf", "", "")
		config.EBPF = jsontext.Value(invalid)
		if _, err := Parse(encodeConfig(t, config)); err == nil {
			t.Fatalf("接受非法 eBPF 对象: %s", invalid)
		}
	}
	config := fixture(t, "tun", "", "")
	config.TUN = jsontext.Value(strings.TrimSuffix(testTUN, "}") + `,"platform":{"http_proxy":{"unknown":1}}}`)
	if _, err := Parse(encodeConfig(t, config)); err == nil {
		t.Fatal("接受未知嵌套原生字段")
	}
}

func TestNativeNetworkListAndListableRoundTrip(t *testing.T) {
	for _, network := range []string{`"tcp"`, `["tcp","udp"]`, `["udp","tcp","tcp"]`, `[]`} {
		config := fixture(t, "ebpf", `{"type":"ebpf","tag":"netproxy-in","network":`+network+`,"udp_timeout":"90s","udp_fragment":true,"tc_priority":7,"local":{"include_uid":123,"bypass_port":53,"bypass_exclude":"10.0.0.0/8","bypass_rule_set":"geoip/cn"}}`, "")
		native, err := config.EBPFOptions()
		if err != nil {
			t.Fatal(err)
		}
		if time.Duration(native.UDPTimeout) != 90*time.Second || native.TCPriority != 7 {
			t.Fatal(native)
		}
		built, err := config.BuildWithResolver(nil)
		if err != nil {
			t.Fatal(err)
		}
		encoded := built.Runtime.Inbounds[0]
		var output map[string]jsontext.Value
		if err := json.Unmarshal(encoded, &output); err != nil {
			t.Fatal(err)
		}
		if native.Network != "" && (len(output["network"]) == 0 || output["network"][0] != '[') {
			t.Fatalf("NetworkList 不是原生网络数组: %s", encoded)
		}
		var restored ebpfInbound
		if err := unmarshalNative(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		normalized, _ := normalizeEBPF(native)
		want, _ := marshalNative(ebpfInbound{Type: "ebpf", Tag: Tag, EBPFInboundOptions: normalized})
		got, _ := marshalNative(restored)
		if !bytes.Equal(got, want) {
			t.Fatalf("原生选项往返改变: %s %s", got, want)
		}
		again, err := marshalNative(restored)
		if err != nil || !bytes.Equal(again, encoded) {
			t.Fatalf("非确定性输出: %s %s %v", encoded, again, err)
		}
	}
	for _, value := range []string{`"icmp"`, `["tcp","bad"]`, `"tcp\nudp"`, `[1]`} {
		config := fixture(t, "ebpf", "", "")
		config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in","network":` + value + `}`)
		if _, err := config.EBPFOptions(); err == nil {
			t.Fatalf("接受非法 NetworkList: %s", value)
		}
	}
}

func TestTUNNativeFieldsRoundTrip(t *testing.T) {
	config := fixture(t, "tun", "", `{"type":"tun","tag":"netproxy-in","interface_name":"netproxy","address":"172.19.0.1/30","auto_route":true,"auto_redirect":true,"dns_mode":"hijack","dns_address":"172.19.0.2","mtu":1400,"udp_timeout":"2m","udp_mapping":"address_dependent","udp_filtering":"address_and_port_dependent","strict_route":true,"multi_queue":true,"auto_redirect_input_mark":"0x1234","include_interface":"wlan0","exclude_interface":[],"route_address":"10.0.0.0/8","route_address_set":"test-route","route_exclude_address":"192.168.0.0/16","route_exclude_address_set":["test-exclude"],"include_uid":123,"include_uid_range":"200:300","include_mac_address":"02:11:22:33:44:55","loopback_address":"10.0.0.1","platform":{"http_proxy":{"enabled":false,"server":"127.0.0.1","server_port":8080,"bypass_domain":"example.test"}}}`)
	native, err := config.TUNOptions()
	if err != nil {
		t.Fatal(err)
	}
	if native.AutoRedirectInputMark != option.FwMark(0x1234) || time.Duration(native.UDPTimeout) != 2*time.Minute {
		t.Fatal(native)
	}
	built, err := config.BuildWithResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	var restored tunInbound
	if err := unmarshalNative(built.Runtime.Inbounds[0], &restored); err != nil {
		t.Fatal(err)
	}
	normalized, _ := normalizeTUN(native)
	want, _ := marshalNative(tunInbound{Type: "tun", Tag: Tag, TunInboundOptions: normalized})
	got, _ := marshalNative(restored)
	if !bytes.Equal(got, want) {
		t.Fatalf("TUN 原生选项丢失: %s %s", got, want)
	}
	for _, key := range []string{`"stack"`, `"mode"`, `"inet4_address"`, `"local"`, `"shared"`} {
		if bytes.Contains(built.Runtime.Inbounds[0], []byte(key)) {
			t.Fatalf("生成了非原生 TUN 字段 %s", key)
		}
	}
}

func TestNativeDurationScalarAndArrayRoundTrips(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, duration := range []string{`90`, `"90s"`, `"1m30s"`, `"1500ms"`} {
			config := fixture(t, backend, "", "")
			if backend == "ebpf" {
				config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in","udp_timeout":` + duration + `,"local":{"include_android_user":10,"include_package":"com.example.native"}}`)
			} else {
				config.TUN = jsontext.Value(strings.TrimSuffix(testTUN, "}") + `,"udp_timeout":` + duration + `,"include_android_user":10,"include_package":"com.example.native"}`)
			}
			built, err := config.BuildWithResolver(nil)
			if err != nil {
				t.Fatal(err)
			}
			var first, second []byte
			if backend == "ebpf" {
				native, _ := config.EBPFOptions()
				var restored ebpfInbound
				if err := unmarshalNative(built.Runtime.Inbounds[0], &restored); err != nil {
					t.Fatal(err)
				}
				if restored.UDPTimeout != native.UDPTimeout || len(restored.Local.IncludeAndroidUser) != 1 || restored.Local.IncludeAndroidUser[0] != 10 {
					t.Fatal(restored)
				}
				first, _ = marshalNative(restored)
				second, _ = marshalNative(restored)
			} else {
				native, _ := config.TUNOptions()
				var restored tunInbound
				if err := unmarshalNative(built.Runtime.Inbounds[0], &restored); err != nil {
					t.Fatal(err)
				}
				if restored.UDPTimeout != native.UDPTimeout || len(restored.IncludeAndroidUser) != 1 || restored.IncludeAndroidUser[0] != 10 {
					t.Fatal(restored)
				}
				first, _ = marshalNative(restored)
				second, _ = marshalNative(restored)
			}
			if !bytes.Equal(first, second) || !bytes.Equal(first, built.Runtime.Inbounds[0]) {
				t.Fatal("自定义时长往返不稳定")
			}
		}
	}
}

func TestTUNRequiredConstraintsAndNoAliases(t *testing.T) {
	for _, field := range []string{`"auto_route":false`, `"auto_redirect":false`, `"address":[]`, `"address":null`, `"address":"bad"`, `"address":"0.0.0.0/0"`, `"dns_mode":"off"`, `"udp_timeout":"-1s"`, `"mode":"local"`, `"TUN_ADDRESS":"172.19.0.1/30"`, `"stack":"system"`, `"inet4_address":"172.19.0.1/30"`, `"gso":false`, `"sniff":false`, `"endpoint_independent_nat":false`} {
		config := fixture(t, "tun", "", "")
		var fields map[string]jsontext.Value
		if err := json.Unmarshal(config.TUN, &fields); err != nil {
			t.Fatal(err)
		}
		var extra map[string]jsontext.Value
		if err := json.Unmarshal([]byte("{"+field+"}"), &extra); err != nil {
			t.Fatal(err)
		}
		for key, value := range extra {
			fields[key] = value
		}
		config.TUN, _ = json.Marshal(fields, json.Deterministic(true))
		if _, err := Parse(encodeConfig(t, config)); err == nil {
			t.Fatalf("接受非法 TUN 字段: %s", field)
		}
	}
}

func TestNativeRuntimeDoesNotInjectOmittedDefaults(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		built, err := config.BuildWithResolver(nil)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]jsontext.Value
		if err := json.Unmarshal(built.Runtime.Inbounds[0], &fields); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"udp_timeout", "tc_priority", "auto_redirect_input_mark", "auto_redirect_output_mark", "auto_redirect_reset_mark", "auto_redirect_tproxy_mark", "auto_redirect_nfqueue", "iproute2_table_index", "iproute2_rule_index", "auto_redirect_iproute2_fallback_rule_index", "udp_mapping", "udp_filtering", "mtu", "stack"} {
			if _, exists := fields[field]; exists {
				t.Fatal("凭空输出原生默认字段", backend, field)
			}
		}
	}
}

func TestEBPFEnablementAndDisabledPathProjection(t *testing.T) {
	for _, raw := range []string{
		`{"type":"ebpf","tag":"netproxy-in"}`,
		`{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true},"shared":{"enabled":false,"interface":"wlan2","data_plane":"socket_assign","ipv6":true}}`,
		`{"type":"ebpf","tag":"netproxy-in","local":{"enabled":false,"data_plane":"cgroup","dns_mode":"off","include_uid":123},"shared":{"enabled":true,"interface":"wlan2"}}`,
		`{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true},"shared":{"enabled":true,"interface":"wlan2"}}`,
	} {
		config := fixture(t, "ebpf", raw, "")
		native, _ := config.EBPFOptions()
		local, shared := native.EffectiveEnablement()
		built, err := config.BuildWithResolver(nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(built.Runtime.Inbounds) != 1 || built.Backend != "ebpf" {
			t.Fatal(built)
		}
		var output map[string]jsontext.Value
		if err := json.Unmarshal(built.Runtime.Inbounds[0], &output); err != nil {
			t.Fatal(err)
		}
		for _, path := range []struct {
			name    string
			enabled bool
		}{{"local", local}, {"shared", shared}} {
			if !path.enabled && string(output[path.name]) != `{"enabled":false}` {
				t.Fatalf("禁用路径泄露字段: %s", output[path.name])
			}
		}
		if string(output["tag"]) != `"netproxy-in"` || string(output["type"]) != `"ebpf"` {
			t.Fatal(output)
		}
	}
	for _, invalid := range []string{
		`{"local":{"enabled":false},"shared":{"enabled":false}}`,
		`{"local":{"enabled":true,"data_plane":"legacy"}}`,
		`{"local":{"data_plane":"tc","cgroup_path":"/sys/fs/cgroup"}}`,
		`{"local":{"cgroup_path":"relative"}}`,
		`{"local":{"dns_mode":"bad"}}`,
		`{"shared":{"enabled":true}}`,
		`{"shared":{"enabled":true,"interface":""}}`,
		`{"shared":{"enabled":true,"interface":"wlan2","data_plane":"tc"}}`,
		`{"shared":{"enabled":true,"interface":"wlan2","dns_mode":"bad"}}`,
		`{"shared":{"enabled":true,"interface":"wlan2","include_mac_address":"bad"}}`,
		`{"local":{"bypass_port":0}}`,
		`{"local":{"bypass_port":65536}}`,
		`{"shared":{"enabled":false,"bypass_port_range":"8000"}}`,
		`{"shared":{"enabled":false,"bypass_port_range":"9000:8000"}}`,
		`{"shared":{"enabled":false,"bypass_port_range":"0:8000"}}`,
		`{"local":{"include_uid_range":"1:4294967296"}}`,
		`{"shared":{"enabled":false,"data_plane":"legacy"}}`,
		`{"udp_timeout":"-1s"}`,
		`{"fakeip_icmp":"legacy"}`,
		`{"tc_priority":65536}`,
	} {
		config := fixture(t, "ebpf", "", "")
		config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in",` + strings.TrimPrefix(invalid, "{"))
		if _, err := config.EBPFOptions(); err == nil {
			t.Fatalf("接受非法 eBPF 配置: %s", invalid)
		}
	}
}
