package traffic

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestMediaClassification(t *testing.T) {
	for _, tt := range []struct {
		kind, disposition string
		media             bool
	}{
		{"image/jpeg", "", true},
		{"video/mp4", "", true},
		{"audio/mpeg", "", true},
		{"application/pdf", "", true},
		{"application/octet-stream", "", true},
		{"text/plain; charset=utf-8", "attachment; filename=note.txt", true},
		{"image/svg+xml", "", false},
		{"application/problem+json", "", false},
		{"application/x-www-form-urlencoded", "", false},
		{"", "", false},
	} {
		if got := mediaBody(http.Header{"Content-Type": {tt.kind}, "Content-Disposition": {tt.disposition}}); got != tt.media {
			t.Errorf("classification for %q / %q: got %t", tt.kind, tt.disposition, got)
		}
	}
}

func TestMediaHistoryRetentionAndCleanup(t *testing.T) {
	layout := paths.New(t.TempDir())
	c := New(layout)
	c.history.mediaLimit = int64(MaxMediaBody) + 1<<20
	capture := func(size int) (string, string) {
		t.Helper()
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		headers := http.Header{
			"Content-Type": {"video/mp4"}, "Content-Disposition": {"attachment; filename=fixture.mp4"},
			"Content-Range": {"bytes 0-99/200"},
		}
		var capture bodyCapture
		capture.initMedia(headers, c.mediaFactory(c.session))
		chunk := bytes.Repeat([]byte("A"), mediaChunk)
		for left := size; left > 0; left -= len(chunk) {
			_, _ = capture.Write(chunk[:min(left, len(chunk))])
		}
		detail := &HTTPDetail{}
		detail.Response.setBody(capture.body(headers, int64(size), false))
		c.recordMediaDetail(c.session, Entry{Method: "GET"}, detail, nil, capture.finishMedia())
		id, batch := c.Entries()[0].ID, c.batch.ID
		c.Stop()
		return id, batch
	}

	id, batch := capture(MaxMediaBody + 100)
	orphan := c.history.path(batch, mediaName(strings.Repeat("A", 26), "response"))
	if err := os.WriteFile(orphan, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	c = New(layout)
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("blob from interrupted commit remains: %v", err)
	}
	c.history.mediaLimit = int64(MaxMediaBody) + 1<<20
	record, ok := c.Record(id)
	if body := bodyFor(record, "response"); !ok || body == nil || body.Data != "" || !body.Blob ||
		body.CapturedSize != MaxMediaBody || body.Size != MaxMediaBody+100 || !body.Truncated ||
		body.Filename != "fixture.mp4" || body.ContentRange != "bytes 0-99/200" {
		t.Fatalf("incorrect recovered metadata: %+v", body)
	}
	file, ok := c.OpenBody(id, "response")
	if !ok {
		t.Fatal("retained blob not available")
	}
	if _, err := file.Seek(MaxMediaBody-3, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if string(data) != "AAA" || err != nil {
		t.Fatalf("incorrect bytes at end of blob: %q, %v", data, err)
	}
	next, nextBatch := capture(2 << 20)
	if _, err := os.Stat(c.history.path(batch)); !os.IsNotExist(err) {
		t.Fatalf("oldest whole batch still exists: %v", err)
	}
	if _, ok := c.OpenBody(id, "response"); ok {
		t.Fatal("evicted blob still readable")
	}
	if found, err := c.DeleteBatch(nextBatch); !found || err != nil {
		t.Fatalf("delete batch: %t, %v", found, err)
	}
	if _, ok := c.OpenBody(next, "response"); ok {
		t.Fatal("deleted blob still readable")
	}
	_, clearBatch := capture(128 << 10)
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.history.path(clearBatch)); !os.IsNotExist(err) {
		t.Fatalf("cleared media batch remains: %v", err)
	}
}
