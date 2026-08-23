package ebpf

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

const (
	defaultMode           = "local"
	defaultUDPTimeout     = "5m"
	defaultLocalIPv6Mode  = "auto"
	defaultSharedIPv6Mode = "always"
	defaultSharedIface    = "wlan2"
	defaultDNSMode        = "hijack"
	defaultDataPlane      = "auto"
	defaultStateCapacity  = 0
	defaultTCPriority     = 1
	maxStateCapacity      = 1048576
	maxTCPriority         = 65535
)

var allowedKeys = map[string]bool{
	"EBPF_MODE":                          true,
	"EBPF_NETWORK":                       true,
	"EBPF_UDP_TIMEOUT":                   true,
	"EBPF_TCP_SPLICE":                    true,
	"EBPF_BYPASS_RULE_SET":               true,
	"EBPF_LOCAL_DNS_MODE":                true,
	"EBPF_LOCAL_CGROUP_PATH":             true,
	"EBPF_LOCAL_IPV6_MODE":               true,
	"EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS":  true,
	"EBPF_LOCAL_INCLUDE_UID":             true,
	"EBPF_LOCAL_INCLUDE_UID_RANGE":       true,
	"EBPF_LOCAL_EXCLUDE_UID":             true,
	"EBPF_LOCAL_EXCLUDE_UID_RANGE":       true,
	"EBPF_LOCAL_INCLUDE_ANDROID_USER":    true,
	"EBPF_LOCAL_INCLUDE_PACKAGE":         true,
	"EBPF_LOCAL_EXCLUDE_PACKAGE":         true,
	"EBPF_LOCAL_STATE_CAPACITY":          true,
	"EBPF_SHARED_DNS_MODE":               true,
	"EBPF_SHARED_INTERFACES":             true,
	"EBPF_SHARED_IPV6_MODE":              true,
	"EBPF_SHARED_BYPASS_PRIVATE_ADDRESS": true,
	"EBPF_SHARED_INCLUDE_SOURCE_CIDR":    true,
	"EBPF_SHARED_EXCLUDE_SOURCE_CIDR":    true,
	"EBPF_SHARED_INCLUDE_MAC_ADDRESS":    true,
	"EBPF_SHARED_EXCLUDE_MAC_ADDRESS":    true,
	"EBPF_SHARED_STATE_CAPACITY":         true,
	"EBPF_SHARED_TC_PRIORITY":            true,
	"EBPF_SHARED_DATA_PLANE":             true,
	"EBPF_SHARED_ROUTING_MARK":           true,
	"EBPF_SHARED_ROUTING_TABLE":          true,
	"APP_PROXY_ENABLE":                   true,
	"APP_PROXY_MODE":                     true,
	"PROXY_APPS_LIST":                    true,
	"BYPASS_APPS_LIST":                   true,
}

// Config 描述 ebpf.conf 的类型化配置。
type Config struct {
	Mode           string
	Network        []string
	UDPTimeout     string
	TCPSplice      bool
	BypassRuleSets []string
	Local          LocalConfig
	Shared         SharedConfig
	AppProxyEnable bool
	AppProxyMode   string
	ProxyPackages  []PackageRef
	BypassPackages []PackageRef
}

// LocalConfig 描述 sing-box eBPF 的本机数据路径。
type LocalConfig struct {
	DNSMode              string
	CgroupPath           string
	IPv6Mode             string
	BypassPrivateAddress bool
	IncludeUID           []uint32
	IncludeUIDRange      []string
	ExcludeUID           []uint32
	ExcludeUIDRange      []string
	IncludeAndroidUser   []int
	IncludePackage       []string
	ExcludePackage       []string
	StateCapacity        uint32
}

// SharedConfig 描述 sing-box eBPF 的共享网络数据路径。
type SharedConfig struct {
	DNSMode              string
	Interfaces           []string
	IPv6Mode             string
	BypassPrivateAddress bool
	IncludeSourceCIDR    []string
	ExcludeSourceCIDR    []string
	IncludeMACAddress    []string
	ExcludeMACAddress    []string
	StateCapacity        uint32
	TCPriority           uint16
	DataPlane            string
	RoutingMark          uint32
	RoutingTable         uint32
}

