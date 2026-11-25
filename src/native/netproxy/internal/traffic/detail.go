package traffic

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
)

const (
	captureBodyLimit    = 64 << 10
	captureHeaderLimit  = 8 << 10
	captureURLLimit     = 36 << 10
	captureDetailBudget = 8 << 20
)

type Record struct {
	Entry
	HTTP *HTTPDetail `json:"http,omitempty"`
}

type HTTPDetail struct {
	URL          string           `json:"url"`
	URLTruncated bool             `json:"url_truncated"`
	Request      CapturedMessage  `json:"request"`
	Response     CapturedMessage  `json:"response"`
	Rewrites     []RewriteMatch   `json:"rewrites,omitempty"`
	WebSocket    *WebSocketDetail `json:"websocket,omitempty"`
}

type CapturedMessage struct {
	Headers          http.Header  `json:"headers"`
	HeadersTruncated bool         `json:"headers_truncated"`
	Body             CapturedBody `json:"body"`
	size             int
}

type CapturedBody struct {
	Data            string `json:"data"`
	Format          string `json:"format"`
	ContentType     string `json:"content_type,omitempty"`
	ContentEncoding string `json:"content_encoding,omitempty"`
	Truncated       bool   `json:"truncated"`
	Size            int64  `json:"size"`
	CapturedSize    int    `json:"captured_size"`
	Decoded         bool   `json:"decoded,omitempty"`
}

func (d *HTTPDetail) size() int {
	if d == nil {
		return 0
	}
	size := 256 + len(d.URL) + d.Request.size + d.Response.size
	for _, m := range d.Rewrites {
		size += m.size()
	}
	if d.WebSocket != nil {
		for _, frame := range d.WebSocket.Frames {
			size += 128 + len(frame.Text)
		}
	}
	return size
}

type bodyCapture struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
	total     int64
}

func (c *bodyCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := min(captureBodyLimit-len(c.data), len(p))
	if n > 0 {
		c.data = append(c.data, p[:n]...)
	}
	c.truncated = c.truncated || n < len(p)
	c.total += int64(len(p))
	return len(p), nil
}

func (c *bodyCapture) body(headers http.Header, length int64, incomplete bool) CapturedBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	kind := strings.Clone(validPrefix(headers.Get("Content-Type"), 512))
	encoding := strings.Clone(validPrefix(strings.Join(headers.Values("Content-Encoding"), ", "), 256))
	b := CapturedBody{Format: "text", ContentType: kind, ContentEncoding: encoding,
		Truncated: c.truncated || incomplete || length > int64(len(c.data)),
		Size:      max(length, c.total), CapturedSize: len(c.data)}
	if len(c.data) == 0 {
		return b
	}
	mediaType, _, _ := mime.ParseMediaType(kind)
	textual := mediaType == "" || strings.HasPrefix(mediaType, "text/") ||
		strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml") ||
		mediaType == "application/json" || mediaType == "application/xml" ||
		mediaType == "application/x-www-form-urlencoded" || mediaType == "application/javascript"
	data := c.data
	if preview, ok := decodedBody(data, encoding, b.Truncated); ok {
		data = preview[:min(len(preview), captureBodyLimit)]
		b.Decoded = true
		b.Truncated = b.Truncated || len(preview) > captureBodyLimit
		b.CapturedSize = len(data)
	}
	if b.Truncated {
		for i := len(data) - 1; i >= max(0, len(data)-utf8.UTFMax); i-- {
			if utf8.RuneStart(data[i]) {
				if !utf8.FullRune(data[i:]) {
					data = data[:i]
				}
				break
			}
		}
	}
	if textual && (b.Decoded || encoding == "" || strings.EqualFold(encoding, "identity")) && utf8.Valid(data) {
		b.Data = string(data)
		if b.Decoded {
			b.CapturedSize = len(data)
		}
	} else {
		if !b.Decoded {
			data = c.data
		}
		b.Format, b.Data = "base64", base64.StdEncoding.EncodeToString(data)
	}
	return b
}

func decodedBody(raw []byte, encoding string, truncated bool) ([]byte, bool) {
	var reader io.ReadCloser
	var err error
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "gzip", "x-gzip":
		reader, err = gzip.NewReader(bytes.NewReader(raw))
	case "deflate":
		reader, err = zlib.NewReader(bytes.NewReader(raw))
		if err == zlib.ErrHeader {
			reader, err = flate.NewReader(bytes.NewReader(raw)), nil
		}
	case "br":
		reader = io.NopCloser(brotli.NewReader(bytes.NewReader(raw)))
	default:
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	defer reader.Close()
	preview, err := io.ReadAll(io.LimitReader(reader, captureBodyLimit+1))
	return preview, err == nil || truncated && err == io.ErrUnexpectedEOF && len(preview) > 0
}

func validPrefix(value string, limit int) string {
	if len(value) > limit {
		for limit > 0 && !utf8.RuneStart(value[limit]) {
			limit--
		}
	}
	return strings.ToValidUTF8(value[:min(len(value), limit)], "?")
}

func captureMessage(source http.Header) CapturedMessage {
	result := CapturedMessage{Headers: make(http.Header)}
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range source[key] {
			cost := len(key) + len(value) + 64
			if cost > captureHeaderLimit-result.size {
				result.HeadersTruncated = true
				continue
			}
			result.Headers.Add(strings.Clone(key), strings.Clone(strings.ToValidUTF8(value, "?")))
			result.size += cost
		}
	}
	return result
}

func (m *CapturedMessage) setBody(body CapturedBody) {
	m.Body = body
	m.size += len(body.Data) + len(body.ContentEncoding) + len(body.ContentType)
}

type captureWriter struct {
	http.ResponseWriter
	message *CapturedMessage
	body    bodyCapture
	failed  bool
	head    bool
}

func (w *captureWriter) WriteHeader(status int) {
	*w.message = captureMessage(w.Header())
	w.ResponseWriter.WriteHeader(status)
}

func (w *captureWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if !w.head {
		_, _ = w.body.Write(p[:n])
	}
	w.failed = w.failed || err != nil
	return n, err
}

type captureReader struct {
	io.ReadCloser
	body bodyCapture
	eof  atomic.Bool
}

func (r *captureReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	_, _ = r.body.Write(p[:n])
	if err == io.EOF {
		r.eof.Store(true)
	}
	return n, err
}
