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
	"strings"
	"testing"
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
					if !reflect.DeepEqual(include, []uint32{0, 1010123}) || !reflect.DeepEqual(includeRange, []string{"10120:10130"}) || len(exclude)+len(excludeRange) != 0 {
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

func TestWhitelistMissingAndEmptyStillOnlyRoot(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		for _, values := range [][]string{nil, {"10:com.example.missing"}} {
			config := fixture(t, backend, "", "")
			config.App = AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: values}
			built, err := config.BuildWithResolver(func(refs []PackageRef) (PackageUIDResolution, error) { return PackageUIDResolution{Missing: refs}, nil })
			if err != nil {
				t.Fatal(err)
			}
			include, ranges, _, _ := builtFilters(t, built)
			if !reflect.DeepEqual(include, []uint32{0}) || len(ranges) != 0 || len(built.MissingPackages) != len(values) {
				t.Fatal(built)
			}
		}
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
			opposite := "include"
			if mode == "whitelist" {
				opposite = "exclude"
			}
			for _, filters := range []string{`"` + opposite + `_uid":123`, `"` + opposite + `_uid_range":"123:456"`, `"include_android_user":0`, `"include_package":"com.example.app"`, `"exclude_package":"com.example.app"`} {
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
