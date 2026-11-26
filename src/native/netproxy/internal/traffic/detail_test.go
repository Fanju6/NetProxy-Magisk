package traffic

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestCaptureDetailBoundsAndEncoding(t *testing.T) {
	header := http.Header{"Content-Type": {"text/plain"}, "X-Large": {strings.Repeat("x", captureHeaderLimit)}}
	message := captureMessage(header)
	if !message.HeadersTruncated || message.Headers.Get("X-Large") != "" {
		t.Fatal("oversized header not limited")
	}
	text := &bodyCapture{}
	input := []byte(strings.Repeat("x", captureBodyLimit-1) + "界")
	if n, err := text.Write(input); err != nil || n != len(input) {
		t.Fatal("capture blocked forwarding")
	}
	body := text.body(header, -1, false)
	if !body.Truncated || body.Format != "text" || len(body.Data) != captureBodyLimit-1 {
		t.Fatal("invalid truncated text capture")
	}
	header.Set("Content-Encoding", "br")
	encoded := &bodyCapture{}
	binary := []byte{0, 255, 12}
	_, _ = encoded.Write(binary)
	body = encoded.body(header, -1, false)
	raw, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil || !bytes.Equal(binary, raw) || body.Format != "base64" || body.ContentEncoding != "br" || body.Truncated {
		t.Fatal("encoded payload not preserved")
	}
}

func TestCaptureEvictsOldestDetailWithinBudget(t *testing.T) {
	c := New(paths.New(t.TempDir()))
	current := &session{}
	c.session = current
	const recordSize = 128 << 10
	for range captureDetailBudget/recordSize + 2 {
		c.recordDetail(current, Entry{}, &HTTPDetail{Request: CapturedMessage{size: recordSize}})
	}
	if c.detailBytes > captureDetailBudget || c.count >= captureDetailBudget/recordSize {
		t.Fatalf("capture retained too much: %d bytes, %d entries", c.detailBytes, c.count)
	}
	id := c.Entries()[0].ID
	if record, ok := c.Record(id); !ok || record.HTTP == nil {
		t.Fatal("latest detail missing")
	}
	c.Clear()
	if _, ok := c.Record(id); ok || c.detailBytes != 0 {
		t.Fatal("clear retained detail")
	}
}
