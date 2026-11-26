package traffic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	MaxHARImportBytes   = 4 << 20
	MaxHARImportRecords = 2000
)

var (
	ErrHARInvalid = errors.New("HAR 文件无效或超出导入限制")
	ErrHARMissing = errors.New("抓包记录或批次不存在")
	ErrHARFilter  = errors.New("HAR 导出筛选条件无效")
)

type HARFilter struct {
	Batch, ID, Host, Query, Method, TLS, App, Outbound string
	StatusClass                                        int
	Rewritten                                          bool
	ResponseTypes                                      ResponseTypes
}

type harPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harRequest struct {
	Method      string    `json:"method"`
	URL         string    `json:"url"`
	HTTPVersion string    `json:"httpVersion"`
	Cookies     []harPair `json:"cookies"`
	Headers     []harPair `json:"headers"`
	Query       []harPair `json:"queryString"`
	HeadersSize int64     `json:"headersSize"`
	BodySize    int64     `json:"bodySize"`
	PostData    *harBody  `json:"postData,omitempty"`
}

type harResponse struct {
	Status      int       `json:"status"`
	StatusText  string    `json:"statusText"`
	HTTPVersion string    `json:"httpVersion"`
	Cookies     []harPair `json:"cookies"`
	Headers     []harPair `json:"headers"`
	RedirectURL string    `json:"redirectURL"`
	HeadersSize int64     `json:"headersSize"`
	BodySize    int64     `json:"bodySize"`
	Content     *harBody  `json:"content,omitempty"`
}

type harBody struct {
	MimeType string  `json:"mimeType"`
	Size     int64   `json:"size"`
	Text     *string `json:"text,omitempty"`
	Encoding string  `json:"encoding,omitempty"`
}

type harMetadata struct {
	SourceApp                 *SourceApp        `json:"source_app,omitempty"`
	Exit                      *Exit             `json:"exit,omitempty"`
	Outbound                  string            `json:"outbound,omitempty"`
	TLS                       string            `json:"tls,omitempty"`
	PassthroughReason         string            `json:"passthrough_reason,omitempty"`
	Rewrites                  []RewriteMatch    `json:"rewrites,omitempty"`
	Breakpoints               []BreakpointAudit `json:"breakpoints,omitempty"`
	Rewritten                 bool              `json:"rewritten,omitempty"`
	Rewrite                   string            `json:"rewrite,omitempty"`
	URLTruncated              bool              `json:"url_truncated,omitempty"`
	RequestTruncated          bool              `json:"request_truncated,omitempty"`
	ResponseTruncated         bool              `json:"response_truncated,omitempty"`
	RequestHeadersTruncated   bool              `json:"request_headers_truncated,omitempty"`
	ResponseHeadersTruncated  bool              `json:"response_headers_truncated,omitempty"`
	RequestDecoded            bool              `json:"request_decoded,omitempty"`
	ResponseDecoded           bool              `json:"response_decoded,omitempty"`
	AggregatedTiming          bool              `json:"aggregated_timing,omitempty"`
	NoHTTP                    bool              `json:"no_http,omitempty"`
	Host                      string            `json:"host,omitempty"`
	Target                    string            `json:"target,omitempty"`
	Protocol                  string            `json:"protocol,omitempty"`
	Sequence                  uint64            `json:"sequence,omitempty"`
	CAStores                  *CAStores         `json:"ca_stores,omitempty"`
	RequestTrailers           []harPair         `json:"request_trailers,omitempty"`
	ResponseTrailers          []harPair         `json:"response_trailers,omitempty"`
	RequestTrailersTruncated  bool              `json:"request_trailers_truncated,omitempty"`
	ResponseTrailersTruncated bool              `json:"response_trailers_truncated,omitempty"`
	RequestGRPC               *GRPCDetail       `json:"request_grpc,omitempty"`
	ResponseGRPC              *GRPCDetail       `json:"response_grpc,omitempty"`
	WebSocket                 *WebSocketDetail  `json:"websocket,omitempty"`
}

type harEntry struct {
	Started  time.Time          `json:"startedDateTime"`
	Time     float64            `json:"time"`
	Request  *harRequest        `json:"request,omitempty"`
	Response *harResponse       `json:"response,omitempty"`
	Cache    map[string]any     `json:"cache"`
	Timings  map[string]float64 `json:"timings"`
	NetProxy harMetadata        `json:"_netproxy"`
}

type HARExport struct {
	controller *Controller
	filter     HARFilter
	batchID    string
	saved      []savedRecord
	live       []Record
}

