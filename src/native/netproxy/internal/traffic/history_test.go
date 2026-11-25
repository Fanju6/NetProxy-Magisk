package traffic

import (
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestCaptureHistoryRecoversBatchesAndDetails(t *testing.T) {
	layout := paths.New(t.TempDir())
	c := New(layout)
	start := func() string {
		t.Helper()
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		return c.batch.ID
	}
	first := start()
	c.recordDetail(c.session, Entry{Method: "GET"}, &HTTPDetail{Request: CapturedMessage{
		Body: CapturedBody{Data: "saved body"},
	}})
	id := c.Entries()[0].ID
	c.Stop()
	second := start()
	c.record(c.session, Entry{Method: "TCP"})
	if _, err := c.DeleteBatch(second); !errors.Is(err, ErrActiveBatch) {
		t.Fatalf("deleting active batch: %v", err)
	}
	c.writer.stop()
	c.writer = nil
	c.Stop()
	if err := os.WriteFile(c.history.path(second, "0000000000000002-"+rand.Text()+".json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.history.path(second, ".netproxy-partial.tmp"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened := New(layout)
	batches := reopened.Batches()
	if len(batches) != 2 || batches[0].ID != second || batches[0].Count != 1 || batches[0].Running ||
		batches[0].StoppedAt == nil || batches[1].ID != first || batches[1].Size <= 0 {
		t.Fatalf("unexpected recovered batches: %+v", batches)
	}
	record, ok := reopened.Record(id)
	if !ok || record.HTTP == nil || record.HTTP.Request.Body.Data != "saved body" {
		t.Fatal("historical detail missing")
	}
	if entries, ok := reopened.BatchEntries(first); !ok || len(entries) != 1 || entries[0].ID != id {
		t.Fatalf("historical batch entries: %+v", entries)
	}
	if found, err := reopened.DeleteBatch(first); !found || err != nil {
		t.Fatalf("delete history: %t, %v", found, err)
	}
	if err := reopened.Clear(); err != nil || len(New(layout).Batches()) != 0 {
		t.Fatalf("clear history: %v", err)
	}
}

func TestCaptureHistoryEvictsWholeOldestBatch(t *testing.T) {
	c := New(paths.New(t.TempDir()))
	c.history.byteLimit = 2048
	for i := 0; i < 3; i++ {
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		c.record(c.session, Entry{Host: strings.Repeat("a", 800)})
		c.Stop()
	}
	batches := c.Batches()
	if len(batches) != 2 || batches[0].Count != 1 || batches[1].Count != 1 ||
		batches[0].Size+batches[1].Size > c.history.byteLimit {
		t.Fatalf("retained batches: %+v", batches)
	}
}
