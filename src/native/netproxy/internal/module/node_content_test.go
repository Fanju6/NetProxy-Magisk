package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/sagernet/sing-box/option"
)

func TestNodeImportContentAppendsSupportedFormats(t *testing.T) {
	for name, content := range map[string]string{
		"link":   "socks://example.test:1080#IMPORTED",
		"base64": base64.StdEncoding.EncodeToString([]byte("socks://example.test:1080#IMPORTED")),
		"clash":  "proxies:\n  - {name: IMPORTED, type: socks5, server: example.test, port: 1080}\n",
		"json":   `{"outbounds":[{"type":"socks","tag":"IMPORTED","server":"example.test","server_port":1080}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			options := selectionFixture(t)
			result, err := NodeImportContent(t.Context(), options, []byte(content), false)
			if err != nil || result.GroupID != "default" || result.NodeCount != 3 || result.Revision != 3 || result.StructureChanged {
				t.Fatal("内容导入结果错误", result, err)
			}
			document, err := catalog.GroupProvider(t.Context(), options.CatalogRoot, "default")
			if err != nil {
				t.Fatal(err)
			}
			for _, tag := range []string{"NODE", "节点 / 02", "IMPORTED"} {
				if _, found := provider.Select(document, tag); !found {
					t.Fatal("导入覆盖了已有 tag", tag)
				}
			}
			for _, path := range []string{options.StateFile, options.WorkerPIDFile, options.RuntimeDir} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("停止时导入改变了运行状态", path, err)
				}
			}
		})
	}
}

func TestNodeImportContentAppliesAllowInsecure(t *testing.T) {
	options := selectionFixture(t)
	content := []byte(`{"outbounds":[{"type":"trojan","tag":"TLS","server":"example.test","server_port":443,"password":"fixture-password","tls":{"enabled":true}}]}`)
	if _, err := NodeImportContent(t.Context(), options, content, true); err != nil {
		t.Fatal(err)
	}
	document, err := catalog.GroupProvider(t.Context(), options.CatalogRoot, "default")
	if err != nil {
		t.Fatal(err)
	}
	selected, found := provider.Select(document, "TLS")
	if !found {
		t.Fatal("没有导入 TLS 节点")
	}
	tls := selected.Outbounds[0].Options.(*option.TrojanOutboundOptions).TLS
	if tls == nil || !tls.Insecure {
		t.Fatal("内容入口丢失 allowInsecure")
	}
}

func TestNodeContentDoesNotReadSystemPaths(t *testing.T) {
	options := selectionFixture(t)
	source := writeSectionSource(t, "socks://example.test:1080#FILE_ONLY")
	inputFile := writeSectionSource(t, source)
	providerPath := filepath.Join(options.CatalogRoot, "default", "provider.json")
	metadataPath := filepath.Join(options.CatalogRoot, "default", "meta.json")
	beforeProvider, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func() (catalog.MutationResult, error){
		"content-import": func() (catalog.MutationResult, error) {
			return NodeImportContent(t.Context(), options, []byte(source), false)
		},
		"empty-import": func() (catalog.MutationResult, error) { return NodeImportContent(t.Context(), options, nil, false) },
		"content-append": func() (catalog.MutationResult, error) {
			return NodeAppendContent(t.Context(), options, "default", []byte(source), false)
		},
		"content-edit": func() (catalog.MutationResult, error) {
			return NodeEditContent(t.Context(), options, "default/NODE", []byte(source), false)
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := mutate()
			var diagnostics *convert.DiagnosticsError
			if !errors.As(err, &diagnostics) || len(diagnostics.Diagnostics) == 0 || result.Revision != 0 {
				t.Fatal("系统路径被读取或缺失结构化诊断", result, err)
			}
			for path, before := range map[string][]byte{providerPath: beforeProvider, metadataPath: beforeMetadata} {
				if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, before) {
					t.Fatal("无效内容修改了 Catalog", path, err)
				}
			}
		})
	}
	if err := os.WriteFile(inputFile, []byte("socks://example.test:1080#PRIVATE_INPUT"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NodeAppend(t.Context(), options, "default", inputFile, false); err != nil {
		t.Fatal("私有内容文件无法安全添加节点", err)
	}
	if _, err := NodeEdit(t.Context(), options, "default/NODE", inputFile, false); err != nil {
		t.Fatal("私有内容文件无法安全编辑节点", err)
	}
}

func TestNodeImportContentWaitsForLifecycleLock(t *testing.T) {
	options := selectionFixture(t)
	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := NodeImportContent(ctx, options, []byte("socks://example.test:1080#WAIT"), false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("内容导入绕过生命周期锁或丢失取消", err)
	}
	document, err := catalog.GroupProvider(t.Context(), options.CatalogRoot, "default")
	if err != nil || len(document.Outbounds) != 2 {
		t.Fatal("等待锁时仍提交了节点", document, err)
	}
}

func TestNodeImportContentKeepsPersistedFailureSemantics(t *testing.T) {
	options := selectionFixture(t)
	if err := moduleconfig.UpdateSelection(t.Context(), options.ModuleConfig, moduleconfig.Selection{}); err != nil {
		t.Fatal(err)
	}
	result, err := nodeAppend(t.Context(), options, "default", "import", false, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		result, err := catalog.AppendNodeContent(t.Context(), mutationOptions, []byte("socks://example.test:1080#COMMITTED"))
		if err != nil {
			return result, err
		}
		if err := os.WriteFile(options.ModuleConfig, []byte(`{"broken":`), 0o600); err != nil {
			t.Fatal(err)
		}
		return result, nil
	})
	var persisted *service.Error
	if result.Revision != 3 || !errors.As(err, &persisted) || persisted.Code != "node.persisted_effect_failed" {
		t.Fatal("提交后错误语义发生变化", result, err)
	}
	data, ok := persisted.Data.(map[string]any)
	if !ok || data["persisted"] != true {
		t.Fatal("提交后未报告 persisted=true", persisted.Data)
	}
	document, err := catalog.GroupProvider(t.Context(), options.CatalogRoot, "default")
	if err != nil {
		t.Fatal(err)
	}
	if _, found := provider.Select(document, "COMMITTED"); !found {
		t.Fatal("本地收尾失败撤销了已提交节点")
	}
}

func TestNodeImportContentFinishesSelectionAfterCommitCancellation(t *testing.T) {
	options := selectionFixture(t)
	if err := moduleconfig.UpdateSelection(t.Context(), options.ModuleConfig, moduleconfig.Selection{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := nodeAppend(ctx, options, "default", "import", false, func(mutationOptions catalog.MutationOptions) (catalog.MutationResult, error) {
		result, err := catalog.AppendNodeContent(ctx, mutationOptions, []byte("socks://example.test:1080#COMMITTED"))
		cancel()
		return result, err
	})
	if err != nil || result.Revision != 3 {
		t.Fatal("取消导致已提交内容未完成本地收尾", result, err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.ActiveGroupID != "default" {
		t.Fatal("取消后没有保存完整选择", module, err)
	}
}

func TestWorkerOptionsIncludesAPIConfig(t *testing.T) {
	options := newTestOptions(t.TempDir())
	if got := workerOptions(options).APIConfig; got != paths.New(options.ModuleDir).APIConfig() {
		t.Fatal("后台宿主未取得 API 配置路径", got)
	}
}
