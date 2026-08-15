package module

import (
	"errors"
	"fmt"
	"os"
)

const singBoxRootCgroupProcs = "/sys/fs/cgroup/cgroup.procs"

// ensureSingBoxRootCgroup 将 sing-box 放入 cgroup v2 根组，避免被应用组的冻结策略挂起。
func ensureSingBoxRootCgroup(pid int) error {
	return moveProcessToRootCgroup(pid, singBoxRootCgroupProcs)
}

func moveProcessToRootCgroup(pid int, procsPath string) error {
	if pid <= 0 {
		return fmt.Errorf("无效的 sing-box PID: %d", pid)
	}
	file, err := os.OpenFile(procsPath, os.O_WRONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("打开 root cgroup 控制文件失败: %w", err)
	}
	defer file.Close()
	if _, err := fmt.Fprintf(file, "%d\n", pid); err != nil {
		return fmt.Errorf("写入 root cgroup 失败: %w", err)
	}
	return nil
}
