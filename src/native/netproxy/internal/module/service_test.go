package module

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServiceStartFailureReportsCheckError(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	options.StateFile = filepath.Join(root, "state", "service.json")
	if err := os.MkdirAll(filepath.Join(options.SingBoxDir, "confdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.EBPFConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.EBPFConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 使用不存在的核心路径，跨 Windows 与 Android 主机稳定模拟启动检查失败。
	options.SingBoxPath = filepath.Join(root, "missing-sing-box")
	_, err := Check(context.Background(), options, true)
	if err == nil || !strings.Contains(err.Error(), "sing-box 配置检查失败") {
		t.Fatalf("无效 sing-box 检查未返回明确错误: %v", err)
	}
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		if info, statErr := os.Stat(filepath.Join(options.RuntimeDir, name)); statErr != nil || info.Size() == 0 {
			t.Fatalf("配置检查失败前未生成可校验的运行时文件 %s: %v", name, statErr)
		}
	}
	// 完整的启动失败回滚由生命周期控制器在 Android 真机验证；
	// module.Check 本身只负责生成配置并调用 sing-box check，不持有服务状态。
}

func TestCheckServiceRejectsMissingBinary(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Join(options.SingBoxDir, "confdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.EBPFConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.EBPFConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.StateFile = filepath.Join(root, "state", "service.json")
	options.SingBoxPath = filepath.Join(root, "missing-sing-box")

	if err := CheckService(context.Background(), options); err == nil {
		t.Fatal("缺少 sing-box 时配置检查应失败")
	}
}

func TestLifecycleLockRejectsConcurrentOperation(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "service.json")
	first, err := acquireLifecycleLock(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer first.release()
	if _, err := acquireLifecycleLock(stateFile); err == nil {
		t.Fatal("并发服务操作应被锁拒绝")
	}
	first.release()
	second, err := acquireLifecycleLock(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(stateFile), "service.lock", "action")); !os.IsNotExist(err) {
		t.Fatalf("服务锁不应再创建无读取方的 action 文件: %v", err)
	}
	second.release()
}

func TestLifecycleLockRecoversReusedPIDMetadata(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "service.json")
	lockDir := filepath.Join(filepath.Dir(stateFile), "service.lock")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	lock, err := acquireLifecycleLock(stateFile)
	if err != nil {
		t.Fatalf("复用 PID 的残留服务锁不应阻塞新操作: %v", err)
	}
	lock.release()
}

func TestToggleServiceAction(t *testing.T) {
	tests := map[string]string{
		"":        "start",
		"stopped": "start",
		"failed":  "start",
		"ready":   "stop",
		"unknown": "start",
	}
	for state, want := range tests {
		t.Run(state, func(t *testing.T) {
			got, err := toggleServiceAction(state)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("切换状态 %q 得到 %q，期望 %q", state, got, want)
			}
		})
	}

	for _, state := range []string{"preparing", "starting", "stopping"} {
		t.Run(state+"-busy", func(t *testing.T) {
			if _, err := toggleServiceAction(state); err == nil {
				t.Fatalf("状态 %q 应拒绝重复切换", state)
			}
		})
	}
}

func TestWorkerOptionsKeepNetworkWatcherEnabled(t *testing.T) {
	options := workerOptions(newTestOptions(t.TempDir()))
	if !options.NetworkWatchEnabled {
		t.Fatal("Worker 必须默认监听 Android 网络变化")
	}
	if options.NetworkEvaluate == nil {
		t.Fatal("Worker 必须配置网络策略评估回调")
	}
}

func TestWriteServiceStateReportsAtomicReplaceFailure(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "service.json")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteServiceState(statePath, "ready", 123, 1, 2, ""); err == nil {
		t.Fatal("state write to a directory unexpectedly succeeded")
	}
}

func TestFailedServiceStartPreservesStateWriteError(t *testing.T) {
	original := writeServiceState
	stateErr := errors.New("state atomic replace failed")
	writeServiceState = func(string, string, int64, int64, int64, string) error { return stateErr }
	t.Cleanup(func() { writeServiceState = original })

	options := newTestOptions(t.TempDir())
	cause := errors.New("configuration check failed")
	err := failServiceStart(options, 0, 0, "service start failed", cause)
	if !errors.Is(err, cause) {
		t.Fatalf("start cause was lost: %v", err)
	}
	if !errors.Is(err, stateErr) {
		t.Fatalf("state write error was lost: %v", err)
	}
}

