package ebpf

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

const (
	defaultUDPTimeout     = "5m"
	defaultCgroupIPv6Mode = "always"
	defaultSharedIface    = "wlan2"
	defaultMapCapacity    = 65536
	maxMapCapacity        = 1048576
)

var allowedKeys = map[string]bool{
	"EBPF_NETWORK":                      true,
	"EBPF_UDP_TIMEOUT":                  true,
	"EBPF_DNS_MODE":                     true,
	"EBPF_CGROUP_ENABLED":               true,
	"EBPF_CGROUP_PATH":                  true,
	"EBPF_CGROUP_IPV6_MODE":             true,
	"EBPF_BYPASS_PRIVATE_ADDRESS":       true,
	"EBPF_BYPASS_RULE_SETS":             true,
	"APP_PROXY_ENABLE":                  true,
	"APP_PROXY_MODE":                    true,
	"APP_ANDROID_USERS":                 true,
	"PROXY_APPS_LIST":                   true,
	"BYPASS_APPS_LIST":                  true,
	"EBPF_SHARED_NETWORK":               true,
	"EBPF_SHARED_INTERFACES":            true,
	"EBPF_SHARED_INCLUDE_SOURCE_CIDRS":  true,
	"EBPF_SHARED_EXCLUDE_SOURCE_CIDRS":  true,
	"EBPF_SHARED_INCLUDE_MAC_ADDRESSES": true,
	"EBPF_SHARED_EXCLUDE_MAC_ADDRESSES": true,
	"EBPF_TCP_MAP_CAPACITY":             true,
	"EBPF_UDP_MAP_CAPACITY":             true,
	"EBPF_SOCKET_MAP_CAPACITY":          true,
	"EBPF_SHARED_PROXY_MAP_CAPACITY":    true,
	"EBPF_SHARED_BYPASS_MAP_CAPACITY":   true,
	"EBPF_SHARED_FRAGMENT_MAP_CAPACITY": true,
}

// Config 描述 ebpf.conf 的类型化配置。
type Config struct {
	Network                   string
	UDPTimeout                string
	DNSMode                   string
	CgroupEnabled             bool
	CgroupPath                string
	CgroupIPv6Mode            string
	BypassPrivateAddress      bool
	BypassRuleSets            []string
	AppProxyEnable            bool
	AppProxyMode              string
	AndroidUsers              []uint64
	ProxyPackages             []string
	BypassPackages            []string
	SharedNetwork             bool
	SharedInterfaces          []string
	SharedIncludeSourceCIDRs  []string
	SharedExcludeSourceCIDRs  []string
	SharedIncludeMACAddresses []string
	SharedExcludeMACAddresses []string
	TCPMapCapacity            uint64
	UDPMapCapacity            uint64
	SocketMapCapacity         uint64
	SharedProxyMapCapacity    uint64
	SharedBypassMapCapacity   uint64
	SharedFragmentMapCapacity uint64
}

// Diagnostic 描述可以直接展示给用户的配置问题。
type Diagnostic struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// Summary 描述诊断所需的安全配置摘要。
// ValidationError 包含可供 CLI 转发的中文配置诊断。
type ValidationError struct {
	Diagnostics []Diagnostic
}

func (e *ValidationError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "eBPF 配置无效"
	}
	return e.Diagnostics[0].Message
}

