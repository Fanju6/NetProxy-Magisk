package policy

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

// Policy 独立保存用户策略，运行时投影不改写 Catalog 的 Provider。
type Policy struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Icon     string   `json:"icon"`
	Type     string   `json:"type"`
	RuleSets []string `json:"rule_sets"`
	// PackageNames 覆盖没有稳定域名的应用连接，例如 ChatGPT 的 API 流量。
	PackageNames   []string `json:"package_names,omitempty"`
	DomainSuffixes []string `json:"domain_suffixes,omitempty"`
	Ports          []int    `json:"ports,omitempty"`
	GroupID        string   `json:"group_id"`
	NodeTag        string   `json:"node_tag"`
	Mode           string   `json:"mode"`
	Region         string   `json:"region,omitempty"`
	RuntimeTag     string   `json:"runtime_tag"`
	Disabled       bool     `json:"disabled,omitempty"`
}

type file struct {
	Schema   int      `json:"schema"`
	Policies []Policy `json:"policies"`
}

func Defaults() []Policy {
	return []Policy{
		{ID: "ai", Name: "AI", Icon: "AI", Type: "domain", RuleSets: []string{"geosite/category-ai-!cn"}, PackageNames: []string{
			"com.openai.chatgpt",
			"com.anthropic.claude",
			"com.google.android.apps.bard",
			"com.microsoft.copilot",
			"ai.perplexity.app.android",
			"ai.x.grok",
		}, DomainSuffixes: []string{
			"openai.com",
			"chatgpt.com",
			"oaistatic.com",
			"oaiusercontent.com",
		}, GroupID: "default", Mode: "auto", RuntimeTag: "Proxy"},
		{ID: "youtube", Name: "YouTube", Icon: "YT", Type: "domain", RuleSets: []string{"geosite/youtube"}, GroupID: "default", Mode: "auto", RuntimeTag: "Proxy"},
		{ID: "tiktok", Name: "TikTok", Icon: "TK", Type: "domain", RuleSets: []string{"geosite/tiktok"}, GroupID: "default", Mode: "auto", RuntimeTag: "Proxy"},
		{ID: "netflix", Name: "Netflix", Icon: "NF", Type: "domain", RuleSets: []string{"geosite/netflix"}, GroupID: "default", Mode: "auto", RuntimeTag: "Proxy"},
		{ID: "github", Name: "GitHub", Icon: "GH", Type: "domain", RuleSets: []string{"geosite/github"}, GroupID: "default", Mode: "auto", RuntimeTag: "Proxy"},
		{ID: "google", Name: "Google", Icon: "G", Type: "domain", RuleSets: []string{"geosite/google"}, GroupID: "default", Mode: "auto", RuntimeTag: "Proxy"},
		{ID: "fcm", Name: "Google FCM", Icon: "FCM", Type: "domain", RuleSets: []string{"fcm"}, Ports: []int{5228, 5229, 5230}, GroupID: "default", Mode: "proxy", RuntimeTag: "Proxy"},
	}
}

func IsBuiltin(id string) bool {
	for _, item := range Defaults() {
		if item.ID == id {
			return true
		}
	}
	return false
}

func Load(ctx context.Context, path string) ([]Policy, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return nil, err
	}
	var stored file
	if err := json.Unmarshal(content, &stored); err != nil {
		return nil, fmt.Errorf("读取策略配置: %w", err)
	}
	if stored.Schema != 1 {
		return nil, fmt.Errorf("策略配置 schema 无效: %d", stored.Schema)
	}
	return mergeDefaults(stored.Policies), nil
}

func Save(ctx context.Context, path string, policies []Policy) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if len(policies) == 0 {
		return errors.New("策略列表不能为空")
	}
	content, err := json.Marshal(file{Schema: 1, Policies: policies}, json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'))
}

func mergeDefaults(stored []Policy) []Policy {
	byID := make(map[string]Policy, len(stored))
	for _, item := range stored {
		byID[item.ID] = item
	}
	defaults := Defaults()
	result := make([]Policy, 0, len(defaults)+len(stored))
	known := make(map[string]struct{}, len(defaults))
	for _, item := range defaults {
		known[item.ID] = struct{}{}
		if saved, ok := byID[item.ID]; ok {
			// 补充内置 AI 匹配条件时保留已有出口选择。
			if saved.ID == "ai" {
				if len(saved.PackageNames) == 0 {
					saved.PackageNames = item.PackageNames
				}
				if len(saved.DomainSuffixes) == 0 {
					saved.DomainSuffixes = item.DomainSuffixes
				}
			}
			result = append(result, saved)
		} else {
			result = append(result, item)
		}
	}
	for _, item := range stored {
		if _, ok := known[item.ID]; !ok {
			result = append(result, item)
		}
	}
	return result
}

