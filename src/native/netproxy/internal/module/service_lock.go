package module

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

type lifecycleLock struct {
	path  string
	pid   int
	guard *processlock.Lock
}

func acquireLifecycleLock(stateFile string) (*lifecycleLock, error) {
	if strings.TrimSpace(stateFile) == "" {
		return nil, errors.New("服务状态文件路径不能为空")
	}
	path := filepath.Join(filepath.Dir(stateFile), "service.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// OS 锁必须位于可回收的 PID 目录之外，否则崩溃清理会制造两个独立锁入口。
	guard, err := processlock.TryAcquire(path + ".flock")
	if errors.Is(err, processlock.ErrBusy) {
		return nil, errors.New("已有服务操作正在执行")
	}
	if err != nil {
		return nil, err
	}
	lock := &lifecycleLock{path: path, pid: os.Getpid(), guard: guard}
	if err := lock.create(); err != nil {
		_ = guard.Release()
		return nil, err
	}
	return lock, nil
}

func (lock *lifecycleLock) create() error {
	if err := os.RemoveAll(lock.path); err != nil {
		return err
	}
	if err := os.Mkdir(lock.path, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(lock.path, "pid"), []byte(strconv.Itoa(lock.pid)+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(lock.path)
		return err
	}
	return nil
}

func (lock *lifecycleLock) release() {
	_ = os.RemoveAll(lock.path)
	_ = lock.guard.Release()
}
