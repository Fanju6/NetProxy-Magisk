package config

import (
	"context"
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

func TestModuleJSONDefaultsAndStrictValidation(t *testing.T) {
	got, err := ParseModule([]byte("{}"))
	if err != nil || !reflect.DeepEqual(got, DefaultModule()) {
		t.Fatalf("默认值不一致: %+v %v", got, err)
	}
	for _, content := range []string{
		"", "null", "[]", "true", "{}{}", "{",
		`{"auto_start":1}`, `{"auto_start":"true"}`, `{"auto_start":null}`,
		`{"auto_start":true,"auto_start":false}`, `{"AUTO_START":true}`,
		`{"unknown":1}`, `{"selection":null}`, `{"selection":[]}`,
		`{"selection":{"group_id":1}}`, `{"selection":{"node_tag":null}}`,
		`{"selection":{"group_id":"","node_tag":"NODE"}}`,
		`{"selection":{"node_tag":" "}}`, `{"selection":{"mode":"manual"}}`,
		`{"wifi":null}`, `{"wifi":[]}`, `{"wifi":{"enabled":"false"}}`,
		`{"wifi":{"proxy_on_non_wifi":0}}`, `{"wifi":{"mode":"off"}}`,
		`{"wifi":{"ssid_list":[]}}`, `{"wifi":{"blacklist":null}}`,
		`{"wifi":{"whitelist":[1]}}`, `{"wifi":{"blacklist":[""]}}`,
		`{"wifi":{"blacklist":["same","same"]}}`,
		`{"wifi":{"blacklist":["abcdefghijklmnopqrstuvwxyz0123456"]}}`,
		`{"wifi":{"blacklist":["escaped\nname"]}}`,
		`{"wifi":{"blacklist":["escaped\u007fname"]}}`,
		`{"wifi":{"blacklist":["same"],"blacklist":[]}}`,
		"AUTO_START=0\n",
		string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}),
	} {
		if _, err := ParseModule([]byte(content)); err == nil {
			t.Fatalf("无效配置被接受: %q", content)
		}
	}
}

func TestWiFiListsStrictAndLossless(t *testing.T) {
	names := []string{" Home ", "home", "办公,Wi-Fi", "Quote\"Wifi", "null"}
	module := DefaultModule()
	module.WiFi.Blacklist = names
	module.WiFi.Whitelist = []string{"independent"}
	content, err := json.Marshal(module, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseModule(content)
	if err != nil || !reflect.DeepEqual(got, module) {
		t.Fatalf("Wi-Fi 名称未原样保留: %+v %v", got, err)
	}
}

func TestSelectionUpdateKeepsOtherFieldsAndSkipsUnchangedWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.json")
	original := `{"auto_start":true,"selection":{"group_id":"default"},"wifi":{"mode":"whitelist","blacklist":["Home"],"whitelist":["Office"]}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if err := UpdateSelection(t.Context(), path, Selection{ActiveGroupID: "default"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	content, _ := os.ReadFile(path)
	if !os.SameFile(before, after) || string(content) != original {
		t.Fatal("无变更仍替换了原文件")
	}
	for _, tag := range []string{"香港 / 节点", ""} {
		if err := UpdateSelection(t.Context(), path, Selection{ActiveGroupID: "fixture", SelectedNodeTag: tag}); err != nil {
			t.Fatal(err)
		}
		got, err := LoadModule(path)
		if err != nil || got.WiFi.Enabled || got.WiFi.Mode != "whitelist" || !got.AutoStart || got.SelectedNodeTag != tag ||
			!reflect.DeepEqual(got.WiFi.Blacklist, []string{"Home"}) || !reflect.DeepEqual(got.WiFi.Whitelist, []string{"Office"}) {
			t.Fatalf("更新丢失其他字段: %+v %v", got, err)
		}
	}
}

func TestSelectionUpdateFailurePreservesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.json")
	original := `{"selection":{"group_id":"default"}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSelection(t.Context(), path, Selection{SelectedNodeTag: "NODE"}); err == nil {
		t.Fatal("接受了无效更新")
	}
	content, _ := os.ReadFile(path)
	if string(content) != original {
		t.Fatal("失败更新修改了原文件")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := UpdateSelection(ctx, path, Selection{ActiveGroupID: "default", SelectedNodeTag: "NODE"}); err == nil {
		t.Fatal("取消没有传播")
	}
	content, _ = os.ReadFile(path)
	if string(content) != original {
		t.Fatal("取消更新修改了原文件")
	}
}

func TestSelectionDerivesModeAndReference(t *testing.T) {
	for _, tag := range []string{"", "香港 / 🇭🇰 节点"} {
		selection := Selection{ActiveGroupID: "default", SelectedNodeTag: tag}
		mode, ref, group, node := "urltest", "", "Auto/本地配置", ""
		if tag != "" {
			mode, ref, group, node = "manual", "default/"+tag, "Select/本地配置", "本地配置/"+tag
		}
		runtimeGroup, runtimeNode := selection.RuntimeTargets("本地配置")
		if selection.Mode() != mode || selection.Ref() != ref || runtimeGroup != group || runtimeNode != node {
			t.Fatalf("选择派生不一致: %+v", selection)
		}
	}
}

func TestConfigLockHelper(t *testing.T) {
	if os.Getenv("NETPROXY_CONFIG_LOCK_HELPER") != "1" {
		return
	}
	lock, err := processlock.Acquire(context.Background(), os.Getenv("NETPROXY_CONFIG_LOCK_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if err := os.WriteFile(os.Getenv("NETPROXY_CONFIG_LOCK_READY"), []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestConfigLockRecoversAfterHolderExit(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "module.json.lock")
	ready := filepath.Join(root, "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestConfigLockHelper$")
	command.Env = append(os.Environ(),
		"NETPROXY_CONFIG_LOCK_HELPER=1",
		"NETPROXY_CONFIG_LOCK_PATH="+lockPath,
		"NETPROXY_CONFIG_LOCK_READY="+ready,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		if !waited {
			_ = command.Wait()
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等待配置锁持有进程超时")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("被终止的配置锁持有进程意外成功退出")
	}
	waited = true

	lock, err := processlock.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatalf("持锁进程退出后配置锁未恢复: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}
