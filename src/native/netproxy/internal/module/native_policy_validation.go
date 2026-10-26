package module

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

type nativePolicyConfig struct {
	Outbounds   []jsontext.Value `json:"outbounds"`
	Endpoints   []jsontext.Value `json:"endpoints"`
	Providers   []jsontext.Value `json:"providers"`
	HTTPClients []jsontext.Value `json:"http_clients"`
	NTP         jsontext.Value   `json:"ntp"`
	DNS         *struct {
		Servers []jsontext.Value `json:"servers"`
		Rules   []jsontext.Value `json:"rules"`
	} `json:"dns"`
	Route *struct {
		Final string           `json:"final"`
		Rules []jsontext.Value `json:"rules"`
	} `json:"route"`
}

type nativePolicyNamespace struct {
	outbounds       map[string]string
	providers       map[string]bool
	nativeProviders map[string]bool
}

func readNativePolicyConfig(options Options) (nativePolicyConfig, error) {
	content, err := os.ReadFile(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return nativePolicyConfig{}, err
	}
	var config *nativePolicyConfig
	if err := json.Unmarshal(content, &config); err != nil || config == nil {
		return nativePolicyConfig{}, nativePolicyError("policy.config_invalid", "config", "")
	}
	return *config, nil
}

func nativePolicyIdentity(raw jsontext.Value, index int) (option.Outbound, error) {
	// Only project identity; protocol parsing and unknown fields belong to the core.
	var identity *option.Outbound
	if err := json.Unmarshal(raw, &identity); err != nil || identity == nil {
		return option.Outbound{}, nativePolicyError("policy.config_invalid", "tag", "")
	}
	if identity.Tag == "" {
		identity.Tag = strconv.Itoa(index)
	}
	return *identity, nil
}

func newNativePolicyNamespace(config nativePolicyConfig) (*nativePolicyNamespace, error) {
	namespace := &nativePolicyNamespace{
		// sing-box appends this direct outbound even when none is configured.
		outbounds:       map[string]string{"Compatible": "core"},
		providers:       make(map[string]bool),
		nativeProviders: make(map[string]bool),
	}
	for _, section := range []struct {
		name string
		list []jsontext.Value
	}{{"outbounds", config.Outbounds}, {"endpoints", config.Endpoints}, {"providers", config.Providers}} {
		for index, raw := range section.list {
			identity, err := nativePolicyIdentity(raw, index)
			if err != nil {
				return nil, err
			}
			field := fmt.Sprintf("%s[%d].tag", section.name, index)
			if section.name == "providers" {
				if namespace.providers[identity.Tag] {
					return nil, nativePolicyError("policy.provider_duplicate", field, identity.Tag)
				}
				namespace.providers[identity.Tag] = true
				namespace.nativeProviders[identity.Tag] = true
			} else {
				if _, exists := namespace.outbounds[identity.Tag]; exists {
					return nil, nativePolicyError("policy.outbound_duplicate", field, identity.Tag)
				}
				namespace.outbounds[identity.Tag] = field
			}
		}
	}
	return namespace, nil
}

func (namespace *nativePolicyNamespace) addResource(groupID string, document provider.Document) error {
	if len(document.Outbounds)+len(document.Endpoints) == 0 {
		return nil
	}
	tag := catalog.ProviderTag(groupID)
	if namespace.providers[tag] {
		return nativePolicyError("catalog.provider_conflict", "providers", tag)
	}
	namespace.providers[tag] = true
	for _, node := range provider.Inspect(document) {
		member := tag + "/" + node.Tag
		if field, exists := namespace.outbounds[member]; exists {
			return nativePolicyError("catalog.outbound_conflict", field, member)
		}
		namespace.outbounds[member] = "catalog"
	}
	return nil
}

// validateNativeResourceCollisions is safe inside a Catalog mutation: it reads
// only native config and the supplied candidate, never reacquiring Catalog locks.
func validateNativeResourceCollisions(options Options, groupID string, document provider.Document) error {
	config, err := readNativePolicyConfig(options)
	if err != nil {
		return err
	}
	namespace, err := newNativePolicyNamespace(config)
	if err != nil {
		return err
	}
	return namespace.addResource(groupID, document)
}

