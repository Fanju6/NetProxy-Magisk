package subscription

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

func TestEditUpdatesSchedulingWithoutDownloading(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	metadata := catalog.NewMetadata("sub-test", "测试订阅", "subscription", "https://example.test/sub", now)
	metadata.AutoUpdate = true
	metadata.UpdateInterval = 900
	ScheduleAt(&metadata, now)
	groupDir := filepath.Join(root, metadata.ID)
	if err := SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	disabled := false
	result, err := Edit(context.Background(), EditOptions{
		Root: root, GroupID: metadata.ID, AutoUpdate: &disabled, Now: now,
	})
	if err != nil {
		t.Fatalf("disable auto update: %v", err)
	}
	if result.RequiresUpdate {
		t.Fatal("auto update toggle unexpectedly downloaded subscription")
	}
	updated, err := LoadMetadata(filepath.Join(groupDir, "meta.json"), metadata.ID)
	if err != nil {
		t.Fatalf("load edited metadata: %v", err)
	}
	if updated.AutoUpdate || updated.NextUpdateEpoch != 0 || updated.NextUpdateAt != "" {
		t.Fatalf("schedule was not cleared: %+v", updated)
	}
}
