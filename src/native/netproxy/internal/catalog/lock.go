package catalog

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	catalogRootLocks  sync.Map
	catalogGroupLocks sync.Map
)

// AcquireRoot 获取 Catalog 根级跨进程锁。调用方必须在释放函数返回前完成读取、恢复或提交。
func AcquireRoot(root string) (func(), error) {
	return acquireFileLock(root, "root", &catalogRootLocks)
}

// Acquire 获取指定 Catalog 分组的跨进程逻辑锁。
// 分组锁只保护同组的长操作，访问磁盘前仍必须按“分组锁 -> 根锁”获取根锁。
func Acquire(root, groupID string) (func(), error) {
	if strings.TrimSpace(groupID) == "" || !isValidGroupID(groupID) {
		return nil, fmt.Errorf("非法 Catalog 分组 ID: %s", groupID)
	}
	return acquireFileLock(root, "group-"+groupID, &catalogGroupLocks)
}

func acquireCatalogMutation(root, groupID string) (func(), error) {
	groupRelease, err := Acquire(root, groupID)
	if err != nil {
		return nil, err
	}
	rootRelease, err := acquireCatalogRootAndRecover(root)
	if err != nil {
		groupRelease()
		return nil, err
	}
	return func() {
		rootRelease()
		groupRelease()
	}, nil
}

func acquireCatalogRootAndRecover(root string) (func(), error) {
	release, err := AcquireRoot(root)
	if err != nil {
		return nil, err
	}
	if err := recoverTransactionsLocked(root); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func acquireFileLock(root, scope string, locks *sync.Map) (func(), error) {
	path, err := catalogLockPath(root, scope)
	if err != nil {
		return nil, err
	}
	value, _ := locks.LoadOrStore(path, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		mutex.Unlock()
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		mutex.Unlock()
		return nil, err
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		mutex.Unlock()
		return nil, err
	}
	if err := writeLockOwner(file); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		mutex.Unlock()
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			_ = unlockFile(file)
			_ = file.Close()
			mutex.Unlock()
		})
	}, nil
}

func catalogLockPath(root, scope string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("Catalog 根目录不能为空")
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(absRoot + "\x00" + scope))
	base := filepath.Base(absRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "catalog"
	}
	return filepath.Join(filepath.Dir(absRoot), "."+base+".netproxy-"+hex.EncodeToString(digest[:8])+".lock"), nil
}

func writeLockOwner(file *os.File) error {
	content := lockOwnerContent()
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		return err
	}
	return file.Sync()
}

func lockOwnerContent() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		copy(token[:], fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	return fmt.Sprintf("pid=%d\nprocess=%s\ncreated_at=%s\ntoken=%s\n",
		os.Getpid(), filepath.Base(os.Args[0]), time.Now().UTC().Format(time.RFC3339Nano), hex.EncodeToString(token[:]))
}

func lockOwnerJournal() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		copy(token[:], fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	return fmt.Sprintf("owner_pid=%d\nowner_process=%s\nowner_created_at=%s\nowner_token=%s\n",
		os.Getpid(), filepath.Base(os.Args[0]), time.Now().UTC().Format(time.RFC3339Nano), hex.EncodeToString(token[:]))
}
