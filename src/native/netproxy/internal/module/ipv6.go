package module

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"slices"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
)

func projectIPv4Only(document map[string]jsontext.Value) error {
	dns, err := runtimeSection(document, "dns")
	if err != nil {
		return err
	}
	dns["strategy"] = jsontext.Value(`"ipv4_only"`)
	dnsRules, err := runtimeRules(dns)
	if err != nil {
		return err
	}
	for i, rule := range dnsRules {
		fields, err := configObject(rule)
		if err != nil {
			return err
		}
		if _, overridesStrategy := fields["strategy"]; overridesStrategy {
			fields["strategy"] = dns["strategy"]
			dnsRules[i], err = json.Marshal(fields, json.Deterministic(true))
			if err != nil {
				return err
			}
		}
	}
	noAAAA := jsontext.Value(`{"query_type":["AAAA"],"action":"predefined","rcode":"NOERROR"}`)
	dns["rules"], err = json.Marshal(slices.Insert(dnsRules, 0, noAAAA))
	if err != nil {
		return err
	}
	document["dns"], err = json.Marshal(dns, json.Deterministic(true))
	if err != nil {
		return err
	}

	route, err := runtimeSection(document, "route")
	if err != nil {
		return err
	}
	rules, err := runtimeRules(route)
	if err != nil {
		return err
	}
	lastDNS := -1
	firstDirect := false
	for i, rule := range rules {
		var action struct {
			Action string `json:"action"`
			Mode   string `json:"clash_mode"`
		}
		if err := json.Unmarshal(rule, &action); err != nil {
			return err
		}
		if i == 0 {
			firstDirect = action.Mode == "Direct" && action.Action == "route"
		}
		if action.Action == "hijack-dns" {
			lastDNS = i
		}
	}
	reject := map[string]any{"inbound": inbound.Tag, "ip_version": 6, "action": "reject", "no_drop": true}
	rejectRule, err := json.Marshal(reject, json.Deterministic(true))
	if err != nil {
		return err
	}
	ruleIndex := lastDNS + 1
	if firstDirect && ruleIndex == 0 {
		ruleIndex = 1
	}
	rules = slices.Insert(rules, ruleIndex, jsontext.Value(rejectRule))
	route["rules"], err = json.Marshal(rules)
	if err != nil {
		return err
	}
	document["route"], err = json.Marshal(route, json.Deterministic(true))
	return err
}

func runtimeSection(document map[string]jsontext.Value, name string) (map[string]jsontext.Value, error) {
	if raw, ok := document[name]; ok {
		section, err := configObject(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", name, err)
		}
		return section, nil
	}
	return make(map[string]jsontext.Value), nil
}

func runtimeRules(section map[string]jsontext.Value) ([]jsontext.Value, error) {
	var rules []jsontext.Value
	if raw, ok := section["rules"]; ok {
		if err := json.Unmarshal(raw, &rules); err != nil {
			return nil, err
		}
	}
	return rules, nil
}