// PackageRef 是一个带 Android 用户范围的应用包名。
type PackageRef struct {
	UserID  uint32
	Package string
}

// Diagnostic 描述可以直接展示给用户的配置问题。
type Diagnostic struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

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
		Mode:           defaultMode,
		UDPTimeout:     defaultUDPTimeout,
		BypassRuleSets: []string{"direct", "ChinaIP"},
		Local: LocalConfig{
			DNSMode:              defaultDNSMode,
			IPv6Mode:             defaultLocalIPv6Mode,
			BypassPrivateAddress: true,
			StateCapacity:        defaultStateCapacity,
		},
		Shared: SharedConfig{
			DNSMode:              defaultDNSMode,
			Interfaces:           []string{defaultSharedIface},
			IPv6Mode:             defaultSharedIPv6Mode,
			BypassPrivateAddress: true,
			StateCapacity:        defaultStateCapacity,
			TCPriority:           defaultTCPriority,
			DataPlane:            defaultDataPlane,
		},
		AppProxyEnable: true,
		AppProxyMode:   "blacklist",
	}
	var parseErr error
	config.Mode = valueOr(values, "EBPF_MODE", config.Mode)
	config.Network = CommaSeparated(valueOr(values, "EBPF_NETWORK", ""))
	config.UDPTimeout = valueOr(values, "EBPF_UDP_TIMEOUT", config.UDPTimeout)
	config.TCPSplice, parseErr = boolValue(values, "EBPF_TCP_SPLICE", false)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.BypassRuleSets = CommaSeparated(valueOr(values, "EBPF_BYPASS_RULE_SET", "direct,ChinaIP"))

	config.Local.DNSMode = valueOr(values, "EBPF_LOCAL_DNS_MODE", config.Local.DNSMode)
	config.Local.CgroupPath = valueOr(values, "EBPF_LOCAL_CGROUP_PATH", "")
	config.Local.IPv6Mode = valueOr(values, "EBPF_LOCAL_IPV6_MODE", config.Local.IPv6Mode)
	config.Local.BypassPrivateAddress, parseErr = boolValue(values, "EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS", config.Local.BypassPrivateAddress)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.IncludeUID, parseErr = parseUint32List(values, "EBPF_LOCAL_INCLUDE_UID")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.IncludeUIDRange, parseErr = parseRanges(values, "EBPF_LOCAL_INCLUDE_UID_RANGE")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.ExcludeUID, parseErr = parseUint32List(values, "EBPF_LOCAL_EXCLUDE_UID")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.ExcludeUIDRange, parseErr = parseRanges(values, "EBPF_LOCAL_EXCLUDE_UID_RANGE")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.IncludeAndroidUser, parseErr = parseIntList(values, "EBPF_LOCAL_INCLUDE_ANDROID_USER")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.IncludePackage, parseErr = parsePackages(valueOr(values, "EBPF_LOCAL_INCLUDE_PACKAGE", ""), "EBPF_LOCAL_INCLUDE_PACKAGE")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.ExcludePackage, parseErr = parsePackages(valueOr(values, "EBPF_LOCAL_EXCLUDE_PACKAGE", ""), "EBPF_LOCAL_EXCLUDE_PACKAGE")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Local.StateCapacity, parseErr = stateCapacity(values, "EBPF_LOCAL_STATE_CAPACITY")
	if parseErr != nil {
		return Config{}, parseErr
	}

	config.Shared.DNSMode = valueOr(values, "EBPF_SHARED_DNS_MODE", config.Shared.DNSMode)
	config.Shared.Interfaces = CommaSeparated(valueOr(values, "EBPF_SHARED_INTERFACES", defaultSharedIface))
	config.Shared.IPv6Mode = valueOr(values, "EBPF_SHARED_IPV6_MODE", config.Shared.IPv6Mode)
	config.Shared.BypassPrivateAddress, parseErr = boolValue(values, "EBPF_SHARED_BYPASS_PRIVATE_ADDRESS", config.Shared.BypassPrivateAddress)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.IncludeSourceCIDR, parseErr = parseCIDRs(valueOr(values, "EBPF_SHARED_INCLUDE_SOURCE_CIDR", ""), "EBPF_SHARED_INCLUDE_SOURCE_CIDR")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.ExcludeSourceCIDR, parseErr = parseCIDRs(valueOr(values, "EBPF_SHARED_EXCLUDE_SOURCE_CIDR", ""), "EBPF_SHARED_EXCLUDE_SOURCE_CIDR")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.IncludeMACAddress, parseErr = parseMACs(valueOr(values, "EBPF_SHARED_INCLUDE_MAC_ADDRESS", ""), "EBPF_SHARED_INCLUDE_MAC_ADDRESS")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.ExcludeMACAddress, parseErr = parseMACs(valueOr(values, "EBPF_SHARED_EXCLUDE_MAC_ADDRESS", ""), "EBPF_SHARED_EXCLUDE_MAC_ADDRESS")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.StateCapacity, parseErr = stateCapacity(values, "EBPF_SHARED_STATE_CAPACITY")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.TCPriority, parseErr = tcpPriority(values, "EBPF_SHARED_TC_PRIORITY")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.DataPlane = valueOr(values, "EBPF_SHARED_DATA_PLANE", config.Shared.DataPlane)
	config.Shared.RoutingMark, parseErr = uint32Value(values, "EBPF_SHARED_ROUTING_MARK", 0)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.Shared.RoutingTable, parseErr = uint32Value(values, "EBPF_SHARED_ROUTING_TABLE", 0)
	if parseErr != nil {
		return Config{}, parseErr
	}

	config.AppProxyEnable, parseErr = boolValue(values, "APP_PROXY_ENABLE", config.AppProxyEnable)
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.AppProxyMode = valueOr(values, "APP_PROXY_MODE", config.AppProxyMode)
	config.ProxyPackages, parseErr = ParsePackageRefs(valueOr(values, "PROXY_APPS_LIST", ""), "PROXY_APPS_LIST")
	if parseErr != nil {
		return Config{}, parseErr
	}
	config.BypassPackages, parseErr = ParsePackageRefs(valueOr(values, "BYPASS_APPS_LIST", ""), "BYPASS_APPS_LIST")
	if parseErr != nil {
		return Config{}, parseErr
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate 检查新 eBPF 配置之间的组合约束。
func (c Config) Validate() error {
	if c.Mode != "local" && c.Mode != "shared" && c.Mode != "hybrid" {
		return validationError("ebpf.mode_invalid", "EBPF_MODE", "eBPF 模式只能是 local、shared 或 hybrid")
	}
	for _, network := range c.Network {
		if network != "tcp" && network != "udp" {
			return validationError("ebpf.network_invalid", "EBPF_NETWORK", "代理协议只能是 tcp 或 udp")
		}
	}
	if duration, err := time.ParseDuration(c.UDPTimeout); err != nil || duration <= 0 {
		return validationError("ebpf.udp_timeout_invalid", "EBPF_UDP_TIMEOUT", "UDP 会话超时必须是正的时间长度，例如 5m")
	}
	if !validDNSMode(c.Local.DNSMode) {
		return validationError("ebpf.local_dns_mode_invalid", "EBPF_LOCAL_DNS_MODE", "本机 DNS 模式只能是 hijack、respect_policy 或 off")
	}
	if !validDNSMode(c.Shared.DNSMode) {
		return validationError("ebpf.shared_dns_mode_invalid", "EBPF_SHARED_DNS_MODE", "共享网络 DNS 模式只能是 hijack、respect_policy 或 off")
	}
	if c.Shared.DataPlane != "auto" && c.Shared.DataPlane != "socket_assign" && c.Shared.DataPlane != "rewrite" {
		return validationError("ebpf.shared_data_plane_invalid", "EBPF_SHARED_DATA_PLANE", "共享网络数据面只能是 auto、socket_assign 或 rewrite")
	}
	if c.Mode == "local" || c.Mode == "hybrid" {
		if c.Local.IPv6Mode != "always" && c.Local.IPv6Mode != "auto" && c.Local.IPv6Mode != "off" {
			return validationError("ebpf.local_ipv6_mode_invalid", "EBPF_LOCAL_IPV6_MODE", "本机 IPv6 模式只能是 always、auto 或 off")
		}
	}
	if c.Mode == "shared" || c.Mode == "hybrid" {
		if len(c.Shared.Interfaces) == 0 {
			return validationError("ebpf.shared_interface_required", "EBPF_SHARED_INTERFACES", "共享网络模式至少需要一个下游接口")
		}
		if c.Shared.IPv6Mode != "always" && c.Shared.IPv6Mode != "off" {
			return validationError("ebpf.shared_ipv6_mode_invalid", "EBPF_SHARED_IPV6_MODE", "共享网络 IPv6 模式只能是 always 或 off")
		}
	}
	if c.AppProxyMode != "blacklist" && c.AppProxyMode != "whitelist" {
		return validationError("ebpf.app_mode_invalid", "APP_PROXY_MODE", "分应用代理模式只能是 blacklist 或 whitelist")
	}
	return nil
}

// PackageUIDResolver 将带用户范围的包名转换为 UID，并报告当前设备上不存在的引用。
type PackageUIDResolver func([]PackageRef) (PackageUIDResolution, error)

// BuildResult 描述生成的运行时文档以及被跳过的应用配置。
type BuildResult struct {
	Runtime         Runtime
	MissingPackages []PackageRef
}

// Build 生成 sing-box eBPF inbound 的类型化运行时文档。
func (c Config) Build() (BuildResult, error) {
	return c.BuildWithResolver(ResolvePackageUIDs)
}

// BuildWithResolver 使用指定的包名解析器生成运行时文档，测试可注入确定性解析结果。
func (c Config) BuildWithResolver(resolve PackageUIDResolver) (BuildResult, error) {
	if err := c.Validate(); err != nil {
		return BuildResult{}, err
	}
	localEnabled := c.Mode == "local" || c.Mode == "hybrid"
	sharedEnabled := c.Mode == "shared" || c.Mode == "hybrid"
	inbound := Inbound{
		Type:          "ebpf",
		Tag:           "ebpf-in",
		Mode:          c.Mode,
		Network:       c.Network,
		UDPTimeout:    c.UDPTimeout,
		TCPSplice:     c.TCPSplice,
		BypassRuleSet: c.BypassRuleSets,
	}
	missing := make([]PackageRef, 0)
	if localEnabled {
		local := LocalRuntime{
			DNSMode:              c.Local.DNSMode,
			CgroupPath:           c.Local.CgroupPath,
			IPv6Mode:             c.Local.IPv6Mode,
			BypassPrivateAddress: c.Local.BypassPrivateAddress,
			IncludeUID:           append([]uint32{}, c.Local.IncludeUID...),
			IncludeUIDRange:      append([]string{}, c.Local.IncludeUIDRange...),
			ExcludeUID:           append([]uint32{}, c.Local.ExcludeUID...),
			ExcludeUIDRange:      append([]string{}, c.Local.ExcludeUIDRange...),
			IncludeAndroidUser:   append([]int{}, c.Local.IncludeAndroidUser...),
			IncludePackage:       append([]string{}, c.Local.IncludePackage...),
			ExcludePackage:       append([]string{}, c.Local.ExcludePackage...),
			StateCapacity:        c.Local.StateCapacity,
		}
		if c.AppProxyEnable {
			if resolve == nil {
				return BuildResult{}, errors.New("eBPF 分应用策略需要包名 UID 解析器")
			}
			switch c.AppProxyMode {
			case "whitelist":
				local.IncludeUID = append(local.IncludeUID, 0)
				resolution, err := resolve(c.ProxyPackages)
				if err != nil {
					return BuildResult{}, err
				}
				local.IncludeUID = append(local.IncludeUID, resolution.UIDs...)
				missing = append(missing, resolution.Missing...)
			case "blacklist":
				resolution, err := resolve(c.BypassPackages)
				if err != nil {
					return BuildResult{}, err
				}
				local.ExcludeUID = append(local.ExcludeUID, resolution.UIDs...)
				missing = append(missing, resolution.Missing...)
			}
		}
		local.IncludeUID = uniqueUint32(local.IncludeUID)
		local.ExcludeUID = uniqueUint32(local.ExcludeUID)
		inbound.Local = &local
	}
	if sharedEnabled {
		inbound.Shared = &SharedRuntime{
			DNSMode:              c.Shared.DNSMode,
			Interface:            c.Shared.Interfaces,
			IPv6Mode:             c.Shared.IPv6Mode,
			BypassPrivateAddress: c.Shared.BypassPrivateAddress,
			IncludeSourceCIDR:    c.Shared.IncludeSourceCIDR,
			ExcludeSourceCIDR:    c.Shared.ExcludeSourceCIDR,
			IncludeMACAddress:    c.Shared.IncludeMACAddress,
			ExcludeMACAddress:    c.Shared.ExcludeMACAddress,
			StateCapacity:        c.Shared.StateCapacity,
			Advanced: SharedAdvancedRuntime{
				TCPriority:   c.Shared.TCPriority,
				DataPlane:    c.Shared.DataPlane,
				RoutingMark:  c.Shared.RoutingMark,
				RoutingTable: c.Shared.RoutingTable,
			},
		}
	}
	return BuildResult{
		Runtime:         Runtime{Inbounds: []Inbound{inbound}},
		MissingPackages: missing,
	}, nil
}

// Runtime 是 sing-box 运行时配置文档。
type Runtime struct {
	Inbounds []Inbound `json:"inbounds"`
}

// Inbound 是新 eBPF 入站的固定字段模型。
type Inbound struct {
	Type          string
	Tag           string
	Mode          string
	Network       []string
	UDPTimeout    string
	TCPSplice     bool
	BypassRuleSet []string
	Local         *LocalRuntime
	Shared        *SharedRuntime
}

// LocalRuntime 是仅在 local 或 hybrid 模式输出的字段。
type LocalRuntime struct {
	DNSMode              string   `json:"dns_mode,omitempty"`
	CgroupPath           string   `json:"cgroup_path,omitempty"`
	IPv6Mode             string   `json:"ipv6_mode,omitempty"`
	BypassPrivateAddress bool     `json:"bypass_private_address"`
	IncludeUID           []uint32 `json:"include_uid,omitempty"`
	IncludeUIDRange      []string `json:"include_uid_range,omitempty"`
	ExcludeUID           []uint32 `json:"exclude_uid,omitempty"`
	ExcludeUIDRange      []string `json:"exclude_uid_range,omitempty"`
	IncludeAndroidUser   []int    `json:"include_android_user,omitempty"`
	IncludePackage       []string `json:"include_package,omitempty"`
	ExcludePackage       []string `json:"exclude_package,omitempty"`
	StateCapacity        uint32   `json:"state_capacity,omitempty"`
}

// SharedRuntime 是仅在 shared 或 hybrid 模式输出的字段。
type SharedRuntime struct {
	DNSMode              string                `json:"dns_mode,omitempty"`
	Interface            []string              `json:"interface,omitempty"`
	IPv6Mode             string                `json:"ipv6_mode,omitempty"`
	BypassPrivateAddress bool                  `json:"bypass_private_address"`
	IncludeSourceCIDR    []string              `json:"include_source_cidr,omitempty"`
	ExcludeSourceCIDR    []string              `json:"exclude_source_cidr,omitempty"`
	IncludeMACAddress    []string              `json:"include_mac_address,omitempty"`
	ExcludeMACAddress    []string              `json:"exclude_mac_address,omitempty"`
	StateCapacity        uint32                `json:"state_capacity,omitempty"`
	Advanced             SharedAdvancedRuntime `json:"advanced,omitempty"`
}

// SharedAdvancedRuntime 是 shared 数据路径的高级内核参数。
type SharedAdvancedRuntime struct {
	TCPriority   uint16 `json:"tc_priority,omitempty"`
	DataPlane    string `json:"data_plane,omitempty"`
	RoutingMark  uint32 `json:"routing_mark,omitempty"`
	RoutingTable uint32 `json:"routing_table,omitempty"`
}

// MarshalJSON 只输出与当前 mode 对应的数据路径，避免 sing-box 拒绝无效字段。
func (i Inbound) MarshalJSON() ([]byte, error) {
	value := map[string]any{
		"type":            i.Type,
		"tag":             i.Tag,
		"mode":            i.Mode,
		"udp_timeout":     i.UDPTimeout,
		"bypass_rule_set": i.BypassRuleSet,
	}
	if i.TCPSplice {
		value["tcp_splice"] = true
	}
	if len(i.Network) > 0 {
		value["network"] = i.Network
	}
	if i.Local != nil {
		value["local"] = i.Local
	}
	if i.Shared != nil {
		value["shared"] = i.Shared
	}
	return json.Marshal(value)
}

// WriteAtomic 校验并原子写入运行时 eBPF 配置。
func WriteAtomic(path string, config Config) ([]PackageRef, error) {
	built, err := config.Build()
	if err != nil {
		return nil, err
	}
	content, err := json.MarshalIndent(built.Runtime, "", "  ")
	if err != nil {
		return nil, err
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ebpf-")
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return nil, err
	}
	return built.MissingPackages, nil
}

// ParsePackageRef 解析一个严格的 <用户ID>:<包名> 应用引用。
func ParsePackageRef(value string) (PackageRef, error) {
	items, err := ParsePackageRefs(value, "应用引用")
	if err != nil {
		return PackageRef{}, err
	}
	if len(items) != 1 {
		return PackageRef{}, validationError("ebpf.package_invalid", "应用引用", "应用引用不能为空")
	}
	return items[0], nil
}

// ParsePackageRefs 解析逗号分隔的 <用户ID>:<包名> 列表。
func ParsePackageRefs(value, field string) ([]PackageRef, error) {
	items := CommaSeparated(value)
	refs := make([]PackageRef, 0, len(items))
	seen := make(map[PackageRef]struct{}, len(items))
	for _, item := range items {
		user, packageName, found := strings.Cut(item, ":")
		if !found || user == "" || packageName == "" || strings.Contains(packageName, ":") {
			return nil, validationError("ebpf.package_invalid", field, "应用必须使用 <用户ID>:<包名> 格式")
		}
		parsedUser, err := strconv.ParseUint(user, 10, 32)
		if err != nil {
			return nil, validationError("ebpf.android_user_invalid", field, "应用用户 ID 必须是 0 到 4294967295 的整数")
		}
		if err := validatePackageName(packageName); err != nil {
			return nil, validationError("ebpf.package_invalid", field, "应用包名格式无效: "+packageName)
		}
		ref := PackageRef{UserID: uint32(parsedUser), Package: packageName}
		if _, ok := seen[ref]; !ok {
			refs = append(refs, ref)
			seen[ref] = struct{}{}
		}
	}
	return refs, nil
}

func (r PackageRef) String() string {
	return strconv.FormatUint(uint64(r.UserID), 10) + ":" + r.Package
}

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
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false":
		return false, nil
	case "1", "true":
		return true, nil
	default:
		return false, validationError("ebpf.boolean_invalid", key, key+" 必须为 0、1、true 或 false")
	}
}

