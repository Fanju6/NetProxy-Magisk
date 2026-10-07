package config

import (
	json "encoding/json/v2"
	"errors"
	"os"
	"slices"

	"github.com/sagernet/sing-box/experimental/clashmode"
	"github.com/sagernet/sing-box/option"
)

// Modes 描述主配置默认模式与内核根据规则计算的可选模式。
type Modes struct {
	Mode      string   `json:"mode"`
	Available []string `json:"available"`
}

func LoadModes(path string) (Modes, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Modes{}, err
	}
	return ParseModes(content)
}

func ParseModes(content []byte) (Modes, error) {
	// 只解码模式相关分区，不要求注册入站、DNS transport 或协议引擎。
	var document *struct {
		Experimental *struct {
			ClashAPI *option.ClashAPIOptions `json:"clash_api"`
		} `json:"experimental"`
		Route *struct {
			Rules []modeRule `json:"rules"`
		} `json:"route"`
		DNS *struct {
			Rules []modeRule `json:"rules"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return Modes{}, err
	}
	if document == nil {
		return Modes{}, errors.New("主配置必须是 JSON 对象")
	}
	mode := "Rule"
	if document.Experimental != nil && document.Experimental.ClashAPI != nil && document.Experimental.ClashAPI.DefaultMode != "" {
		mode = document.Experimental.ClashAPI.DefaultMode
	}
	var native option.Options
	if document.Route != nil {
		native.Route = &option.RouteOptions{Rules: routeModeRules(document.Route.Rules)}
	}
	if document.DNS != nil {
		native.DNS = &option.DNSOptions{Rules: dnsModeRules(document.DNS.Rules)}
	}
	available := clashmode.CalculateModeList(native)
	if !slices.Contains(available, mode) {
		available = append([]string{mode}, available...)
	}
	return Modes{Mode: mode, Available: available}, nil
}

// 上游规则使用上下文 JSON 解码；此投影保留标准库 JSON v2 的严格校验。
type modeRule struct {
	Type  string     `json:"type"`
	Modes modeNames  `json:"clash_mode"`
	Rules []modeRule `json:"rules"`
}

type modeNames []string

func (names *modeNames) UnmarshalJSON(content []byte) error {
	if len(content) > 0 && content[0] == '"' {
		var name string
		if err := json.Unmarshal(content, &name); err != nil {
			return err
		}
		*names = []string{name}
		return nil
	}
	return json.Unmarshal(content, (*[]string)(names))
}

func (rule *modeRule) UnmarshalJSON(content []byte) error {
	type plain modeRule
	if err := json.Unmarshal(content, (*plain)(rule)); err != nil {
		return err
	}
	switch rule.Type {
	case "", "default", "logical":
		return nil
	default:
		return errors.New("未知规则类型: " + rule.Type)
	}
}

func routeModeRules(rules []modeRule) []option.Rule {
	result := make([]option.Rule, len(rules))
	for index, rule := range rules {
		if rule.Type == "logical" {
			result[index] = option.Rule{Type: "logical", LogicalOptions: option.LogicalRule{Rules: routeModeRules(rule.Rules)}}
		} else {
			result[index].Type = "default"
			result[index].DefaultOptions.ClashMode = []string(rule.Modes)
		}
	}
	return result
}

func dnsModeRules(rules []modeRule) []option.DNSRule {
	result := make([]option.DNSRule, len(rules))
	for index, rule := range rules {
		if rule.Type == "logical" {
			result[index] = option.DNSRule{Type: "logical", LogicalOptions: option.LogicalDNSRule{Rules: dnsModeRules(rule.Rules)}}
		} else {
			result[index].Type = "default"
			result[index].DefaultOptions.ClashMode = []string(rule.Modes)
		}
	}
	return result
}
