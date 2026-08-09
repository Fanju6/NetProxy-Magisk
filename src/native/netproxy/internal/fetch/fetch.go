package fetch

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const maxSubscriptionSize = 20 << 20

// 订阅服务普遍按 User-Agent 白名单决定是否返回 Subscription-Userinfo、
// Profile-Title 等扩展响应头，自定义 UA 会被静默忽略（响应 200 但无这些头）。
// 本模块内核即 sing-box，故使用该标识：既如实描述客户端，也在白名单内。
// 不带内核版本号，避免内核升级后 UA 失真。
const defaultUserAgent = "sing-box"

// Android may leave resolv.conf pointing at a loopback DNS listener owned by the stopped core.
var fallbackDNSServers = []string{
	"223.5.5.5:53",
	"119.29.29.29:53",
	"1.1.1.1:53",
	"8.8.8.8:53",
}

type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

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
		request.UserAgent = defaultUserAgent
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           subscriptionDialContext,
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
			return Response{}, fmt.Errorf("subscription request failed: %w", urlError.Err)
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

func subscriptionDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	host, port, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) != nil {
		return dialer.DialContext(ctx, network, address)
	}

	addresses, err := lookupIPAddresses(ctx, host, subscriptionResolvers())
	if err != nil {
		return nil, err
	}
	var lastError error
	for _, resolved := range addresses {
		conn, dialError := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
		if dialError == nil {
			return conn, nil
		}
		lastError = dialError
	}
	if lastError == nil {
		lastError = errors.New("DNS returned no usable address")
	}
	return nil, lastError
}

func subscriptionResolvers() []ipResolver {
	resolvers := []ipResolver{net.DefaultResolver}
	for _, server := range fallbackDNSServers {
		server := server
		resolvers = append(resolvers, &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, server)
			},
		})
	}
	return resolvers
}

func lookupIPAddresses(ctx context.Context, host string, resolvers []ipResolver) ([]net.IPAddr, error) {
	var lastError error
	for _, resolver := range resolvers {
		lookupContext, cancel := context.WithTimeout(ctx, 3*time.Second)
		addresses, err := resolver.LookupIPAddr(lookupContext, host)
		cancel()
		if err == nil && len(addresses) > 0 {
			return addresses, nil
		}
		if err != nil {
			lastError = err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if lastError == nil {
		lastError = errors.New("DNS returned no address")
	}
	return nil, fmt.Errorf("lookup %s: %w", host, lastError)
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
		metadata.FileName = decodeDispositionFileName(metadata.ContentDisposition)
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
		// 空值是合法写法：机场用 expire= 表示永不过期，不应记为畸形字段
		if strings.TrimSpace(rawValue) == "" {
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

// decodeHeaderValue 解码订阅服务返回的标题类响应头。
// 依次尝试机场惯用的 base64: 前缀与标准 RFC 2047 编码字；均不适用时原样返回。
func decodeHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// 机场普遍使用 "base64:<标准 base64>" 携带非 ASCII 标题，非标准但事实通行
	if rest, found := cutPrefixFold(value, "base64:"); found {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rest)); err == nil {
			if text := strings.TrimSpace(string(decoded)); text != "" && utf8.ValidString(text) {
				return text
			}
		}
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

// cutPrefixFold 按大小写不敏感方式切除前缀
func cutPrefixFold(value, prefix string) (string, bool) {
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return value, false
	}
	return value[len(prefix):], true
}

// decodeDispositionFileName 从 Content-Disposition 取出订阅名。
// mime.ParseMediaType 已处理 RFC 5987 的 filename*=UTF-8”xxx；但普通
// filename="xxx" 携带原始 UTF-8 字节时会被按 latin-1 逐字节解读成乱码，
// 需要还原。同时兼容部分服务对 filename 做百分号编码的写法。
func decodeDispositionFileName(disposition string) string {
	if strings.TrimSpace(disposition) == "" {
		return ""
	}
	_, parameters, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(parameters["filename"])
	if name == "" {
		return ""
	}
	if !utf8.ValidString(name) {
		// latin-1 误读的还原：每个 rune 实际是一个原始字节
		raw := make([]byte, 0, len(name))
		for _, r := range name {
			if r > 0xFF {
				raw = nil
				break
			}
			raw = append(raw, byte(r))
		}
		if len(raw) > 0 && utf8.Valid(raw) {
			name = string(raw)
		}
	}
	if strings.Contains(name, "%") {
		// PathUnescape 而非 QueryUnescape：后者会把 '+' 变成空格，破坏字面加号
		if decoded, err := url.PathUnescape(name); err == nil && strings.TrimSpace(decoded) != "" {
			name = strings.TrimSpace(decoded)
		}
	}
	// 防路径注入：文件名可能被用于展示与命名，不允许携带路径分隔符
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	switch name {
	case ".", "..", "/":
		return ""
	}
	lower := strings.ToLower(name)
	for _, extension := range []string{".yaml", ".yml", ".txt", ".json"} {
		if strings.HasSuffix(lower, extension) {
			name = name[:len(name)-len(extension)]
			break
		}
	}
	return strings.TrimSpace(name)
}

func allowedCustomHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "user-agent", "x-hwid", "if-none-match", "if-modified-since", "host", "content-length":
		return false
	default:
		return true
	}
}
