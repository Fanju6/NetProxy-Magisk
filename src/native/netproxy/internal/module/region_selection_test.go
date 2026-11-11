package module

import (
	"os"
	"testing"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

func TestSelectRegionPersistsAndAutoClearsRegion(t *testing.T) {
	options := selectionFixture(t)
	result, err := SelectRegion(t.Context(), options, "default", "other")
	if err != nil {
		t.Fatal(err)
	}
	config, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || config.Region != "other" || config.SelectedNodeTag != "" || result["selected"] != "Auto/本地配置 [other]" {
		t.Fatalf("region selection not persisted: %+v %+v %v", config, result, err)
	}
	before, err := os.ReadFile(options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SelectRegion(t.Context(), options, "default", "jp"); err == nil {
		t.Fatal("accepted a region without nodes")
	}
	after, err := os.ReadFile(options.ModuleConfig)
	if err != nil || string(after) != string(before) {
		t.Fatalf("invalid region changed configuration: %v", err)
	}
	if _, err := SelectNode(t.Context(), options, "auto", "default"); err != nil {
		t.Fatal(err)
	}
	config, err = moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || config.Region != "" || len(config.WiFi.Blacklist) != 1 {
		t.Fatalf("global auto did not clear region or lost other settings: %+v %v", config, err)
	}
}
