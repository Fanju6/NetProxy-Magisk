package module

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/sagernet/sing-box/option"
)

const targetCoreEnvironment = "NETPROXY_TEST_SINGBOX"

const targetCoreStaticConfig = `{
  "log": {"disabled": true},
  "dns": {"servers": [{"type": "local", "tag": "dns-local"}], "final": "dns-local"},
  "experimental": {"clash_api": {"external_controller": "127.0.0.1:9999", "secret": "singbox"}},
  "services": [{"type": "api", "listen": "127.0.0.1", "listen_port": 9090, "secret": "singbox"}],
  "inbounds": [{"type": "mixed", "tag": "user-mixed", "listen": "127.0.0.1", "listen_port": 1080}],
  "route": {
    "auto_detect_interface": true,
    "default_domain_resolver": "dns-local",
    "final": "Proxy",
    "rules": [
      {"inbound": "netproxy-in", "protocol": "dns", "action": "hijack-dns"},
      {"ip_is_private": true, "action": "route", "outbound": "direct"},
      {"inbound": "netproxy-in", "action": "route", "outbound": "Proxy"}
    ],
    "rule_set": [
      {"type": "inline", "tag": "geoip/cn", "rules": [{"ip_cidr": ["192.0.2.0/24", "2001:db8::/32"]}]},
      {"type": "inline", "tag": "test-private", "rules": [{"ip_cidr": "10.0.0.0/8"}]}
    ]
  }
}`

