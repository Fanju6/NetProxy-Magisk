package inbound

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/ranges"
)

func builtFilters(t *testing.T, built BuildResult) (include []uint32, includeRange []string, exclude []uint32, excludeRange []string) {
	t.Helper()
	if len(built.Runtime.Inbounds) != 1 {
		t.Fatal(built)
	}
	if built.Backend == "ebpf" {
		var native ebpfInbound
		if err := unmarshalNative(built.Runtime.Inbounds[0], &native); err != nil {
			t.Fatal(err)
		}
		return native.Local.IncludeUID, native.Local.IncludeUIDRange, native.Local.ExcludeUID, native.Local.ExcludeUIDRange
	}
	var native tunInbound
	if err := unmarshalNative(built.Runtime.Inbounds[0], &native); err != nil {
		t.Fatal(err)
	}
	return native.IncludeUID, native.IncludeUIDRange, native.ExcludeUID, native.ExcludeUIDRange
}

func TestDNSBypassDoesNotChangeSavedPreferences(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		original := fixture(t, backend, `{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"dns_mode":"respect_policy"},"shared":{"enabled":false,"dns_mode":"hijack"}}`, strings.TrimSuffix(testTUN, "}")+`,"dns_mode":"native"}`)
		saved := encodeConfig(t, original)
		effective, err := original.WithDNSBypass(true)
		if err != nil {
			t.Fatal(err)
		}
		built, err := effective.Build(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		content, _ := json.Marshal(built.Runtime)
		state, err := RuntimeDNSState(content)
		wanted, _ := effective.DNSState()
		if err != nil || state != wanted {
			t.Fatalf("%s: %s %s %v", backend, state, wanted, err)
		}
		if backend == "ebpf" && (strings.Contains(string(content), `"dns_mode":"respect_policy"`) || strings.Contains(string(content), `"dns_mode":"hijack"`) || !strings.Contains(string(content), `"shared":{"enabled":false}`)) {
			t.Fatal(string(content))
		}
		if backend == "tun" && !strings.Contains(string(content), `"dns_mode":"disabled"`) {
			t.Fatal(string(content))
		}
		if !bytes.Equal(saved, encodeConfig(t, original)) {
			t.Fatal("改变了保存偏好")
		}
		restored, _ := original.WithDNSBypass(false)
		if !bytes.Equal(saved, encodeConfig(t, restored)) {
			t.Fatal("未恢复原 DNS 配置")
		}
	}
	for _, bad := range []string{`{}`, `{"inbounds":[]}`, `{"inbounds":[{},{}]}`, `{"inbounds":[{"type":"other"}]}`} {
		if _, err := RuntimeDNSState([]byte(bad)); err == nil {
			t.Fatalf("接受了无效运行时: %s", bad)
		}
	}
}

func addFilters(t *testing.T, config *Config, filters string) {
	t.Helper()
	if config.Backend == "ebpf" {
		config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in","local":{` + filters + `}}`)
	} else {
		config.TUN = jsontext.Value(strings.TrimSuffix(testTUN, "}") + `,` + filters + `}`)
	}
}

