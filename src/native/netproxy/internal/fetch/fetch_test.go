package fetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