func TestTargetCoreStderrLogCapturesStartupReloadAndFailure(t *testing.T) {
	core := targetCoreBinary(t)
	options := NewOptions(t.TempDir())
	options.SingBoxPath = core
	prepared := PrepareResult{
		Providers: filepath.Join(options.RuntimeDir, "providers.json"),
		Outbounds: filepath.Join(options.RuntimeDir, "outbounds.json"),
		Inbound:   filepath.Join(options.RuntimeDir, "inbound.json"),
	}
	files := map[string]string{
		paths.SingBoxConfig(options.SingBoxDir): `{"log":{"output":"stderr","level":"info","timestamp":true},"outbounds":[{"type":"direct","tag":"direct"}]}`,
		prepared.Providers:                      "{}", prepared.Outbounds: "{}", prepared.Inbound: "{}",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		targetCoreWrite(t, path, []byte(content))
	}
	command, file, err := newSingBoxCommand(options, prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	running := command
	done := make(chan struct{})
	go func() {
		_ = running.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = running.Process.Kill()
		<-done
	})
	path, err := LogFile(options, "core")
	if err != nil {
		t.Fatal(err)
	}
	waitForStarts := func(count int) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			content := targetCoreRead(t, path)
			if strings.Count(string(content), "sing-box started") == count {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("真实核心启动或重载日志缺失: %s", content)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitForStarts(1)
	if runtime.GOOS != "windows" {
		if err := signalServiceReload(running.Process.Pid); err != nil {
			t.Fatal(err)
		}
		waitForStarts(2)
	}
	if err := running.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
	targetCoreWrite(t, paths.SingBoxConfig(options.SingBoxDir), []byte(`{"log":{"output":"stderr"},"inbounds":[{"type":"netproxy-invalid-inbound"}]}`))
	command, file, err = newSingBoxCommand(options, prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := command.Run(); err == nil {
		t.Fatal("无效核心配置未返回错误")
	}
	content := targetCoreRead(t, path)
	if !bytes.Contains(content, []byte("sing-box started")) || !bytes.Contains(content, []byte("FATAL")) || !bytes.Contains(content, []byte("netproxy-invalid-inbound")) || bytes.Contains(content, []byte("\x1b")) {
		t.Fatalf("真实核心 FATAL 丢失或包含颜色控制符: %s", content)
	}
}

func targetCoreBinary(t *testing.T) string {
	t.Helper()
	path, supplied := os.LookupEnv(targetCoreEnvironment)
	if !supplied {
		t.Skip("未设置 " + targetCoreEnvironment + "，跳过真实目标核心 check")
	}
	if strings.TrimSpace(path) == "" {
		t.Fatal(targetCoreEnvironment + " 已设置但路径为空")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("目标核心不是可读的普通文件: %s: %v", absolute, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, absolute, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("显式指定的目标核心不能执行: %v\n%s", err, output)
	}
	t.Logf("目标核心 %s\n%s", absolute, output)
	return absolute
}

func targetCoreFixture(t *testing.T, core string, content []byte) (Options, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	options := NewOptions(root)
	options.SingBoxPath = core
	options.StateFile = filepath.Join(root, "state", "service.json")
	options.WorkerPIDFile = filepath.Join(root, "state", "worker.pid")
	options.ProgressDir = filepath.Join(root, "state", "subscriptions")
	options.WiFiStateFile = filepath.Join(root, "state", "wifi_state")
	options.Telemetry = nil
	files := map[string][]byte{
		options.ModuleConfig:                    []byte("{\"selection\":{\"group_id\":\"default\",\"node_tag\":\"\"}}"),
		options.InboundConfig:                   content,
		paths.SingBoxConfig(options.SingBoxDir): []byte(targetCoreStaticConfig),
	}
	for path, value := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		targetCoreWrite(t, path, value)
	}
	for index, id := range []string{"default", "secondary"} {
		name := fmt.Sprintf("Core Check %d", index+1)
		input := "vless://00000000-0000-0000-0000-000000000001@192.0.2.1:443?security=none&type=tcp#CHECK_1"
		if index == 1 {
			input = "socks://127.0.0.1:1082#CHECK_2"
		}
		if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{
			Root: options.CatalogRoot, GroupID: id, Name: name, Type: "local",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
			GroupDir: filepath.Join(options.CatalogRoot, id), GroupID: id, Name: name, Type: "local",
			Input: input,
		}); err != nil {
			t.Fatal(err)
		}
		for _, filename := range []string{"meta.json", "provider.json"} {
			path := filepath.Join(options.CatalogRoot, id, filename)
			files[path] = targetCoreRead(t, path)
		}
	}
	return options, files
}

func targetCoreRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func targetCoreWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func targetCoreJSONEqual(t *testing.T, left, right []byte) bool {
	t.Helper()
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
	return bytes.Equal(canonical(left), canonical(right))
}

func targetCoreCheckCatalogNodes(t *testing.T, options Options, prepared PrepareResult) {
	t.Helper()
	var document struct {
		Providers []map[string]jsontext.Value `json:"providers"`
	}
	if err := json.Unmarshal(targetCoreRead(t, prepared.Providers), &document); err != nil {
		t.Fatal(err)
	}
	for _, entry := range document.Providers {
		var path string
		if err := json.Unmarshal(entry["path"], &path); err != nil {
			t.Fatal(err)
		}
		var nodes map[string]jsontext.Value
		if err := json.Unmarshal(targetCoreRead(t, path), &nodes); err != nil {
			t.Fatal(err)
		}
		delete(entry, "path")
		entry["type"] = jsontext.Value(`"inline"`)
		for key, value := range nodes {
			entry[key] = value
		}
	}
	content, err := json.Marshal(document, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	prepared.Providers = filepath.Join(options.RuntimeDir, "check-providers-inline.json")
	targetCoreWrite(t, prepared.Providers, content)
	// Local Provider 的节点在 Start 才加载；inline 投影令真实 check 也构造同一批匿名节点。
	if output, err := targetCoreRunCheck(t, options, prepared); err != nil {
		t.Fatalf("Catalog 节点的真实核心构造失败: %v\n%s", err, output)
	}
}

func targetCoreRunCheck(t *testing.T, options Options, prepared PrepareResult) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, options.SingBoxPath, "check",
		"-c", paths.SingBoxConfig(options.SingBoxDir), "-c", prepared.Providers,
		"-c", prepared.Outbounds, "-c", prepared.Inbound)
	command.Dir = options.SingBoxDir
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("目标核心 check 超时或取消: %v\n%s", ctx.Err(), output)
	}
	return output, err
}

