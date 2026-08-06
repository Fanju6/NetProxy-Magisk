package sharelink_test

import (
	"context"
	"strings"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/sharelink"
)

func TestExportVLESSRealityRoundTrip(t *testing.T) {
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeVLESS,
		Tag:  "测试节点",
		Options: &option.VLESSOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "example.com", ServerPort: 443},
			UUID:          "11111111-2222-3333-4444-555555555555",
			Flow:          "xtls-rprx-vision",
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{
				Enabled: true, ServerName: "cdn.example.com",
				UTLS:    &option.OutboundUTLSOptions{Enabled: true, Fingerprint: "chrome"},
				Reality: &option.OutboundRealityOptions{Enabled: true, PublicKey: "public-key", ShortID: "abcd"},
			}},
		},
	}}}

	result, err := sharelink.Export(document, "测试节点")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Link, "vless://") || !strings.Contains(result.Link, "security=reality") {
		t.Fatalf("unexpected link: %s", result.Link)
	}
	parsed, err := convert.Link(context.Background(), result.Link, false)
	if err != nil {
		t.Fatal(err)
	}
	options := parsed.Document.Outbounds[0].Options.(*option.VLESSOutboundOptions)
	if options.UUID != "11111111-2222-3333-4444-555555555555" || options.Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected round trip: %#v", options)
	}
}

func TestExportSOCKSRoundTrip(t *testing.T) {
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeSOCKS,
		Tag:  "socks",
		Options: &option.SOCKSOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "2001:db8::1", ServerPort: 1080},
			Username:      "user",
			Password:      "password",
		},
	}}}

	result, err := sharelink.Export(document, "socks")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := convert.Link(context.Background(), result.Link, false)
	if err != nil {
		t.Fatal(err)
	}
	options := parsed.Document.Outbounds[0].Options.(*option.SOCKSOutboundOptions)
	if options.Server != "2001:db8::1" || options.Username != "user" || options.Password != "password" {
		t.Fatalf("unexpected round trip: %#v", options)
	}
}

func TestExportShadowsocksRoundTrip(t *testing.T) {
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeShadowsocks,
		Tag:  "ss",
		Options: &option.ShadowsocksOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "example.com", ServerPort: 8388},
			Method:        "aes-128-gcm",
			Password:      "password",
		},
	}}}

	result, err := sharelink.Export(document, "ss")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := convert.Link(context.Background(), result.Link, false)
	if err != nil {
		t.Fatal(err)
	}
	options := parsed.Document.Outbounds[0].Options.(*option.ShadowsocksOutboundOptions)
	if options.Method != "aes-128-gcm" || options.Password != "password" {
		t.Fatalf("unexpected round trip: %#v", options)
	}
}

func TestExportRejectsUnknownTag(t *testing.T) {
	_, err := sharelink.Export(provider.Document{}, "missing")
	if err == nil {
		t.Fatal("expected missing tag error")
	}
}
