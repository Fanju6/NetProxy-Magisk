//go:build unix

package module

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestExportLogsPreservesExistingDestinationOwnership(t *testing.T) {
	options := NewOptions(t.TempDir())
	destination := filepath.Join(options.ModuleDir, "diagnostic.tar.gz")
	if err := os.WriteFile(destination, []byte("previous export"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExportLogs(options, destination); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	beforeStat, ok := before.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("无法读取导出前文件所有者")
	}
	afterStat, ok := after.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("无法读取导出后文件所有者")
	}
	if beforeStat.Uid != afterStat.Uid || beforeStat.Gid != afterStat.Gid {
		t.Fatalf("已有目标文件所有者未保留: %d:%d -> %d:%d", beforeStat.Uid, beforeStat.Gid, afterStat.Uid, afterStat.Gid)
	}
}