func assertTargetCoreRuntime(t *testing.T, options Options, prepared PrepareResult, backend string) {
	t.Helper()
	if prepared.Backend != backend || prepared.GroupCount != 2 || prepared.NodeCount != 2 ||
		prepared.ActiveGroup != "default" || prepared.ActiveGroupTag != "Core Check 1" || prepared.SelectorMode != "urltest" {
		t.Fatalf("没有生成非空活动 Catalog: %+v", prepared)
	}
	var providers struct {
		Providers []struct {
			Type string `json:"type"`
			Tag  string `json:"tag"`
			Path string `json:"path"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(targetCoreRead(t, prepared.Providers), &providers); err != nil {
		t.Fatal(err)
	}
	if len(providers.Providers) != 2 {
		t.Fatalf("Provider 投影为空或不完整: %+v", providers)
	}
	for index, id := range []string{"default", "secondary"} {
		entry := providers.Providers[index]
		if entry.Type != "local" || entry.Tag != fmt.Sprintf("Core Check %d", index+1) ||
			entry.Path != filepath.Join(options.CatalogRoot, id, "provider.json") {
			t.Fatalf("Provider 没有引用真实 Catalog 文件: %+v", entry)
		}
		var nodes struct {
			Outbounds []struct {
				Type string `json:"type"`
				Tag  string `json:"tag"`
			} `json:"outbounds"`
		}
		if err := json.Unmarshal(targetCoreRead(t, entry.Path), &nodes); err != nil {
			t.Fatal(err)
		}
		protocol := "vless"
		if index == 1 {
			protocol = "socks"
		}
		if len(nodes.Outbounds) != 1 || nodes.Outbounds[0].Type != protocol || nodes.Outbounds[0].Tag != fmt.Sprintf("CHECK_%d", index+1) {
			t.Fatalf("Catalog 节点未实际写入: %+v", nodes)
		}
		if index == 0 {
			document, err := provider.Load(t.Context(), entry.Path)
			if err != nil {
				t.Fatal(err)
			}
			vless, ok := document.Outbounds[0].Options.(*option.VLESSOutboundOptions)
			if !ok || vless.RoutingMark != 0 || vless.BindInterface != "" || vless.Detour != "" ||
				vless.Inet4BindAddress != nil || vless.Inet6BindAddress != nil || vless.NetNs != "" || vless.ProtectPath != "" {
				t.Fatalf("Provider 序列化改变了匿名 VLESS 的默认拨号条件: %+v", document.Outbounds[0].Options)
			}
		}
	}
	var outbounds struct {
		Outbounds []struct {
			Type      string   `json:"type"`
			Tag       string   `json:"tag"`
			Default   string   `json:"default"`
			Providers []string `json:"providers"`
			Outbounds []string `json:"outbounds"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(targetCoreRead(t, prepared.Outbounds), &outbounds); err != nil {
		t.Fatal(err)
	}
	if len(outbounds.Outbounds) != 7 {
		t.Fatalf("出站图未完整生成: %+v", outbounds)
	}
	for index := range 2 {
		name := fmt.Sprintf("Core Check %d", index+1)
		auto, manual := outbounds.Outbounds[2+index*2], outbounds.Outbounds[3+index*2]
		if auto.Type != "urltest" || auto.Tag != "Auto/"+name || !slices.Equal(auto.Providers, []string{name}) ||
			manual.Type != "selector" || manual.Tag != "Select/"+name || !slices.Equal(manual.Providers, []string{name}) {
			t.Fatalf("分组出站没有引用 Provider: auto=%+v manual=%+v", auto, manual)
		}
	}
	proxy := outbounds.Outbounds[6]
	if proxy.Type != "selector" || proxy.Tag != "Proxy" || proxy.Default != "Auto/Core Check 1" ||
		!slices.Equal(proxy.Outbounds, []string{"Auto/Core Check 1", "Select/Core Check 1", "Auto/Core Check 2", "Select/Core Check 2"}) {
		t.Fatalf("Proxy 静默退回 direct 或未连接完整出站图: %+v", proxy)
	}
	var runtime inbound.Runtime
	if err := json.Unmarshal(targetCoreRead(t, prepared.Inbound), &runtime); err != nil {
		t.Fatal(err)
	}
	if len(runtime.Inbounds) != 1 {
		t.Fatalf("应只生成当前选中的一个受管入站: %+v", runtime)
	}
	var identity struct {
		Type string `json:"type"`
		Tag  string `json:"tag"`
	}
	if err := json.Unmarshal(runtime.Inbounds[0], &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Type != backend || identity.Tag != inbound.Tag {
		t.Fatalf("受管入站身份不符: %+v", identity)
	}
	if backend == "tun" {
		config, err := inbound.Load(options.InboundConfig)
		if err != nil {
			t.Fatal(err)
		}
		template, err := config.TUNOptions()
		if err != nil {
			t.Fatal(err)
		}
		config.TUN = runtime.Inbounds[0]
		generated, err := config.TUNOptions()
		if err != nil {
			t.Fatal(err)
		}
		if !generated.AutoRoute || !generated.AutoRedirect || generated.AutoRedirectDisableMarkMode != template.AutoRedirectDisableMarkMode ||
			generated.AutoRedirectInputMark != template.AutoRedirectInputMark || generated.AutoRedirectOutputMark != template.AutoRedirectOutputMark ||
			generated.AutoRedirectResetMark != template.AutoRedirectResetMark || generated.AutoRedirectTProxyMark != template.AutoRedirectTProxyMark ||
			generated.AutoRedirectNFQueue != template.AutoRedirectNFQueue || generated.AutoRedirectFallbackRuleIndex != template.AutoRedirectFallbackRuleIndex ||
			generated.IPRoute2TableIndex != template.IPRoute2TableIndex || generated.IPRoute2RuleIndex != template.IPRoute2RuleIndex || generated.NetNs != template.NetNs {
			t.Fatalf("TUN 生成物丢失 auto-redirect 或路由字段: template=%+v generated=%+v", template, generated)
		}
	}
}

func TestInboundTargetCoreCheck(t *testing.T) {
	core := targetCoreBinary(t)
	for _, fixture := range []struct {
		name    string
		backend string
	}{
		{"ebpf-default", "ebpf"},
		{"ebpf-local", "ebpf"},
		{"ebpf-shared", "ebpf"},
		{"ebpf-both", "ebpf"},
		{"tun-default", "tun"},
		{"tun", "tun"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			content := []byte(strings.Replace(testInboundConfig, `"backend": "ebpf"`, `"backend": "`+fixture.backend+`"`, 1))
			if !strings.HasSuffix(fixture.name, "-default") {
				content = targetCoreRead(t, filepath.Join("..", "inbound", "testdata", fixture.name+".json"))
			}
			options, originals := targetCoreFixture(t, core, content)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			prepared, err := Check(ctx, options, false)
			if err != nil {
				t.Fatalf("完整配置的真实目标核心 check 失败: %v", err)
			}
			assertTargetCoreRuntime(t, options, prepared, fixture.backend)
			targetCoreCheckCatalogNodes(t, options, prepared)
			if !strings.HasSuffix(fixture.name, "-default") {
				want := targetCoreRead(t, filepath.Join("..", "inbound", "testdata", fixture.name+".golden.json"))
				got := targetCoreRead(t, prepared.Inbound)
				if !targetCoreJSONEqual(t, got, want) {
					t.Fatalf("生成物与匿名原生契约不符:\ngot %s\nwant %s", got, want)
				}
			}
			// 逐份破坏并恢复，保证真实 check 读取全部四份配置，而非只返回成功。
			for _, fragment := range []struct {
				name string
				path string
				bad  string
			}{
				{"static", paths.SingBoxConfig(options.SingBoxDir), `{"services":[{"type":"netproxy-invalid-service"}]}`},
				{"providers", prepared.Providers, `{"providers":[{"type":"netproxy-invalid-provider","tag":"invalid-provider"}]}`},
				{"outbounds", prepared.Outbounds, `{"outbounds":[{"type":"netproxy-invalid-outbound","tag":"invalid-outbound"}]}`},
				{"inbound", prepared.Inbound, `{"inbounds":[{"type":"netproxy-invalid-inbound","tag":"netproxy-in"}]}`},
			} {
				t.Run("reject-"+fragment.name, func(t *testing.T) {
					original := targetCoreRead(t, fragment.path)
					defer targetCoreWrite(t, fragment.path, original)
					targetCoreWrite(t, fragment.path, []byte(fragment.bad))
					output, err := targetCoreRunCheck(t, options, prepared)
					var exit *exec.ExitError
					if err == nil || !strings.Contains(string(output), "netproxy-invalid-") || !errors.As(err, &exit) {
						t.Fatalf("核心没有拒绝实际传入的 %s: err=%v\n%s", fragment.name, err, output)
					}
				})
			}
			if output, err := targetCoreRunCheck(t, options, prepared); err != nil {
				t.Fatalf("恢复后的完整配置未通过 check: %v\n%s", err, output)
			}
			for path, original := range originals {
				if !bytes.Equal(targetCoreRead(t, path), original) {
					t.Fatalf("check 修改了持久事实源: %s", path)
				}
			}
			for _, path := range []string{options.StateFile, options.WorkerPIDFile, configTransactionPath(options)} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("只读 check 不应创建运行态或事务: %s: %v", path, err)
				}
			}
		})
	}
}

