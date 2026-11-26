package module

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestEffectiveConfigPreservesNativeFieldsAndOrdering(t *testing.T) {
	dir := t.TempDir()
	source := paths.SingBoxConfig(dir)
	original := []byte(`{"route":{"final":"custom","rules":[{"outbound":"direct"}]},"experimental":{"custom":{"value":9007199254740993}},"inbounds":[{"tag":"user"}],"outbounds":[{"tag":"custom"}],"providers":[{"tag":"external"}]}`)
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	projection := []jsontext.Value{jsontext.Value(`{"tag":"managed-provider"}`)}
	dest := filepath.Join(dir, "runtime", "config.json")
	inbounds := []jsontext.Value{jsontext.Value(`{"tag":"netproxy-in"}`)}
	if err := writeEffectiveConfig(t.Context(), dir, dest, inbounds, projection, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"route":{"final":"custom","rules":[{"outbound":"direct"}]},"experimental":{"custom":{"value":9007199254740993}},"inbounds":[{"tag":"user"},{"tag":"netproxy-in"}],"outbounds":[{"tag":"custom"}],"providers":[{"tag":"external"},{"tag":"managed-provider"}]}`)
	var gotDocument, wantDocument jsontext.Value = bytes.Clone(raw), want
	if err := gotDocument.Canonicalize(jsontext.CanonicalizeRawInts(false)); err != nil {
		t.Fatal(err)
	}
	if err := wantDocument.Canonicalize(jsontext.CanonicalizeRawInts(false)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotDocument, wantDocument) {
		t.Fatalf("unexpected assembled configuration: %s", raw)
	}
	if current, err := os.ReadFile(source); err != nil || !bytes.Equal(current, original) {
		t.Fatalf("user configuration changed: %v", err)
	}
	if err := os.WriteFile(source, []byte(`{"providers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var invalid *json.SemanticError
	if err := writeEffectiveConfig(t.Context(), dir, dest, inbounds, projection, false); !errors.As(err, &invalid) {
		t.Fatalf("invalid provider array not rejected: %v", err)
	}
	if current, err := os.ReadFile(dest); err != nil || !bytes.Equal(current, raw) {
		t.Fatalf("failed assembly replaced the previous configuration: %v", err)
	}
}

func TestIPv4OnlyRuntimeKeepsUserDNSAndRouteOnDisk(t *testing.T) {
	dir := t.TempDir()
	source := paths.SingBoxConfig(dir)
	const direct = `{"clash_mode":"Direct","action":"route","outbound":"direct"}`
	const hijack = `{"port":53,"action":"hijack-dns"}`
	original := `{"dns":{"strategy":"prefer_ipv6","rules":[{"domain":"example.com","server":"local","strategy":"ipv6_only"}]},"route":{"rules":[` + direct + `,` + hijack + `,{"outbound":"proxy"}]}}`
	if err := os.WriteFile(source, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "runtime", "config.json")
	if err := writeEffectiveConfig(t.Context(), dir, dest, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	var projected struct {
		DNS struct {
			Strategy string                      `json:"strategy"`
			Rules    []map[string]jsontext.Value `json:"rules"`
		} `json:"dns"`
		Route struct {
			Rules []map[string]jsontext.Value `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(raw, &projected); err != nil {
		t.Fatal(err)
	}
	dns, route := projected.DNS, projected.Route
	if dns.Strategy != "ipv4_only" || len(dns.Rules) != 2 {
		t.Fatalf("unexpected DNS: %s", raw)
	}
	var queryTypes []string
	if err := json.Unmarshal(dns.Rules[0]["query_type"], &queryTypes); err != nil {
		t.Fatal(err)
	}
	if string(dns.Rules[0]["action"]) != `"predefined"` || len(queryTypes) != 1 || queryTypes[0] != "AAAA" ||
		string(dns.Rules[1]["strategy"]) != `"ipv4_only"` {
		t.Fatalf("unexpected DNS: %s", raw)
	}
	if len(route.Rules) != 4 || string(route.Rules[0]["clash_mode"]) != `"Direct"` ||
		string(route.Rules[0]["action"]) != `"route"` || string(route.Rules[1]["action"]) != `"hijack-dns"` ||
		string(route.Rules[2]["inbound"]) != `"netproxy-in"` || string(route.Rules[2]["ip_version"]) != "6" ||
		string(route.Rules[2]["no_drop"]) != "true" || string(route.Rules[3]["outbound"]) != `"proxy"` {
		t.Fatalf("unexpected route: %s", raw)
	}
	if saved, err := os.ReadFile(source); err != nil || string(saved) != original {
		t.Fatal("saved sing-box preferences changed", err)
	}
}
