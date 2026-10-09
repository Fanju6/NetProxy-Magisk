package config

import (
	"context"
	json "encoding/json/v2"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWiFiListsStrictAndLossless(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	names := []string{" Home ", "home", "办公,Wi-Fi", "Quote\"Wifi"}
	encoded, _ := json.Marshal(names)
	os.WriteFile(path, []byte("WIFI_SSID_BLACKLIST="+string(encoded)+"\nWIFI_SSID_WHITELIST=[\"independent\"]\n"), 0600)
	got, err := LoadModule(path)
	if err != nil || got.WiFiAutoSwitch || got.WiFiSSIDMode != "blacklist" || !reflect.DeepEqual(got.WiFiSSIDBlacklist, names) || !reflect.DeepEqual(got.WiFiSSIDWhitelist, []string{"independent"}) {
		t.Fatalf("%+v %v", got, err)
	}
	for _, line := range []string{
		"WIFI_AUTO_SWITCH=invalid", "WIFI_SSID_LIST=Home", "PROXY_ON_CELLULAR=1",
		"WIFI_SSID_MODE=off", "WIFI_SSID_MODE=invalid", "WIFI_SSID_BLACKLIST=null", "WIFI_SSID_WHITELIST=[1]",
		"WIFI_SSID_BLACKLIST=[\"\"]", "WIFI_SSID_BLACKLIST=[\"same\",\"same\"]",
		"WIFI_SSID_BLACKLIST=[\"abcdefghijklmnopqrstuvwxyz0123456\"]",
		"WIFI_SSID_BLACKLIST=[\"escaped\\nname\"]", "WIFI_SSID_BLACKLIST=[\"escaped\\u007fname\"]",
	} {
		os.WriteFile(path, []byte(line+"\n"), 0600)
		if _, err := LoadModule(path); err == nil {
			t.Fatalf("无效配置被接受: %s", line)
		}
	}
}

func TestWiFiSwitchKeepsModeAndLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	original := "WIFI_SSID_MODE=whitelist\nWIFI_SSID_BLACKLIST=[\"Home\"]\nWIFI_SSID_WHITELIST=[\"Office\"]\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"1", "false", "true", "0"} {
		if err := UpdateModule(t.Context(), path, map[string]string{"WIFI_AUTO_SWITCH": value}); err != nil {
			t.Fatal(err)
		}
		got, err := LoadModule(path)
		if err != nil || got.WiFiAutoSwitch != (value == "1" || value == "true") || got.WiFiSSIDMode != "whitelist" ||
			!reflect.DeepEqual(got.WiFiSSIDBlacklist, []string{"Home"}) || !reflect.DeepEqual(got.WiFiSSIDWhitelist, []string{"Office"}) {
			t.Fatalf("开关改写了模式或名单: %+v %v", got, err)
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

func TestReadStrictRejectsShellLikeInputAndDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	if err := os.WriteFile(path, []byte("AUTO_START=1\nAUTO_START=0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadStrict(path); err == nil {
		t.Fatal("expected duplicate key to fail")
	}

	if err := os.WriteFile(path, []byte("AUTO_START=$(id)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := ReadStrict(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["AUTO_START"] != "$(id)" {
		t.Fatalf("unexpected literal value: %#v", values)
	}
}

func TestLoadModuleDefaultsAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	content := "AUTO_START=0\nACTIVE_GROUP_ID=default\nSELECTED_NODE_TAG=\nWIFI_AUTO_SWITCH=1\nWIFI_SSID_MODE=whitelist\nWIFI_SSID_WHITELIST=[\"TestWiFi\"]\nPROXY_ON_NON_WIFI=0\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadModule(path)
	if err != nil {
		t.Fatal(err)
	}
	if !config.WiFiAutoSwitch || config.WiFiSSIDMode != "whitelist" || config.ProxyOnNonWiFi || len(config.WiFiSSIDWhitelist) != 1 {
		t.Fatalf("unexpected module config: %#v", config)
	}

	if err := os.WriteFile(path, []byte("UNKNOWN_OPTION=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModule(path); err == nil {
		t.Fatal("expected unknown module key to fail")
	}

	if err := os.WriteFile(path, []byte("OUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModule(path); err == nil {
		t.Fatal("接受了已移除的模块出站模式字段")
	}

	for _, old := range []string{"SELECTOR_MODE=urltest", "SELECTOR_MODE=manual", "SELECTED_NODE_REF=default/NODE"} {
		if err := os.WriteFile(path, []byte(old+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadModule(path); err == nil {
			t.Fatalf("旧选择字段不应继续被接受: %s", old)
		}
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
		if selection.Mode() != mode || selection.Ref() != ref || runtimeGroup != group || runtimeNode != node || len(selection.Updates()) != 2 {
			t.Fatalf("选择派生不一致: %+v", selection)
		}
	}
}

func TestLoadModuleRejectsManualNodeWithoutGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	for _, content := range []string{"ACTIVE_GROUP_ID=\nSELECTED_NODE_TAG=NODE\n", "SELECTED_NODE_TAG=\" \"\n"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadModule(path); err == nil {
			t.Fatalf("接受了无效手动选择: %s", content)
		}
	}
}

func TestUpdateModuleKeepsOriginalWhenCandidateIsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	original := "AUTO_START=0\nACTIVE_GROUP_ID=default\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateModule(context.Background(), path, map[string]string{"SELECTOR_MODE": "invalid"}); err == nil {
		t.Fatal("expected typed update to fail")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("invalid update changed original file: %q", content)
	}
}

func TestConfigLockRecoversAfterHolderExit(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "module.conf.lock")
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

func TestUpdateWhitespaceKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	if err := os.WriteFile(path, []byte(" ACTIVE_GROUP_ID = \"default\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModule(path); err != nil {
		t.Fatalf("initial valid config rejected: %v", err)
	}
	if err := UpdateModule(t.Context(), path, map[string]string{"ACTIVE_GROUP_ID": Quote("fixture")}); err != nil {
		t.Fatalf("read accepted whitespace, but update failed: %v", err)
	}
	updated, err := LoadModule(path)
	if err != nil || updated.ActiveGroupID != "fixture" {
		t.Fatalf("带空格的键未正确更新: %+v %v", updated, err)
	}
}