func parseUint32List(values map[string]string, key string) ([]uint32, error) {
	items := CommaSeparated(values[key])
	result := make([]uint32, 0, len(items))
	for _, item := range items {
		value, err := strconv.ParseUint(item, 10, 32)
		if err != nil {
			return nil, validationError("ebpf.uid_invalid", key, key+" 必须是 0 到 4294967295 的整数")
		}
		result = append(result, uint32(value))
	}
	return uniqueUint32(result), nil
}

func parseIntList(values map[string]string, key string) ([]int, error) {
	items := CommaSeparated(values[key])
	result := make([]int, 0, len(items))
	for _, item := range items {
		value, err := strconv.ParseInt(item, 10, 31)
		if err != nil || value < 0 {
			return nil, validationError("ebpf.android_user_invalid", key, key+" 必须是非负整数")
		}
		result = append(result, int(value))
	}
	return result, nil
}

func parseRanges(values map[string]string, key string) ([]string, error) {
	items := CommaSeparated(values[key])
	for _, item := range items {
		start, end, found := strings.Cut(item, ":")
		if !found || start == "" || end == "" {
			return nil, validationError("ebpf.uid_range_invalid", key, key+" 必须使用 start:end 格式")
		}
		startValue, startErr := strconv.ParseUint(start, 10, 32)
		endValue, endErr := strconv.ParseUint(end, 10, 32)
		if startErr != nil || endErr != nil || startValue > endValue {
			return nil, validationError("ebpf.uid_range_invalid", key, key+" 的范围无效")
		}
	}
	return items, nil
}

