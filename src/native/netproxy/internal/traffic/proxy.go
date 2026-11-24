package traffic

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

type session struct {
	controller  *Controller
	access      Access
	cancel      context.CancelFunc
	server      *http.Server
	transport   *http.Transport
	slots       chan struct{}
	scriptSlots chan struct{}
	dial        func(context.Context, string, string) (net.Conn, error)
}

func (s *session) close() {
	s.cancel()
	_ = s.server.Close()
	s.transport.CloseIdleConnections()
}

func (s *session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(s.access.Username+":"+s.access.Password))
	authorization := r.Header.Values("Proxy-Authorization")
	if len(authorization) != 1 || subtle.ConstantTimeCompare([]byte(authorization[0]), []byte(expected)) != 1 {
		w.Header().Set("Proxy-Authenticate", `Basic realm="NetProxy"`)
		http.Error(w, "Proxy Authentication Required", http.StatusProxyAuthRequired)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		http.Error(w, "Proxy Busy", http.StatusServiceUnavailable)
		return
	}
	now := time.Now()
	if r.Method == http.MethodConnect {
		s.tunnel(w, r, now)
		return
	}
	if r.URL.Scheme != "http" || !validTarget(r.URL) || r.Header.Get("Upgrade") != "" {
		http.Error(w, "Use an HTTP proxy URL or CONNECT", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	target := r.URL.String()
	rules := s.controller.snapshotRules()
	method := r.Method
	if len(method) > 16 {
		method = "OTHER"
	}
	entry := Entry{Time: now, Method: method, Host: r.URL.Host}
	defer func() {
		entry.DurationMS = time.Since(now).Milliseconds()
		s.controller.record(s, entry)
	}()
	request := r.Clone(ctx)
	request.RequestURI = ""
	request.Host = request.URL.Host
	request.Header = cleanHeaders(request.Header)
	request.Trailer = nil
	if request.ContentLength == 0 {
		request.Body = nil
	}
	if rule := findRule(rules, "request", target); rule != nil {
		result, ok := s.runScript(ctx, rule, scriptInput{URL: target, Method: r.Method})
		if !ok {
			entry.Rewrite = "request_skipped"
		} else {
			entry.Rewrite = "request"
			if result.Reject {
				entry.Status = http.StatusForbidden
				http.Error(w, "Rejected by rewrite rule", entry.Status)
				return
			}
			if result.Redirect != "" {
				entry.Status = http.StatusFound
				w.Header().Set("Location", result.Redirect)
				w.WriteHeader(entry.Status)
				return
			}
			applyHeaders(request.Header, result)
		}
	}
	rule := findRule(rules, "response", target)
	if rule != nil && request.Header.Get("Range") == "" {
		request.Header.Del("Accept-Encoding")
	}
	response, err := s.transport.RoundTrip(request)
	if err != nil {
		entry.Status = http.StatusBadGateway
		http.Error(w, "sing-box upstream unavailable", entry.Status)
		return
	}
	defer response.Body.Close()
	response.Header = cleanHeaders(response.Header)
	entry.Status = response.StatusCode
	if rule != nil {
		state := s.rewriteResponse(ctx, rule, scriptInput{URL: target, Method: r.Method, Status: entry.Status}, response)
		if entry.Rewrite == "" {
			entry.Rewrite = state
		} else {
			entry.Rewrite += "+" + state
		}
	}
	for name, values := range response.Header {
		w.Header()[name] = values
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (s *session) runScript(ctx context.Context, rule *compiledRule, input scriptInput) (rewrite, bool) {
	select {
	case s.scriptSlots <- struct{}{}:
		defer func() { <-s.scriptSlots }()
		return rule.run(ctx, input)
	default:
		return rewrite{}, false
	}
}

func (s *session) rewriteResponse(ctx context.Context, rule *compiledRule, input scriptInput, response *http.Response) string {
	body := response.Body
	if accessibleBody(input, response) {
		content, err := io.ReadAll(io.LimitReader(body, maxBody+1))
		response.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(content), body), body}
		if err == nil && len(content) <= maxBody && utf8.Valid(content) {
			text := string(content)
			input.Body = &text
		}
	}
	result, ok := s.runScript(ctx, rule, input)
	if !ok || (result.Body != nil && input.Body == nil) {
		return "response_skipped"
	}
	applyHeaders(response.Header, result)
	if result.Body != nil {
		response.Body = io.NopCloser(strings.NewReader(*result.Body))
		response.Header.Set("Content-Length", strconv.Itoa(len(*result.Body)))
		for _, header := range []string{"ETag", "Last-Modified", "Content-MD5", "Digest", "Content-Digest", "Repr-Digest", "Accept-Ranges"} {
			response.Header.Del(header)
		}
	}
	return "response"
}

func accessibleBody(input scriptInput, response *http.Response) bool {
	kind, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return input.Method != http.MethodHead && response.StatusCode >= 200 &&
		response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusResetContent &&
		response.StatusCode != http.StatusNotModified && response.StatusCode != http.StatusPartialContent &&
		response.ContentLength <= maxBody && response.Header.Get("Content-Encoding") == "" &&
		(strings.HasPrefix(kind, "text/") || kind == "application/json" || strings.HasSuffix(kind, "+json"))
}

func (s *session) tunnel(w http.ResponseWriter, r *http.Request, now time.Time) {
	host, port, err := net.SplitHostPort(r.URL.Host)
	portNumber, portErr := strconv.ParseUint(port, 10, 16)
	if err != nil || portErr != nil || portNumber == 0 || host == "" || len(r.URL.Host) > 259 ||
		!httpguts.ValidHostHeader(r.URL.Host) || r.URL.User != nil || r.URL.Path != "" ||
		r.URL.RawQuery != "" || r.ContentLength != 0 {
		http.Error(w, "Invalid CONNECT target", http.StatusBadRequest)
		return
	}
	dialCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	upstream, err := s.dial(dialCtx, "tcp", r.URL.Host)
	cancel()
	entry := Entry{Time: now, Method: http.MethodConnect, Host: r.URL.Host, DurationMS: time.Since(now).Milliseconds()}
	if err != nil {
		entry.Status = http.StatusBadGateway
		s.controller.record(s, entry)
		http.Error(w, "sing-box upstream unavailable", entry.Status)
		return
	}
	defer upstream.Close()
	client, buffered, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, "Tunnel unavailable", http.StatusBadGateway)
		return
	}
	defer client.Close()
	defer context.AfterFunc(r.Context(), func() {
		_ = client.Close()
		_ = upstream.Close()
	})()
	deadline := now.Add(10 * time.Minute)
	_ = client.SetDeadline(deadline)
	_ = upstream.SetDeadline(deadline)
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil || buffered.Flush() != nil {
		return
	}
	entry.Status = http.StatusOK
	s.controller.record(s, entry)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(upstream, buffered)
		_ = upstream.Close()
	}()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	<-done
}

func validTarget(target *url.URL) bool {
	return (target.Scheme == "http" || target.Scheme == "https") && target.Hostname() != "" &&
		len(target.Host) <= 259 && httpguts.ValidHostHeader(target.Host) &&
		target.User == nil && target.Fragment == "" && target.Opaque == ""
}

func cleanHeaders(source http.Header) http.Header {
	for _, list := range source.Values("Connection") {
		for name := range strings.SplitSeq(list, ",") {
			source.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Proxy-Connection", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		source.Del(name)
	}
	return source
}
