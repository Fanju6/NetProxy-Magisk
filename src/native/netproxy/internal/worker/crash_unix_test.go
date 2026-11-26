//go:build !windows

package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetachedWorkerCapturesRuntimePanic(t *testing.T) {
	if gate := os.Getenv("NETPROXY_WORKER_TEST_CRASH_GATE"); gate != "" {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			if _, err := os.Stat(gate); err == nil {
				go func() { panic("worker crash diagnostic fixture") }()
				select {}
			}
		}
		t.Fatal("Worker test did not receive the crash trigger")
	}
	dir := t.TempDir()
	options := NewOptions(dir)
	options.PIDFile = filepath.Join(dir, "worker.pid")
	options.ModuleConfig = filepath.Join(dir, "module.json")
	options.APIConfig = filepath.Join(dir, "api.json")
	options.CrashLog = filepath.Join(dir, "logs", "worker-crash.log")
	gate := filepath.Join(dir, "crash")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NETPROXY_WORKER_TEST_EXECUTABLE", executable)
	t.Setenv("NETPROXY_WORKER_TEST_PID", options.PIDFile)
	t.Setenv("NETPROXY_WORKER_TEST_CRASH_GATE", gate)
	wrapper := filepath.Join(dir, "worker")
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
printf '%s\n' "$$" > "$NETPROXY_WORKER_TEST_PID"
exec "$NETPROXY_WORKER_TEST_EXECUTABLE" -test.run '^TestDetachedWorkerCapturesRuntimePanic$'
`), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(t.Context(), options, wrapper); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		data, err := os.ReadFile(options.CrashLog)
		if err == nil && strings.Contains(string(data), "panic: worker crash diagnostic fixture") &&
			strings.Contains(string(data), "TestDetachedWorkerCapturesRuntimePanic.func") {
			info, err := os.Stat(options.CrashLog)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("crash log mode = %o", info.Mode().Perm())
			}
			return
		}
	}
	t.Fatal("detached Worker's panic and traceback were not captured")
}
