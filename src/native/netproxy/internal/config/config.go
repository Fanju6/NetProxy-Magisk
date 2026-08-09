package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ModuleConfig 描述 module.conf 中由运行时使用的全部设置。
type ModuleConfig struct {
	AutoStart       bool   `json:"auto_start"`
	OutboundMode    string `json:"outbound_mode"`
	SelectorMode    string `json:"selector_mode"`
	ActiveGroupID   string `json:"active_group_id"`
	SelectedNodeRef string `json:"selected_node_ref"`
	WiFiAutoSwitch  bool   `json:"wifi_auto_switch"`
	WiFiSSIDMode    string `json:"wifi_ssid_mode"`
	WiFiSSIDList    string `json:"wifi_ssid_list"`
	ProxyOnCellular bool   `json:"proxy_on_cellular"`
}

// ReadStrict 读取受限的 KEY=value 配置，不执行任何 Shell 语义。
func ReadStrict(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for lineNumber, line := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		position := strings.IndexByte(line, '=')
		if position <= 0 {
			return nil, fmt.Errorf("第 %d 行不是有效的 KEY=value 配置", lineNumber+1)
		}
		key := strings.TrimSpace(line[:position])
		if !validKey(key) {
			return nil, fmt.Errorf("第 %d 行包含非法配置键: %s", lineNumber+1, key)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("配置键重复: %s", key)
		}
		value, err := decodeValueStrict(strings.TrimSpace(line[position+1:]))
		if err != nil {
			return nil, fmt.Errorf("配置键 %s 的值无效: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}

// LoadModule 读取并校验 module.conf 的类型化模型。
func LoadModule(path string) (ModuleConfig, error) {
	values, err := ReadStrict(path)
	if err != nil {
		return ModuleConfig{}, err
	}
	allowed := map[string]bool{
		"AUTO_START": true, "OUTBOUND_MODE": true, "SELECTOR_MODE": true,
		"ACTIVE_GROUP_ID": true, "SELECTED_NODE_REF": true,
		"WIFI_AUTO_SWITCH": true, "WIFI_SSID_MODE": true,
		"WIFI_SSID_LIST": true, "PROXY_ON_CELLULAR": true,
	}
	for key := range values {
		if !allowed[key] {
			return ModuleConfig{}, fmt.Errorf("不支持的 module.conf 配置键: %s", key)
		}
	}
	config := ModuleConfig{
		OutboundMode:    "rule",
		SelectorMode:    "urltest",
		ActiveGroupID:   "default",
		WiFiSSIDMode:    "blacklist",
		ProxyOnCellular: true,
	}
	if config.AutoStart, err = boolValue(values, "AUTO_START", config.AutoStart); err != nil {
		return ModuleConfig{}, err
	}
	if config.OutboundMode = valueOr(values, "OUTBOUND_MODE", config.OutboundMode); config.OutboundMode != "rule" && config.OutboundMode != "global" && config.OutboundMode != "direct" && config.OutboundMode != "AllowAds" {
		return ModuleConfig{}, fmt.Errorf("OUTBOUND_MODE 无效: %s", config.OutboundMode)
	}
	if config.SelectorMode = valueOr(values, "SELECTOR_MODE", config.SelectorMode); config.SelectorMode != "urltest" && config.SelectorMode != "manual" && config.SelectorMode != "auto" && config.SelectorMode != "selector" {
		return ModuleConfig{}, fmt.Errorf("SELECTOR_MODE 无效: %s", config.SelectorMode)
	}
	config.ActiveGroupID = valueOr(values, "ACTIVE_GROUP_ID", config.ActiveGroupID)
	config.SelectedNodeRef = valueOr(values, "SELECTED_NODE_REF", "")
	// 没有任何 Catalog 分组时允许为空；下一次导入非空分组时由应用服务重新设置。
	if config.WiFiAutoSwitch, err = boolValue(values, "WIFI_AUTO_SWITCH", config.WiFiAutoSwitch); err != nil {
		return ModuleConfig{}, err
	}
	config.WiFiSSIDMode = valueOr(values, "WIFI_SSID_MODE", config.WiFiSSIDMode)
	if config.WiFiSSIDMode != "blacklist" && config.WiFiSSIDMode != "whitelist" {
		return ModuleConfig{}, fmt.Errorf("WIFI_SSID_MODE 无效: %s", config.WiFiSSIDMode)
	}
	config.WiFiSSIDList = valueOr(values, "WIFI_SSID_LIST", "")
	if strings.ContainsAny(config.WiFiSSIDList, "\r\n\t") {
		return ModuleConfig{}, errors.New("WIFI_SSID_LIST 不能包含换行或制表符")
	}
	if config.ProxyOnCellular, err = boolValue(values, "PROXY_ON_CELLULAR", config.ProxyOnCellular); err != nil {
		return ModuleConfig{}, err
	}
	return config, nil
}

// UpdateModule 更新并校验 module.conf，校验失败时不会替换原文件。
func UpdateModule(path string, updates map[string]string) error {
	return UpdateValidated(path, updates, func(candidate string) error {
		_, err := LoadModule(candidate)
		return err
	})
}

// UpdateValidated 使用候选文件完成校验后再原子替换原配置。
func UpdateValidated(path string, updates map[string]string, validate func(string) error) error {
	if len(updates) == 0 {
		return nil
	}
	for key := range updates {
		if !validKey(key) {
			return fmt.Errorf("非法配置键: %s", key)
		}
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lockPath := path + ".lock"
	if err := acquireLock(lockPath); err != nil {
		return err
	}
	defer os.RemoveAll(lockPath)

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	written := make(map[string]bool, len(updates))
	for index, line := range lines {
		key, _, found := strings.Cut(line, "=")
		if found {
			if value, ok := updates[key]; ok {
				lines[index] = key + "=" + value
				written[key] = true
			}
		}
	}
	for _, key := range keys {
		value := updates[key]
		if !written[key] {
			lines = append(lines, key+"="+value)
		}
	}
	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".module-conf-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = tmp.WriteString(updated); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if validate != nil {
		if err = validate(tmpPath); err != nil {
			return err
		}
	}
	return os.Rename(tmpPath, path)
}

// Quote 生成与模块配置兼容的双引号值。
func Quote(value string) string {
	return strconv.Quote(value)
}

func validKey(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return false
	}
	return true
}

func decodeValueStrict(value string) (string, error) {
	if value == "" || value[0] != '"' {
		if strings.ContainsAny(value, "\r\n\t") {
			return "", errors.New("不能包含换行或制表符")
		}
		return value, nil
	}
	if len(value) < 2 || value[len(value)-1] != '"' {
		return "", errors.New("双引号未闭合")
	}
	decoded, err := strconv.Unquote(value)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(decoded, "\r\n\t") {
		return "", errors.New("不能包含换行或制表符")
	}
	return decoded, nil
}

func valueOr(values map[string]string, key, fallback string) string {
	if value, ok := values[key]; ok {
		return value
	}
	return fallback
}

func boolValue(values map[string]string, key string, fallback bool) (bool, error) {
	value, ok := values[key]
	if !ok {
		return fallback, nil
	}
	switch value {
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s 必须为 0、1、true 或 false", key)
	}
}

func acquireLock(path string) error {
	for attempt := 0; attempt < 50; attempt++ {
		if err := os.Mkdir(path, 0o700); err == nil {
			return nil
		} else if !os.IsExist(err) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("配置文件正忙: %s", path)
}
