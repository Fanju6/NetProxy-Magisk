package traffic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/http/httpguts"
)

var (
	errReplayOffer    = errors.New("replay offer")
	ErrReplayCore     = errors.New("抓包核心不可用")
	ErrReplayRoute    = errors.New("核心无法为重放建立路由")
	ErrReplayRequest  = errors.New("重放请求无效或原始请求不完整")
	ErrReplayRecord   = errors.New("找不到抓包记录")
	ErrReplayResponse = errors.New("重放未收到有效响应")
)

type ReplayInput struct {
	ID   string      `json:"id"`
	Edit *ReplayEdit `json:"edit,omitempty"`
}

type ReplayEdit struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
	Base64  bool        `json:"body_base64,omitempty"`
}

func (c *Controller) Replay(ctx context.Context, input ReplayInput) (string, error) {
	if !ValidRecordID(input.ID) {
		return "", ErrReplayRequest
	}
	original, ok := c.Record(input.ID)
	if !ok {
		return "", ErrReplayRecord
	}
	if original.HTTP == nil || original.HTTP.WebSocket != nil {
		return "", ErrReplayRequest
	}
	edit := input.Edit
	if edit == nil {
		request := original.HTTP.Request
		if original.HTTP.URLTruncated || request.HeadersTruncated || request.Body.Truncated ||
			request.Body.Decoded || len(request.Trailers) > 0 || request.TrailersTruncated {
			return "", ErrReplayRequest
		}
		edit = &ReplayEdit{Method: original.Method, URL: original.HTTP.URL, Headers: request.Headers,
			Body: request.Body.Data, Base64: request.Body.Format == "base64"}
	}
	req, err := edit.request(ctx)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == nil {
		return "", ErrReplayCore
	}
	return session.replay(req, original)
}

func (edit *ReplayEdit) request(ctx context.Context) (*http.Request, error) {
	const maxBody = 256 << 10
	target, err := url.Parse(edit.URL)
	if err != nil || !validTarget(target) || len(edit.URL) > captureURLLimit ||
		edit.Method == "" || len(edit.Method) > 16 || edit.Method == http.MethodConnect || !httpguts.ValidHeaderFieldName(edit.Method) {
		return nil, ErrReplayRequest
	}
	if port := target.Port(); port != "" {
		if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
			return nil, ErrReplayRequest
		}
	}
	if len(edit.Body) > base64.StdEncoding.EncodedLen(maxBody) {
		return nil, ErrReplayRequest
	}
	body := []byte(edit.Body)
	if edit.Base64 {
		body, err = base64.StdEncoding.Strict().DecodeString(edit.Body)
	}
	if err != nil || len(body) > maxBody {
		return nil, ErrReplayRequest
	}
	req, err := http.NewRequestWithContext(ctx, edit.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrReplayRequest
	}
	var size int
	for name, values := range edit.Headers {
		if !httpguts.ValidHeaderFieldName(name) || len(values) == 0 {
			return nil, ErrReplayRequest
		}
		for _, value := range values {
			size += len(name) + len(value) + 4
			if size > 32<<10 || !httpguts.ValidHeaderFieldValue(value) {
				return nil, ErrReplayRequest
			}
			req.Header.Add(name, value)
		}
	}
	if req.Header.Get("Upgrade") != "" {
		return nil, ErrReplayRequest
	}
	req.Header = cleanHeaders(req.Header)
	req.Header.Del("Host")
	req.Header.Del("Content-Length")
	return req, nil
}

type replayConn struct {
	net.Conn
	reader *bufio.Reader
	once   sync.Once
	route  flow
	err    error
}

func (c *replayConn) Read(p []byte) (int, error) {
	c.once.Do(func() {
		line, err := c.reader.ReadSlice('\n')
		if err != nil || json.Unmarshal(line, &c.route) != nil || c.route.Exit == nil {
			c.err = ErrReplayRoute
		}
	})
	if c.err != nil {
		return 0, c.err
	}
	return c.reader.Read(p)
}

