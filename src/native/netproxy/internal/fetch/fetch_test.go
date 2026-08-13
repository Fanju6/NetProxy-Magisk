package fetch_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
)

func TestSubscriptionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=100; expire=2000000000")
		writer.Header().Set("Profile-Title", "NetProxy Test")
		writer.Header().Set("Profile-Update-Interval", "12")
		writer.Header().Set("ETag", "revision-1")
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer server.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.Metadata.Usage == nil || *response.Metadata.Usage.Total != 100 {
		t.Fatalf("usage metadata was not parsed: %#v", response.Metadata)
	}
	if response.Metadata.UpdateIntervalSeconds == nil || *response.Metadata.UpdateIntervalSeconds != 12*60*60 {
		t.Fatalf("update interval was not parsed: %#v", response.Metadata)
	}
	if response.Metadata.ETag != "revision-1" || string(response.Body) == "" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestSubscriptionDefaultUserAgentAndEmptyExpire(t *testing.T) {
	// 还原真实机场行为：仅当 UA 命中白名单才返回扩展头，且 expire 可能为空值
	var seenUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenUserAgent = request.UserAgent()
		writer.Header().Set("Subscription-Userinfo", "upload=1681342417; download=141967302921; total=1073741824000; expire=")
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer server.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if seenUserAgent != "sing-box" {
		t.Fatalf("默认 UA 必须命中订阅服务白名单，实际发送: %q", seenUserAgent)
	}
	usage := response.Metadata.Usage
	if usage == nil || usage.Total == nil || *usage.Total != 1073741824000 {
		t.Fatalf("expire 为空时仍须解析出其余用量字段: %#v", response.Metadata)
	}
	if usage.Expire != nil {
		t.Fatalf("expire 为空应视为永不过期而非 0: %#v", usage)
	}
	// 空值是合法写法，不得记为畸形字段
	for _, diagnostic := range response.Metadata.Diagnostics {
		if diagnostic.Code == "header.subscription_userinfo_invalid" {
			t.Fatalf("expire 为空不应产生诊断: %#v", response.Metadata.Diagnostics)
		}
	}
}

func TestSubscriptionDecodesTitleAndFileName(t *testing.T) {
	// 真实机场行为：Profile-Title 用 base64: 前缀，filename 直接携带原始 UTF-8 字节
	const want = "良心云"
	cases := []struct {
		name        string
		title       string
		disposition string
	}{
		{
			name:        "base64 前缀与原始 UTF-8 filename",
			title:       "base64:" + base64.StdEncoding.EncodeToString([]byte(want)),
			disposition: `attachment; filename="` + want + `"`,
		},
		{
			name:        "RFC 5987 filename*",
			title:       want,
			disposition: `attachment;filename*=UTF-8''%E8%89%AF%E5%BF%83%E4%BA%91`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Profile-Title", testCase.title)
				writer.Header().Set("Content-Disposition", testCase.disposition)
				_, _ = writer.Write([]byte("socks://example.com:1080#node"))
			}))
			defer server.Close()

			response, err := fetch.Subscription(context.Background(), fetch.Request{URL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			if response.Metadata.ProfileTitle != want {
				t.Fatalf("profile title 未解码: %q", response.Metadata.ProfileTitle)
			}
			if response.Metadata.FileName != want {
				t.Fatalf("file name 未解码: %q", response.Metadata.FileName)
			}
		})
	}
}

func TestSubscriptionFileNameRejectsPathTraversal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Disposition", `attachment; filename="../../etc/passwd.yaml"`)
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer server.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.Metadata.FileName != "passwd" {
		t.Fatalf("file name 必须去除路径与扩展名: %q", response.Metadata.FileName)
	}
}

func TestSubscriptionErrorRedactsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()

	_, err := fetch.Subscription(context.Background(), fetch.Request{URL: serverURL + "/sub?token=secret-token"})
	if err == nil {
		t.Fatal("expected request error")
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "/sub") {
		t.Fatalf("request URL leaked through error: %v", err)
	}
}

func TestSubscriptionNotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("If-None-Match") != "revision-1" {
			t.Error("conditional ETag was not sent")
		}
		if request.Header.Get("If-Modified-Since") != "Wed, 21 Oct 2015 07:28:00 GMT" {
			t.Error("conditional Last-Modified was not sent")
		}
		writer.Header().Set("ETag", "revision-2")
		writer.Header().Set("Last-Modified", "Thu, 22 Oct 2015 07:28:00 GMT")
		writer.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: server.URL, ETag: "revision-1", LastModified: "Wed, 21 Oct 2015 07:28:00 GMT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Metadata.NotModified || len(response.Body) != 0 {
		t.Fatalf("unexpected 304 response: %#v", response)
	}
	if response.Metadata.ETag != "revision-2" || response.Metadata.LastModified != "Thu, 22 Oct 2015 07:28:00 GMT" {
		t.Fatalf("304 response metadata was not preserved: %#v", response.Metadata)
	}
}

