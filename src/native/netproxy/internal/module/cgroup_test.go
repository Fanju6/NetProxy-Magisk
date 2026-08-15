package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveProcessToRootCgroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cgroup.procs")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := moveProcessToRootCgroup(1234, path); err != nil {
		t.Fatalf("移动进程到 root cgroup 失败: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "1234\n" {
		t.Fatalf("root cgroup 收到的 PID 不正确: %q", content)
	}
}

func TestMoveProcessToRootCgroupSkipsUnavailableCgroupV2(t *testing.T) {
	if err := moveProcessToRootCgroup(1234, filepath.Join(t.TempDir(), "missing", "cgroup.procs")); err != nil {
		t.Fatalf("不存在 cgroup v2 控制文件时不应阻止启动: %v", err)
	}
}
