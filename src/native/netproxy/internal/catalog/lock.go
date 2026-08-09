package catalog

import (
	"strings"
	"sync"
)

// lockGroup 返回 Catalog 分组对应的进程内锁。
func lockGroup(groupID string) *sync.Mutex {
	return For(groupID)
}

func recoverTransactions(root string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	return Recover(root)
}