func TestRootPolicyTargetCoreCheck(t *testing.T) {
	core := targetCoreBinary(t)
	for _, backend := range []string{"ebpf", "tun"} {
		for _, policy := range []string{"default", "include", "exclude"} {
			t.Run(backend+"/"+policy, func(t *testing.T) {
				config, err := inbound.Parse([]byte(testInboundConfig))
				if err != nil {
					t.Fatal(err)
				}
				config.Backend, config.RootPolicy = backend, policy
				config.App = inbound.AppPolicy{Enabled: true, Mode: "whitelist", ProxyApps: []string{}, BypassApps: []string{}}
				content, err := json.Marshal(config, json.Deterministic(true))
				if err != nil {
					t.Fatal(err)
				}
				options, originals := targetCoreFixture(t, core, content)
				prepared, err := Check(t.Context(), options, false)
				if err != nil {
					t.Fatalf("Root 策略真实核心 check 失败: %v", err)
				}
				assertTargetCoreRuntime(t, options, prepared, backend)
				for path, original := range originals {
					if !bytes.Equal(original, targetCoreRead(t, path)) {
						t.Fatal("check 修改持久文件", path)
					}
				}
			})
		}
	}
}

func targetCoreSetRoutingMark(t *testing.T, options Options, source string, mark jsontext.Value) (string, string) {
	t.Helper()
	path, field := paths.SingBoxConfig(options.SingBoxDir), "routing_mark"
	if id, found := strings.CutPrefix(source, "catalog-"); found {
		path = filepath.Join(options.CatalogRoot, id, "provider.json")
	}
	var object map[string]jsontext.Value
	if err := json.Unmarshal(targetCoreRead(t, path), &object); err != nil {
		t.Fatal(err)
	}
	key := "outbounds"
	var value any
	if source == "route" {
		key, field = "route", "default_mark"
		var route map[string]jsontext.Value
		if err := json.Unmarshal(object[key], &route); err != nil {
			t.Fatal(err)
		}
		route[field] = mark
		value = route
	} else if source == "static-outbound" {
		value = []map[string]jsontext.Value{
			{"type": jsontext.Value(`"direct"`), "tag": jsontext.Value(`"static-clean"`)},
			{"type": jsontext.Value(`"direct"`), "tag": jsontext.Value(`"static-marked"`), field: mark},
		}
	} else {
		var outbounds []map[string]jsontext.Value
		if err := json.Unmarshal(object[key], &outbounds); err != nil {
			t.Fatal(err)
		}
		outbounds[0][field] = mark
		value = outbounds
	}
	encoded, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	object[key] = encoded
	content, err := json.Marshal(object, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	targetCoreWrite(t, path, content)
	return path, field
}

func assertTargetCoreRoutingMarkConflict(t *testing.T, err error, field string) {
	t.Helper()
	var validation *inbound.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("没有返回 TUN 组合校验错误: %v", err)
	}
	for _, diagnostic := range validation.Diagnostics {
		if diagnostic.Code == "tun.routing_mark_conflict" && diagnostic.Level == "error" && strings.Contains(diagnostic.Field, field) {
			return
		}
	}
	t.Fatalf("缺少带字段定位的 tun.routing_mark_conflict: %+v", validation.Diagnostics)
}