func (c *Controller) PrepareHARExport(filter HARFilter) (*HARExport, error) {
	if filter.Batch != "" && !ValidRecordID(filter.Batch) || filter.ID != "" && !ValidRecordID(filter.ID) ||
		filter.Batch != "" && filter.ID != "" || filter.StatusClass < 0 || filter.StatusClass > 5 ||
		filter.TLS != "" && filter.TLS != "decrypted" && filter.TLS != "tunnel" ||
		len(filter.Host)+len(filter.Query)+len(filter.Method)+len(filter.App)+len(filter.Outbound) > 2048 {
		return nil, ErrHARFilter
	}
	filter.Host, filter.Query = historyHost(filter.Host), strings.ToLower(strings.TrimSpace(filter.Query))
	export := &HARExport{controller: c, filter: filter, batchID: filter.Batch}
	if filter.ID != "" {
		record, ok := c.Record(filter.ID)
		if !ok {
			return nil, ErrHARMissing
		}
		export.live = []Record{record}
		return export, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	history := c.history
	if history == nil {
		return export, nil
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	if export.batchID == "" && c.session != nil && c.batch != nil {
		export.batchID = c.batch.ID
	} else if export.batchID == "" && len(history.batches) != 0 {
		export.batchID = history.batches[len(history.batches)-1].ID
	}
	if c.batch != nil && export.batchID == c.batch.ID {
		for i := range c.count {
			export.live = append(export.live, c.entries[(c.next+maxEntries-c.count+i)%maxEntries])
		}
	}
	for i := len(history.batches) - 1; i >= 0; i-- {
		batch := history.batches[i]
		if export.batchID == "" || export.batchID == batch.ID {
			export.batchID = batch.ID
			export.saved = slices.Clone(batch.records)
			return export, nil
		}
	}
	if filter.Batch != "" && len(export.live) == 0 {
		return nil, ErrHARMissing
	}
	return export, nil
}

func (f HARFilter) matches(entry Entry) bool {
	if f.Host != "" && historyHost(entry.Host) != f.Host ||
		f.Method != "" && entry.Method != f.Method || f.StatusClass != 0 && entry.Status/100 != f.StatusClass ||
		f.Rewritten && !entry.Rewritten {
		return false
	}
	if f.TLS == "decrypted" && entry.TLS != "decrypted" ||
		f.TLS == "tunnel" && (entry.TLS == "decrypted" || entry.TLS == "" && entry.Method != "TCP") {
		return false
	}
	app := ""
	if a := entry.SourceApp; a != nil {
		if a.Package != "" {
			app = fmt.Sprintf("%d:%s", a.UserID, a.Package)
		} else {
			app = fmt.Sprintf("UID %d", a.UID)
		}
	} else if exit := entry.Exit; exit != nil {
		app = exit.Package
		if app == "" && exit.UID != nil {
			app = fmt.Sprintf("UID %d", *exit.UID)
		} else if app == "" {
			app = exit.Process
		}
	}
	if f.App != "" && f.App != app {
		return false
	}
	if f.Outbound != "" && f.Outbound != entry.Outbound &&
		(entry.Exit == nil || !slices.ContainsFunc(entry.Exit.Chain, func(h ExitHop) bool { return h.Tag == f.Outbound })) {
		return false
	}
	host := strings.ToLower(entry.Host + entry.Target)
	scheme := "http://"
	if entry.TLS == "decrypted" {
		scheme = "https://"
	}
	return f.Query == "" || strings.Contains(host, f.Query) || strings.Contains(scheme+host, f.Query)
}

func (e *HARExport) Write(ctx context.Context, w io.Writer) error {
	if _, err := io.WriteString(w, `{"log":{"version":"1.2","creator":{"name":"NetProxy","version":"1"},"entries":[`); err != nil {
		return err
	}
	seen := make(map[string]bool, len(e.live))
	first := true
	write := func(record Record) error {
		if !e.filter.matches(record.Entry) || e.filter.ResponseTypes != 0 && !e.filter.ResponseTypes.matches(responseBodyType(record)) {
			return nil
		}
		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		first = false
		return e.writeRecord(w, record)
	}
	batch := &savedBatch{Batch: Batch{ID: e.batchID}}
	for _, saved := range e.saved {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.filter.Host != "" && saved.host != e.filter.Host || !e.filter.ResponseTypes.matches(saved.responseType) {
			continue
		}
		record, ok := e.controller.history.readRecord(batch, saved)
		if ok {
			seen[saved.id] = true
			if err := write(record); err != nil {
				return err
			}
		}
	}
	for _, record := range e.live {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !seen[record.ID] {
			if err := write(record); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(w, "]}}\n")
	return err
}

func (e *HARExport) writeRecord(w io.Writer, record Record) error {
	detail := record.HTTP
	if detail == nil {
		detail = &HTTPDetail{URL: (&url.URL{Scheme: "https", Host: record.Host, Path: "/"}).String()}
	}
	requestFile := e.openBody(record, "request")
	responseFile := e.openBody(record, "response")
	if requestFile != nil {
		defer requestFile.Close()
	}
	if responseFile != nil {
		defer responseFile.Close()
	}
	version := "HTTP/1.1"
	if record.Protocol == "h2" {
		version = "HTTP/2.0"
	} else if record.Protocol == "h3" {
		version = "HTTP/3"
	} else if record.Protocol == "http/1.0" {
		version = "HTTP/1.0"
	}
	query := []harPair{}
	if target, err := url.Parse(detail.URL); err == nil {
		query = harPairs(http.Header(target.Query()))
	}
	entry := harEntry{Started: record.Time, Time: float64(record.DurationMS), Cache: map[string]any{},
		Timings: map[string]float64{"send": 0, "wait": float64(record.DurationMS), "receive": 0},
		NetProxy: harMetadata{SourceApp: record.SourceApp, Exit: record.Exit, Outbound: record.Outbound, TLS: record.TLS,
			PassthroughReason: record.PassthroughReason, Rewrite: record.Rewrite, Rewritten: record.Rewritten,
			Rewrites: detail.Rewrites, Breakpoints: detail.Breakpoints, URLTruncated: detail.URLTruncated,
			RequestTruncated:        detail.Request.Body.Truncated || detail.Request.Body.Blob && requestFile == nil,
			ResponseTruncated:       detail.Response.Body.Truncated || detail.Response.Body.Blob && responseFile == nil,
			RequestHeadersTruncated: detail.Request.HeadersTruncated, ResponseHeadersTruncated: detail.Response.HeadersTruncated,
			RequestDecoded: detail.Request.Body.Decoded, ResponseDecoded: detail.Response.Body.Decoded, AggregatedTiming: true,
			NoHTTP: record.HTTP == nil, Host: record.Host, Target: record.Target,
			Protocol: record.Protocol, Sequence: record.Sequence, CAStores: record.CAStores,
			RequestTrailers: harPairs(detail.Request.Trailers), ResponseTrailers: harPairs(detail.Response.Trailers),
			RequestTrailersTruncated: detail.Request.TrailersTruncated, ResponseTrailersTruncated: detail.Response.TrailersTruncated,
			RequestGRPC: detail.Request.GRPC, ResponseGRPC: detail.Response.GRPC, WebSocket: detail.WebSocket}}
	req := harRequest{Method: record.Method, URL: detail.URL, HTTPVersion: version, Cookies: []harPair{},
		Headers: harPairs(detail.Request.Headers), Query: query, HeadersSize: -1, BodySize: detail.Request.Body.Size}
	res := harResponse{Status: record.Status, StatusText: http.StatusText(record.Status), HTTPVersion: version,
		Cookies: []harPair{}, Headers: harPairs(detail.Response.Headers), RedirectURL: detail.Response.Headers.Get("Location"),
		HeadersSize: -1, BodySize: detail.Response.Body.Size}
	return harObject(w, entry, func() error {
		if _, err := io.WriteString(w, `,"request":`); err != nil {
			return err
		}
		requestBody := func() error { return writeHARBody(w, "postData", detail.Request.Body, requestFile) }
		if err := harObject(w, req, requestBody); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `,"response":`); err != nil {
			return err
		}
		return harObject(w, res, func() error { return writeHARBody(w, "content", detail.Response.Body, responseFile) })
	})
}

func (e *HARExport) openBody(record Record, side string) *os.File {
	if body := bodyFor(record, side); body == nil || !body.Blob {
		return nil
	}
	if e.filter.ID == "" {
		file, _ := os.Open(e.controller.history.path(e.batchID, mediaName(record.ID, side)))
		return file
	}
	file, _ := e.controller.OpenBody(record.ID, side)
	return file
}

func harPairs(headers http.Header) []harPair {
	result := []harPair{}
	for name, values := range headers {
		for _, value := range values {
			result = append(result, harPair{name, value})
		}
	}
	slices.SortStableFunc(result, func(a, b harPair) int { return strings.Compare(a.Name, b.Name) })
	return result
}

func harObject(w io.Writer, object any, tail func() error) error {
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	if _, err := w.Write(data[:len(data)-1]); err != nil {
		return err
	}
	if err := tail(); err != nil {
		return err
	}
	_, err = io.WriteString(w, "}")
	return err
}

func writeHARBody(w io.Writer, name string, body CapturedBody, file *os.File) error {
	if name == "postData" && body.Size == 0 && body.Data == "" && file == nil {
		return nil
	}
	value := harBody{Size: body.Size, MimeType: body.ContentType}
	if value.MimeType == "" {
		value.MimeType = "application/octet-stream"
	}
	if file == nil && (body.Data != "" || body.Size == 0) {
		value.Text = &body.Data
	}
	if file != nil || body.Format == "base64" {
		value.Encoding = "base64"
	}
	if _, err := fmt.Fprintf(w, ",%q:", name); err != nil {
		return err
	}
	return harObject(w, value, func() error {
		if file == nil {
			return nil
		}
		if _, err := io.WriteString(w, `,"text":"`); err != nil {
			return err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, w)
		if _, err := io.CopyN(encoder, file, int64(body.CapturedSize)); err != nil {
			return err
		}
		if err := encoder.Close(); err != nil {
			return err
		}
		_, err := io.WriteString(w, `"`)
		return err
	})
}
