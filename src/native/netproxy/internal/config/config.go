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
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

type ModuleConfig struct {
	AutoStart bool       `json:"auto_start"`
	WiFi      WiFiPolicy `json:"wifi"`
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

// DefaultModule 返回全新配置使用的唯一默认值集合。
func DefaultModule() ModuleConfig {
	return ModuleConfig{
		WiFi: WiFiPolicy{Mode: "blacklist", Blacklist: []string{}, Whitelist: []string{}, ProxyOnNonWiFi: true},
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