func Find(policies []Policy, id string) (Policy, bool) {
	for _, item := range policies {
		if item.ID == id {
			return item, true
		}
	}
	return Policy{}, false
}

func Validate(value Policy) error {
	if strings.TrimSpace(value.ID) == "" || strings.TrimSpace(value.Name) == "" {
		return errors.New("策略 ID 和名称不能为空")
	}
	if value.Mode != "auto" && value.Mode != "manual" && value.Mode != "direct" && value.Mode != "proxy" {
		return errors.New("策略模式必须为 auto、manual、direct 或 proxy")
	}
	for _, port := range value.Ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("策略端口必须在 1 到 65535 之间: %d", port)
		}
	}
	if !IsBuiltin(value.ID) && len(value.RuleSets) == 0 && len(value.PackageNames) == 0 && len(value.DomainSuffixes) == 0 && len(value.Ports) == 0 {
		return errors.New("自定义策略至少需要一个应用、域名、端口或规则集匹配条件")
	}
	if (value.Mode == "auto" || value.Mode == "manual") && value.GroupID == "" {
		return errors.New("策略分组不能为空")
	}
	if value.Mode == "manual" && value.NodeTag == "" {
		return errors.New("手动策略必须选择节点")
	}
	if value.Mode != "auto" && value.Region != "" {
		return errors.New("地区自动选择只能用于 auto 模式")
	}
	return nil
}

// BuildRuntime 只生成路由规则，在启动和检查时作为独立配置加载；Provider 与出站仍由 Catalog 管理。
func BuildRuntime(ctx context.Context, path, catalogRoot string, policies []Policy) (bool, error) {
	groups, err := catalog.Scan(ctx, catalog.ScanOptions{Root: catalogRoot, Type: "all", WithNodes: true})
	if err != nil {
		return false, err
	}
	byID := make(map[string]catalog.GroupSnapshot, len(groups))
	for _, group := range groups {
		byID[group.Group.ID] = group
	}
	// 策略先于基础路由加载，需先处理 DNS，避免应用策略把原始 DNS 连接送往代理。
	rules := []map[string]any{
		{
			"type": "logical",
			"mode": "or",
			"rules": []map[string]any{
				{"port": 53},
				{"protocol": "dns"},
			},
			"action": "hijack-dns",
		},
	}
	for _, item := range policies {
		if item.Disabled {
			continue
		}
		target := item.RuntimeTag
		if target == "" {
			target = "Proxy"
		}
		switch item.Mode {
		case "direct":
			target = "direct"
		case "proxy":
			target = "Proxy"
		case "auto", "manual":
			// 分组模式在下方解析。
		default:
			continue
		}
		if item.Mode == "auto" || item.Mode == "manual" {
			group, ok := byID[item.GroupID]
			if !ok || len(group.Nodes) == 0 {
				continue
			}
			target = "Auto/" + group.Group.RuntimeTag
			if item.Mode == "auto" && item.Region != "" {
				regionTags := make([]string, 0)
				for _, node := range group.Nodes {
					if catalog.RegionKey(node.Tag) == item.Region {
						regionTags = append(regionTags, node.Tag)
					}
				}
				if len(regionTags) > 0 {
					target = catalog.RegionAutoTag(group.Group.RuntimeTag, item.Region)
				}
			}
			if item.Mode == "manual" {
				found := false
				for _, node := range group.Nodes {
					if node.Tag == item.NodeTag {
						target = group.Group.RuntimeTag + "/" + node.Tag
						found = true
						break
					}
				}
				if !found {
					target = "Auto/" + group.Group.RuntimeTag
				}
			}
		}
		if len(item.RuleSets) > 0 {
			rules = append(rules, map[string]any{
				"rule_set": item.RuleSets,
				"action":   "route",
				"outbound": target,
			})
		}
		if len(item.PackageNames) > 0 {
			rules = append(rules, map[string]any{
				"package_name": item.PackageNames,
				"action":       "route",
				"outbound":     target,
			})
		}
		if len(item.DomainSuffixes) > 0 {
			rules = append(rules, map[string]any{
				"domain_suffix": item.DomainSuffixes,
				"action":        "route",
				"outbound":      target,
			})
		}
		if len(item.Ports) > 0 {
			rules = append(rules, map[string]any{
				"network":  "tcp",
				"port":     item.Ports,
				"action":   "route",
				"outbound": target,
			})
		}
	}
	content, err := json.Marshal(map[string]any{"route": map[string]any{"rules": rules}}, json.Deterministic(true))
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := writeAtomic(path, append(content, '\n')); err != nil {
		return false, err
	}
	return true, nil
}

func writeAtomic(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".policies-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
