package module

import (
	json "encoding/json/v2"
	"fmt"
)

// Route actions resolve their target at runtime. Config check alone cannot protect
// an existing route from deletion or renaming of its configured group.
func validateRemovedRouteGroups(current, candidate []byte) error {
	before, err := configObject(current)
	if err != nil {
		return err
	}
	after, err := configObject(candidate)
	if err != nil {
		return err
	}
	previous, err := readPolicyReferences(before)
	if err != nil {
		return err
	}
	next, err := readPolicyReferences(after)
	if err != nil {
		return err
	}
	removed := make(map[string]bool)
	for _, group := range previous {
		if group.isGroup() && group.Tag != "" {
			removed[group.Tag] = true
		}
	}
	for _, outbound := range next {
		delete(removed, outbound.Tag)
	}
	if len(removed) == 0 || before["route"] == nil {
		return nil
	}
	var route struct {
		Final string             `json:"final"`
		Rules []routeOutboundUse `json:"rules"`
	}
	if err := json.Unmarshal(before["route"], &route); err != nil {
		return err
	}
	inUse := route.Final
	if !removed[inUse] {
		inUse = removedRouteGroup(route.Rules, removed)
	}
	if inUse != "" {
		return fmt.Errorf("路由规则或默认出站仍引用策略组 %q，请先修改引用", inUse)
	}
	return nil
}

type routeOutboundUse struct {
	Outbound string             `json:"outbound"`
	Rules    []routeOutboundUse `json:"rules"`
}

func removedRouteGroup(rules []routeOutboundUse, removed map[string]bool) string {
	for _, rule := range rules {
		if removed[rule.Outbound] {
			return rule.Outbound
		}
		if tag := removedRouteGroup(rule.Rules, removed); tag != "" {
			return tag
		}
	}
	return ""
}
