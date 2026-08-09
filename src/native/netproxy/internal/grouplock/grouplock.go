// Package grouplock 提供进程内的 Catalog 分组锁。
package grouplock

import "sync"

var groupLocks sync.Map

// For 返回指定分组对应的进程内互斥锁。
func For(groupID string) *sync.Mutex {
	value, _ := groupLocks.LoadOrStore(groupID, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// Acquire 获取分组锁并返回释放函数。
func Acquire(groupID string) func() {
	mutex := For(groupID)
	mutex.Lock()
	return mutex.Unlock
}
