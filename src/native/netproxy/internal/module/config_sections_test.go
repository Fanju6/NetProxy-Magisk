package module

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSectionSource(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "section.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigSectionEditsPreserveOtherSections(t *testing.T) {
	options, destination, _, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	initial := `{"dns":{"final":"old"},"route":{"rules":[{"port":80},{"port":443}]},"custom":{"number":9007199254740993}}`
	if err := os.WriteFile(destination, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	dns, err := ReadConfig(options, "singbox/dns")
	if err != nil {
		t.Fatal(err)
	}
	route, err := ReadConfig(options, "singbox/route")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dns["content"], "route") {
		t.Fatal("DNS 编辑内容泄漏了其他分区")
	}
	if _, err := ApplyConfig(context.Background(), options, "singbox/route", writeSectionSource(t, `{"route":{"final":"Proxy","rules":[{"port":443},{"port":80}]}}`), false, route["revision"]); err != nil {
		t.Fatal(err)
	}
	revision, err := ApplyConfig(context.Background(), options, "singbox/dns", writeSectionSource(t, `{"dns":{"final":"new","servers":[{"tag":"\u006eew","type":"local"}],"rules":[{"domain":["example.test"],"server":"new"}]}}`), false, dns["revision"])
	if err != nil {
		t.Fatalf("不同分区的更新不应互相冲突: %v", err)
	}
	read, err := ReadConfig(options, "singbox/dns")
	if err != nil || read["revision"] != revision {
		t.Fatalf("保存与读取的版本不一致: %v, %v", read, err)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	object, err := configObject(content)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(object["custom"], []byte("9007199254740993")) || !bytes.Contains(object["route"], []byte(`"Proxy"`)) {
		t.Fatalf("修改分区覆盖其他配置或丢失数字精度: %s", content)
	}
	if bytes.Index(object["route"], []byte("443")) > bytes.Index(object["route"], []byte("80")) {
		t.Fatal("路由规则顺序发生变化")
	}
	if _, err := ApplyConfig(context.Background(), options, "singbox/dns", writeSectionSource(t, `{"dns":{"final":"stale"}}`), false, dns["revision"]); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("过期 DNS 编辑必须冲突: %v", err)
	}
	if _, err := ApplyConfig(context.Background(), options, "singbox/dns", writeSectionSource(t, `{"dns":{"final":"latest"}}`), false, revision); err != nil {
		t.Fatalf("保存返回的版本必须能继续编辑: %v", err)
	}
	assertRuntimeContent(t, options, runtimeContent)
}

func TestConfigSectionValidateRollbackAndDelete(t *testing.T) {
	options, destination, _, runtimeContent := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	initial := []byte(`{"dns":{"final":"old"},"route":{"final":"Proxy"}}`)
	if err := os.WriteFile(destination, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	source := writeSectionSource(t, `{"dns":{"final":"new"}}`)
	if _, err := ApplyConfig(context.Background(), options, "singbox/dns", source, true, ""); err != nil {
		t.Fatal(err)
	}
	withFakeSingBoxResult(t, true, func() {
		if _, err := ApplyConfig(context.Background(), options, "singbox/dns", source, false, ""); err == nil {
			t.Fatal("完整配置检查失败不能保存分区")
		}
	})
	content, _ := os.ReadFile(destination)
	if !bytes.Equal(content, initial) {
		t.Fatalf("预检查或检查失败修改了主配置: %s", content)
	}
	assertRuntimeContent(t, options, runtimeContent)
	if _, err := ApplyConfig(context.Background(), options, "singbox/dns", writeSectionSource(t, `{}`), false, ""); err != nil {
		t.Fatal(err)
	}
	missing, _ := ReadConfig(options, "singbox/dns")
	if missing["content"] != "{}" {
		t.Fatalf("空分区应删除字段: %s", missing["content"])
	}
	if _, err := ApplyConfig(context.Background(), options, "singbox/dns", writeSectionSource(t, `{"dns":null}`), false, missing["revision"]); err != nil {
		t.Fatal(err)
	}
	null, _ := ReadConfig(options, "singbox/dns")
	if null["revision"] == missing["revision"] {
		t.Fatal("缺失字段与 null 必须具有不同版本")
	}
	configProcessRunning = func(string) bool { return true }
	configReload = func(context.Context, Options) error { return errors.New("reload failed") }
	configRestoreReload = func(context.Context, Options, configApplyJournal) error { return nil }
	if _, err := ApplyConfig(context.Background(), options, "singbox/dns", source, false, null["revision"]); err == nil {
		t.Fatal("reload 失败应返回错误")
	}
	after, _ := ReadConfig(options, "singbox/dns")
	if after["revision"] != null["revision"] {
		t.Fatal("reload 失败未恢复原分区")
	}
}

func TestConfigSectionRejectsInvalidOrForeignFields(t *testing.T) {
	for _, source := range []string{`[]`, `null`, `{"dns":{},"route":{}}`, `{"dns":{},"dns":{}}`, `{"dns":`} {
		t.Run(source, func(t *testing.T) {
			options, destination, _, _ := configApplyOptions(t)
			isolateConfigApplyHooks(t, false)
			before, _ := os.ReadFile(destination)
			if _, err := ApplyConfig(context.Background(), options, "singbox/dns", writeSectionSource(t, source), false, ""); err == nil {
				t.Fatalf("接受了非法分区: %s", source)
			}
			after, _ := os.ReadFile(destination)
			if !bytes.Equal(before, after) {
				t.Fatal("非法输入修改了文件")
			}
		})
	}
}

func TestConfigListAndFullDocumentConflict(t *testing.T) {
	options, destination, source, _ := configApplyOptions(t)
	isolateConfigApplyHooks(t, false)
	if err := os.WriteFile(destination, []byte(`{"ntp":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	documents, err := ListConfigs(options)
	if err != nil {
		t.Fatal(err)
	}
	foundFull, foundDNS, foundNTP := false, false, false
	for _, document := range documents {
		foundFull = foundFull || document.ID == "singbox/config.json" && document.Section == ""
		foundDNS = foundDNS || document.ID == "singbox/dns" && document.Section == "dns"
		foundNTP = foundNTP || document.ID == "singbox/ntp" && document.Section == "ntp"
		if document.ID == "singbox/providers" {
			t.Fatal("缺失的可选分区不应出现在列表中")
		}
		if document.Editable {
			if _, err := ReadConfig(options, document.ID); err != nil {
				t.Fatalf("列表目标不可读: %s: %v", document.ID, err)
			}
		}
	}
	if !foundFull || !foundDNS || !foundNTP {
		t.Fatalf("缺少完整配置或分区入口: %+v", documents)
	}
	full, _ := ReadConfig(options, "singbox/config.json")
	if _, err := ApplyConfig(context.Background(), options, "singbox/dns", writeSectionSource(t, `{"dns":{}}`), false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, full["revision"]); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("完整编辑必须检测其他分区更新: %v", err)
	}
	if err := os.WriteFile(destination, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListConfigs(options); err != nil {
		t.Fatalf("损坏配置仍应提供完整编辑入口: %v", err)
	}
	broken, err := ReadConfig(options, "singbox/config.json")
	if err != nil {
		t.Fatalf("完整编辑必须允许读取损坏文件进行修复: %v", err)
	}
	if _, err := ApplyConfig(context.Background(), options, "singbox/config.json", source, false, broken["revision"]); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"dns", "singbox/unknown", "singbox/../config.json", "singbox/rules/remote/test.json", "singbox/dns/route"} {
		if _, err := ReadConfig(options, target); err == nil {
			t.Fatalf("接受了非法配置目标: %s", target)
		}
	}
}
