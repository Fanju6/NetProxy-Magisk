package traffic

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/andybalholm/brotli"
)

func TestCompressedBodyPreviewIsDecodedAndBounded(t *testing.T) {
	for encoding, writer := range map[string]func(io.Writer) io.WriteCloser{
		"gzip":    func(w io.Writer) io.WriteCloser { return gzip.NewWriter(w) },
		"deflate": func(w io.Writer) io.WriteCloser { return zlib.NewWriter(w) },
		"br":      func(w io.Writer) io.WriteCloser { return brotli.NewWriter(w) },
	} {
		t.Run(encoding, func(t *testing.T) {
			for _, size := range []int{5, captureBodyLimit * 3} {
				var encoded bytes.Buffer
				compressor := writer(&encoded)
				if _, err := io.WriteString(compressor, strings.Repeat("a", size)); err != nil {
					t.Fatal(err)
				}
				if err := compressor.Close(); err != nil {
					t.Fatal(err)
				}
				var capture bodyCapture
				_, _ = capture.Write(encoded.Bytes())
				body := capture.body(http.Header{"Content-Type": {"text/plain"}, "Content-Encoding": {encoding}}, -1, false)
				if !body.Decoded || body.ContentEncoding != encoding || body.Size != int64(encoded.Len()) ||
					body.Truncated != (size > captureBodyLimit) || body.Data != strings.Repeat("a", min(size, captureBodyLimit)) {
					t.Fatalf("unexpected compressed preview: decoded %t, size %d, retained %d, truncated %t",
						body.Decoded, body.Size, body.CapturedSize, body.Truncated)
				}
			}
		})
	}
}

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
