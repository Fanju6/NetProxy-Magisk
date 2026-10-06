//go:build unix

package module

import (
	"fmt"
	"os"
	"syscall"
)

// applyExportTargetMetadata 在替换前把目标的访问身份赋给完整临时包，保证 Root 导出后调用方仍能读取结果。
func applyExportTargetMetadata(path string, target exportTarget) error {
	if target.info == nil {
		return os.Chmod(path, 0o600)
	}
	targetStat, ok := target.info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("无法读取诊断包目标所有者")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	currentStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("无法读取诊断包临时文件所有者")
	}
	if currentStat.Uid != targetStat.Uid || currentStat.Gid != targetStat.Gid {
		if err := os.Chown(path, int(targetStat.Uid), int(targetStat.Gid)); err != nil {
			return err
		}
	}
	return os.Chmod(path, target.info.Mode().Perm())
}
