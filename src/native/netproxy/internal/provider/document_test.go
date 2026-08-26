package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
			Server: "example.com", ServerPort: 1080,
			Version: "5",
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
			Server: "node.internal.example.com", ServerPort: 1080,
			Username: "user",
			Password: "secret",
		},
	}}}
	summary := provider.Inspect(document)
	if len(summary) != 1 || summary[0].Server != "*.example.com" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestInspectFileMatchesTypedSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	document := provider.Document{
		Outbounds: []option.Outbound{{
			Type: C.TypeSOCKS,
			Tag:  "private",
			Options: &option.SOCKSOutboundOptions{
				Server: "node.internal.example.com", ServerPort: 1080,
				Username: "user", Password: "secret",
			},
		}},
		Endpoints: []option.Endpoint{{
			Type:    C.TypeWireGuard,
			Tag:     "wireguard",
			Options: &option.WireGuardEndpointOptions{},
		}},
	}
	if err := provider.SaveAtomic(context.Background(), path, document); err != nil {
		t.Fatal(err)
	}
	want := provider.Inspect(document)
	got, err := provider.InspectFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stream summary mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), `"username": ""`) {
		t.Fatal("test fixture did not contain credentials")
	}
}

func TestInspectFileRejectsInvalidShapeAndDuplicateTags(t *testing.T) {
	for name, content := range map[string]string{
		"unknown-field": `{"outbounds":[],"legacy":[]}`,
		"duplicate-tag": `{"outbounds":[{"type":"socks","tag":"same"}],"endpoints":[{"type":"wireguard","tag":"same"}]}`,
		"missing-type":  `{"outbounds":[{"tag":"node"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "provider.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.InspectFile(context.Background(), path); err == nil {
				t.Fatal("invalid provider summary was accepted")
			}
		})
	}
}

func TestFileContainsTagStopsAfterMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	content := `{"outbounds":[{"type":"socks","tag":"first"},{"broken":`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := provider.FileContainsTag(context.Background(), path, "first")
	if err != nil || !found {
		t.Fatalf("early tag lookup failed: found=%v err=%v", found, err)
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

func TestLoadAllowEmptyRejectsDuplicateObjectNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := os.WriteFile(path, []byte(`{"outbounds":[],"outbounds":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.LoadAllowEmpty(context.Background(), path); err == nil {
		t.Fatal("重复对象名称未被 JSON v2 拒绝")
	}
}

func TestValidateRejectsControlCharactersInTag(t *testing.T) {
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeSOCKS,
		Tag:  "invalid\ttag",
		Options: &option.SOCKSOutboundOptions{
			Server: "example.com", ServerPort: 1080,
		},
	}}}
	if err := provider.Validate(document); err == nil {
		t.Fatal("expected tag validation failure")
	}
}

func TestRemoveLastNodeWritesEmptyProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	document := provider.Document{Outbounds: []option.Outbound{{
		Type: C.TypeSOCKS,
		Tag:  "only",
		Options: &option.SOCKSOutboundOptions{
			Server: "example.com", ServerPort: 1080,
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
