package module

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func ebpfProbeFixture(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	options := NewOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.InboundConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.InboundConfig, []byte(testInboundConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return options
}

func TestProbeEBPFReport(t *testing.T) {
	options := ebpfProbeFixture(t)
	binary := filepath.Join(t.TempDir(), "fake-core")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if output, err := exec.Command("go", "build", "-o", binary, "../inbound/testdata/fake-package").CombinedOutput(); err != nil {
		t.Fatalf("构建诊断 fixture 失败: %v\n%s", err, output)
	}
	options.SingBoxPath = binary
	t.Setenv("NETPROXY_TEST_COMMAND_MODE", "probe")
	for _, test := range []struct {
		name, backend, result, code string
		raw, fail                   bool
	}{
		{"passed", "ebpf", "preflight_passed", "", false, false},
		{"tun", "tun", "preflight_passed", "", false, false},
		{"raw", "tun", "preflight_passed", "", true, false},
		{"unsupported", "ebpf", "unsupported", "ebpf.unsupported", false, true},
		{"inconclusive", "tun", "inconclusive", "ebpf.unsupported", false, false},
		{"nonzero", "ebpf", "preflight_passed", "ebpf.unsupported", false, true},
		{"unsupported-raw", "ebpf", "unsupported", "ebpf.unsupported", true, true},
		{"invalid", "tun", "supported", "ebpf.status_invalid", false, false},
		{"invalid-nonzero", "tun", "supported", "ebpf.status_invalid", false, true},
		{"invalid-raw", "ebpf", "supported", "ebpf.status_invalid", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := strings.Replace(testInboundConfig, `"backend": "ebpf"`, `"backend": "`+test.backend+`"`, 1)
			if err := os.WriteFile(options.InboundConfig, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			report := inbound.ProbeReport{Mode: "local", LocalDataPlane: "cgroup", Network: []string{"tcp", "udp"}, Findings: []inbound.ProbeFinding{}, Preflight: true, ExactObjectLoad: true, Result: test.result}
			payload, err := json.Marshal(report, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("NETPROXY_TEST_PROBE_REPORT", string(payload))
			t.Setenv("NETPROXY_TEST_PROBE_FAIL", map[bool]string{false: "0", true: "1"}[test.fail])
			data, err := ProbeEBPF(t.Context(), options, "configured", test.raw)
			var probeErr error
			if test.fail {
				probeErr = errors.New("exit status 1: capability probe failed")
			}
			if test.code != "" {
				structured, ok := errors.AsType[*service.Error](err)
				if !ok || structured.Code != test.code || data != nil {
					t.Fatalf("结构化错误丢失: data=%#v err=%v", data, err)
				}
				data = structured.Data.(map[string]any)
				if test.code == "ebpf.status_invalid" {
					_, parseErr := inbound.ParseProbeReport(string(payload))
					content := "无法读取 eBPF 诊断报告，请检查内核版本与核心日志。"
					if probeErr != nil {
						content = "eBPF 诊断命令未能完成，请检查核心文件与 Root 权限。\n" + probeErr.Error()
					}
					if test.raw {
						content = string(payload)
					}
					if structured.Message != parseErr.Error() || !reflect.DeepEqual(data, map[string]any{"raw": test.raw, "content": content}) {
						t.Fatalf("解析失败契约变化: %#v", structured)
					}
				} else if structured.Message != "eBPF 能力检查未通过" {
					t.Fatalf("能力失败消息变化: %#v", structured)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if test.code != "ebpf.status_invalid" {
				content := inbound.FormatProbeOutput(report, probeErr)
				if test.raw {
					content = string(payload)
				}
				if !reflect.DeepEqual(data, map[string]any{"mode": "configured", "raw": test.raw, "content": content, "report": report}) {
					t.Fatalf("诊断数据变化: %#v", data)
				}
			}
			if saved, err := os.ReadFile(options.InboundConfig); err != nil || string(saved) != config {
				t.Fatalf("诊断修改了模板: %v", err)
			}
			if _, err := os.Stat(options.RuntimeDir); !os.IsNotExist(err) {
				t.Fatalf("诊断生成了运行时: %v", err)
			}
		})
	}
	t.Run("cancel-during-probe", func(t *testing.T) {
		t.Setenv("NETPROXY_TEST_COMMAND_MODE", "")
		started := filepath.Join(t.TempDir(), "started")
		t.Setenv("NETPROXY_PACKAGE_STARTED", started)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := ProbeEBPF(ctx, options, "configured", false)
			done <- err
		}()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()
		for {
			if _, err := os.Stat(started); err == nil {
				break
			}
			select {
			case err := <-done:
				t.Fatalf("诊断提前退出: %v", err)
			case <-timeout.C:
				t.Fatal("伪核心未启动")
			case <-ticker.C:
			}
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("运行中取消被解析错误覆盖: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("取消未结束诊断进程")
		}
	})
}

func TestProbeEBPFErrorsAndCancellation(t *testing.T) {
	options := ebpfProbeFixture(t)
	data, err := ProbeEBPF(t.Context(), options, "invalid", false)
	structured, ok := errors.AsType[*service.Error](err)
	if !ok || data != nil || structured.Code != "ebpf.status_failed" || structured.Message != "eBPF 检查范围无效: invalid" || structured.Data != nil {
		t.Fatalf("范围错误契约变化: data=%#v err=%v", data, err)
	}
	options.SingBoxPath = ""
	for _, raw := range []bool{false, true} {
		data, err := ProbeEBPF(t.Context(), options, "configured", raw)
		structured, ok := errors.AsType[*service.Error](err)
		content := "eBPF 诊断命令未能完成，请检查核心文件与 Root 权限。\nsing-box 路径为空"
		if raw {
			content = ""
		}
		if !ok || data != nil || structured.Code != "ebpf.status_invalid" || structured.Message != "sing-box 未返回 eBPF JSON 诊断报告" || !reflect.DeepEqual(structured.Data, map[string]any{"raw": raw, "content": content}) {
			t.Fatalf("核心不可执行契约变化: data=%#v err=%v", data, err)
		}
	}
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		want := context.Canceled
		if expired {
			cancel()
			ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			cancel()
		}
		data, err := ProbeEBPF(ctx, options, "configured", false)
		cancel()
		if data != nil || !errors.Is(err, want) {
			t.Fatalf("取消被诊断错误覆盖: data=%#v err=%v want=%v", data, err, want)
		}
	}
	if err := os.Remove(options.InboundConfig); err != nil {
		t.Fatal(err)
	}
	if data, err := ProbeEBPF(t.Context(), options, "configured", false); data != nil || !os.IsNotExist(err) {
		t.Fatalf("加载错误被包装: data=%#v err=%v", data, err)
	}
	if err := os.WriteFile(options.InboundConfig, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := ProbeEBPF(t.Context(), options, "configured", false); data != nil {
		t.Fatalf("无效配置返回了诊断数据: %#v", data)
	} else if _, ok := errors.AsType[*inbound.ValidationError](err); !ok {
		t.Fatalf("配置诊断错误被包装: %v", err)
	}
}