func TestStartStateWriteFailureStopsAndConvergesState(t *testing.T) {
	for _, test := range []struct {
		state       string
		failedError error
	}{
		{state: "starting"},
		{state: "ready"},
		{state: "starting", failedError: errors.New("failed state write failed")},
		{state: "ready", failedError: errors.New("failed state write failed")},
	} {
		name := test.state
		if test.failedError != nil {
			name += "-failed-write"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			options := newTestOptions(root)
			options.StateFile = filepath.Join(root, "state", "service.json")
			if err := WriteServiceState(options.StateFile, test.state, 42, 1, 2, ""); err != nil {
				t.Fatal(err)
			}

			stateWriteError := errors.New(test.state + " state write failed")
			stopError := errors.New("sing-box stop failed")
			originalWrite, originalTerminate := writeServiceState, terminateServiceForStart
			var states []string
			writeServiceState = func(path, state string, pid, startedAt, readyAt int64, message string) error {
				states = append(states, state)
				if state == "failed" {
					if test.failedError != nil {
						return test.failedError
					}
					return WriteServiceState(path, state, pid, startedAt, readyAt, message)
				}
				return stateWriteError
			}
			terminateServiceForStart = func(Options, int) error { return stopError }
			t.Cleanup(func() {
				writeServiceState = originalWrite
				terminateServiceForStart = originalTerminate
			})

			err := failServiceStateWrite(options, 42, 1, test.state, stateWriteError)
			for _, want := range []error{stateWriteError, stopError, test.failedError} {
				if want != nil && !errors.Is(err, want) {
					t.Fatalf("error %v was lost: %v", want, err)
				}
			}
			if len(states) != 1 || states[0] != "failed" {
				t.Fatalf("failed state was not attempted: %v", states)
			}
			state, readErr := ReadServiceState(options.StateFile)
			if test.failedError == nil {
				if readErr != nil || state.State != "failed" {
					t.Fatalf("service state did not converge to failed: %+v, %v", state, readErr)
				}
			} else if readErr != nil || state.State != "stopped" {
				t.Fatalf("stale state was not removed after failed-state write failure: %+v, %v", state, readErr)
			}
		})
	}
}

func TestStartServiceDoesNotReachReadyAfterStateWriteFailure(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	options.StateFile = filepath.Join(root, "state", "service.json")
	options.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.MkdirAll(filepath.Join(options.SingBoxDir, "confdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.SingBoxPath, []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.EBPFConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.EBPFConfig, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	original := writeServiceState
	stateErr := errors.New("preparing state write failed")
	readySeen := false
	writeServiceState = func(_ string, state string, _ int64, _ int64, _ int64, _ string) error {
		if state == "ready" {
			readySeen = true
		}
		if state == "preparing" {
			return stateErr
		}
		return nil
	}
	t.Cleanup(func() { writeServiceState = original })

	if err := StartService(context.Background(), options); !errors.Is(err, stateErr) {
		t.Fatalf("state write failure was not returned: %v", err)
	}
	if readySeen {
		t.Fatal("service reached ready after preparing state write failure")
	}
}

func TestStopServiceAttemptsFinalStateAfterStoppingWriteFailure(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	options.StateFile = filepath.Join(root, "state", "service.json")
	if err := os.MkdirAll(filepath.Dir(options.StateFile), 0o700); err != nil {
		t.Fatal(err)
	}
	var states []string
	original := writeServiceState
	stateErr := errors.New("stopping state write failed")
	writeServiceState = func(_ string, state string, _ int64, _ int64, _ int64, _ string) error {
		states = append(states, state)
		if state == "stopping" {
			return stateErr
		}
		return nil
	}
	t.Cleanup(func() { writeServiceState = original })

	if err := StopService(context.Background(), options); !errors.Is(err, stateErr) {
		t.Fatalf("stopping write error was not returned: %v", err)
	}
	if len(states) != 2 || states[0] != "stopping" || states[1] != "stopped" {
		t.Fatalf("stop did not continue state transition after write failure: %v", states)
	}
}

func TestPrepareDoesNotPersistSelectionBeforeCheck(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Join(options.SingBoxDir, "confdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("ACTIVE_GROUP_ID=missing\nSELECTOR_MODE=manual\nSELECTED_NODE_REF=missing/node\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.EBPFConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.EBPFConfig, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Prepare(context.Background(), options, false); err == nil {
		t.Fatal("空 Catalog 应该拒绝生成运行时配置")
	}
	content, err := os.ReadFile(options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "ACTIVE_GROUP_ID=missing\nSELECTOR_MODE=manual\nSELECTED_NODE_REF=missing/node\n" {
		t.Fatalf("配置检查前不应修改选择状态: %s", content)
	}
}
