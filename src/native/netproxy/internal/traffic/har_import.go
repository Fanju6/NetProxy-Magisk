package traffic

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/http/httpguts"
)

func (c *Controller) ImportHAR(ctx context.Context, name string, input io.Reader) (Batch, error) {
	h := c.history
	if h == nil {
		return Batch{}, ErrHARInvalid
	}
	name = validPrefix(strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/"))), 256)
	if name == "" || name == "." || name == ".." || name == "/" || strings.ContainsFunc(name, unicode.IsControl) {
		return Batch{}, ErrHARInvalid
	}
	data, err := io.ReadAll(io.LimitReader(input, MaxHARImportBytes+1))
	if err != nil {
		return Batch{}, err
	}
	if len(data) > MaxHARImportBytes {
		return Batch{}, ErrHARInvalid
	}
	var archive struct {
		Log struct {
			Version string            `json:"version"`
			Entries []json.RawMessage `json:"entries"`
		} `json:"log"`
	}
	if json.Unmarshal(data, &archive) != nil || archive.Log.Version != "1.2" ||
		archive.Log.Entries == nil || len(archive.Log.Entries) > MaxHARImportRecords {
		return Batch{}, ErrHARInvalid
	}
	if err := os.MkdirAll(h.path(), 0o700); err != nil {
		return Batch{}, err
	}
	staging, err := os.MkdirTemp(h.path(), ".har-")
	if err != nil {
		return Batch{}, err
	}
	defer os.RemoveAll(staging)
	now := time.Now().UTC()
	batch := &savedBatch{Batch: Batch{ID: rand.Text(), Name: name, Imported: true, StartedAt: now, StoppedAt: &now}}
	for _, raw := range archive.Log.Entries {
		if err := ctx.Err(); err != nil {
			return Batch{}, err
		}
		var entry harEntry
		if json.Unmarshal(raw, &entry) != nil {
			return Batch{}, ErrHARInvalid
		}
		record, blobs, err := importHAREntry(entry)
		if err != nil {
			return Batch{}, err
		}
		for side, blob := range blobs {
			if err := os.WriteFile(filepath.Join(staging, mediaName(record.ID, side)), blob, 0o600); err != nil {
				return Batch{}, err
			}
		}
		recordJSON, err := json.Marshal(record)
		if err != nil || len(recordJSON) > historyRecordLimit {
			return Batch{}, ErrHARInvalid
		}
		batch.sequence++
		filename := fmt.Sprintf("%016x-%s.json", batch.sequence, record.ID)
		mediaSize := int64(len(blobs["request"]) + len(blobs["response"]))
		size := int64(len(recordJSON))
		batch.records = append(batch.records, savedRecord{filename, record.ID, historyHost(record.Host), size, batch.sequence, mediaSize})
		batch.Count++
		batch.Size += size + mediaSize
		batch.mediaSize += mediaSize
		if batch.Size-batch.mediaSize > h.byteLimit || batch.Size > h.mediaLimit {
			return Batch{}, ErrHARInvalid
		}
		if err := os.WriteFile(filepath.Join(staging, filename), recordJSON, 0o600); err != nil {
			return Batch{}, err
		}
	}
	metadata, err := json.Marshal(batch.Batch)
	if err != nil {
		return Batch{}, err
	}
	if err := os.WriteFile(filepath.Join(staging, "batch.json"), metadata, 0o600); err != nil {
		return Batch{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := os.Rename(staging, h.path(batch.ID)); err != nil {
		return Batch{}, err
	}
	h.batches = append(h.batches, batch)
	if err := h.prune(batch, 0, 0); err != nil {
		h.warn()
	}
	if err := h.pruneMedia(batch, 0); err != nil {
		h.warn()
	}
	return batch.Batch, nil
}

func importHAREntry(entry harEntry) (Record, map[string][]byte, error) {
	req, res, ext := entry.Request, entry.Response, entry.NetProxy
	if req == nil || res == nil || entry.Started.IsZero() || entry.Time < 0 || entry.Time >= math.MaxInt64 ||
		res.Status < 0 || res.Status != 0 && (res.Status < 100 || res.Status > 599) ||
		!httpguts.ValidHeaderFieldName(req.Method) || len(req.Method) > 64 || len(req.URL) > captureURLLimit {
		return Record{}, nil, ErrHARInvalid
	}
	for _, grpc := range []*GRPCDetail{ext.RequestGRPC, ext.ResponseGRPC} {
		if grpc == nil {
			continue
		}
		if len(grpc.Messages) > grpcMessageLimit {
			return Record{}, nil, ErrHARInvalid
		}
		for _, message := range grpc.Messages {
			if !validHARProtoFields(message.Fields, 0) {
				return Record{}, nil, ErrHARInvalid
			}
		}
	}
	target, err := url.Parse(req.URL)
	if !ext.NoHTTP && (err != nil || !validTarget(target)) {
		return Record{}, nil, ErrHARInvalid
	}
	protocol := "http/1.1"
	switch strings.ToUpper(req.HTTPVersion) {
	case "HTTP/1.0":
		protocol = "http/1.0"
	case "HTTP/2", "HTTP/2.0", "H2":
		protocol = "h2"
	case "HTTP/3", "HTTP/3.0", "H3":
		protocol = "h3"
	}
	blobs := make(map[string][]byte, 2)
	request, blob, err := importHARMessage(req.Headers, req.PostData, req.BodySize, ext.RequestTruncated, ext.RequestDecoded)
	if err != nil {
		return Record{}, nil, err
	}
	if blob != nil {
		blobs["request"] = blob
	}
	response, blob, err := importHARMessage(res.Headers, res.Content, res.BodySize, ext.ResponseTruncated, ext.ResponseDecoded)
	if err != nil {
		return Record{}, nil, err
	}
	if blob != nil {
		blobs["response"] = blob
	}
	request.HeadersTruncated = request.HeadersTruncated || ext.RequestHeadersTruncated
	response.HeadersTruncated = response.HeadersTruncated || ext.ResponseHeadersTruncated
	if ext.NoHTTP {
		if !httpguts.ValidHostHeader(ext.Host) || ext.Host == "" || len(ext.Host) > 259 {
			return Record{}, nil, ErrHARInvalid
		}
		return Record{Entry: Entry{ID: rand.Text(), Time: entry.Started, Imported: true, Method: req.Method,
			Host: ext.Host, Target: validPrefix(ext.Target, 1024), Status: res.Status, DurationMS: int64(entry.Time),
			TLS: ext.TLS, PassthroughReason: ext.PassthroughReason, Outbound: ext.Outbound, Protocol: ext.Protocol,
			CAStores: ext.CAStores, Sequence: ext.Sequence,
			Exit: ext.Exit, SourceApp: ext.SourceApp}}, nil, nil
	}
	for i, trailers := range [][]harPair{ext.RequestTrailers, ext.ResponseTrailers} {
		message := []*CapturedMessage{&request, &response}[i]
		parsed, err := importHARHeaders(trailers)
		if err != nil {
			return Record{}, nil, err
		}
		message.setTrailers(parsed)
	}
	request.TrailersTruncated = request.TrailersTruncated || ext.RequestTrailersTruncated
	response.TrailersTruncated = response.TrailersTruncated || ext.ResponseTrailersTruncated
	request.GRPC, response.GRPC = ext.RequestGRPC, ext.ResponseGRPC
	return Record{
		Entry: Entry{ID: rand.Text(), Time: entry.Started, Method: req.Method, Host: target.Host,
			Target: validPrefix(target.RequestURI(), 1024), Status: res.Status, Protocol: protocol, Imported: true,
			DurationMS: int64(entry.Time), TLS: ext.TLS, PassthroughReason: ext.PassthroughReason,
			CAStores: ext.CAStores, Sequence: ext.Sequence,
			Outbound: ext.Outbound, Exit: ext.Exit, SourceApp: ext.SourceApp, Rewrite: ext.Rewrite, Rewritten: ext.Rewritten},
		HTTP: &HTTPDetail{URL: req.URL, URLTruncated: ext.URLTruncated, Request: request, Response: response,
			Rewrites: ext.Rewrites, Breakpoints: ext.Breakpoints, WebSocket: ext.WebSocket},
	}, blobs, nil
}

func validHARProtoFields(fields []ProtoField, depth int) bool {
	if depth > 3 {
		return len(fields) == 0
	}
	for _, field := range fields {
		if !validHARProtoFields(field.Fields, depth+1) {
			return false
		}
	}
	return true
}

func importHARHeaders(pairs []harPair) (http.Header, error) {
	headers := make(http.Header)
	for _, pair := range pairs {
		switch strings.ToLower(pair.Name) {
		case ":authority", ":method", ":scheme", ":path", ":status":
			continue
		}
		if !httpguts.ValidHeaderFieldName(pair.Name) || !httpguts.ValidHeaderFieldValue(pair.Value) {
			return nil, ErrHARInvalid
		}
		headers.Add(pair.Name, pair.Value)
	}
	return headers, nil
}

func importHARMessage(pairs []harPair, content *harBody, size int64, truncated, decoded bool) (CapturedMessage, []byte, error) {
	headers, err := importHARHeaders(pairs)
	if err != nil {
		return CapturedMessage{}, nil, err
	}
	message := captureMessage(headers)
	body := CapturedBody{Format: "text", Size: max(0, size), Truncated: truncated, Decoded: decoded,
		ContentType: validPrefix(headers.Get("Content-Type"), 512), ContentEncoding: validPrefix(headers.Get("Content-Encoding"), 256)}
	var blob []byte
	if content != nil {
		if body.ContentType == "" {
			body.ContentType = validPrefix(content.MimeType, 512)
		}
		if size == 0 || size == -1 {
			body.Size = max(0, content.Size)
		}
		if content.Encoding != "" && content.Encoding != "base64" {
			return CapturedMessage{}, nil, ErrHARInvalid
		}
		if content.Text != nil {
			body.Data, body.CapturedSize = *content.Text, len(*content.Text)
			if content.Encoding == "base64" {
				data, err := base64.StdEncoding.DecodeString(body.Data)
				if err != nil || len(data) > MaxMediaBody {
					return CapturedMessage{}, nil, ErrHARInvalid
				}
				body.Format, body.CapturedSize = "base64", len(data)
				if len(data) > captureBodyLimit || mediaBody(headers) && len(data) != 0 {
					body.Format, body.Data, body.Blob, blob = "binary", "", true, data
				}
			} else if body.CapturedSize > maxBody {
				body.Data = validPrefix(body.Data, maxBody)
				body.CapturedSize = len(body.Data)
				body.Truncated = true
			}
			if !decoded {
				body.Size = max(body.Size, int64(body.CapturedSize))
			}
		}
	}
	body.Truncated = body.Truncated || !decoded && int64(body.CapturedSize) < body.Size
	message.setBody(body)
	return message, blob, nil
}
