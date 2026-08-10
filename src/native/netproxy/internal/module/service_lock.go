package module

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type lifecycleLock struct {
	path string
	pid  int
}

func acquireLifecycleLock(stateFile, action string) (*lifecycleLock, error) {
	if strings.TrimSpace(stateFile) == "" {
		return nil, errors.New("服务状态文件路径不能为空")
	}
	path := filepath.Join(filepath.Dir(stateFile), "service.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	lock := &lifecycleLock{path: path, pid: os.Getpid()}
	if err := lock.create(action); err == nil {
		return lock, nil
	} else if !os.IsExist(err) {
		return nil, err
	}

	if lifecycleLockAlive(path) {
		return nil, errors.New("已有服务操作正在执行")
	}
	stale := path + ".stale." + strconv.Itoa(os.Getpid())
	_ = os.RemoveAll(stale)
	if err := os.Rename(path, stale); err != nil {
		if os.IsNotExist(err) {
			return acquireLifecycleLock(stateFile, action)
		}
		return nil, fmt.Errorf("清理残留服务锁失败: %w", err)
	}
	_ = os.RemoveAll(stale)
	if err := lock.create(action); err != nil {
		return nil, err
	}
	return lock, nil
}

func (lock *lifecycleLock) create(action string) error {
	if err := os.Mkdir(lock.path, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(lock.path, "pid"), []byte(strconv.Itoa(lock.pid)+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(lock.path)
		return err
	}
	if err := os.WriteFile(filepath.Join(lock.path, "action"), []byte(action+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(lock.path)
		return err
	}
	return nil
}

func (lock *lifecycleLock) release() {
	content, err := os.ReadFile(filepath.Join(lock.path, "pid"))
	if err != nil || strings.TrimSpace(string(content)) != strconv.Itoa(lock.pid) {
		return
	}
	_ = os.RemoveAll(lock.path)
}

func lifecycleLockAlive(path string) bool {
	content, err := os.ReadFile(filepath.Join(path, "pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return false
	}
	return pid == os.Getpid() || serviceProcessAlive(pid)
}
