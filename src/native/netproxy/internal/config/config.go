package config

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

type ModuleConfig struct {
	AutoStart bool `json:"auto_start"`
	Selection `json:"selection"`
	WiFi      WiFiPolicy `json:"wifi"`
}

type Selection struct {
	ActiveGroupID   string `json:"group_id"`
	SelectedNodeTag string `json:"node_tag"`
}

type WiFiPolicy struct {
	Enabled        bool     `json:"enabled"`
	Mode           string   `json:"mode"`
	Blacklist      []string `json:"blacklist"`
	Whitelist      []string `json:"whitelist"`
	ProxyOnNonWiFi bool     `json:"proxy_on_non_wifi"`
}

func (policy WiFiPolicy) Equal(other WiFiPolicy) bool {
	return policy.Enabled == other.Enabled && policy.Mode == other.Mode &&
		policy.ProxyOnNonWiFi == other.ProxyOnNonWiFi &&
		slices.Equal(policy.Blacklist, other.Blacklist) && slices.Equal(policy.Whitelist, other.Whitelist)
}

func (selection Selection) Mode() string {
	if selection.SelectedNodeTag == "" {
		return "urltest"
	}
	return "manual"
}

func (selection Selection) Ref() string {
	if selection.SelectedNodeTag == "" {
		return ""
	}
	return selection.ActiveGroupID + "/" + selection.SelectedNodeTag
}

func (selection Selection) RuntimeTargets(runtimeTag string) (group, node string) {
	if selection.SelectedNodeTag == "" {
		return "Auto/" + runtimeTag, ""
	}
	return "Select/" + runtimeTag, runtimeTag + "/" + selection.SelectedNodeTag
}

// DefaultModule 返回全新配置使用的唯一默认值集合。
func DefaultModule() ModuleConfig {
	return ModuleConfig{
		Selection: Selection{ActiveGroupID: "default"},
		WiFi:      WiFiPolicy{Mode: "blacklist", Blacklist: []string{}, Whitelist: []string{}, ProxyOnNonWiFi: true},
	}
}

func LoadModule(path string) (ModuleConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return ModuleConfig{}, err
	}
	return ParseModule(content)
}

func ParseModule(content []byte) (ModuleConfig, error) {
	// JSON null 会把布尔、字符串和结构体静默置零；模块配置不允许这种隐式重置。
	decoder := jsontext.NewDecoder(bytes.NewReader(content))
	for {
		token, err := decoder.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ModuleConfig{}, fmt.Errorf("模块 JSON 配置无效: %w", err)
		}
		if token.Kind() == 'n' {
			return ModuleConfig{}, errors.New("模块配置字段不能为 null")
		}
	}
	config := DefaultModule()
	if err := json.Unmarshal(content, &config, json.RejectUnknownMembers(true)); err != nil {
		return ModuleConfig{}, fmt.Errorf("模块 JSON 配置无效: %w", err)
	}
	return config, config.validate()
}

func (config ModuleConfig) validate() error {
	if config.SelectedNodeTag != "" && (config.ActiveGroupID == "" || strings.TrimSpace(config.SelectedNodeTag) == "") {
		return errors.New("手动选择必须指定活动分组和有效节点 tag")
	}
	if config.WiFi.Mode != "blacklist" && config.WiFi.Mode != "whitelist" {
		return errors.New("wifi.mode 只能是 blacklist 或 whitelist")
	}
	for _, list := range []struct {
		field string
		ssids []string
	}{{"blacklist", config.WiFi.Blacklist}, {"whitelist", config.WiFi.Whitelist}} {
		seen := make(map[string]bool, len(list.ssids))
		for _, ssid := range list.ssids {
			if err := ValidateSSID(ssid); err != nil {
				return fmt.Errorf("wifi.%s: %w", list.field, err)
			}
			if seen[ssid] {
				return fmt.Errorf("wifi.%s 包含重复的 Wi-Fi 名称", list.field)
			}
			seen[ssid] = true
		}
	}
	return nil
}

func ValidateSSID(ssid string) error {
	if len(ssid) == 0 || len(ssid) > 32 || !utf8.ValidString(ssid) || strings.ContainsFunc(ssid, unicode.IsControl) {
		return errors.New("Wi-Fi 名称必须为 1 至 32 字节的 UTF-8 文本，不能包含控制字符")
	}
	return nil
}

func UpdateSelection(ctx context.Context, path string, selection Selection) error {
	editor, err := Lock(ctx, path)
	if err != nil {
		return err
	}
	defer editor.Release()
	return editor.UpdateSelection(selection)
}

// Editor 在显式持有文件锁期间完成配置读改写。
type Editor struct {
	path string
	*processlock.Lock
}

func Lock(ctx context.Context, path string) (*Editor, error) {
	lock, err := processlock.Acquire(ctx, path+".lock")
	if err != nil {
		return nil, err
	}
	return &Editor{path: path, Lock: lock}, nil
}

func (editor *Editor) UpdateSelection(selection Selection) error {
	content, err := os.ReadFile(editor.path)
	if err != nil {
		return err
	}
	config, err := ParseModule(content)
	if err != nil {
		return err
	}
	if config.Selection == selection {
		return nil
	}
	config.Selection = selection
	if err := config.validate(); err != nil {
		return err
	}
	// 保留未修改分区的原始字段，内部选择同步不能改变 Wi-Fi 草稿的 revision。
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(content, &fields); err != nil {
		return err
	}
	fields["selection"], err = json.Marshal(selection, json.Deterministic(true))
	if err != nil {
		return err
	}
	content, err = json.Marshal(fields, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(editor.path), ".module-json-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(append(content, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), editor.path)
}
