package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
