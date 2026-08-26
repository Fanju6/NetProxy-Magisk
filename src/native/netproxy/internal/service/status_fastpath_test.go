package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func writeStatusModule(t *testing.T, path, activeID, selector, selected string) {
	t.Helper()
	content := "OUTBOUND_MODE=rule\nSELECTOR_MODE=" + selector + "\nACTIVE_GROUP_ID=" + activeID + "\nSELECTED_NODE_REF=\"" + selected + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeStatusGroup(t *testing.T, root, groupID, name string, nodeCount int, providerContent []byte) {
	t.Helper()
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := catalog.NewMetadata(groupID, name, "local", "", time.Now())
	metadata.NodeCount = nodeCount
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), providerContent, 0o600); err != nil {
		t.Fatal(err)
	}
}

func statusFixtureProvider(tag string) []byte {
	return []byte(`{"outbounds":[{"type":"socks","tag":"` + tag + `","server":"example.com","server_port":1080}]}`)
}

func statusOptions(root, moduleConfig string) Options {
	return Options{
		CatalogRoot:  root,
		ModuleConfig: moduleConfig,
		SingBoxPath:  filepath.Join(root, "missing-sing-box"),
	}
}

func TestReadStatusFastPathSkipsInactiveProvider(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusGroup(t, catalogRoot, "default", "本地配置", 2, statusFixtureProvider("ACTIVE"))
	writeStatusGroup(t, catalogRoot, "remote", "远程订阅", 9000, bytes.Repeat([]byte("x"), 1<<20))
	writeStatusModule(t, moduleConfig, "default", "manual", "default/ACTIVE")

	status, err := ReadStatus(context.Background(), statusOptions(catalogRoot, moduleConfig))
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveGroupName != "本地配置" || status.ActiveGroupNodeCount != 2 || status.SelectedNodeRef != "default/ACTIVE" {
		t.Fatalf("活动分组摘要错误: %#v", status)
	}
	if strings.Contains(status.Error, "remote") {
		t.Fatalf("非活动分组 Provider 不应影响 status: %#v", status)
	}
}

func TestReadGroupSummaryKeepsRuntimeTagDisambiguation(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusGroup(t, catalogRoot, "default", "同名分组", 1, statusFixtureProvider("LOCAL"))
	writeStatusGroup(t, catalogRoot, "remote", "同名分组", 1, statusFixtureProvider("REMOTE"))

	summary, err := catalog.ReadGroupSummary(context.Background(), catalogRoot, "default", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.RuntimeTag != "同名分组 [default]" || summary.NodeCount != 1 {
		t.Fatalf("RuntimeTag 或摘要节点数错误: %#v", summary)
	}
}

func TestReadStatusStoppedReadsCatalogSummary(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusGroup(t, catalogRoot, "default", "本地配置", 1, statusFixtureProvider("NODE"))
	writeStatusModule(t, moduleConfig, "default", "urltest", "")

	status, err := ReadStatus(context.Background(), statusOptions(catalogRoot, moduleConfig))
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || status.ActiveGroupID != "default" ||
		status.ActiveGroupName != "本地配置" || status.ActiveGroupNodeCount != 1 {
		t.Fatalf("服务停止时未读取 Catalog 摘要: %#v", status)
	}
}

func TestReadStatusEmptyCatalogDegradesClearly(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusModule(t, moduleConfig, "default", "urltest", "")

	status, err := ReadStatus(context.Background(), statusOptions(catalogRoot, moduleConfig))
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveGroupName != "default" || status.ActiveGroupNodeCount != 0 ||
		!strings.Contains(status.Error, "活动分组不存在") {
		t.Fatalf("空 Catalog 未返回明确降级结果: %#v", status)
	}
}

func TestReadStatusMissingActiveGroupDoesNotShowOtherGroup(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusGroup(t, catalogRoot, "remote", "远程订阅", 4, statusFixtureProvider("REMOTE"))
	writeStatusModule(t, moduleConfig, "default", "urltest", "")

	status, err := ReadStatus(context.Background(), statusOptions(catalogRoot, moduleConfig))
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveGroupName != "default" || status.ActiveGroupNodeCount != 0 ||
		!strings.Contains(status.Error, "活动分组不存在") {
		t.Fatalf("活动分组缺失时错误显示其他分组: %#v", status)
	}
}

func TestReadStatusUsesMetadataWithoutParsingActiveProvider(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusGroup(t, catalogRoot, "default", "本地配置", 3, []byte(`{"outbounds":[`))
	writeStatusModule(t, moduleConfig, "default", "urltest", "")

	status, err := ReadStatus(context.Background(), statusOptions(catalogRoot, moduleConfig))
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveGroupName != "本地配置" || status.ActiveGroupNodeCount != 3 || status.Error != "" {
		t.Fatalf("状态轮询不应解析活动 Provider: %#v", status)
	}
}

func TestReadStatusActiveGroupSwitchIsVisibleImmediately(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusGroup(t, catalogRoot, "default", "本地配置", 1, statusFixtureProvider("LOCAL"))
	writeStatusGroup(t, catalogRoot, "remote", "远程订阅", 5, statusFixtureProvider("REMOTE"))
	writeStatusModule(t, moduleConfig, "default", "urltest", "")
	options := statusOptions(catalogRoot, moduleConfig)

	status, err := ReadStatus(context.Background(), options)
	if err != nil || status.ActiveGroupName != "本地配置" || status.ActiveGroupNodeCount != 1 {
		t.Fatalf("初始活动分组错误: %#v, err=%v", status, err)
	}
	writeStatusModule(t, moduleConfig, "remote", "manual", "remote/REMOTE")
	status, err = ReadStatus(context.Background(), options)
	if err != nil || status.ActiveGroupName != "远程订阅" || status.ActiveGroupNodeCount != 5 ||
		status.SelectorMode != "manual" || status.SelectedNodeRef != "remote/REMOTE" {
		t.Fatalf("活动分组切换未立即可见: %#v, err=%v", status, err)
	}
}
