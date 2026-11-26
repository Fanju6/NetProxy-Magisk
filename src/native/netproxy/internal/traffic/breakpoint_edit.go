package traffic

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

func validateBreakpointEdit(view PausedStream, edit BreakpointEdit) (BreakpointEdit, error) {
	if view.Phase == "response" && (edit.URL != nil || edit.Method != nil) ||
		view.Phase == "request" && edit.Status != nil ||
		edit.Status != nil && (*edit.Status < 200 || *edit.Status > 599) {
		return edit, ErrBreakpointEdit
	}
	method, target := view.Method, view.URL
	if edit.Method != nil {
		method = *edit.Method
	}
	if edit.URL != nil {
		target = *edit.URL
	}
	request, err := (&ReplayEdit{Method: method, URL: target, Headers: edit.Headers}).request(context.Background())
	if err != nil {
		return edit, ErrBreakpointEdit
	}
	if edit.Headers != nil {
		edit.Headers = request.Header
	}
	if edit.Body != nil {
		status, headers := view.Status, view.Headers
		if edit.Status != nil {
			status = *edit.Status
		}
		if edit.Headers != nil {
			headers = edit.Headers
		}
		if view.Body == nil || len(*edit.Body) > maxBody || !utf8.ValidString(*edit.Body) ||
			len(headers.Values("Content-Encoding")) != 0 ||
			view.Phase == "response" && !breakpointResponseBody(method, status) {
			return edit, ErrBreakpointEdit
		}
	}
	return edit, nil
}

func breakpointResponseBody(method string, status int) bool {
	return method != http.MethodHead && status >= 200 && status != http.StatusNoContent &&
		status != http.StatusResetContent && status != http.StatusNotModified && status != http.StatusPartialContent
}

func breakpointText(body io.ReadCloser, length int64, headers http.Header) (io.ReadCloser, *string) {
	if body == nil || body == http.NoBody || length == 0 {
		empty := ""
		return body, &empty
	}
	if length < 0 || headers.Get("Content-Range") != "" || !accessibleBody(length, headers) {
		return body, nil
	}
	return readRewriteBody(body)
}

func breakpointChangedHeaders(a, b http.Header) bool {
	stripLength := func(headers http.Header) http.Header {
		copy := headers.Clone()
		copy.Del("Content-Length")
		return copy
	}
	return !maps.EqualFunc(stripLength(a), stripLength(b), slices.Equal[[]string])
}

func applyBreakpointRequest(request *http.Request, edit *BreakpointEdit, audit *BreakpointAudit) {
	if edit.Method != nil && request.Method != *edit.Method {
		request.Method = *edit.Method
		audit.Changes = append(audit.Changes, "method")
	}
	if edit.URL != nil && request.URL.String() != *edit.URL {
		target, _ := url.Parse(*edit.URL)
		audit.OriginChanged = originKey(request.URL) != originKey(target)
		request.URL, request.Host = target, target.Host
		audit.Changes = append(audit.Changes, "url")
	}
	if edit.Headers != nil && breakpointChangedHeaders(request.Header, edit.Headers) {
		request.Header = edit.Headers
		audit.Changes = append(audit.Changes, "headers")
	}
	if edit.Body != nil {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		request.Body = io.NopCloser(strings.NewReader(*edit.Body))
		request.ContentLength, request.GetBody = int64(len(*edit.Body)), nil
		if request.ContentLength == 0 {
			request.Body = http.NoBody
		}
		request.TransferEncoding, request.Trailer = nil, nil
		request.Header.Del("Content-Length")
		audit.Changes = append(audit.Changes, "body")
	}
}

func applyBreakpointResponse(response *http.Response, edit *BreakpointEdit, audit *BreakpointAudit) {
	if edit.Status != nil && response.StatusCode != *edit.Status {
		response.StatusCode = *edit.Status
		audit.Changes = append(audit.Changes, "status")
	}
	if edit.Headers != nil && breakpointChangedHeaders(response.Header, edit.Headers) {
		response.Header = edit.Headers
		if response.ContentLength >= 0 {
			response.Header.Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
		}
		audit.Changes = append(audit.Changes, "headers")
	}
	if edit.Body != nil || response.StatusCode == http.StatusNoContent ||
		response.StatusCode == http.StatusResetContent || response.StatusCode == http.StatusNotModified {
		_ = response.Body.Close()
		response.Body, response.ContentLength = http.NoBody, 0
		response.Header.Del("Content-Length")
		response.Trailer = nil
		if edit.Body != nil {
			response.Body = io.NopCloser(strings.NewReader(*edit.Body))
			response.ContentLength = int64(len(*edit.Body))
			response.Header.Set("Content-Length", strconv.Itoa(len(*edit.Body)))
			audit.Changes = append(audit.Changes, "body")
		}
	}
}

func originKey(target *url.URL) [3]string {
	port := target.Port()
	if port == "" {
		port = "80"
		if target.Scheme == "https" {
			port = "443"
		}
	}
	return [3]string{target.Scheme, strings.ToLower(target.Hostname()), port}
}

func newBreakpointAudit(view PausedStream, decision breakpointDecision) BreakpointAudit {
	result := BreakpointAudit{Rule: view.Rule, Phase: view.Phase, Outcome: decision.outcome}
	switch decision.outcome {
	case "edited", "continued", "aborted":
		result.Actor = "operator"
	default:
		result.Actor = "worker"
	}
	return result
}