func (s *session) replay(req *http.Request, original Record) (string, error) {
	ctx, cancel := context.WithTimeout(req.Context(), 90*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	owner := original.SourceApp
	if owner == nil || owner.UID > math.MaxInt32 || owner.UserID != owner.UID/androidUIDRange || owner.Package == "" {
		owner = nil
	}
	port := req.URL.Port()
	if port == "" {
		port = "80"
		if req.URL.Scheme == "https" {
			port = "443"
		}
	}
	s.replayMu.Lock()
	bridge := s.replayOffer
	s.replayOffer = nil
	s.replayMu.Unlock()
	if bridge == nil {
		return "", ErrReplayCore
	}
	defer bridge.Close()
	defer context.AfterFunc(s.ctx, func() { bridge.Close() })()
	defer context.AfterFunc(ctx, func() { bridge.Close() })()
	_ = bridge.SetDeadline(time.Now().Add(5 * time.Second))
	err := json.NewEncoder(bridge).Encode(struct {
		Destination string  `json:"destination"`
		UID         *uint32 `json:"uid,omitempty"`
		UserID      uint32  `json:"user_id,omitempty"`
		Package     string  `json:"package,omitempty"`
	}{net.JoinHostPort(req.URL.Hostname(), port), replayUID(owner), replayUser(owner), replayPackage(owner)})
	var ack [1]byte
	if err != nil {
		return "", ErrReplayCore
	}
	if _, err := io.ReadFull(bridge, ack[:]); err != nil || ack[0] != 1 {
		return "", ErrReplayCore
	}
	_ = bridge.SetDeadline(time.Time{})
	conn := &replayConn{Conn: bridge, reader: bufio.NewReaderSize(bridge, 4096)}
	dialed := false
	var dialMu sync.Mutex
	transport := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialMu.Lock()
			defer dialMu.Unlock()
			if dialed {
				return nil, ErrReplayCore
			}
			dialed = true
			return conn, nil
		},
		ForceAttemptHTTP2: true, DisableKeepAlives: true, DisableCompression: true,
		TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	defer transport.CloseIdleConnections()
	now := time.Now()
	capture := &captureReader{ReadCloser: req.Body}
	if req.Body != nil && req.Body != http.NoBody {
		req.Body = capture
	}
	response, err := transport.RoundTrip(req)
	if err != nil {
		if errors.Is(err, ErrReplayRoute) {
			return "", ErrReplayRoute
		}
		return "", ErrReplayResponse
	}
	defer response.Body.Close()
	entry := conn.route.entry(now, req.Method, req.URL.Host)
	entry.Status, entry.Protocol, entry.Sequence = response.StatusCode, "http/1.1", 1
	if response.ProtoMajor == 2 {
		entry.Protocol = "h2"
	} else if response.ProtoMinor == 0 {
		entry.Protocol = "http/1.0"
	}
	if req.URL.Scheme == "https" {
		entry.TLS = "decrypted"
	}
	entry.Target = validPrefix(req.URL.RequestURI(), 1024)
	entry.SourceApp, entry.ReplayOf, entry.ReplayOriginal = owner, original.ID, original.Exit
	if entry.ReplayOriginal == nil && original.Outbound != "" {
		entry.ReplayOriginal = &Exit{Chain: []ExitHop{{Tag: original.Outbound}}}
	}
	entry.ReplayUnattributed = owner == nil
	entry.ReplayRouteChanged = entry.ReplayOriginal != nil && !slices.Equal(entry.ReplayOriginal.Chain, entry.Exit.Chain)
	headers := req.Header.Clone()
	headers.Set("Host", req.URL.Host)
	detail := &HTTPDetail{URL: req.URL.String(), Request: captureMessage(headers), Response: captureMessage(response.Header)}
	detail.Request.setBody(capture.body.body(headers, req.ContentLength, !capture.eof.Load() && req.ContentLength < 0))
	var responseBody bodyCapture
	responseBody.initMedia(response.Header, s.controller.mediaFactory(s))
	_, err = io.Copy(&responseBody, response.Body)
	detail.Response.setTrailers(response.Trailer)
	detail.Response.setBody(responseBody.body(response.Header, response.ContentLength, err != nil))
	entry.DurationMS = time.Since(now).Milliseconds()
	id := s.controller.recordMediaDetail(s, entry, detail, nil, responseBody.finishMedia())
	if id == "" {
		return "", ErrReplayCore
	}
	return id, nil
}

func replayUID(owner *SourceApp) *uint32 {
	if owner != nil {
		return &owner.UID
	}
	return nil
}

func replayUser(owner *SourceApp) uint32 {
	if owner != nil {
		return owner.UserID
	}
	return 0
}

func replayPackage(owner *SourceApp) string {
	if owner != nil {
		return owner.Package
	}
	return ""
}
