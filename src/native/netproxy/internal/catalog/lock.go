package catalog

import (
	"strings"
	"sync"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalogtxn"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/grouplock"
)

// lockGroup 返回 Catalog 分组对应的进程内锁。
func lockGroup(groupID string) *sync.Mutex {
	return grouplock.For(groupID)
}

func recoverTransactions(root string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	return catalogtxn.Recover(root)
}
