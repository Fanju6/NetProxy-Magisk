package fetch_test

import (
	"context"
	"encoding/base64"
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
		writer.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	response, err := fetch.Subscription(context.Background(), fetch.Request{URL: server.URL, ETag: "revision-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Metadata.NotModified || len(response.Body) != 0 {
		t.Fatalf("unexpected 304 response: %#v", response)
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
