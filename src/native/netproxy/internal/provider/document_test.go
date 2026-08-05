package provider_test

import (
	"context"
	"path/filepath"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func TestSaveAtomicRoundTripAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeSOCKS,
		Tag:  "first",
		Options: &option.SOCKSOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "example.com", ServerPort: 1080},
			Version:       "5",
		},
	}}}
	if err := provider.SaveAtomic(context.Background(), path, document); err != nil {
		t.Fatal(err)
	}
	document.Outbounds[0].Tag = "second"
	if err := provider.SaveAtomic(context.Background(), path, document); err != nil {
		t.Fatal(err)
	}
	loaded, err := provider.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Outbounds) != 1 || loaded.Outbounds[0].Tag != "second" {
		t.Fatalf("unexpected round trip: %#v", loaded)
	}
}

func TestInspectDoesNotExposeCredentials(t *testing.T) {
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeSOCKS,
		Tag:  "private",
		Options: &option.SOCKSOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "node.internal.example.com", ServerPort: 1080},
			Username:      "user",
			Password:      "secret",
		},
	}}}
	summary := provider.Inspect(document)
	if len(summary) != 1 || summary[0].Server != "*.example.com" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestLoadAllowEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := provider.WriteAtomic(path, []byte("{\n  \"outbounds\": []\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := provider.LoadAllowEmpty(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Outbounds) != 0 || len(document.Endpoints) != 0 {
		t.Fatalf("expected empty document: %#v", document)
	}
}

func TestRemoveLastNodeWritesEmptyProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeSOCKS,
		Tag:  "only",
		Options: &option.SOCKSOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "example.com", ServerPort: 1080},
		},
	}}}
	if !provider.Remove(&document, "only") {
		t.Fatal("node was not removed")
	}
	if err := provider.SaveAtomicAllowEmpty(context.Background(), path, document); err != nil {
		t.Fatal(err)
	}
	loaded, err := provider.LoadAllowEmpty(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Outbounds)+len(loaded.Endpoints) != 0 {
		t.Fatalf("expected empty provider: %#v", loaded)
	}
}