func parsePackages(value, field string) ([]string, error) {
	packages := CommaSeparated(value)
	for _, packageName := range packages {
		if err := validatePackageName(packageName); err != nil {
			return nil, validationError("ebpf.package_invalid", field, "应用包名格式无效: "+packageName)
		}
	}
	return packages, nil
}

func validatePackageName(value string) error {
	if value == "" {
		return errors.New("应用包名不能为空")
	}
	for _, char := range value {
		if !(char == '.' || char == '_' || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
			return errors.New("应用包名只能包含字母、数字、点和下划线")
		}
	}
	return nil
}

func parseCIDRs(value, field string) ([]string, error) {
	items := CommaSeparated(value)
	for _, item := range items {
		if _, _, err := net.ParseCIDR(item); err != nil {
			return nil, validationError("ebpf.cidr_invalid", field, field+" 格式无效: "+item)
		}
	}
	return items, nil
}

func parseMACs(value, field string) ([]string, error) {
	items := CommaSeparated(value)
	for _, item := range items {
		parsed, err := net.ParseMAC(item)
		if err != nil || len(parsed) != 6 {
			return nil, validationError("ebpf.mac_invalid", field, field+" 必须是 EUI-48 地址: "+item)
		}
	}
	return items, nil
}

func stateCapacity(values map[string]string, key string) (uint32, error) {
	value := valueOr(values, key, strconv.Itoa(defaultStateCapacity))
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil || parsed > maxStateCapacity {
		return 0, validationError("ebpf.state_capacity_invalid", key, key+" 必须是 0 到 1048576 之间的整数")
	}
	return uint32(parsed), nil
}

func tcpPriority(values map[string]string, key string) (uint16, error) {
	value := valueOr(values, key, strconv.Itoa(defaultTCPriority))
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil || parsed < 1 || parsed > maxTCPriority {
		return 0, validationError("ebpf.tc_priority_invalid", key, key+" 必须是 1 到 65535 之间的整数")
	}
	return uint16(parsed), nil
}

func uint32Value(values map[string]string, key string, fallback uint32) (uint32, error) {
	value := valueOr(values, key, strconv.FormatUint(uint64(fallback), 10))
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, validationError("ebpf.uint32_invalid", key, key+" 必须是 0 到 4294967295 之间的整数")
	}
	return uint32(parsed), nil
}

func validDNSMode(value string) bool {
	return value == "hijack" || value == "respect_policy" || value == "off"
}

func uniqueUint32(values []uint32) []uint32 {
	seen := make(map[uint32]struct{}, len(values))
	result := make([]uint32, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
