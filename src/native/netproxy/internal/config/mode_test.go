package config

import (
	"slices"
	"testing"
)

func TestModesFollowNativeRulesAndDefault(t *testing.T) {
	for _, test := range []struct {
		content   string
		mode      string
		available []string
	}{
		{`{}`, "Rule", []string{"Rule"}},
		{`{"experimental":{"clash_api":{"default_mode":"Office"}},"route":{"rules":[{"clash_mode":["Direct","Proxy"]},{"type":"logical","rules":[{"clash_mode":"Office"},{"clash_mode":"Zeta"}]}]},"dns":{"rules":[{"type":"logical","rules":[{"clash_mode":["RuleAllowAds","Office"]}]}]}}`, "Office", []string{"Office", "Proxy", "RuleAllowAds", "Zeta", "Direct"}},
		{`{"experimental":{"clash_api":{"default_mode":""}},"dns":{"rules":[{"clash_mode":"Direct"}]}}`, "Rule", []string{"Rule", "Direct"}},
	} {
		result, err := ParseModes([]byte(test.content))
		if err != nil || result.Mode != test.mode || !slices.Equal(result.Available, test.available) {
			t.Fatalf("%s: %+v, %v", test.content, result, err)
		}
	}
}

func TestModesRejectMalformedJSON(t *testing.T) {
	for _, content := range []string{`null`, `[]`, `{"route":{"rules":[{"clash_mode":123}]}}`, `{"dns":{"rules":[{"clash_mode":[1]}]}}`, `{"route":{"rules":[{"type":"unsupported"}]}}`, `{"experimental":{"clash_api":{"default_mode":1}}}`, `{"route":{},"route":{}}`, "{\"extra\":\"\xff\"}"} {
		if _, err := ParseModes([]byte(content)); err == nil {
			t.Fatalf("接受了非法配置: %s", content)
		}
	}
}
