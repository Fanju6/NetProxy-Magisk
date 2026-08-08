package subworker

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestNextUpdateUsesNearestEnabledSubscription(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_000, 0)
	for _, entry := range []struct {
		id       string
		interval int64
		auto     bool
	}{
		{id: "first", interval: 900, auto: true},
		{id: "later", interval: 3_600, auto: true},
		{id: "manual", interval: 900, auto: false},
	} {
		metadata := subscription.NewMetadata(entry.id, entry.id, "subscription", "https://example.invalid/"+entry.id, now)
		metadata.AutoUpdate = entry.auto
		metadata.UpdateInterval = entry.interval
		if entry.auto {
			subscription.ScheduleAt(&metadata, now)
		}
		group := filepath.Join(root, entry.id)
		if err := os.MkdirAll(group, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := subscription.SaveMetadataAtomic(filepath.Join(group, "meta.json"), metadata); err != nil {
			t.Fatal(err)
		}
	}
	options := NewOptions(root)
	options.ModuleConf = filepath.Join(root, "module.conf")
	options.Now = func() time.Time { return now }
	got, err := NextUpdate(root, now)
	if err != nil {
		t.Fatal(err)
	}
	if got != now.Unix()+900 {
		t.Fatalf("nearest = %d, want %d", got, now.Unix()+900)
	}
}

func TestRunExitsWhenNoAutomaticSubscription(t *testing.T) {
	root := t.TempDir()
	moduleConf := filepath.Join(root, "module.conf")
	if err := os.WriteFile(moduleConf, []byte("ACTIVE_GROUP_ID=\"default\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := NewOptions(root)
	options.ModuleConf = moduleConf
	options.Now = time.Now
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Run(ctx, options, nil, log.New(os.Stderr, "", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(options.PIDFile); !os.IsNotExist(err) {
		t.Fatalf("PID file still exists: %v", err)
	}
}
