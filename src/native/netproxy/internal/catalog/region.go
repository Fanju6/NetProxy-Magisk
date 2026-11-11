package catalog

import (
	"regexp"
	"strings"

	"github.com/sagernet/sing/common/json/badoption"
)

// RegionKey is the stable, locale-neutral region identifier used by policy
// selection and the generated URL-test groups.
func RegionKey(tag string) string {
	value := strings.ToLower(tag)
	for _, region := range catalogRegions {
		for _, token := range region.tokens {
			if strings.Contains(value, token) {
				return region.key
			}
		}
	}
	return "other"
}

func regexpToBadOption(value *regexp.Regexp) *badoption.Regexp {
	if value == nil {
		return nil
	}
	converted := badoption.Regexp(*value)
	return &converted
}

// RegionAutoTag is deliberately stable so policy overlays can refer to the
// generated group without persisting provider changes.
func RegionAutoTag(runtimeTag, region string) string {
	return "Auto/" + runtimeTag + " [" + region + "]"
}

// RegionIncludeRegex matches the runtime provider names exposed by sing-box.
// Provider nodes are namespaced as "<runtime group>/<node>", so matching the
// persisted node tag alone leaves a region URLTest with no members.
func RegionIncludeRegex(runtimeTag string, tags []string) *regexp.Regexp {
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		parts = append(parts, regexp.QuoteMeta(runtimeTag+"/"+tag))
	}
	if len(parts) == 0 {
		return nil
	}
	return regexp.MustCompile("^(?:" + strings.Join(parts, "|") + ")$")
}

type regionRule struct {
	key    string
	tokens []string
}

var catalogRegions = []regionRule{
	{key: "hk", tokens: []string{"🇭🇰", "香港", "hong kong", "hongkong"}},
	{key: "mo", tokens: []string{"🇲🇴", "澳门", "macau"}},
	{key: "tw", tokens: []string{"🇹🇼", "台湾", "taiwan"}},
	{key: "jp", tokens: []string{"🇯🇵", "日本", "japan"}},
	{key: "kr", tokens: []string{"🇰🇷", "韩国", "korea"}},
	{key: "sg", tokens: []string{"🇸🇬", "新加坡", "singapore"}},
	{key: "us", tokens: []string{"🇺🇸", "美国", "united states", "usa", "us-"}},
	{key: "ca", tokens: []string{"🇨🇦", "加拿大", "canada"}},
	{key: "gb", tokens: []string{"🇬🇧", "英国", "united kingdom", "uk-"}},
	{key: "de", tokens: []string{"🇩🇪", "德国", "germany"}},
	{key: "fr", tokens: []string{"🇫🇷", "法国", "france"}},
	{key: "nl", tokens: []string{"🇳🇱", "荷兰", "netherlands"}},
	{key: "au", tokens: []string{"🇦🇺", "澳大利亚", "australia"}},
	{key: "in", tokens: []string{"🇮🇳", "印度", "india"}},
	{key: "tr", tokens: []string{"🇹🇷", "土耳其", "turkey"}},
	{key: "ru", tokens: []string{"🇷🇺", "俄罗斯", "russia"}},
}
