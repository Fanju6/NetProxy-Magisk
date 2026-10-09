package module

import (
	"context"
	"encoding/binary"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
	"google.golang.org/protobuf/encoding/protowire"
)

func writeModeConfig(t *testing.T, options Options, mode string) {
	t.Helper()
	path := paths.SingBoxConfig(options.SingBoxDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	name, _ := json.Marshal(mode)
	content := `{"experimental":{"clash_api":{"default_mode":` + string(name) + `,"secret":"fixture"},"cache_file":{"enabled":true}},"route":{"rules":[{"clash_mode":"Direct","action":"route","outbound":"direct"},{"clash_mode":"Office"},{"domain":"example.test","outbound":"Proxy"}]},"custom":{"keep":true}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeModeReconciliation(t *testing.T) {
	for _, test := range []struct {
		name, config, current, network, ssid, wanted string
		wifi, ignore, unavailable, failAPI           bool
		sets                                         int32
		wantError                                    bool
	}{
		{name: "cached-direct", config: "Rule", current: "Direct", wanted: "Rule", sets: 1},
		{name: "already-correct", config: "Office", current: "Office", wanted: "Office"},
		{name: "wifi-bypass", config: "Office", current: "Office", network: "wifi", ssid: "Home", wifi: true, wanted: "Direct", sets: 1},
		{name: "wifi-base", config: "Office", current: "Direct", network: "wifi", ssid: "Other", wifi: true, wanted: "Office", sets: 1},
		{name: "cellular-base", config: "Office", current: "Direct", network: "not_wifi", wifi: true, wanted: "Office", sets: 1},
		{name: "boot-network-pending", config: "Rule", current: "Direct", wifi: true, unavailable: true, wanted: "Rule", sets: 1},
		{name: "ignored-api", config: "Office", current: "Direct", ignore: true, sets: 1, wantError: true},
		{name: "unavailable-api", config: "Office", current: "Direct", failAPI: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := newTestOptions(t.TempDir())
			writeModeConfig(t, options, test.config)
			module := "{\"wifi\":{\"enabled\":false}}"
			if test.wifi {
				module = "{\"wifi\":{\"blacklist\":[\"Home\"],\"enabled\":true,\"mode\":\"blacklist\"}}"
			}
			if err := os.WriteFile(options.ModuleConfig, []byte(module), 0o600); err != nil {
				t.Fatal(err)
			}
			var reads int
			options.NetworkStateReader = func(context.Context) (worker.NetworkState, error) {
				reads++
				if test.unavailable {
					return worker.NetworkState{}, worker.ErrNetworkUnavailable
				}
				return worker.NetworkState{NetworkType: test.network, SSID: test.ssid}, nil
			}
			var current atomic.Value
			current.Store(test.current)
			var sets atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.failAPI {
					http.Error(w, "unavailable", 503)
					return
				}
				var payload []byte
				switch r.URL.Path {
				case "/daemon.StartedService/GetClashModeStatus":
					payload = protowire.AppendTag(payload, 2, protowire.BytesType)
					payload = protowire.AppendString(payload, current.Load().(string))
				case "/daemon.StartedService/SetClashMode":
					sets.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil || len(body) < 6 {
						t.Error("无效模式请求")
						return
					}
					_, _, n := protowire.ConsumeTag(body[5:])
					mode, n := protowire.ConsumeString(body[5+n:])
					if n < 0 {
						t.Error("无效模式正文")
						return
					}
					if !test.ignore {
						current.Store(mode)
					}
				}
				header := make([]byte, 5)
				binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
				_, _ = w.Write(append(header, payload...))
			}))
			defer server.Close()
			options.ServiceAddress = server.URL
			result, err := syncConfiguredMode(t.Context(), options)
			if (err != nil) != test.wantError || sets.Load() != test.sets {
				t.Fatalf("结果 %+v，错误 %v，SetMode 次数 %d", result, err, sets.Load())
			}
			if !test.wantError && result.RuntimeMode != test.wanted {
				t.Fatalf("实际模式: %+v", result)
			}
			if !test.wifi && reads != 0 {
				t.Fatal("Wi-Fi 关闭时仍查询系统网络")
			}
			modes, err := moduleconfig.LoadModes(paths.SingBoxConfig(options.SingBoxDir))
			if err != nil || modes.Mode != test.config {
				t.Fatalf("网络策略覆盖了默认模式: %+v %v", modes, err)
			}
		})
	}
}

func TestApplyModeStoppedUpdatesOnlyDefaultMode(t *testing.T) {
	options := newTestOptions(t.TempDir())
	options.SingBoxPath = filepath.Join(options.ModuleDir, "missing-core")
	writeModeConfig(t, options, "Rule")
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"auto_start\":false}"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := paths.SingBoxConfig(options.SingBoxDir)
	before, _ := os.ReadFile(path)
	result, err := ApplyMode(t.Context(), options, "Office")
	if err != nil || result.Mode != "Office" || result.RuntimeMode != "" {
		t.Fatalf("停止状态保存: %+v %v", result, err)
	}
	after, _ := os.ReadFile(path)
	var first, second map[string]any
	_ = json.Unmarshal(before, &first)
	_ = json.Unmarshal(after, &second)
	first["experimental"].(map[string]any)["clash_api"].(map[string]any)["default_mode"] = "Office"
	wanted, _ := json.Marshal(first, json.Deterministic(true))
	actual, _ := json.Marshal(second, json.Deterministic(true))
	if string(wanted) != string(actual) {
		t.Fatalf("覆盖了模式以外的配置: %s", after)
	}
	module, _ := os.ReadFile(options.ModuleConfig)
	if string(module) != `{"auto_start":false}` {
		t.Fatalf("修改了模块配置: %s", module)
	}
	for _, path := range []string{options.StateFile, options.WorkerPIDFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("停服保存产生了运行副作用: %s %v", path, err)
		}
	}
	if _, err := ApplyMode(t.Context(), options, "proxy"); err == nil {
		t.Fatal("接受了配置中不存在的模式")
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != string(after) {
		t.Fatal("非法模式覆盖了主配置")
	}
}

func TestApplyModeUsesSharedConfigLock(t *testing.T) {
	options := newTestOptions(t.TempDir())
	writeModeConfig(t, options, "Rule")
	editor, err := moduleconfig.Lock(t.Context(), paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		t.Fatal(err)
	}
	defer editor.Release()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	if _, err := ApplyMode(ctx, options, "Office"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("等待配置锁未响应取消: %v", err)
	}
}

func TestApplyModeWaitsForLifecycleLockWithContext(t *testing.T) {
	options := newTestOptions(t.TempDir())
	writeModeConfig(t, options, "Rule")
	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	if _, err := ApplyMode(ctx, options, "Office"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("等待生命周期锁未响应取消: %v", err)
	}
}

func TestNetworkPolicyRejectsMissingDirectMode(t *testing.T) {
	options := newTestOptions(t.TempDir())
	if err := os.MkdirAll(options.SingBoxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	targetCoreWrite(t, paths.SingBoxConfig(options.SingBoxDir), []byte(`{}`))
	targetCoreWrite(t, options.ModuleConfig, []byte("{\"wifi\":{\"enabled\":true,\"mode\":\"blacklist\",\"proxy_on_non_wifi\":false}}"))
	_, err := EvaluateNetwork(t.Context(), options, "not_wifi", "")
	structured, ok := errors.AsType[*service.Error](err)
	if !ok || structured.Code != "mode.unavailable" {
		t.Fatalf("缺少 Direct 时未返回明确错误: %v", err)
	}
	if content := targetCoreRead(t, paths.SingBoxConfig(options.SingBoxDir)); string(content) != `{}` {
		t.Fatalf("网络策略自动补写了主配置: %s", content)
	}
}

func TestModeSaveInvalidatesEditorRevision(t *testing.T) {
	options, _, _, _ := configApplyOptions(t)
	writeModeConfig(t, options, "Rule")
	before, err := ReadConfig(options, "singbox/experimental")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyMode(t.Context(), options, "Office"); err != nil {
		t.Fatal(err)
	}
	after, err := ReadConfig(options, "singbox/experimental")
	if err != nil || before["revision"] == after["revision"] {
		t.Fatalf("模式写入未更新编辑器 revision: %+v %v", after, err)
	}
	source := filepath.Join(options.ModuleDir, "stale-experimental.json")
	targetCoreWrite(t, source, []byte(before["content"]))
	_, err = ApplyConfig(t.Context(), options, "singbox/experimental", source, false, before["revision"])
	if !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("过期编辑器快照覆盖了模式: %v", err)
	}
}

func TestModeTargetCoreRestoresCacheAndReconcilesDefault(t *testing.T) {
	core := targetCoreBinary(t)
	options := newTestOptions(t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	_ = listener.Close()
	options.ServiceAddress = address.String()
	options.ServiceSecret = "fixture"
	path := paths.SingBoxConfig(options.SingBoxDir)
	if err := os.MkdirAll(options.SingBoxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`{"log":{"disabled":true},"experimental":{"cache_file":{"enabled":true,"path":"cache.db"},"clash_api":{"default_mode":"Rule"}},"services":[{"type":"api","listen":"127.0.0.1","listen_port":%d,"secret":"fixture"}],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"rules":[{"clash_mode":["Rule","Proxy","Direct","RuleAllowAds","Office"],"action":"route","outbound":"direct"}]}}`, address.Port)
	targetCoreWrite(t, path, []byte(content))
	targetCoreWrite(t, options.ModuleConfig, []byte("{\"wifi\":{\"enabled\":false}}"))
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	start := func() func() {
		command := exec.CommandContext(t.Context(), core, "run", "-c", path)
		command.Dir = options.SingBoxDir
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		stopped := false
		stop := func() {
			if !stopped {
				_ = command.Process.Kill()
				<-done
				stopped = true
			}
		}
		t.Cleanup(stop)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		for {
			if _, err := client.Ready(ctx); err == nil {
				return stop
			}
			select {
			case <-ctx.Done():
				t.Fatalf("真实核心 API 未就绪: %v", ctx.Err())
			case err := <-done:
				stopped = true
				t.Fatalf("真实核心提前退出: %v", err)
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	stop := start()
	if err := service.SetMode(t.Context(), networkControlOptions(options), "Direct"); err != nil {
		t.Fatal(err)
	}
	stop()
	updated, err := replaceDefaultMode([]byte(content), "Proxy")
	if err != nil {
		t.Fatal(err)
	}
	targetCoreWrite(t, path, updated)
	stop = start()
	defer stop()
	mode, err := client.Mode(t.Context())
	if err != nil || mode.Current != "Direct" {
		t.Fatalf("没有复现内核缓存优先: %+v %v", mode, err)
	}
	result, err := syncConfiguredMode(t.Context(), options)
	if err != nil || result.RuntimeMode != "Proxy" {
		t.Fatalf("默认模式校准失败: %+v %v", result, err)
	}
	mode, err = client.Mode(t.Context())
	modes, loadErr := moduleconfig.LoadModes(path)
	if err != nil || loadErr != nil || mode.Current != "Proxy" || !slices.Equal(mode.Available, modes.Available) {
		t.Fatalf("模式列表或实际模式与内核不一致: native=%+v parsed=%+v errors=%v/%v", mode, modes, err, loadErr)
	}
	if err := service.SetMode(t.Context(), networkControlOptions(options), "RuleAllowAds"); err != nil {
		t.Fatal(err)
	}
	mode, err = client.Mode(t.Context())
	if err != nil || mode.Current != "RuleAllowAds" {
		t.Fatalf("真实核心未应用允许广告的规则模式: %+v %v", mode, err)
	}
}

func TestModeTargetCoreDefaultConfig(t *testing.T) {
	staticDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "module", "config", "singbox"))
	if err != nil {
		t.Fatal(err)
	}
	content := targetCoreRead(t, paths.SingBoxConfig(staticDir))
	modes, err := moduleconfig.ParseModes(content)
	if err != nil || modes.Mode != "Rule" || !slices.Equal(modes.Available, []string{"Proxy", "RuleAllowAds", "Rule", "Direct"}) {
		t.Fatalf("默认配置模式不完整: %+v %v", modes, err)
	}
	core := targetCoreBinary(t)
	root, err := configObject(content)
	if err != nil {
		t.Fatal(err)
	}
	// Host 校验隔离 Android 的日志、缓存和面板路径，规则文件与路由内容保持原样。
	root["log"] = jsontext.Value(`{"disabled":true}`)
	root["experimental"] = jsontext.Value(`{"clash_api":{"default_mode":"Rule"}}`)
	root["services"] = jsontext.Value(`[{"type":"api","listen":"127.0.0.1","listen_port":9090,"secret":"fixture"}]`)
	root["outbounds"] = jsontext.Value(`[{"type":"direct","tag":"direct"},{"type":"selector","tag":"Proxy","outbounds":["direct"]}]`)
	content, err = json.Marshal(root, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	targetCoreWrite(t, path, content)
	command := exec.CommandContext(t.Context(), core, "check", "-c", path)
	command.Dir = staticDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("默认模式路由的真实核心 check 失败: %v\n%s", err, output)
	}
}
