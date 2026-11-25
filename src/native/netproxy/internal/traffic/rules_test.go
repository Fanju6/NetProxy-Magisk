package traffic

import (
	"os"
	"reflect"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func TestRewriteIndexLimitsRegexToConfiguredDomain(t *testing.T) {
	rules, err := compileRules([]Rule{
		{Pattern: ".*", Phase: "request", Script: "return {}", Domains: []string{" Other.Example. "}},
		{Pattern: "/inspect", Phase: "request", Script: "return {}", Domains: []string{"example.org", "example.org"}},
		{Pattern: "/inspect", Phase: "response", Script: "return {}", Domains: []string{"example.org"}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	index := indexRules(rules)
	if !reflect.DeepEqual(index["example.org"], []int{1, 2}) || !reflect.DeepEqual(index["other.example"], []int{0}) {
		t.Fatalf("unexpected candidates: %v", index)
	}
	matches, req, res := matchingRules(rules, index["example.org"], "https://example.org/inspect")
	if len(matches) != 2 || req != 0 || res != 1 || matches[0].RuleIndex != 1 {
		t.Fatalf("wrong matches: %+v", matches)
	}
}

func TestReplaceRulesKeepsUnchangedInactiveLegacyRule(t *testing.T) {
	layout := paths.New(t.TempDir())
	if err := os.MkdirAll(layout.Config(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(layout.TrafficRules(), []byte(`[{"pattern":".*","phase":"request","script":"return {}"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	controller := New(layout)
	rules, err := controller.Rules()
	if err != nil || len(rules) != 1 || len(rules[0].Domains) != 0 {
		t.Fatalf("legacy rule not retained: %v, %v", rules, err)
	}
	scoped := Rule{Pattern: ".*", Phase: "response", Script: "return {}", Domains: []string{"example.org"}}
	if err := controller.ReplaceRules(append(rules, scoped)); err != nil {
		t.Fatal(err)
	}
	if _, candidates := controller.snapshotRules("example.org"); !reflect.DeepEqual(candidates, []int{1}) {
		t.Fatalf("inactive legacy rule was indexed: %v", candidates)
	}
	rules[0].Script = "return {reject:true}"
	if err := controller.ReplaceRules(rules); err == nil {
		t.Fatal("modified unscoped legacy rule was accepted")
	}
}

func TestLegacyRulesOnlyMigrateAnchoredLiteralDomains(t *testing.T) {
	legacy := []Rule{
		{Pattern: "^https://example[.]org/", Phase: "request", Script: "return {}"},
		{Pattern: ".*", Phase: "response", Script: "return {}"},
	}
	if _, err := compileRules(legacy, false); err == nil {
		t.Fatal("new unscoped rules were accepted")
	}
	rules, err := compileRules(legacy, true)
	if err != nil || !reflect.DeepEqual(indexRules(rules), map[string][]int{"example.org": {0}}) {
		t.Fatalf("unsafe legacy migration: %v, %v", rules, err)
	}
}
