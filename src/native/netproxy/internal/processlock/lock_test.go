package processlock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessLockHelper(t *testing.T) {
	if os.Getenv("NETPROXY_PROCESS_LOCK_HELPER") != "1" {
		return
	}
	path := os.Getenv("NETPROXY_PROCESS_LOCK_PATH")
	ready := os.Getenv("NETPROXY_PROCESS_LOCK_READY")
	lock, err := TryAcquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestTryAcquireRecoversAfterHolderExit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "operation.lock")
	ready := filepath.Join(root, "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestProcessLockHelper$")
	command.Env = append(os.Environ(),
		"NETPROXY_PROCESS_LOCK_HELPER=1",
		"NETPROXY_PROCESS_LOCK_PATH="+path,
		"NETPROXY_PROCESS_LOCK_READY="+ready,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		if !waited {
			_ = command.Wait()
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等待持锁子进程超时")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := TryAcquire(path); !errors.Is(err, ErrBusy) {
		t.Fatalf("跨进程竞争未返回 ErrBusy: %v", err)
	}

	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("被终止的持锁子进程意外成功退出")
	}
	waited = true

	lock, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("持锁进程退出后未释放 OS 锁: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}
