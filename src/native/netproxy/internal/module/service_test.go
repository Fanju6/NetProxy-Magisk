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
	// 完整的启动失败回滚由生命周期控制器在 Android 真机验证；
	// module.Check 本身只负责生成配置并调用 sing-box check，不持有服务状态。
}

func TestCheckServiceRejectsMissingBinary(t *testing.T) {
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
	options.StateFile = filepath.Join(root, "state", "service.json")
	options.SingBoxPath = filepath.Join(root, "missing-sing-box")

	if err := CheckService(context.Background(), options); err == nil {
		t.Fatal("缺少 sing-box 时配置检查应失败")
	}
}

func TestLifecycleLockRejectsConcurrentOperation(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "service.json")
	first, err := acquireLifecycleLock(stateFile, "start")
	if err != nil {
		t.Fatal(err)
	}
	defer first.release()
	if _, err := acquireLifecycleLock(stateFile, "reload"); err == nil {
		t.Fatal("并发服务操作应被锁拒绝")
	}
	first.release()
	second, err := acquireLifecycleLock(stateFile, "reload")
	if err != nil {
		t.Fatal(err)
	}
	second.release()
}

func TestWorkerOptionsKeepNetworkWatcherEnabled(t *testing.T) {
	options := workerOptions(NewOptions(t.TempDir()))
	if !options.NetworkWatchEnabled {
		t.Fatal("Worker 必须默认监听 Android 网络变化")
	}
	if options.NetworkEvaluate == nil {
		t.Fatal("Worker 必须配置网络策略评估回调")
	}
}

func TestPrepareDoesNotPersistSelectionBeforeCheck(t *testing.T) {
	root := t.TempDir()
	options := NewOptions(root)
	if err := os.MkdirAll(filepath.Join(options.SingBoxDir, "confdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(options.CatalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("ACTIVE_GROUP_ID=missing\nSELECTOR_MODE=manual\nSELECTED_NODE_REF=missing/node\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.EBPFConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.EBPFConfig, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Prepare(context.Background(), options, false); err == nil {
		t.Fatal("空 Catalog 应该拒绝生成运行时配置")
	}
	content, err := os.ReadFile(options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "ACTIVE_GROUP_ID=missing\nSELECTOR_MODE=manual\nSELECTED_NODE_REF=missing/node\n" {
		t.Fatalf("配置检查前不应修改选择状态: %s", content)
	}
}