func TestBuildAppPolicyBothBackends(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, mode := range []string{"whitelist", "blacklist"} {
			t.Run(backend+"/"+mode, func(t *testing.T) {
				config := fixture(t, backend, "", "")
				config.App = AppPolicy{Enabled: true, Mode: mode, ProxyApps: []string{"0:com.example.app", "10:com.example.app", "0:com.example.app", "10:com.example.missing"}, BypassApps: []string{"0:com.example.app", "10:com.example.app", "0:com.example.app", "10:com.example.missing"}}
				field := "include"
				if mode == "blacklist" {
					field = "exclude"
				}
				addFilters(t, &config, `"`+field+`_uid":[10123,10123,10130],"`+field+`_uid_range":["10120:10125","10124:10129"]`)
				original := encodeConfig(t, config)
				calls := 0
				built, err := config.BuildWithResolver(func(refs []PackageRef) (PackageUIDResolution, error) {
					calls++
					want := []PackageRef{{0, "com.example.app"}, {10, "com.example.app"}, {10, "com.example.missing"}}
					if !reflect.DeepEqual(refs, want) {
						t.Fatalf("引用未按用户保留并去重: %#v", refs)
					}
					return PackageUIDResolution{UIDs: []uint32{10123, 10123, 10126, 10130, 1010123}, Missing: refs[2:]}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if calls != 1 || !reflect.DeepEqual(built.MissingPackages, []PackageRef{{10, "com.example.missing"}}) {
					t.Fatal(built)
				}
				include, includeRange, exclude, excludeRange := builtFilters(t, built)
				if mode == "whitelist" {
					if !reflect.DeepEqual(include, []uint32{1010123}) || !reflect.DeepEqual(includeRange, []string{"10120:10130"}) || len(exclude)+len(excludeRange) != 0 {
						t.Fatalf("白名单合并错误: %v %v %v %v", include, includeRange, exclude, excludeRange)
					}
				} else if !reflect.DeepEqual(exclude, []uint32{1010123}) || !reflect.DeepEqual(excludeRange, []string{"10120:10130"}) || len(include)+len(includeRange) != 0 {
					t.Fatalf("黑名单合并错误: %v %v %v %v", include, includeRange, exclude, excludeRange)
				}
				if !bytes.Equal(original, encodeConfig(t, config)) {
					t.Fatal("运行时 UID 写回持久模板")
				}
			})
		}
	}
}

func TestWhitelistMissingAndEmptyBypassAllLocalUIDs(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, values := range [][]string{nil, {"10:com.example.missing"}} {
			config := fixture(t, backend, "", "")
			config.App = AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: values}
			original := encodeConfig(t, config)
			built, err := config.BuildWithResolver(func(refs []PackageRef) (PackageUIDResolution, error) { return PackageUIDResolution{Missing: refs}, nil })
			if err != nil {
				t.Fatal(err)
			}
			include, ranges, exclude, excludeRanges := builtFilters(t, built)
			if len(include)+len(ranges)+len(exclude) != 0 || !reflect.DeepEqual(excludeRanges, []string{"0:4294967294"}) || len(built.MissingPackages) != len(values) {
				t.Fatal(built)
			}
			if !bytes.Equal(original, encodeConfig(t, config)) {
				t.Fatal("空白名单投影写回持久模板")
			}
		}
	}
}