// check does not start Local Providers or resolve group members. Supplement it
// with known native references, without constructing a core or fetching a URL.
func validateNativePolicyReferences(ctx context.Context, options Options) error {
	config, err := readNativePolicyConfig(options)
	if err != nil {
		return err
	}
	namespace, err := newNativePolicyNamespace(config)
	if err != nil {
		return err
	}
	groups, err := catalog.GroupIDs(ctx, options.CatalogRoot, "all")
	if err != nil {
		return err
	}
	for _, groupID := range groups {
		document, err := catalog.GroupProvider(ctx, options.CatalogRoot, groupID)
		if err != nil {
			return err
		}
		if err := namespace.addResource(groupID, document); err != nil {
			return err
		}
	}
	if err := namespace.validateSelectorDefaults(config); err != nil {
		return err
	}
	if err := visitNativePolicyReferences(config, namespace.requireOutbound, func(field, tag string) error {
		if !namespace.providers[tag] {
			return nativePolicyError("policy.provider_not_found", field, tag)
		}
		return nil
	}); err != nil {
		return err
	}
	return ctx.Err()
}

func (namespace *nativePolicyNamespace) validateSelectorDefaults(config nativePolicyConfig) error {
	for index, raw := range config.Outbounds {
		identity, err := nativePolicyIdentity(raw, index)
		if err != nil {
			return err
		}
		if identity.Type != C.TypeSelector {
			continue
		}
		field := fmt.Sprintf("outbounds[%d].default", index)
		var selector option.SelectorOutboundOptions
		if err := json.Unmarshal(raw, &selector); err != nil {
			return nativePolicyError("policy.config_invalid", fmt.Sprintf("outbounds[%d]", index), identity.Tag)
		}
		if err := namespace.requireOutbound(field, selector.Default); err != nil {
			return err
		}
		// alpha11 在 Provider 回调前只装载显式成员；空成员列表仅补入 Compatible。
		// 此限制只用于启动预检，不能改变删除守卫对真实 default 引用的保护。
		if selector.Default == "" || slices.Contains(selector.Outbounds, selector.Default) || len(selector.Outbounds) == 0 && selector.Default == "Compatible" {
			continue
		}
		return &service.Error{Code: "policy.selector_default_invalid", Message: fmt.Sprintf("%s 必须属于 Selector 的显式 outbounds；仅空列表可使用 Compatible，Provider-only 成员无法作为启动 default", field)}
	}
	return nil
}

func visitNativePolicyReferences(config nativePolicyConfig, outbound, providerRef func(string, string) error) error {
	for index, raw := range config.Outbounds {
		identity, err := nativePolicyIdentity(raw, index)
		if err != nil {
			return err
		}
		field := fmt.Sprintf("outbounds[%d]", index)
		var common option.GroupCommonOption
		switch identity.Type {
		case C.TypeSelector:
			var selector option.SelectorOutboundOptions
			if err := json.Unmarshal(raw, &selector); err != nil {
				return nativePolicyError("policy.config_invalid", field, identity.Tag)
			}
			if err := outbound(field+".default", selector.Default); err != nil {
				return err
			}
			common = selector.GroupCommonOption
		case C.TypeURLTest, C.TypeLoadBalance:
			if err := json.Unmarshal(raw, &common); err != nil {
				return nativePolicyError("policy.config_invalid", field, identity.Tag)
			}
		default:
			continue
		}
		// use_all_providers replaces the explicit list in the native core.
		if !common.UseAllProviders {
			for _, tag := range common.Providers {
				if err := providerRef(field+".providers", tag); err != nil {
					return err
				}
			}
		}
		for _, tag := range common.Outbounds {
			if tag == "" {
				return nativePolicyError("policy.outbound_not_found", field+".outbounds", tag)
			}
			if err := outbound(field+".outbounds", tag); err != nil {
				return err
			}
		}
	}
	if config.Route != nil {
		if err := outbound("route.final", config.Route.Final); err != nil {
			return err
		}
		if err := visitNativeRuleReferences(config.Route.Rules, "route.rules", false, false, outbound); err != nil {
			return err
		}
	}
	if config.DNS != nil {
		if err := visitNativeRuleReferences(config.DNS.Rules, "dns.rules", true, false, outbound); err != nil {
			return err
		}
	}
	sections := []struct {
		name string
		list []jsontext.Value
	}{{"outbounds", config.Outbounds}, {"endpoints", config.Endpoints}, {"http_clients", config.HTTPClients}}
	if config.DNS != nil {
		sections = append(sections, struct {
			name string
			list []jsontext.Value
		}{"dns.servers", config.DNS.Servers})
	}
	if len(config.NTP) > 0 && string(config.NTP) != "null" {
		sections = append(sections, struct {
			name string
			list []jsontext.Value
		}{"ntp", []jsontext.Value{config.NTP}})
	}
	for _, section := range sections {
		for index, raw := range section.list {
			var dialer option.DialerOptions
			if err := json.Unmarshal(raw, &dialer); err != nil {
				return nativePolicyError("policy.config_invalid", section.name, "")
			}
			if err := outbound(fmt.Sprintf("%s[%d].detour", section.name, index), dialer.Detour); err != nil {
				return err
			}
		}
	}
	return nil
}

