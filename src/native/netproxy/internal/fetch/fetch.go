package fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const maxSubscriptionSize = 20 << 20

type Request struct {
	URL           string
	UserAgent     string
	HWID          string
	Headers       map[string]string
	ETag          string
	LastModified  string
	ProxyURL      string
	AllowInsecure bool
	Timeout       time.Duration
}

type Usage struct {
	Upload   *int64 `json:"upload,omitempty"`
	Download *int64 `json:"download,omitempty"`
	Total    *int64 `json:"total,omitempty"`
	Expire   *int64 `json:"expire,omitempty"`
}

type Metadata struct {
	StatusCode            int                   `json:"status_code"`
	NotModified           bool                  `json:"not_modified"`
	ETag                  string                `json:"etag,omitempty"`
	LastModified          string                `json:"last_modified,omitempty"`
	ContentDisposition    string                `json:"content_disposition,omitempty"`
	FileName              string                `json:"file_name,omitempty"`
	ProfileTitle          string                `json:"profile_title,omitempty"`
	ProfileWebPageURL     string                `json:"profile_web_page_url,omitempty"`
	UpdateIntervalSeconds *int64                `json:"update_interval_seconds,omitempty"`
	Usage                 *Usage                `json:"usage,omitempty"`
	Diagnostics           []provider.Diagnostic `json:"diagnostics,omitempty"`
}

type Response struct {
	Body     []byte
	Metadata Metadata
}

func Subscription(ctx context.Context, request Request) (Response, error) {
	if strings.TrimSpace(request.URL) == "" {
		return Response{}, errors.New("subscription URL is required")
	}
	if request.Timeout <= 0 {
		request.Timeout = 60 * time.Second
	}
	if request.UserAgent == "" {
		request.UserAgent = "NetProxy/8.0"
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: request.Timeout,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: request.AllowInsecure, // Explicit user option.
		},
	}
	if request.ProxyURL != "" {
		proxyURL, err := url.Parse(request.ProxyURL)
		if err != nil {
			return Response{}, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Transport: transport, Timeout: request.Timeout}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return Response{}, errors.New("invalid subscription URL")
	}
	httpRequest.Header.Set("User-Agent", request.UserAgent)
	httpRequest.Header.Set("Accept", "*/*")
	if request.HWID != "" {
		httpRequest.Header.Set("X-HWID", request.HWID)
	}
	for key, value := range request.Headers {
		if !allowedCustomHeader(key) {
			return Response{}, fmt.Errorf("custom header %q is managed by NetProxy", key)
		}
		httpRequest.Header.Set(key, value)
	}
	if request.ETag != "" {
		httpRequest.Header.Set("If-None-Match", request.ETag)
	}
	if request.LastModified != "" {
		httpRequest.Header.Set("If-Modified-Since", request.LastModified)
	}

	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		var urlError *url.Error
		if errors.As(err, &urlError) {
			return Response{}, fmt.Errorf("subscription request failed: %v", urlError.Err)
		}
		return Response{}, errors.New("subscription request failed")
	}
	defer httpResponse.Body.Close()
	metadata := parseMetadata(httpResponse)
	if httpResponse.StatusCode == http.StatusNotModified {
		metadata.NotModified = true
		return Response{Metadata: metadata}, nil
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return Response{Metadata: metadata}, fmt.Errorf("subscription request failed: HTTP %d", httpResponse.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxSubscriptionSize+1))
	if err != nil {
		return Response{Metadata: metadata}, err
	}
	if len(body) > maxSubscriptionSize {
		return Response{Metadata: metadata}, fmt.Errorf("subscription content exceeds %d bytes", maxSubscriptionSize)
	}
	return Response{Body: body, Metadata: metadata}, nil
}

func parseMetadata(response *http.Response) Metadata {
	metadata := Metadata{
		StatusCode:         response.StatusCode,
		ETag:               response.Header.Get("ETag"),
		LastModified:       response.Header.Get("Last-Modified"),
		ContentDisposition: response.Header.Get("Content-Disposition"),
		ProfileTitle:       decodeHeaderValue(response.Header.Get("Profile-Title")),
		ProfileWebPageURL:  response.Header.Get("Profile-Web-Page-URL"),
	}
	if metadata.ContentDisposition != "" {
		if _, parameters, err := mime.ParseMediaType(metadata.ContentDisposition); err == nil {
			metadata.FileName = parameters["filename"]
		}
	}
	if rawInterval := strings.TrimSpace(response.Header.Get("Profile-Update-Interval")); rawInterval != "" {
		if hours, err := strconv.ParseInt(rawInterval, 10, 64); err == nil && hours > 0 {
			seconds := hours * int64(time.Hour/time.Second)
			metadata.UpdateIntervalSeconds = &seconds
		} else {
			metadata.Diagnostics = append(metadata.Diagnostics, provider.Diagnostic{
				Code: "header.profile_update_interval_invalid", Message: "invalid profile-update-interval header",
			})
		}
	}
	metadata.Usage, metadata.Diagnostics = parseUsage(response.Header.Get("Subscription-Userinfo"), metadata.Diagnostics)
	return metadata
}

func parseUsage(value string, diagnostics []provider.Diagnostic) (*Usage, []provider.Diagnostic) {
	if strings.TrimSpace(value) == "" {
		return nil, diagnostics
	}
	usage := &Usage{}
	valid := false
	for _, part := range strings.Split(value, ";") {
		key, rawValue, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		number, err := strconv.ParseInt(strings.TrimSpace(rawValue), 10, 64)
		if err != nil || number < 0 {
			diagnostics = append(diagnostics, provider.Diagnostic{
				Source:  strings.ToLower(strings.TrimSpace(key)),
				Code:    "header.subscription_userinfo_invalid",
				Message: "invalid subscription-userinfo field",
			})
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "upload":
			usage.Upload = &number
		case "download":
			usage.Download = &number
		case "total":
			usage.Total = &number
		case "expire":
			usage.Expire = &number
		default:
			continue
		}
		valid = true
	}
	if !valid {
		return nil, diagnostics
	}
	return usage, diagnostics
}

func decodeHeaderValue(value string) string {
	if value == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func allowedCustomHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "user-agent", "x-hwid", "if-none-match", "if-modified-since", "host", "content-length":
		return false
	default:
		return true
	}
}