func TestAppPolicyPreservesNativeUIDFilters(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, test := range []struct {
			name, mode, filters        string
			apps                       []string
			include, exclude           []uint32
			includeRange, excludeRange []string
		}{
			{name: "exclude-root", mode: "whitelist", filters: `"exclude_uid":0`, apps: []string{"0:com.example.app"}, include: []uint32{10123}, exclude: []uint32{0}},
			{name: "explicit-root", mode: "whitelist", filters: `"include_uid":0`, apps: []string{"0:com.example.app"}, include: []uint32{0, 10123}},
			{name: "exclude-included-app", mode: "whitelist", filters: `"exclude_uid":10123`, apps: []string{"0:com.example.app"}, include: []uint32{10123}, exclude: []uint32{10123}},
			{name: "native-whitelist", mode: "whitelist", filters: `"include_uid":0`, include: []uint32{0}},
			{name: "native-whitelist-range", mode: "whitelist", filters: `"include_uid_range":"10000:10010","exclude_uid":0`, includeRange: []string{"10000:10010"}, exclude: []uint32{0}},
			{name: "whitelist-with-exclude-range", mode: "whitelist", filters: `"exclude_uid_range":"0:9999"`, apps: []string{"0:com.example.app"}, include: []uint32{10123}, excludeRange: []string{"0:9999"}},
			{name: "blacklist-with-native-whitelist", mode: "blacklist", filters: `"include_uid":10123`, apps: []string{"0:com.example.app"}, include: []uint32{10123}, exclude: []uint32{10123}},
			{name: "blacklist-with-native-range", mode: "blacklist", filters: `"include_uid_range":"10000:10130","exclude_uid":0`, apps: []string{"0:com.example.app"}, includeRange: []string{"10000:10130"}, exclude: []uint32{0, 10123}},
		} {
			t.Run(backend+"/"+test.name, func(t *testing.T) {
				config := fixture(t, backend, "", "")
				config.App = AppPolicy{Enabled: true, Mode: test.mode, ProxyApps: test.apps, BypassApps: test.apps}
				addFilters(t, &config, test.filters)
				original := encodeConfig(t, config)
				if _, err := Parse(original); err != nil {
					t.Fatal(err)
				}
				if err := Validate(original, ""); err != nil {
					t.Fatal(err)
				}
				if _, err := config.EffectiveContent(); err != nil {
					t.Fatal(err)
				}
				built, err := config.BuildWithResolver(func(refs []PackageRef) (PackageUIDResolution, error) {
					if len(test.apps) == 0 {
						t.Fatal("空应用名单仍查询 UID")
					}
					return PackageUIDResolution{UIDs: []uint32{10123}}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				include, includeRange, exclude, excludeRange := builtFilters(t, built)
				if !slices.Equal(include, test.include) || !slices.Equal(includeRange, test.includeRange) || !slices.Equal(exclude, test.exclude) || !slices.Equal(excludeRange, test.excludeRange) {
					t.Fatalf("原生 UID 筛选变化: %v %v %v %v", include, includeRange, exclude, excludeRange)
				}
				if !bytes.Equal(original, encodeConfig(t, config)) {
					t.Fatal("UID 投影修改持久模板")
				}
			})
		}
	}
}

func TestNativeUIDPolicyExcludesRootAndEmptyWhitelist(t *testing.T) {
	options := tun.Options{
		IncludeUID: []ranges.Range[uint32]{{Start: 0, End: 0}, {Start: 10123, End: 10123}},
		ExcludeUID: []ranges.Range[uint32]{{Start: 0, End: 0}},
	}
	if got, want := options.ExcludedRanges(), ([]ranges.Range[uint32]{{Start: 0, End: 10122}, {Start: 10124, End: 4294967294}}); !slices.Equal(got, want) {
		t.Fatalf("内核未优先排除 UID 0: %v", got)
	}
	options.IncludeUID = nil
	options.ExcludeUID = []ranges.Range[uint32]{{Start: 0, End: 4294967294}}
	if got := options.ExcludedRanges(); !slices.Equal(got, options.ExcludeUID) {
		t.Fatalf("空白名单未排除全部有效 UID: %v", got)
	}
}

func TestRootPolicyOverridesOnlyRootOnBothBackends(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, test := range []struct {
			name, policy, mode, filters string
			apps                        bool
			include, exclude            []uint32
			includeRange, excludeRange  []string
		}{
			{name: "default-no-forced-root", policy: "default", mode: "whitelist", apps: true, include: []uint32{10123}},
			{name: "whitelist-includes-root", policy: "include", mode: "whitelist", apps: true, include: []uint32{0, 10123}},
			{name: "whitelist-bypasses-root", policy: "exclude", mode: "whitelist", apps: true, include: []uint32{10123}, exclude: []uint32{0}},
			{name: "blacklist-includes-root-without-restricting-others", policy: "include", mode: "blacklist", apps: true, exclude: []uint32{10123}},
			{name: "blacklist-bypasses-root", policy: "exclude", mode: "blacklist", apps: true, exclude: []uint32{0, 10123}},
			{name: "app-disabled-root-bypass", policy: "exclude", exclude: []uint32{0}},
			{name: "app-disabled-root-include-unrestricted", policy: "include"},
			{name: "android-user-inclusion-does-not-restrict-apps", policy: "include", filters: `"include_android_user":[0,10]`},
			{name: "native-exclude-root", policy: "include", filters: `"exclude_uid":[0,23]`, exclude: []uint32{23}},
			{name: "native-whitelist", policy: "include", filters: `"include_uid":23`, include: []uint32{0, 23}},
			{name: "range-keeps-other-uids", policy: "include", filters: `"include_uid_range":"10000:10123","exclude_uid_range":["0:20","0:1","23:30"]`, include: []uint32{0}, includeRange: []string{"10000:10123"}, excludeRange: []string{"1:20", "23:30"}},
			{name: "root-range-singleton", policy: "include", filters: `"exclude_uid_range":"0:1"`, exclude: []uint32{1}},
			{name: "root-only-native-whitelist-bypassed", policy: "exclude", filters: `"include_uid":0`, include: []uint32{0}, exclude: []uint32{0}},
		} {
			t.Run(backend+"/"+test.name, func(t *testing.T) {
				config := fixture(t, backend, "", "")
				config.RootPolicy = test.policy
				if test.apps {
					config.App = AppPolicy{Enabled: true, Mode: test.mode, ProxyApps: []string{"0:com.example.app"}, BypassApps: []string{"0:com.example.app"}}
				}
				if test.filters != "" {
					addFilters(t, &config, test.filters)
				}
				original := encodeConfig(t, config)
				built, err := config.BuildWithResolver(func([]PackageRef) (PackageUIDResolution, error) {
					return PackageUIDResolution{UIDs: []uint32{10123}}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				include, includeRange, exclude, excludeRange := builtFilters(t, built)
				if !slices.Equal(include, test.include) || !slices.Equal(includeRange, test.includeRange) || !slices.Equal(exclude, test.exclude) || !slices.Equal(excludeRange, test.excludeRange) {
					t.Fatalf("Root 策略改变其他 UID: %v %v %v %v", include, includeRange, exclude, excludeRange)
				}
				if !bytes.Equal(original, encodeConfig(t, config)) {
					t.Fatal("Root 投影修改持久模板")
				}
			})
		}
		config := fixture(t, backend, "", "")
		config.RootPolicy = "include"
		config.App = AppPolicy{Enabled: true, Mode: "whitelist"}
		built, err := config.BuildWithResolver(nil)
		if err != nil {
			t.Fatal(err)
		}
		include, _, _, excludeRange := builtFilters(t, built)
		if !slices.Equal(include, []uint32{0}) || !slices.Equal(excludeRange, []string{"1:4294967294"}) {
			t.Fatal("空白名单未仅接管 Root", string(built.Runtime.Inbounds[0]))
		}
	}
}

func TestRootPolicyEffectiveChangesAndSharedOnly(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		original, err := config.EffectiveContent()
		if err != nil {
			t.Fatal(err)
		}
		config.RootPolicy = "include"
		unchanged, err := config.EffectiveContent()
		if err != nil || !bytes.Equal(original, unchanged) {
			t.Fatal("不限 UID 时接管 Root 不应改变有效策略", err)
		}
		config.RootPolicy = "exclude"
		changed, err := config.EffectiveContent()
		if err != nil || bytes.Equal(original, changed) {
			t.Fatal("绕过 Root 未影响有效策略", err)
		}
	}
	config := fixture(t, "ebpf", `{"type":"ebpf","tag":"netproxy-in","local":{"enabled":false},"shared":{"enabled":true,"interface":"ap0"}}`, "")
	original, _ := config.EffectiveContent()
	for _, policy := range []string{"include", "exclude"} {
		config.RootPolicy = policy
		effective, err := config.EffectiveContent()
		if err != nil || !bytes.Equal(original, effective) {
			t.Fatal("共享网络应用了本机 Root 策略", err)
		}
		built, err := config.BuildWithResolver(nil)
		if err != nil {
			t.Fatal(err)
		}
		include, includeRange, exclude, excludeRange := builtFilters(t, built)
		if len(include)+len(includeRange)+len(exclude)+len(excludeRange) != 0 {
			t.Fatal("禁用本机路径仍输出 Root 策略")
		}
	}
}

func TestRootPolicyRespectsNativeAndroidUserScope(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		addFilters(t, &config, `"include_android_user":10`)
		config.RootPolicy = "include"
		if _, err := Parse(encodeConfig(t, config)); err == nil {
			t.Fatal("原生用户范围排除 Root 时仍接受接管")
		}
		if _, err := config.BuildWithResolver(nil); err == nil {
			t.Fatal("Root 冲突未阻止运行时生成")
		}
		config.RootPolicy = "exclude"
		if _, err := config.BuildWithResolver(nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmptyWhitelistPreservesEBPFDNSAndSharedPath(t *testing.T) {
	for _, mode := range []string{"respect_policy", "hijack", "off"} {
		t.Run(mode, func(t *testing.T) {
			config := fixture(t, "ebpf", `{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"dns_mode":"`+mode+`"},"shared":{"enabled":true,"interface":"wlan2","dns_mode":"hijack"}}`, "")
			config.App = AppPolicy{Enabled: true, Mode: "whitelist"}
			built, err := config.BuildWithResolver(nil)
			if err != nil {
				t.Fatal(err)
			}
			var native ebpfInbound
			if err := unmarshalNative(built.Runtime.Inbounds[0], &native); err != nil {
				t.Fatal(err)
			}
			local, shared := native.EffectiveEnablement()
			if !local || !shared || native.Local.DNSMode != mode || native.Shared.DNSMode != "hijack" || !slices.Equal(native.Shared.Interface, []string{"wlan2"}) {
				t.Fatalf("空白名单改变 DNS 或共享接管: %+v", native)
			}
			if len(native.Local.IncludeUID)+len(native.Local.IncludeUIDRange) != 0 || !slices.Equal(native.Local.ExcludeUIDRange, []string{"0:4294967294"}) {
				t.Fatal("空白名单未绕过本机 UID", native.Local)
			}
		})
	}
}

func TestSharedOnlyAndDisabledAppNeverResolve(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		config.App.ProxyApps, config.App.BypassApps = []string{"0:com.example.app"}, []string{"10:com.example.app"}
		addFilters(t, &config, `"include_uid":123,"exclude_uid":456,"include_package":"com.example.native","include_android_user":10`)
		if _, err := config.BuildWithResolver(func([]PackageRef) (PackageUIDResolution, error) {
			t.Fatal("禁用 app 仍查询 UID")
			return PackageUIDResolution{}, nil
		}); err != nil {
			t.Fatal(err)
		}
		config.App.Enabled = true
		if backend == "ebpf" {
			config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in","local":{"enabled":false,"include_package":"com.example.native"},"shared":{"enabled":true,"interface":"wlan2"}}`)
			if _, err := config.BuildWithResolver(func([]PackageRef) (PackageUIDResolution, error) {
				t.Fatal("共享路径查询本机 UID")
				return PackageUIDResolution{}, nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAppFilterConflicts(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, mode := range []string{"whitelist", "blacklist"} {
			for _, filters := range []string{`"include_android_user":0`, `"include_package":"com.example.app"`, `"exclude_package":"com.example.app"`} {
				config := fixture(t, backend, "", "")
				config.App.Enabled, config.App.Mode = true, mode
				addFilters(t, &config, filters)
				content := encodeConfig(t, config)
				if _, err := Parse(content); err == nil {
					t.Fatalf("Parse 接受策略冲突: %s", content)
				}
				if err := Validate(content, backend); err == nil {
					t.Fatal("Validate 接受策略冲突")
				}
				if _, err := config.BuildWithResolver(func([]PackageRef) (PackageUIDResolution, error) {
					t.Fatal("冲突后仍查询")
					return PackageUIDResolution{}, nil
				}); err == nil {
					t.Fatal("Build 接受策略冲突")
				}
				if _, err := config.EffectiveContent(); err == nil {
					t.Fatal("EffectiveContent 接受策略冲突")
				}
				config.App.Enabled = false
				if _, err := config.BuildWithResolver(nil); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestBuildPropagatesResolverErrorsAndCancellation(t *testing.T) {
	config := fixture(t, "ebpf", "", "")
	config.App = AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: []string{"0:com.example.app"}}
	for _, expected := range []error{errors.New("package service failed"), context.Canceled, context.DeadlineExceeded} {
		if _, err := config.BuildWithResolver(func([]PackageRef) (PackageUIDResolution, error) { return PackageUIDResolution{}, expected }); !errors.Is(err, expected) {
			t.Fatal(err)
		}
	}
	if _, err := config.BuildWithResolver(nil); err == nil {
		t.Fatal("缺少解析器仍成功")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := config.Build(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	config.App.Enabled = false
	if _, err := config.Build(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("禁用 app 吞掉取消", err)
	}
}

func TestEffectiveContentIgnoresInactiveDifferences(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		before, err := config.EffectiveContent()
		if err != nil {
			t.Fatal(err)
		}
		config.App = AppPolicy{Enabled: false, Mode: "whitelist", ProxyApps: []string{"10:com.example.app"}, BypassApps: []string{"0:com.example.other"}}
		if backend == "ebpf" {
			config.TUN = jsontext.Value(`{"type":"tun","tag":"netproxy-in","unknown":1}`)
			config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true},"shared":{"enabled":false,"interface":"wlan2","data_plane":"socket_assign","dns_mode":"off","include_source_cidr":"192.168.43.0/24"}}`)
		} else {
			config.EBPF = jsontext.Value(`{"type":"ebpf","tag":"netproxy-in","unknown":2}`)
		}
		after, err := config.EffectiveContent()
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("无效差异改变有效内容: %s %s %v", before, after, err)
		}
	}
	config := fixture(t, "ebpf", `{"type":"ebpf","tag":"netproxy-in","local":{"enabled":false},"shared":{"enabled":true,"interface":"wlan2"}}`, "")
	before, _ := config.EffectiveContent()
	config.App = AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: []string{"0:com.example.app"}}
	after, err := config.EffectiveContent()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("仅共享模式 app 影响有效内容", err)
	}
}

func TestEffectiveContentCanonicalAndRealChanges(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		config := fixture(t, backend, "", "")
		config.App = AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: []string{"10:com.example.b", "0:com.example.a"}}
		addFilters(t, &config, `"include_uid":[100,101,102],"include_uid_range":["102:104","100:103"]`)
		before, _ := config.EffectiveContent()
		config.App.ProxyApps = []string{"00:com.example.a", "10:com.example.b", "0:com.example.a"}
		config.App.BypassApps = []string{"10:com.example.ignored"}
		addFilters(t, &config, `"include_uid_range":"100:104"`)
		after, err := config.EffectiveContent()
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("等效名单比较不同: %s %s %v", before, after, err)
		}
		for _, change := range []string{"app", "native", "backend"} {
			changed := config
			switch change {
			case "app":
				changed.App.ProxyApps = []string{"0:com.example.new"}
			case "native":
				addFilters(t, &changed, `"include_uid":200`)
			case "backend":
				if backend == "ebpf" {
					changed.Backend = "tun"
				} else {
					changed.Backend = "ebpf"
				}
			}
			result, err := changed.EffectiveContent()
			if err != nil || bytes.Equal(result, after) {
				t.Fatal("实际变更未影响有效内容", change, err)
			}
		}
	}
}

func TestWriteAtomicPreservesPreviousOnFailure(t *testing.T) {
	config := fixture(t, "tun", "", "")
	path := filepath.Join(t.TempDir(), "runtime", "inbound.json")
	if _, err := WriteAtomic(t.Context(), path, config); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var output Runtime
	if err := json.Unmarshal(before, &output); err != nil || len(output.Inbounds) != 1 {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatal(info.Mode())
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := WriteAtomic(ctx, path, config); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	config.TUN = jsontext.Value(`{"type":"tun","tag":"netproxy-in"}`)
	if _, err := WriteAtomic(t.Context(), path, config); err == nil {
		t.Fatal("非法候选写入成功")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("失败破坏原运行时")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("遗留临时文件", entries)
	}
	config = fixture(t, "ebpf", "", "")
	if _, err := WriteAtomic(t.Context(), path, config); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(path); bytes.Equal(content, before) {
		t.Fatal("未原子替换已有运行时")
	}
	if _, err := WriteAtomic(t.Context(), filepath.Dir(path), config); err == nil {
		t.Fatal("目录目标写入成功")
	}
	entries, _ = os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("失败后遗留临时文件", entries)
	}
}
