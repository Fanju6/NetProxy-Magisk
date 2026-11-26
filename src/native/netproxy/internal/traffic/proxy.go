package traffic

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

func (s *session) proxyHTTP(w http.ResponseWriter, r *http.Request, now time.Time, transport http.RoundTripper, metadata flow, sequence uint64, closeOriginal func()) {
	isWebSocket := webSocketUpgrade(r.Header)
	isH2 := r.ProtoMajor == 2
	if isWebSocket && r.Body != nil && r.Body != http.NoBody {
		http.Error(w, "WebSocket upgrade must not include a request body", http.StatusBadRequest)
		return
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if isWebSocket {
		ctx, cancel = context.WithCancel(r.Context())
	} else {
		ctx, cancel = context.WithTimeout(r.Context(), 2*time.Minute+s.breakpointBudget())
	}
	defer cancel()
	url := *r.URL
	if isWebSocket {
		url.Scheme = "ws"
		if r.URL.Scheme == "https" {
			url.Scheme = "wss"
		}
	}
	target := url.String()
	rules, candidates := s.controller.snapshotRules(r.URL.Hostname())
	method := r.Method
	if len(method) > 16 {
		method = "OTHER"
	}
	entry := metadata.entry(now, method, r.URL.Host)
	entry.Protocol, entry.Sequence = "http/1.1", sequence
	if isH2 {
		entry.Protocol = "h2"
	} else if r.ProtoMinor == 0 {
		entry.Protocol = "http/1.0"
	}
	entry.Target = strings.Clone(validPrefix(r.URL.RequestURI(), 1024))
	if r.URL.Scheme == "https" {
		entry.TLS = "decrypted"
	}
	request := r.Clone(ctx)
	requestHeaders := r.Header.Clone()
	requestHeaders.Set("Host", r.Host)
	if len(r.TransferEncoding) != 0 {
		requestHeaders["Transfer-Encoding"] = r.TransferEncoding
	}
	detail := &HTTPDetail{
		URL: strings.Clone(validPrefix(target, captureURLLimit)), URLTruncated: len(target) > captureURLLimit,
		Request: captureMessage(requestHeaders),
	}
	matches, requestMatch, responseMatch := matchingRules(rules, candidates, target)
	detail.Rewrites = matches
	media := s.controller.mediaFactory(s)
	var capturedRequest *captureReader
	if r.Body != nil && r.Body != http.NoBody {
		capturedRequest = &captureReader{ReadCloser: r.Body}
		capturedRequest.body.initMedia(requestHeaders, media)
		request.Body = capturedRequest
	}
	capturedResponse := &captureWriter{ResponseWriter: w, message: &detail.Response, head: r.Method == http.MethodHead, factory: media}
	w = capturedResponse
	streamDone := make(chan struct{})
	defer close(streamDone)
	defer func() {
		var requestMedia *mediaSpool
		if capturedRequest != nil {
			requestMedia = capturedRequest.body.finishMedia()
			detail.Request.setBody(capturedRequest.body.body(requestHeaders, r.ContentLength,
				r.ContentLength < 0 && !capturedRequest.eof.Load()))
			if isH2 && capturedRequest.eof.Load() {
				detail.Request.setTrailers(r.Trailer)
			}
			if isH2 {
				detail.Request.setGRPC(&capturedRequest.body)
			}
		} else {
			detail.Request.setBody((&bodyCapture{}).body(requestHeaders, 0, false))
		}
		responseMedia := capturedResponse.body.finishMedia()
		var responseLength int64
		if !capturedResponse.head && entry.Status >= 200 && entry.Status != http.StatusNoContent && entry.Status != http.StatusNotModified {
			responseLength, _ = strconv.ParseInt(w.Header().Get("Content-Length"), 10, 64)
		}
		detail.Response.setBody(capturedResponse.body.body(w.Header(), responseLength, capturedResponse.failed))
		entry.DurationMS = time.Since(now).Milliseconds()
		s.controller.recordMediaDetail(s, entry, detail, requestMedia, responseMedia)
	}()
	request.RequestURI = ""
	request.Host = request.URL.Host
	request.Header = cleanHeaders(request.Header)
	if isWebSocket {
		setWebSocketUpgrade(request.Header)
	}
	request.Trailer = nil
	if isH2 {
		if slicesContainFold(r.Header.Values("Te"), "trailers") {
			request.Header.Set("Te", "trailers")
		}
		if capturedRequest != nil {
			request.Trailer = make(http.Header, len(r.Trailer))
			for name := range r.Trailer {
				request.Trailer[name] = nil
			}
			capturedRequest.finish = func() {
				for name, values := range r.Trailer {
					request.Trailer[name] = values
				}
			}
		}
	}
	if request.ContentLength == 0 {
		request.Body = nil
	}
	if requestMatch >= 0 {
		match := &matches[requestMatch]
		input := scriptInput{URL: target, Method: r.Method}
		if request.Body != nil && request.Body != http.NoBody && request.Header.Get("Content-Range") == "" &&
			accessibleBody(request.ContentLength, request.Header) {
			request.Body, input.Body = readRewriteBody(request.Body)
		}
		result := s.runScript(ctx, &rules[match.RuleIndex], input, match)
		if result.Body != nil && input.Body == nil {
			match.Error = "body_unavailable"
		}
		if match.Error != "" {
			entry.Rewrite = "request_skipped"
			match.finish()
		} else {
			entry.Rewrite = "request"
			if result.Reject {
				match.Actions = append(match.Actions, "reject")
				entry.Rewritten = match.finish()
				entry.Status = http.StatusForbidden
				http.Error(w, "Rejected by rewrite rule", entry.Status)
				return
			}
			if result.Redirect != "" {
				match.Actions = append(match.Actions, "redirect")
				match.Redirect = result.Redirect
				entry.Rewritten = match.finish()
				entry.Status = http.StatusFound
				w.Header().Set("Location", result.Redirect)
				w.WriteHeader(entry.Status)
				return
			}
			track := match.trackHeaders(request.Header, result)
			applyHeaders(request.Header, result)
			if result.Body != nil {
				_ = request.Body.Close()
				request.Body = http.NoBody
				if *result.Body != "" {
					request.Body = io.NopCloser(strings.NewReader(*result.Body))
				}
				request.GetBody = nil
				request.ContentLength = int64(len(*result.Body))
				request.TransferEncoding = nil
				request.Header.Del("Content-Length")
				recordBodyChange(match, request.Header, *input.Body, *result.Body)
			}
			track()
			entry.Rewritten = match.finish()
		}
	}
	if responseMatch >= 0 && request.Header.Get("Range") == "" {
		request.Header.Del("Accept-Encoding")
	}
	var routed *replayConn
	var routeAudit *BreakpointAudit
	if !isWebSocket {
		if name, timeout, matched := s.reserveBreakpoint("request", request.URL.Hostname(), request.URL.String()); matched {
			if !isH2 {
				_ = http.NewResponseController(capturedResponse.ResponseWriter).SetWriteDeadline(time.Now().Add(2*time.Minute + 2*timeout))
			}
			var text *string
			request.Body, text = breakpointText(request.Body, request.ContentLength, request.Header)
			view := PausedStream{Rule: name, Phase: "request", URL: request.URL.String(), Method: request.Method,
				Headers: request.Header.Clone(), Body: text}
			decision := s.pauseBreakpoint(ctx, view, timeout, streamDone)
			audit := newBreakpointAudit(view, decision)
			if decision.edit != nil {
				applyBreakpointRequest(request, decision.edit, &audit)
			}
			detail.Breakpoints = append(detail.Breakpoints, audit)
			if decision.outcome == "aborted" {
				entry.Status = http.StatusBadGateway
				http.Error(w, "Aborted by breakpoint", entry.Status)
				return
			}
			if audit.OriginChanged {
				routeAudit = &detail.Breakpoints[len(detail.Breakpoints)-1]
				routeAudit.OriginalExit = entry.Exit
				if routeAudit.OriginalExit == nil && entry.Outbound != "" {
					routeAudit.OriginalExit = &Exit{Chain: []ExitHop{{Tag: entry.Outbound}}}
				}
				owner := attributedOwner(entry.SourceApp)
				routeAudit.Unattributed = owner == nil
				if closeOriginal != nil {
					closeOriginal()
				}
				var closeRoute func()
				var err error
				transport, routed, closeRoute, err = s.openRoutedTransport(request, owner)
				if err != nil {
					routeAudit.RouteFailure = "core_unavailable"
					entry.Status = http.StatusBadGateway
					http.Error(w, "Breakpoint route unavailable", entry.Status)
					return
				}
				defer closeRoute()
			}
		}
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		if routeAudit != nil {
			routeAudit.RouteFailure = "response_failed"
			if errors.Is(err, ErrReplayRoute) {
				routeAudit.RouteFailure = "route_failed"
			}
		}
		entry.Status = http.StatusBadGateway
		entry.PassthroughReason = metadata.upstreamFailure
		http.Error(w, "sing-box upstream unavailable", entry.Status)
		return
	}
	if routed != nil && routed.route.Exit != nil {
		routeAudit.ActualExit = routed.route.Exit
		routeAudit.RouteChanged = routeAudit.OriginalExit != nil &&
			!slices.Equal(routeAudit.OriginalExit.Chain, routed.route.Exit.Chain)
	}
	defer response.Body.Close()
	if isWebSocket && response.StatusCode == http.StatusSwitchingProtocols && webSocketUpgrade(response.Header) {
		entry.Status = response.StatusCode
		if upstream, ok := response.Body.(io.ReadWriteCloser); ok {
			s.captureWebSocket(capturedResponse.ResponseWriter, response, upstream, detail)
		} else {
			entry.Status = http.StatusBadGateway
			http.Error(w, "Invalid upstream WebSocket", entry.Status)
		}
		return
	}
	response.Header = cleanHeaders(response.Header)
	entry.Status = response.StatusCode
	if responseMatch >= 0 {
		match := &matches[responseMatch]
		match.OriginalStatus = response.StatusCode
		state := s.rewriteResponse(ctx, &rules[match.RuleIndex], scriptInput{URL: target, Method: request.Method, Status: entry.Status}, response, match)
		entry.Rewritten = match.finish() || entry.Rewritten
		if entry.Rewrite == "" {
			entry.Rewrite = state
		} else {
			entry.Rewrite += "+" + state
		}
	}
	if !isWebSocket {
		if name, timeout, matched := s.reserveBreakpoint("response", request.URL.Hostname(), request.URL.String()); matched {
			if !isH2 {
				_ = http.NewResponseController(capturedResponse.ResponseWriter).SetWriteDeadline(time.Now().Add(2*time.Minute + timeout))
			}
			var text *string
			if breakpointResponseBody(request.Method, response.StatusCode) {
				response.Body, text = breakpointText(response.Body, response.ContentLength, response.Header)
			}
			view := PausedStream{Rule: name, Phase: "response", URL: request.URL.String(), Method: request.Method,
				Status: response.StatusCode, Headers: response.Header.Clone(), Body: text}
			decision := s.pauseBreakpoint(ctx, view, timeout, streamDone)
			audit := newBreakpointAudit(view, decision)
			if decision.edit != nil {
				applyBreakpointResponse(response, decision.edit, &audit)
			}
			detail.Breakpoints = append(detail.Breakpoints, audit)
			if decision.outcome == "aborted" {
				entry.Status = http.StatusBadGateway
				http.Error(w, "Aborted by breakpoint", entry.Status)
				return
			}
		}
	}
	entry.Status = response.StatusCode
	for name, values := range response.Header {
		w.Header()[name] = values
	}
	w.WriteHeader(response.StatusCode)
	if isH2 && grpcContentType(response.Header) {
		flusher := http.NewResponseController(capturedResponse.ResponseWriter)
		_ = flusher.Flush()
		_, err = io.Copy(flushWriter{w, flusher}, response.Body)
	} else {
		_, err = io.Copy(w, response.Body)
	}
	if isH2 {
		detail.Response.setTrailers(response.Trailer)
		detail.Response.setGRPC(&capturedResponse.body)
		for name, values := range response.Trailer {
			w.Header()[http.TrailerPrefix+name] = values
		}
	}
	capturedResponse.failed = capturedResponse.failed || err != nil
}

func slicesContainFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

type flushWriter struct {
	io.Writer
	*http.ResponseController
}

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if err == nil {
		err = w.Flush()
	}
	return n, err
}

