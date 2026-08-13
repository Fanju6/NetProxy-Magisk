package processlock

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// ErrBusy 表示目标文件已被其他进程持有。
var ErrBusy = errors.New("跨进程锁正忙")

// Lock 表示由操作系统维护生命周期的非阻塞文件锁。
type Lock struct {
	file       *os.File
	once       sync.Once
	releaseErr error
}

// TryAcquire 尝试获取文件锁；进程退出时操作系统会自动释放锁。
func TryAcquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLockFile(file); err != nil {
		_ = file.Close()
		if lockFileBusy(err) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return &Lock{file: file}, nil
}

// Release 释放文件锁；重复调用不会重复解锁或关闭文件。
func (lock *Lock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	lock.once.Do(func() {
		lock.releaseErr = errors.Join(unlockFile(lock.file), lock.file.Close())
	})
	return lock.releaseErr
}
