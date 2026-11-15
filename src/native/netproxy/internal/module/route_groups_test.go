package module

import (
	"strings"
	"testing"
)

func TestValidateRemovedRouteGroups(t *testing.T) {
	const outbounds = `"outbounds":[{"type":"selector","tag":"AI"},{"type":"urltest","tag":"Auto"},{"type":"direct","tag":"direct"}]`
	const candidate = `{"outbounds":[{"type":"urltest","tag":"Auto"},{"type":"direct","tag":"direct"}]}`
	for _, route := range []string{
		`{"rules":[{"type":"default","domain":["example.com"],"action":"route","outbound":"AI"}]}`,
		`{"rules":[{"type":"logical","mode":"and","rules":[{"network":"tcp"}],"action":"route","outbound":"AI"}]}`,
		`{"final":"AI"}`,
	} {
		config := []byte(`{` + outbounds + `,"route":` + route + `}`)
		if err := validateRemovedRouteGroups(config, []byte(candidate)); err == nil || !strings.Contains(err.Error(), "AI") {
			t.Fatalf("expected a reference error, got %v", err)
		}
		if err := validateRemovedRouteGroups(config, config); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := []byte(`{` + outbounds + `,"route":{"final":"Auto","rules":[{"domain":"AI","outbound":"direct"}]}}`)
	if err := validateRemovedRouteGroups(unrelated, []byte(candidate)); err != nil {
		t.Fatal(err)
	}
}
