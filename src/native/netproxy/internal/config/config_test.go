package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigLockHelper(t *testing.T) {
	if os.Getenv("NETPROXY_CONFIG_LOCK_HELPER") != "1" {
		return
	}
	lock, err := acquireLock(os.Getenv("NETPROXY_CONFIG_LOCK_PATH"))
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
	content := "AUTO_START=0\nOUTBOUND_MODE=AllowAds\nSELECTOR_MODE=urltest\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\nWIFI_AUTO_SWITCH=1\nWIFI_SSID_MODE=whitelist\nWIFI_SSID_LIST=TestWiFi\nPROXY_ON_CELLULAR=0\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadModule(path)
	if err != nil {
		t.Fatal(err)
	}
	if !config.WiFiAutoSwitch || config.OutboundMode != "AllowAds" || config.WiFiSSIDMode != "whitelist" || config.ProxyOnCellular {
		t.Fatalf("unexpected module config: %#v", config)
	}

	if err := os.WriteFile(path, []byte("UNKNOWN_OPTION=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModule(path); err == nil {
		t.Fatal("expected unknown module key to fail")
	}

	for _, selector := range []string{"auto", "selector"} {
		if err := os.WriteFile(path, []byte("SELECTOR_MODE="+selector+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadModule(path); err == nil {
			t.Fatalf("SELECTOR_MODE=%s 不应继续被接受", selector)
		}
	}
}

func TestUpdateModuleKeepsOriginalWhenCandidateIsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.conf")
	original := "AUTO_START=0\nOUTBOUND_MODE=rule\nACTIVE_GROUP_ID=default\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateModule(path, map[string]string{"OUTBOUND_MODE": "invalid"}); err == nil {
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

	lock, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("持锁进程退出后配置锁未恢复: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}
