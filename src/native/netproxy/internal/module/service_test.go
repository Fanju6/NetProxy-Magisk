package module

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceStartFailureReportsCheckError(t *testing.T) {
	root := t.TempDir()
	options := NewOptions(root)
	if err := os.MkdirAll(filepath.Join(options.SingBoxDir, "confdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.EBPFConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.EBPFConfig, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 使用不存在的核心路径，跨 Windows 与 Android 主机稳定模拟启动检查失败。
	options.SingBoxPath = filepath.Join(root, "missing-sing-box")
	_, err := Check(context.Background(), options, true)
	if err == nil || !strings.Contains(err.Error(), "sing-box 配置检查失败") {
		t.Fatalf("无效 sing-box 检查未返回明确错误: %v", err)
	}
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		if info, statErr := os.Stat(filepath.Join(options.RuntimeDir, name)); statErr != nil || info.Size() == 0 {
			t.Fatalf("配置检查失败前未生成可校验的运行时文件 %s: %v", name, statErr)
		}
	}
	// TODO: 完整的启动失败清理需要在 Android Shell 中驱动 service.sh，
	// module.Check 本身只负责生成配置并调用 sing-box check，不持有服务状态。
}
