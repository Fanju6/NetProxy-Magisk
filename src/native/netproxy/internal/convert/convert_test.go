package convert_test

import (
	"context"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
)

func TestSOCKSLink(t *testing.T) {
	result, err := convert.Link(context.Background(), "socks5://dXNlcjpwYXNz@example.com:1080#SOCKS", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Outbounds) != 1 {
		t.Fatalf("expected one outbound, got %d", len(result.Document.Outbounds))
	}
	outbound := result.Document.Outbounds[0]
	if outbound.Type != C.TypeSOCKS || outbound.Tag != "SOCKS" {
		t.Fatalf("unexpected outbound: type=%q tag=%q", outbound.Type, outbound.Tag)
	}
	options := outbound.Options.(*option.SOCKSOutboundOptions)
	if options.Server != "example.com" || options.ServerPort != 1080 || options.Username != "user" || options.Password != "pass" {
		t.Fatalf("unexpected SOCKS options: %#v", options)
	}
}

func TestDuplicateTagsReceiveStableSuffix(t *testing.T) {
	content := "socks://example.com:1080#node\nsocks://example.net:1081#node"
	result, err := convert.Content(context.Background(), content, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Document.Outbounds[0].Tag != "node" || result.Document.Outbounds[1].Tag != "node_2" {
		t.Fatalf("unexpected tags: %q, %q", result.Document.Outbounds[0].Tag, result.Document.Outbounds[1].Tag)
	}
}

func TestInvalidPortReturnsDiagnostics(t *testing.T) {
	result, err := convert.Link(context.Background(), "socks://example.com:not-a-port#bad", false)
	if err == nil {
		t.Fatal("expected invalid port error")
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "link.invalid" {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestVLESSRealityLink(t *testing.T) {
	link := "vless://3c0f47e3-a464-470c-a931-36b8a8d62fd6@69.63.218.142:8443?encryption=none&security=reality&sni=it.example.com&fp=chrome&pbk=tkmyb6Xk2aYMFxrQ35q6PMULtbdIKhaYGG9yySPJbHc&sid=fc1b&type=tcp&flow=xtls-rprx-vision#vless-reality"
	result, err := convert.Link(context.Background(), link, false)
	if err != nil {
		t.Fatal(err)
	}
	outbound := result.Document.Outbounds[0]
	if outbound.Type != C.TypeVLESS || outbound.Tag != "vless-reality" {
		t.Fatalf("unexpected outbound: %#v", outbound)
	}
	options := outbound.Options.(*option.VLESSOutboundOptions)
	if options.ServerPort != 8443 || options.TLS == nil || !options.TLS.Reality.Enabled {
		t.Fatalf("Reality options were not preserved: %#v", options)
	}
}

func TestAllowInsecureIsExplicit(t *testing.T) {
	link := "https://user:pass@example.com:443#http"
	secure, err := convert.Link(context.Background(), link, false)
	if err != nil {
		t.Fatal(err)
	}
	insecure, err := convert.Link(context.Background(), link, true)
	if err != nil {
		t.Fatal(err)
	}
	secureOptions := secure.Document.Outbounds[0].Options.(*option.HTTPOutboundOptions)
	insecureOptions := insecure.Document.Outbounds[0].Options.(*option.HTTPOutboundOptions)
	if secureOptions.TLS.Insecure {
		t.Fatal("TLS verification must be enabled by default")
	}
	if !insecureOptions.TLS.Insecure {
		t.Fatal("allow-insecure did not update TLS options")
	}
}