func (s *session) captureWebSocket(w http.ResponseWriter, response *http.Response, upstream io.ReadWriteCloser, detail *HTTPDetail) {
	conn, client, err := hijackStream(w)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Time{})
	var handshake bytes.Buffer
	handshake.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	headers := cleanHeaders(response.Header.Clone())
	setWebSocketUpgrade(headers)
	if headers.Write(&handshake) != nil {
		return
	}
	handshake.WriteString("\r\n")
	if _, err := handshake.WriteTo(conn); err != nil {
		return
	}
	detail.Response = captureMessage(headers)
	frames := &webSocketCapture{}
	frames.configure(headers)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer upstream.Close()
		defer conn.Close()
		frames.relay(upstream, client, "send")
	}()
	frames.relay(conn, upstream, "receive")
	_ = conn.Close()
	_ = upstream.Close()
	<-done
	detail.WebSocket = &frames.detail
}

func (s *session) runScript(ctx context.Context, rule *compiledRule, input scriptInput, match *RewriteMatch) (result rewrite) {
	start := time.Now()
	defer func() {
		match.DurationUS = time.Since(start).Microseconds()
		if recover() != nil {
			s.logPanic("traffic.rewrite")
			match.Error = "script_error"
		}
	}()
	select {
	case s.scriptSlots <- struct{}{}:
		defer func() { <-s.scriptSlots }()
		result, code := rule.run(ctx, input)
		match.Error = code
		return result
	default:
		match.Error = "busy"
		return rewrite{}
	}
}

