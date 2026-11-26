package traffic

import (
	"crypto/rand"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestCaptureHistoryPagesBeyondMemoryRing(t *testing.T) {
	layout := paths.New(t.TempDir())
	h := openHistory(layout)
	batch := &savedBatch{Batch: Batch{ID: rand.Text(), StartedAt: time.Now().UTC()}}
	h.start(batch)
	for i := 1; i <= maxEntries+13; i++ {
		if err := h.append(batch, Record{Entry: Entry{ID: rand.Text(), Host: strconv.Itoa(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	h.finish(batch)
	h = openHistory(layout)
	var before uint64
	for remaining := maxEntries + 13; remaining > 0; {
		page, found := h.page(PageRequest{Batch: batch.ID, Before: before, Limit: 50})
		if !found || len(page.Entries) != min(50, remaining) || page.Oldest != "1" {
			t.Fatalf("missing page after %d: %+v", before, page)
		}
		for _, entry := range page.Entries {
			if entry.Host != strconv.Itoa(remaining) {
				t.Fatalf("expected %d, found %s", remaining, entry.Host)
			}
			remaining--
		}
		before, _ = strconv.ParseUint(page.Before, 10, 64)
		if (before == 0) != (remaining == 0) {
			t.Fatalf("cursor %d with %d remaining", before, remaining)
		}
	}
	page, ok := h.page(PageRequest{Since: maxEntries, Incremental: true, Limit: 5})
	if !ok || page.Since != "205" || len(page.Entries) != 5 || page.Entries[4].Host != "201" || page.Entries[0].Host != "205" {
		t.Fatalf("increment skipped records: %+v", page)
	}
}

func TestCaptureHistoryDomainFilterAfterRestart(t *testing.T) {
	layout := paths.New(t.TempDir())
	h := openHistory(layout)
	batch := &savedBatch{Batch: Batch{ID: rand.Text(), StartedAt: time.Now().UTC()}}
	h.start(batch)
	for _, host := range []string{"EXAMPLE.test.:8080", "other.test", "example.test", "example.test:443", "[2001:db8::1]:443"} {
		if err := h.append(batch, Record{Entry: Entry{ID: rand.Text(), Host: host}}); err != nil {
			t.Fatal(err)
		}
	}
	h.finish(batch)
	h = openHistory(layout)
	domains, ok := h.domains(batch.ID)
	if !ok || len(domains) != 3 || domains[0] != (CaptureDomain{"2001:db8::1", 1}) || domains[1] != (CaptureDomain{"example.test", 3}) {
		t.Fatalf("domain totals: %+v", domains)
	}
	first, _ := h.page(PageRequest{Host: "example.test", Limit: 2})
	if first.Before != "3" || first.Since != "5" || first.Oldest != "1" || len(first.Entries) != 2 || first.Entries[0].Host != "example.test:443" {
		t.Fatalf("first filtered page: %+v", first)
	}
	older, _ := h.page(PageRequest{Host: "example.test", Before: 3, Limit: 2})
	if older.Before != "" || len(older.Entries) != 1 || older.Entries[0].Host != "EXAMPLE.test.:8080" {
		t.Fatalf("older filtered page: %+v", older)
	}
	next, _ := h.page(PageRequest{Host: "example.test", Since: 1, Incremental: true, Limit: 1})
	if next.Since != "3" || len(next.Entries) != 1 || next.Entries[0].Host != "example.test" {
		t.Fatalf("filtered update skipped records: %+v", next)
	}
	empty, _ := h.page(PageRequest{Host: "not-found.test", Limit: 2})
	if empty.Before != "" || empty.Since != "5" || len(empty.Entries) != 0 {
		t.Fatalf("filtered empty page: %+v", empty)
	}
}