func TestSubscriptionSameOriginHTTPSRedirectPreservesHeaders(t *testing.T) {
	var seenHWID, seenToken string
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			http.Redirect(writer, request, server.URL+"/final", http.StatusFound)
		case "/final":
			seenHWID = request.Header.Get("X-HWID")
			seenToken = request.Header.Get("X-Vendor-Token")
			_, _ = writer.Write([]byte("socks://example.com:1080#node"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: server.URL + "/start", AllowInsecure: true, HWID: "hwid-secret",
		Headers: map[string]string{"X-Vendor-Token": "token-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(response.Body) == "" || seenHWID != "hwid-secret" || seenToken != "token-secret" {
		t.Fatalf("same-origin HTTPS redirect did not preserve headers: hwid=%q token=%q", seenHWID, seenToken)
	}
}

func TestSubscriptionSameOriginRelativeRedirectPreservesHeaders(t *testing.T) {
	var seenToken string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			http.Redirect(writer, request, "/final", http.StatusTemporaryRedirect)
		case "/final":
			seenToken = request.Header.Get("X-Vendor-Token")
			_, _ = writer.Write([]byte("socks://example.com:1080#node"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: server.URL + "/start", Headers: map[string]string{"X-Vendor-Token": "token-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seenToken != "token-secret" {
		t.Fatalf("same-origin relative redirect did not preserve custom header: %q", seenToken)
	}
}

func TestSubscriptionMultiLevelRedirectPreservesHeaders(t *testing.T) {
	seen := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/one":
			seen = append(seen, request.Header.Get("X-Vendor-Token"))
			http.Redirect(writer, request, "/two", http.StatusFound)
		case "/two":
			seen = append(seen, request.Header.Get("X-Vendor-Token"))
			http.Redirect(writer, request, "/final", http.StatusFound)
		case "/final":
			seen = append(seen, request.Header.Get("X-Vendor-Token"))
			_, _ = writer.Write([]byte("socks://example.com:1080#node"))
		}
	}))
	defer server.Close()

	_, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: server.URL + "/one", Headers: map[string]string{"X-Vendor-Token": "token-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen[0] != "token-secret" || seen[1] != "token-secret" || seen[2] != "token-secret" {
		t.Fatalf("same-origin multi-level redirect changed headers: %#v", seen)
	}
}

func TestSubscriptionCrossHostRedirectStripsAuthenticationHeaders(t *testing.T) {
	var seenHWID, seenAuthorization, seenToken, seenCookie string
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenHWID = request.Header.Get("X-HWID")
		seenAuthorization = request.Header.Get("Authorization")
		seenToken = request.Header.Get("X-Vendor-Token")
		seenCookie = request.Header.Get("Cookie")
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/final", http.StatusFound)
	}))
	defer source.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: source.URL, HWID: "hwid-secret",
		Headers: map[string]string{
			"Authorization":      "Bearer authorization-secret",
			"X-Vendor-Token":     "token-secret",
			"Cookie":             "session=session-secret",
			"X-Nonstandard-Auth": "other-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Body) == 0 {
		t.Fatal("cross-host redirect did not return the response")
	}
	if seenHWID != "" || seenAuthorization != "" || seenToken != "" || seenCookie != "" {
		t.Fatalf("cross-host redirect leaked authentication headers: hwid=%q authorization=%q token=%q cookie=%q", seenHWID, seenAuthorization, seenToken, seenCookie)
	}
}

func TestSubscriptionPortChangeStripsCustomHeaders(t *testing.T) {
	var seenToken string
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenToken = request.Header.Get("X-Vendor-Token")
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	defer source.Close()

	_, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: source.URL, Headers: map[string]string{"X-Vendor-Token": "token-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seenToken != "" {
		t.Fatalf("port-change redirect leaked custom header: %q", seenToken)
	}
}

func TestSubscriptionRejectsHTTPSDowngradeEvenWhenInsecureIsAllowed(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetHits++
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	defer source.Close()

	_, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: source.URL, AllowInsecure: true, Headers: map[string]string{"X-Vendor-Token": "token-secret"},
	})
	if err == nil {
		t.Fatal("HTTPS downgrade should be rejected")
	}
	var redirectErr *fetch.RedirectError
	if !errors.As(err, &redirectErr) {
		t.Fatalf("HTTPS downgrade did not return RedirectError: %v", err)
	}
	if targetHits != 0 {
		t.Fatalf("HTTPS downgrade target was contacted: %d", targetHits)
	}
	if strings.Contains(err.Error(), "token-secret") {
		t.Fatalf("redirect error leaked a secret: %v", err)
	}
}

func TestSubscriptionUsesConfiguredProxy(t *testing.T) {
	var seenURL, seenToken string
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenURL = request.URL.String()
		seenToken = request.Header.Get("X-Vendor-Token")
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer proxy.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: "http://subscription.invalid/profile", ProxyURL: proxy.URL,
		Headers: map[string]string{"X-Vendor-Token": "token-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Body) == 0 || seenURL != "http://subscription.invalid/profile" || seenToken != "token-secret" {
		t.Fatalf("configured proxy request was not preserved: url=%q token=%q", seenURL, seenToken)
	}
}

func TestSubscriptionHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := fetch.Subscription(ctx, fetch.Request{URL: server.URL, Timeout: time.Second})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("请求未进入服务端")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("取消请求应返回错误")
		}
	case <-time.After(time.Second):
		t.Fatal("取消请求未及时结束")
	}
}

func TestSubscriptionTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer server.Close()

	_, err := fetch.Subscription(context.Background(), fetch.Request{URL: server.URL, Timeout: 20 * time.Millisecond})
	if err == nil {
		t.Fatal("请求超时应返回错误")
	}
}

func TestSubscriptionTLSRequiresExplicitInsecureOption(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("socks://example.com:1080#node"))
	}))
	defer server.Close()

	if _, err := fetch.Subscription(context.Background(), fetch.Request{URL: server.URL}); err == nil {
		t.Fatal("未启用 insecure 时不应接受自签名证书")
	}
	response, err := fetch.Subscription(context.Background(), fetch.Request{
		URL: server.URL, AllowInsecure: true,
	})
	if err != nil {
		t.Fatalf("显式启用 insecure 后请求失败: %v", err)
	}
	if len(response.Body) == 0 {
		t.Fatal("TLS 请求未返回订阅内容")
	}
}