func targetCoreRoutingMarkCases(t *testing.T, core string) {
	t.Helper()
	for _, backend := range []string{"tun", "ebpf"} {
		for _, source := range []string{"route", "static-outbound", "catalog-default", "catalog-secondary"} {
			for _, mark := range []struct {
				name    string
				value   jsontext.Value
				nonzero bool
			}{
				{"decimal", jsontext.Value(`123`), true},
				{"hex", jsontext.Value(`"0x2024"`), true},
				{"zero", jsontext.Value(`0`), false},
				{"hex-zero", jsontext.Value(`"0x0"`), false},
			} {
				t.Run(backend+"/"+source+"/"+mark.name, func(t *testing.T) {
					content := []byte(strings.Replace(testInboundConfig, `"backend": "ebpf"`, `"backend": "`+backend+`"`, 1))
					options, originals := targetCoreFixture(t, core, content)
					ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
					defer cancel()
					prepared, err := Prepare(ctx, options, false)
					if err != nil {
						t.Fatal(err)
					}
					previousRuntime := map[string][]byte{}
					for _, path := range []string{prepared.Providers, prepared.Outbounds, prepared.Inbound} {
						// 有效 JSON 尾部空白可发现失败路径重新生成了语义相同的旧 runtime。
						previousRuntime[path] = append(targetCoreRead(t, path), '\n', ' ', '\n')
						targetCoreWrite(t, path, previousRuntime[path])
					}
					path, field := targetCoreSetRoutingMark(t, options, source, mark.value)
					originals[path] = targetCoreRead(t, path)
					result, err := Prepare(ctx, options, false)
					if backend == "tun" && mark.nonzero {
						assertTargetCoreRoutingMarkConflict(t, err, field)
						if result != (PrepareResult{}) {
							t.Fatalf("拒绝组合时泄漏成功的 Prepare 结果: %+v", result)
						}
						if _, err := Check(ctx, options, false); err == nil {
							t.Fatal("NetProxy Check 绕过了 Prepare 的 TUN mark 校验")
						} else {
							assertTargetCoreRoutingMarkConflict(t, err, field)
						}
						for path, previous := range previousRuntime {
							if !bytes.Equal(targetCoreRead(t, path), previous) {
								t.Fatalf("拒绝组合前改写了正式 runtime: %s", path)
							}
						}
						if core != "" {
							// 直接核心 check 仍会接受；必须由 NetProxy 提前拒绝，不能把责任交回拨号阶段。
							if output, err := targetCoreRunCheck(t, options, prepared); err != nil {
								t.Fatalf("上游 check 的 mark 边界发生变化: %v\n%s", err, output)
							}
							targetCoreCheckCatalogNodes(t, options, prepared)
						}
					} else {
						if err != nil {
							t.Fatalf("eBPF mark 或 TUN 零值 mark 被错误拒绝: %v", err)
						}
						if result.Backend != backend || result.GroupCount != 2 || result.NodeCount != 2 {
							t.Fatalf("接受组合时没有生成完整 Catalog/入站: %+v", result)
						}
						for _, path := range []string{result.Providers, result.Outbounds, result.Inbound} {
							if !jsontext.Value(targetCoreRead(t, path)).IsValid() {
								t.Fatalf("接受组合时 runtime 无效: %s", path)
							}
						}
						if core != "" {
							checked, err := Check(ctx, options, false)
							if err != nil {
								t.Fatalf("eBPF mark 或 TUN 零值 mark 的真实组合 check 失败: %v", err)
							}
							targetCoreCheckCatalogNodes(t, options, checked)
						}
					}
					for path, original := range originals {
						if !bytes.Equal(targetCoreRead(t, path), original) {
							t.Fatalf("组合校验修改了持久事实源: %s", path)
						}
					}
					for _, path := range []string{options.StateFile, options.WorkerPIDFile, configTransactionPath(options)} {
						if _, err := os.Stat(path); !os.IsNotExist(err) {
							t.Fatalf("组合校验不应创建运行态或事务: %s: %v", path, err)
						}
					}
				})
			}
		}
	}
}

func TestInboundPrepareRoutingMarkConflicts(t *testing.T) {
	targetCoreRoutingMarkCases(t, "")
}

func TestInboundTargetCoreCheckRoutingMarkConflicts(t *testing.T) {
	targetCoreRoutingMarkCases(t, targetCoreBinary(t))
}
