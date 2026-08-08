package control

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadStatusWithoutService(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=global\nSELECTOR_MODE=urltest\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStatus(context.Background(), Options{
		CatalogRoot:  filepath.Join(temp, "catalog"),
		ModuleConfig: moduleConfig,
		StateFile:    filepath.Join(temp, "service.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || status.OutboundMode != "global" || status.ActiveGroupID != "default" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.PID != nil || status.SubscriptionWorker != "stopped" {
		t.Fatalf("unexpected process state: %#v", status)
	}
	if status.CPUCount < 1 {
		t.Fatalf("invalid CPU count: %d", status.CPUCount)
	}
	if _, err := json.Marshal(status); err != nil {
		t.Fatal(err)
	}
}

func TestReadGroupsUnavailable(t *testing.T) {
	_, err := ReadGroups(context.Background(), Options{
		ServiceAddress: "127.0.0.1:1",
		RequestTimeout: 10,
	})
	if err == nil {
		t.Fatal("expected an unavailable Service API error")
	}
}
