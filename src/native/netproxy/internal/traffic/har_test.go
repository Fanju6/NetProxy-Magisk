package traffic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestHARRoundTrip(t *testing.T) {
	ctx := context.Background()
	layout := paths.New(t.TempDir())
	c := New(layout)
	input := `{"log":{"version":"1.2","entries":[{"startedDateTime":"2026-01-01T00:00:00Z","time":12,"request":{"method":"POST","url":"https://example.test/round-trip?a=1","httpVersion":"HTTP/2","headers":[{"name":"X-Test","value":"one"},{"name":"X-Test","value":"two"}],"bodySize":4,"postData":{"mimeType":"text/plain","text":"body"}},"response":{"status":202,"bodySize":4,"headers":[{"name":"Content-Type","value":"image/png"}],"content":{"mimeType":"image/png","size":4,"encoding":"base64","text":"AAEC/w=="}},"_netproxy":{"source_app":{"uid":10002,"user_id":0,"package":"test.example"},"tls":"decrypted","breakpoints":[{"phase":"request","outcome":"edited","changes":["body"]}]}}]}}`
	batch, err := c.ImportHAR(ctx, "fixture.har", strings.NewReader(input))
	if err != nil || batch.Count != 1 || batch.Running || !batch.Imported || batch.Name != "fixture.har" {
		t.Fatalf("import = %+v, %v", batch, err)
	}
	export, err := c.PrepareHARExport(HARFilter{Batch: batch.ID, Host: "EXAMPLE.TEST", Query: "/round-trip", Method: "POST",
		StatusClass: 2, TLS: "decrypted", App: "0:test.example"})
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	if err := export.Write(ctx, &result); err != nil {
		t.Fatal(err)
	}
	var archive struct{ Log struct{ Entries []harEntry } }
	if err := json.Unmarshal(result.Bytes(), &archive); err != nil || len(archive.Log.Entries) != 1 {
		t.Fatalf("invalid HAR: %v (%s)", err, result.Bytes())
	}
	entry := archive.Log.Entries[0]
	if entry.Time != 12 || *entry.Request.PostData.Text != "body" ||
		entry.Response.Content.Encoding != "base64" || *entry.Response.Content.Text != base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 255}) {
		t.Fatalf("invalid HAR entry: %+v", entry)
	}
	second, err := c.ImportHAR(ctx, "copy.har", &result)
	if err != nil || second.ID == batch.ID || second.Count != 1 {
		t.Fatalf("reimport = %+v, %v", second, err)
	}
	reopened := New(layout)
	entries, ok := reopened.BatchEntries(second.ID)
	if !ok || len(entries) != 1 || !entries[0].Imported {
		t.Fatalf("reopened import = %+v", entries)
	}
	record, ok := reopened.Record(entries[0].ID)
	if !ok || len(record.HTTP.Request.Headers.Values("X-Test")) != 2 ||
		!record.HTTP.Response.Body.Blob || len(record.HTTP.Breakpoints) != 1 || record.SourceApp.UID != 10002 {
		t.Fatalf("reopened record = %+v", record)
	}
}

func TestHARInvalidImportIsNotPublished(t *testing.T) {
	c := New(paths.New(t.TempDir()))
	input := `{"log":{"version":"1.2","entries":[{"startedDateTime":"2026-01-01T00:00:00Z","request":{"method":"GET","url":"https://example.test/"},"response":{"status":200}},{"request":{"method":"GET","url":"file:///secret"}}]}}`
	if _, err := c.ImportHAR(context.Background(), "invalid.har", strings.NewReader(input)); !errors.Is(err, ErrHARInvalid) {
		t.Fatalf("expected invalid HAR, got %v", err)
	}
	if len(c.Batches()) != 0 {
		t.Fatal("partial import published")
	}
}

func TestHARSingleTunnelRecord(t *testing.T) {
	c := New(paths.New(t.TempDir()))
	id := c.recordMediaDetail(nil, Entry{Time: time.Now(), Host: "example.test", Method: http.MethodConnect,
		TLS: "pinned_skipped", PassthroughReason: "certificate_bypassed"}, nil, nil, nil)
	export, err := c.PrepareHARExport(HARFilter{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	if err := export.Write(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	batch, err := c.ImportHAR(context.Background(), "tunnel.har", &result)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := c.BatchEntries(batch.ID)
	if len(entries) != 1 || entries[0].PassthroughReason != "certificate_bypassed" {
		t.Fatalf("tunnel metadata lost: %+v", entries)
	}
	record, ok := c.Record(entries[0].ID)
	if !ok || record.HTTP != nil {
		t.Fatal("tunnel became replayable HTTP")
	}
}