func visitNativeRuleReferences(rules []jsontext.Value, field string, dns, nested bool, outbound func(string, string) error) error {
	for index, raw := range rules {
		path := fmt.Sprintf("%s[%d]", field, index)
		var rule struct {
			Type   string           `json:"type"`
			Action string           `json:"action"`
			Rules  []jsontext.Value `json:"rules"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			return nativePolicyError("policy.config_invalid", path, "")
		}
		if rule.Type != "" && rule.Type != "default" && rule.Type != "logical" {
			continue
		}
		if dns && rule.Type != "logical" {
			// DNS outbound matches outbound tags; server belongs to the DNS namespace.
			var match struct {
				Outbound jsontext.Value `json:"outbound"`
			}
			if err := json.Unmarshal(raw, &match); err != nil {
				return nativePolicyError("policy.config_invalid", path, "")
			}
			var tags []string
			if match.Outbound.Kind() == '"' {
				var tag string
				if err := json.Unmarshal(match.Outbound, &tag); err != nil {
					return nativePolicyError("policy.config_invalid", path+".outbound", "")
				}
				tags = []string{tag}
			} else if len(match.Outbound) > 0 {
				if err := json.Unmarshal(match.Outbound, &tags); err != nil {
					return nativePolicyError("policy.config_invalid", path+".outbound", "")
				}
			}
			for _, tag := range tags {
				if tag == "any" {
					continue
				}
				if err := outbound(path+".outbound", tag); err != nil {
					return err
				}
			}
		} else if !dns && !nested && (rule.Action == "" || rule.Action == C.RuleActionTypeRoute || rule.Action == C.RuleActionTypeBypass) {
			// Nested route rules are match-only. The core rejects nested actions.
			var action option.RouteActionOptions
			if err := json.Unmarshal(raw, &action); err != nil {
				return nativePolicyError("policy.config_invalid", path, "")
			}
			if err := outbound(path+".outbound", action.Outbound); err != nil {
				return err
			}
		}
		if rule.Type == "logical" {
			if err := visitNativeRuleReferences(rule.Rules, path+".rules", dns, true, outbound); err != nil {
				return err
			}
		}
	}
	return nil
}

func (namespace *nativePolicyNamespace) requireOutbound(field, tag string) error {
	if tag == "" {
		return nil
	}
	if _, exists := namespace.outbounds[tag]; exists {
		return nil
	}
	// User Providers may be remote or unavailable while stopped. Their members
	// are dynamic; only Catalog members can be checked against a local snapshot.
	for providerTag := range namespace.nativeProviders {
		if strings.HasPrefix(tag, providerTag+"/") {
			return nil
		}
	}
	return nativePolicyError("policy.outbound_not_found", field, tag)
}

func nativePolicyError(code, field, tag string) error {
	return &service.Error{Code: code, Message: fmt.Sprintf("Native policy validation failed at %s: %q", field, tag)}
}
