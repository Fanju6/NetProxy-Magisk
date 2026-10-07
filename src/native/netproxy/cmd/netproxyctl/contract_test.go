package main

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func TestRuntimeModeFailureKeepsPersistedStateAfterTimeout(t *testing.T) {
	capture, err := os.CreateTemp(t.TempDir(), "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	previous := os.Stdout
	os.Stdout = capture
	defer func() { os.Stdout = previous }()
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	status := (&cli{}).runCommand(ctx, func(context.Context, []string) error {
		return &service.Error{Code: "mode.runtime_sync_failed", Message: "已保存但未同步", Data: map[string]any{"persisted": true, "mode": "Office"}}
	})
	payload, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Schema int    `json:"schema"`
		OK     bool   `json:"ok"`
		Code   string `json:"code"`
		Data   struct {
			Persisted bool   `json:"persisted"`
			Mode      string `json:"mode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &response); err != nil || status != 1 || response.Schema != 1 || response.OK || response.Code != "mode.runtime_sync_failed" || !response.Data.Persisted || response.Data.Mode != "Office" {
		t.Fatalf("超时覆盖了持久化失败语义: exit=%d response=%s error=%v", status, payload, err)
	}
}

func TestPublicCommandsKeepSingleJSONContract(t *testing.T) {
	root := t.TempDir()
	options := moduleapp.NewOptions(root)
	options.StateFile = filepath.Join(root, "state", "service.json")
	options.ProgressDir = filepath.Join(root, "state", "subscriptions")
	options.WorkerPIDFile = filepath.Join(root, "state", "worker.pid")
	options.WiFiStateFile = filepath.Join(root, "state", "wifi_state")
	for path, content := range map[string]string{
		options.ModuleConfig:                             "ACTIVE_GROUP_ID=default\nSELECTOR_MODE=urltest\n",
		options.InboundConfig:                            `{"backend":"ebpf","app":{"enabled":false,"mode":"blacklist","proxy_apps":[],"bypass_apps":[]},"ebpf":{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true},"shared":{"enabled":false}},"tun":{"type":"tun","tag":"netproxy-in","interface_name":"netproxy","address":["172.19.0.1/30"],"auto_route":true,"auto_redirect":true}}`,
		filepath.Join(options.SingBoxDir, "config.json"): "{}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: "default", Name: "本地配置", Type: "local"}); err != nil {
		t.Fatal(err)
	}
	command := &cli{options: options}
	for _, test := range []struct {
		args   string
		code   string
		status int
	}{
		{"service status", "service.status", 0},
		{"catalog list", "catalog.groups", 0},
		{"catalog show default", "catalog.show", 0},
		{"node list", "node.list", 0},
		{"node current", "node.current", 0},
		{"sub list", "subscription.list", 0},
		{"mode", "mode.current", 0},
		{"mode Rule", "mode.changed", 0},
		{"mode global", "mode.invalid", 1},
		{"mode Rule extra", "usage.invalid", 2},
		{"network evaluate --type not_wifi", "network.evaluated", 0},
		{"app list", "app.list", 0},
		{"config list", "config.list", 0},
		{"config read module", "config.read", 0},
		{"logs show service", "logs.show", 0},
		{"node get invalid", "node.ref_invalid", 2},
		{"catalog show", "usage.invalid", 2},
		{"node import missing.yaml custom", "usage.invalid", 2},
	} {
		t.Run(test.args, func(t *testing.T) {
			capture, err := os.CreateTemp(t.TempDir(), "stdout-")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			previous := os.Stdout
			var status int
			func() {
				os.Stdout = capture
				defer func() { os.Stdout = previous }()
				status = command.run(t.Context(), append([]string{"--json"}, strings.Fields(test.args)...))
			}()
			payload, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Schema int            `json:"schema"`
				OK     bool           `json:"ok"`
				Code   string         `json:"code"`
				Data   jsontext.Value `json:"data"`
			}
			if err := json.Unmarshal(payload, &response); err != nil {
				t.Fatalf("stdout 不是单一 JSON: %s: %v", payload, err)
			}
			if status != test.status || response.Schema != 1 || response.OK != (status == 0) || response.Code != test.code {
				t.Fatalf("契约不匹配: exit=%d, %s", status, payload)
			}
			if test.args == "service status" {
				var data map[string]jsontext.Value
				if err := json.Unmarshal(response.Data, &data); err != nil || string(data["state"]) != `"stopped"` {
					t.Fatalf("服务状态必须直接位于 data: %s: %v", response.Data, err)
				}
				if string(data["configured_outbound_mode"]) != `"Rule"` || string(data["available_outbound_modes"]) != `["Rule"]` {
					t.Fatalf("模式字段未使用主配置: %s", response.Data)
				}
			}
		})
	}
}

func TestEBPFDiagnosticJSONContract(t *testing.T) {
	root := t.TempDir()
	options := moduleapp.NewOptions(root)
	options.Telemetry = nil
	binary := filepath.Join(root, "fake-core")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "../../internal/inbound/testdata/fake-package")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建诊断 fixture 失败: %v\n%s", err, output)
	}
	options.SingBoxPath = binary
	if err := os.MkdirAll(filepath.Dir(options.InboundConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	command := &cli{options: options}
	t.Setenv("NETPROXY_TEST_COMMAND_MODE", "probe")
	for _, test := range []struct {
		name    string
		backend string
		result  string
		raw     bool
		fail    bool
		code    string
		want    string
	}{
		{"ebpf", "ebpf", "preflight_passed", false, false, "ebpf.status", "eBPF 能力预检通过"},
		{"tun", "tun", "preflight_passed", false, false, "ebpf.status", "eBPF 能力预检通过"},
		{"raw", "tun", "preflight_passed", true, false, "ebpf.status", ""},
		{"inconclusive", "tun", "inconclusive", false, true, "ebpf.unsupported", "部分必要检查无法确认"},
		{"unsupported", "ebpf", "unsupported", false, true, "ebpf.unsupported", "缺少必要的 eBPF 能力"},
		{"nonzero", "tun", "preflight_passed", false, true, "ebpf.unsupported", "诊断命令未正常完成"},
		{"required-result-without-exit", "tun", "inconclusive", false, false, "ebpf.unsupported", "部分必要检查无法确认"},
		{"old-result", "ebpf", "supported", false, false, "ebpf.status_invalid", "无法读取 eBPF 诊断报告"},
		{"invalid-raw", "tun", "supported", true, false, "ebpf.status_invalid", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := `{"backend":"` + test.backend + `","app":{"enabled":false,"mode":"blacklist","proxy_apps":[],"bypass_apps":[]},"ebpf":{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true},"shared":{"enabled":false}},"tun":{"type":"tun","tag":"netproxy-in","interface_name":"netproxy","address":["172.19.0.1/30"],"auto_route":true,"auto_redirect":true}}`
			if err := os.WriteFile(options.InboundConfig, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			report := inbound.ProbeReport{Mode: "local", LocalDataPlane: "cgroup", Network: []string{"tcp", "udp"}, Preflight: true, ExactObjectLoad: true, Result: test.result}
			if test.result == "inconclusive" {
				report.Summary.RequiredUnknowns = 1
			} else if test.result == "unsupported" {
				report.Summary.RequiredFailures = 1
			}
			payload, err := json.Marshal(report, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("NETPROXY_TEST_PROBE_REPORT", string(payload))
			t.Setenv("NETPROXY_TEST_PROBE_FAIL", map[bool]string{true: "1", false: "0"}[test.fail])
			capture, err := os.CreateTemp(t.TempDir(), "stdout-")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			args := []string{"ebpf", "status", "configured"}
			if test.raw {
				args = append(args, "--raw")
			}
			previous := os.Stdout
			var status int
			func() {
				os.Stdout = capture
				defer func() { os.Stdout = previous }()
				status = command.run(t.Context(), args)
			}()
			output, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Schema int    `json:"schema"`
				OK     bool   `json:"ok"`
				Code   string `json:"code"`
				Data   struct {
					Raw     bool                `json:"raw"`
					Content string              `json:"content"`
					Report  inbound.ProbeReport `json:"report"`
				} `json:"data"`
			}
			if err := json.Unmarshal(output, &response); err != nil {
				t.Fatalf("stdout 不是单一 JSON: %v\n%s", err, output)
			}
			if response.Schema != 1 || response.Code != test.code || response.OK != (status == 0) || response.OK != (test.code == "ebpf.status") || response.Data.Raw != test.raw {
				t.Fatalf("诊断契约错误: exit=%d\n%s", status, output)
			}
			if test.raw {
				if response.Data.Content != string(payload) {
					t.Fatalf("显式 raw 没有保留原始报告: %s", output)
				}
			} else if !strings.Contains(response.Data.Content, test.want) || jsontext.Value(response.Data.Content).IsValid() {
				t.Fatalf("普通诊断没有返回可读正文: %s", output)
			}
			if test.code != "ebpf.status_invalid" && response.Data.Report.Result != test.result {
				t.Fatalf("机器报告丢失: %s", output)
			}
			if current, err := os.ReadFile(options.InboundConfig); err != nil || string(current) != config {
				t.Fatalf("诊断修改了保存的入站: %v\n%s", err, current)
			}
			if _, err := os.Stat(options.RuntimeDir); !os.IsNotExist(err) {
				t.Fatalf("诊断不应生成运行时或启动服务: %v", err)
			}
		})
	}
}