var bodyValidators = []string{"ETag", "Last-Modified", "Content-MD5", "Digest", "Content-Digest", "Repr-Digest", "Accept-Ranges"}

func (s *session) rewriteResponse(ctx context.Context, rule *compiledRule, input scriptInput, response *http.Response, match *RewriteMatch) string {
	if input.Method != http.MethodHead && response.StatusCode >= 200 &&
		response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusResetContent &&
		response.StatusCode != http.StatusNotModified && response.StatusCode != http.StatusPartialContent &&
		accessibleBody(response.ContentLength, response.Header) {
		response.Body, input.Body = readRewriteBody(response.Body)
	}
	result := s.runScript(ctx, rule, input, match)
	if result.Body != nil && input.Body == nil {
		match.Error = "body_unavailable"
	}
	if match.Error != "" {
		return "response_skipped"
	}
	track := match.trackHeaders(response.Header, result)
	applyHeaders(response.Header, result)
	if result.Body != nil {
		_ = response.Body.Close()
		response.Body = io.NopCloser(strings.NewReader(*result.Body))
		response.ContentLength = int64(len(*result.Body))
		response.Header.Set("Content-Length", strconv.Itoa(len(*result.Body)))
		recordBodyChange(match, response.Header, *input.Body, *result.Body)
	}
	track()
	return "response"
}

func accessibleBody(length int64, headers http.Header) bool {
	kind, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	return length <= maxBody && len(headers.Values("Content-Encoding")) == 0 &&
		(strings.HasPrefix(kind, "text/") || kind == "application/json" || strings.HasSuffix(kind, "+json"))
}

func readRewriteBody(body io.ReadCloser) (io.ReadCloser, *string) {
	content, err := io.ReadAll(io.LimitReader(body, maxBody+1))
	replay := struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(content), body), body}
	if err != nil || len(content) > maxBody || !utf8.Valid(content) {
		return replay, nil
	}
	text := string(content)
	return replay, &text
}

func recordBodyChange(match *RewriteMatch, headers http.Header, before, after string) {
	match.Body = &BodyChange{Before: len(before), After: len(after)}
	match.Actions = append(match.Actions, "body")
	for _, header := range bodyValidators {
		headers.Del(header)
	}
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