// Load 读取并校验 ebpf.conf，不执行 Shell 语义。
func Load(path string) (Config, error) {
	values, err := moduleconfig.ReadStrict(path)
	if err != nil {
		return Config{}, validationError("ebpf.config_parse_failed", "", err.Error())
	}
	for key := range values {
		if !allowedKeys[key] {
			return Config{}, validationError("ebpf.unknown_key", key, "配置项名称不受支持，请检查拼写")
		}
	}
	config := Config{
		UDPTimeout:                defaultUDPTimeout,
		DNSMode:                   "hijack",
		CgroupEnabled:             true,
		CgroupIPv6Mode:            defaultCgroupIPv6Mode,
		BypassPrivateAddress:      true,
		BypassRuleSets:            []string{"direct", "ChinaIP"},
		AppProxyEnable:            true,
		AppProxyMode:              "blacklist",
		SharedInterfaces:          []string{defaultSharedIface},
		TCPMapCapacity:            defaultMapCapacity,
		UDPMapCapacity:            defaultMapCapacity,
		SocketMapCapacity:         defaultMapCapacity,
		SharedProxyMapCapacity:    defaultMapCapacity,
		SharedBypassMapCapacity:   defaultMapCapacity,
		SharedFragmentMapCapacity: defaultMapCapacity,
	}
	var parseErr error
	config.Network = valueOr(values, "EBPF_NETWORK", "")
	config.UDPTimeout = valueOr(values, "EBPF_UDP_TIMEOUT", config.UDPTimeout)
	config.DNSMode = valueOr(values, "EBPF_DNS_MODE", config.DNSMode)
	config.CgroupEnabled, parseErr = boolValue(values, "EBPF_CGROUP_ENABLED", config.CgroupEnabled)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.CgroupPath = valueOr(values, "EBPF_CGROUP_PATH", "")
	config.CgroupIPv6Mode = valueOr(values, "EBPF_CGROUP_IPV6_MODE", config.CgroupIPv6Mode)
	config.BypassPrivateAddress, parseErr = boolValue(values, "EBPF_BYPASS_PRIVATE_ADDRESS", config.BypassPrivateAddress)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.BypassRuleSets = CommaSeparated(valueOr(values, "EBPF_BYPASS_RULE_SETS", "direct,ChinaIP"))
	config.AppProxyEnable, parseErr = boolValue(values, "APP_PROXY_ENABLE", config.AppProxyEnable)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.AppProxyMode = valueOr(values, "APP_PROXY_MODE", config.AppProxyMode)
	config.AndroidUsers, parseErr = parseUsers(valueOr(values, "APP_ANDROID_USERS", ""))
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.ProxyPackages, parseErr = parsePackages(valueOr(values, "PROXY_APPS_LIST", ""))
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.BypassPackages, parseErr = parsePackages(valueOr(values, "BYPASS_APPS_LIST", ""))
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.SharedNetwork, parseErr = boolValue(values, "EBPF_SHARED_NETWORK", false)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.SharedInterfaces = CommaSeparated(valueOr(values, "EBPF_SHARED_INTERFACES", defaultSharedIface))
	config.SharedIncludeSourceCIDRs, parseErr = parseCIDRs(valueOr(values, "EBPF_SHARED_INCLUDE_SOURCE_CIDRS", ""))
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.SharedExcludeSourceCIDRs, parseErr = parseCIDRs(valueOr(values, "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS", ""))
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.SharedIncludeMACAddresses, parseErr = parseMACs(valueOr(values, "EBPF_SHARED_INCLUDE_MAC_ADDRESSES", ""))
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.SharedExcludeMACAddresses, parseErr = parseMACs(valueOr(values, "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES", ""))
	if parseErr != nil {
		return Config{}, parseErr
	}
	if config.CgroupEnabled {
		config.TCPMapCapacity, parseErr = mapCapacity(values, "EBPF_TCP_MAP_CAPACITY", config.TCPMapCapacity)
		if parseErr != nil {
			return Config{}, parseErr
		}
		config.UDPMapCapacity, parseErr = mapCapacity(values, "EBPF_UDP_MAP_CAPACITY", config.UDPMapCapacity)
		if parseErr != nil {
			return Config{}, parseErr
		}
		config.SocketMapCapacity, parseErr = mapCapacity(values, "EBPF_SOCKET_MAP_CAPACITY", config.SocketMapCapacity)
		if parseErr != nil {
			return Config{}, parseErr
		}
	}
	config.SharedProxyMapCapacity, parseErr = mapCapacity(values, "EBPF_SHARED_PROXY_MAP_CAPACITY", config.SharedProxyMapCapacity)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.SharedBypassMapCapacity, parseErr = mapCapacity(values, "EBPF_SHARED_BYPASS_MAP_CAPACITY", config.SharedBypassMapCapacity)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.SharedFragmentMapCapacity, parseErr = mapCapacity(values, "EBPF_SHARED_FRAGMENT_MAP_CAPACITY", config.SharedFragmentMapCapacity)
	if parseErr != nil {
		return Config{}, parseErr
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate 检查 eBPF 配置之间的组合约束。
func (c Config) Validate() error {
	if c.Network != "" && c.Network != "tcp" && c.Network != "udp" {
		return validationError("ebpf.network_invalid", "EBPF_NETWORK", "代理协议只能为空、tcp 或 udp")
	}
	duration, err := time.ParseDuration(c.UDPTimeout)
	if err != nil || duration <= 0 {
		return validationError("ebpf.udp_timeout_invalid", "EBPF_UDP_TIMEOUT", "UDP 会话超时必须是正的时间长度，例如 5m")
	}
	if c.DNSMode != "hijack" && c.DNSMode != "off" {
		return validationError("ebpf.dns_mode_invalid", "EBPF_DNS_MODE", "DNS 模式只能是 hijack 或 off")
	}
	if c.CgroupIPv6Mode != "always" && c.CgroupIPv6Mode != "auto" && c.CgroupIPv6Mode != "off" {
		return validationError("ebpf.cgroup_ipv6_mode_invalid", "EBPF_CGROUP_IPV6_MODE", "本机 IPv6 接管模式只能是 always、auto 或 off")
	}
	if c.AppProxyMode != "blacklist" && c.AppProxyMode != "whitelist" {
		return validationError("ebpf.app_mode_invalid", "APP_PROXY_MODE", "分应用代理模式只能是 blacklist 或 whitelist")
	}
	if c.SharedNetwork && len(c.SharedInterfaces) == 0 {
		return validationError("ebpf.shared_interface_required", "EBPF_SHARED_INTERFACES", "启用共享网络时至少需要一个下游接口")
	}
	if c.SharedNetwork && c.DNSMode == "hijack" && c.Network == "tcp" {
		return validationError("ebpf.shared_dns_requires_udp", "EBPF_NETWORK", "共享网络启用 DNS 劫持时不能只代理 TCP")
	}
	if !c.CgroupEnabled && !c.SharedNetwork {
		return validationError("ebpf.no_data_path", "EBPF_CGROUP_ENABLED", "本机 cgroup 与共享网络不能同时禁用")
	}
	return nil
}

// Build 生成 sing-box eBPF inbound 的类型化运行时文档。
func (c Config) Build() Runtime {
	redirect := []string{"127.128.0.0/9"}
	if (c.CgroupEnabled && c.CgroupIPv6Mode != "off") || c.SharedNetwork {
		redirect = append(redirect, "fd53:696e:672d:626f::/64")
	}
	inbound := Inbound{
		Type:                 "ebpf",
		Tag:                  "ebpf-in",
		CgroupEnabled:        c.CgroupEnabled,
		Network:              c.Network,
		UDPTimeout:           c.UDPTimeout,
		DNSMode:              c.DNSMode,
		BypassPrivateAddress: c.BypassPrivateAddress,
		RedirectAddress:      redirect,
		BypassRuleSet:        c.BypassRuleSets,
		SharedNetwork: SharedNetwork{
			Enabled:           c.SharedNetwork,
			IncludeInterface:  c.SharedInterfaces,
			IncludeSourceCIDR: c.SharedIncludeSourceCIDRs,
			ExcludeSourceCIDR: c.SharedExcludeSourceCIDRs,
			IncludeMACAddress: c.SharedIncludeMACAddresses,
			ExcludeMACAddress: c.SharedExcludeMACAddresses,
			TCPriority:        1,
			MapCapacity: SharedMapCapacity{
				Proxy:    c.SharedProxyMapCapacity,
				Bypass:   c.SharedBypassMapCapacity,
				Fragment: c.SharedFragmentMapCapacity,
			},
		},
	}
	if c.CgroupEnabled {
		includePackages := []string{}
		excludePackages := []string{}
		includeUID := []uint64{}
		if c.AppProxyEnable {
			if c.AppProxyMode == "whitelist" {
				includePackages = append(includePackages, c.ProxyPackages...)
				if len(includePackages) == 0 {
					includeUID = []uint64{4294967295}
				}
			} else {
				excludePackages = append(excludePackages, c.BypassPackages...)
			}
		}
		inbound.Cgroup = &CgroupFields{
			Path:               c.CgroupPath,
			IPv6Mode:           c.CgroupIPv6Mode,
			IncludeUID:         includeUID,
			IncludeAndroidUser: c.AndroidUsers,
			IncludePackages:    includePackages,
			ExcludePackages:    excludePackages,
			TCPMapCapacity:     c.TCPMapCapacity,
			UDPMapCapacity:     c.UDPMapCapacity,
			SocketMapCapacity:  c.SocketMapCapacity,
		}
	}
	return Runtime{Inbounds: []Inbound{inbound}}
}

// Runtime 是 sing-box 运行时配置文档。
type Runtime struct {
	Inbounds []Inbound `json:"inbounds"`
}

// Inbound 是 eBPF 入站的固定字段模型。
type Inbound struct {
	Type                 string
	Tag                  string
	CgroupEnabled        bool
	Network              string
	UDPTimeout           string
	DNSMode              string
	BypassPrivateAddress bool
	Cgroup               *CgroupFields
	RedirectAddress      []string
	BypassRuleSet        []string
	SharedNetwork        SharedNetwork
}

// CgroupFields 是仅在本机 cgroup 路径启用时输出的字段。
type CgroupFields struct {
	Path               string
	IPv6Mode           string
	IncludeUID         []uint64
	IncludeAndroidUser []uint64
	IncludePackages    []string
	ExcludePackages    []string
	TCPMapCapacity     uint64
	UDPMapCapacity     uint64
	SocketMapCapacity  uint64
}

// SharedNetwork 是共享网络数据路径配置。
type SharedNetwork struct {
	Enabled           bool              `json:"enabled"`
	IncludeInterface  []string          `json:"include_interface"`
	IncludeSourceCIDR []string          `json:"include_source_cidr"`
	ExcludeSourceCIDR []string          `json:"exclude_source_cidr"`
	IncludeMACAddress []string          `json:"include_mac_address"`
	ExcludeMACAddress []string          `json:"exclude_mac_address"`
	TCPriority        int               `json:"tc_priority"`
	MapCapacity       SharedMapCapacity `json:"map_capacity"`
}

// SharedMapCapacity 是共享网络三类运行状态 Map 的容量。
type SharedMapCapacity struct {
	Proxy    uint64 `json:"proxy"`
	Bypass   uint64 `json:"bypass"`
	Fragment uint64 `json:"fragment"`
}

// MarshalJSON 根据 cgroup 是否启用裁剪本机路径字段。
func (i Inbound) MarshalJSON() ([]byte, error) {
	value := map[string]any{
		"type":                   i.Type,
		"tag":                    i.Tag,
		"cgroup_enabled":         i.CgroupEnabled,
		"udp_timeout":            i.UDPTimeout,
		"dns_mode":               i.DNSMode,
		"bypass_private_address": i.BypassPrivateAddress,
		"redirect_address":       i.RedirectAddress,
		"bypass_rule_set":        i.BypassRuleSet,
		"shared_network":         i.SharedNetwork,
	}
	if i.Network != "" {
		value["network"] = i.Network
	}
	if i.CgroupEnabled && i.Cgroup != nil {
		value["cgroup_path"] = i.Cgroup.Path
		value["cgroup_ipv6_mode"] = i.Cgroup.IPv6Mode
		value["include_uid"] = i.Cgroup.IncludeUID
		value["include_uid_range"] = []uint64{}
		value["exclude_uid"] = []uint64{}
		value["exclude_uid_range"] = []uint64{}
		value["include_android_user"] = i.Cgroup.IncludeAndroidUser
		value["include_package"] = i.Cgroup.IncludePackages
		value["exclude_package"] = i.Cgroup.ExcludePackages
		value["map_capacity"] = map[string]uint64{
			"tcp_redirect":  i.Cgroup.TCPMapCapacity,
			"udp_redirect":  i.Cgroup.UDPMapCapacity,
			"socket_bypass": i.Cgroup.SocketMapCapacity,
		}
	}
	return json.Marshal(value)
}

// WriteAtomic 校验并原子写入运行时 eBPF 配置。
func WriteAtomic(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	content, err := json.MarshalIndent(config.Build(), "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ebpf-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(content)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Diagnose 返回不包含凭据的配置检查结果。
func validationError(code, field, message string) error {
	return &ValidationError{Diagnostics: []Diagnostic{{Level: "error", Code: code, Field: field, Message: message}}}
}

func valueOr(values map[string]string, key, fallback string) string {
	if value, ok := values[key]; ok {
		return value
	}
	return fallback
}

// CommaSeparated 解析 eBPF 配置使用的逗号分隔值。
func CommaSeparated(value string) []string {
	value = strings.ReplaceAll(value, "，", ",")
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func boolValue(values map[string]string, key string, fallback bool) (bool, error) {
	value, ok := values[key]
	if !ok {
		return fallback, nil
	}
	switch value {
	case "0", "false":
		return false, nil
	case "1", "true":
		return true, nil
	default:
		return false, validationError("ebpf.boolean_invalid", key, key+" 必须为 0、1、true 或 false")
	}
}

func parseUsers(value string) ([]uint64, error) {
	result := CommaSeparated(value)
	users := make([]uint64, 0, len(result))
	for _, item := range result {
		parsed, err := strconv.ParseUint(item, 10, 32)
		if err != nil {
			return nil, validationError("ebpf.android_user_invalid", "APP_ANDROID_USERS", "Android 用户 ID 必须是 0 到 4294967295 的整数")
		}
		users = append(users, parsed)
	}
	return users, nil
}

func parsePackages(value string) ([]string, error) {
	packages := CommaSeparated(value)
	for _, item := range packages {
		for _, char := range item {
			if !(char == '.' || char == '_' || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
				return nil, validationError("ebpf.package_invalid", "PROXY_APPS_LIST/BYPASS_APPS_LIST", "应用包名只能包含字母、数字、点和下划线")
			}
		}
	}
	return packages, nil
}

func parseCIDRs(value string) ([]string, error) {
	items := CommaSeparated(value)
	for _, item := range items {
		if _, _, err := net.ParseCIDR(item); err != nil {
			return nil, validationError("ebpf.cidr_invalid", "EBPF_SHARED_*_SOURCE_CIDRS", "共享网络来源 CIDR 格式无效: "+item)
		}
	}
	return items, nil
}

func parseMACs(value string) ([]string, error) {
	items := CommaSeparated(value)
	for _, item := range items {
		parsed, err := net.ParseMAC(item)
		if err != nil || len(parsed) != 6 {
			return nil, validationError("ebpf.mac_invalid", "EBPF_SHARED_*_MAC_ADDRESSES", "共享网络 MAC 必须是 EUI-48 地址: "+item)
		}
	}
	return items, nil
}

func mapCapacity(values map[string]string, key string, fallback uint64) (uint64, error) {
	value := valueOr(values, key, strconv.FormatUint(fallback, 10))
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil || parsed < 1 || parsed > maxMapCapacity {
		return 0, validationError("ebpf.map_capacity_invalid", key, key+" 必须是 1 到 1048576 之间的整数")
	}
	return parsed, nil
}
