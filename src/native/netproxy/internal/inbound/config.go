package inbound

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/netip"
	"os"
	"slices"
	"strings"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

const Tag = "netproxy-in"

type Config struct {
	Backend    string         `json:"backend"`
	RootPolicy string         `json:"root_policy"`
	App        AppPolicy      `json:"app"`
	EBPF       jsontext.Value `json:"ebpf"`
	TUN        jsontext.Value `json:"tun"`
}

type AppPolicy struct {
	Enabled    bool     `json:"enabled"`
	Mode       string   `json:"mode"`
	ProxyApps  []string `json:"proxy_apps"`
	BypassApps []string `json:"bypass_apps"`
}

type Diagnostic struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type ValidationError struct {
	Diagnostics []Diagnostic
}

func (e *ValidationError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "入站配置无效"
	}
	return e.Diagnostics[0].Message
}

func validationError(code, field, message string) error {
	return &ValidationError{Diagnostics: []Diagnostic{{Level: "error", Code: code, Field: field, Message: message}}}
}

func Load(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(content)
}

func Parse(content []byte) (Config, error) {
	config, err := parseOuter(content)
	if err != nil {
		return Config{}, err
	}
	if err := config.validateSection(config.Backend); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate 不依赖当前 backend，保存未选分区时仍校验该分区；空 section 校验两者。
func Validate(content []byte, section string) error {
	config, err := parseOuter(content)
	if err != nil {
		return err
	}
	if section != "" && section != "ebpf" && section != "tun" {
		return validationError("inbound.section_invalid", "", "入站校验分区只能是 ebpf 或 tun")
	}
	if section != "" {
		return config.validateSection(section)
	}
	if err := config.validateSection("ebpf"); err != nil {
		return err
	}
	return config.validateSection("tun")
}

func parseOuter(content []byte) (Config, error) {
	fields, err := objectFields(content, "inbound")
	if err != nil {
		return Config{}, err
	}
	for _, field := range []string{"backend", "root_policy", "app", "ebpf", "tun"} {
		if _, exists := fields[field]; !exists {
			return Config{}, validationError("inbound.field_required", field, "入站配置缺少必需字段: "+field)
		}
	}
	app, err := objectFields(fields["app"], "app")
	if err != nil {
		return Config{}, err
	}
	for _, field := range []string{"enabled", "mode", "proxy_apps", "bypass_apps"} {
		value, exists := app[field]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Config{}, validationError("inbound.field_required", "app."+field, "应用策略缺少必需字段: "+field)
		}
	}
	var config Config
	if err := json.Unmarshal(content, &config, json.RejectUnknownMembers(true)); err != nil {
		return Config{}, validationError("inbound.config_invalid", "", "入站 JSON 配置无效: "+err.Error())
	}
	if err := config.validateOuter(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func objectFields(content []byte, field string) (map[string]jsontext.Value, error) {
	if len(bytes.TrimSpace(content)) == 0 || bytes.TrimSpace(content)[0] != '{' {
		return nil, validationError("inbound.object_required", field, field+" 必须是 JSON 对象")
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(content, &fields); err != nil {
		return nil, validationError("inbound.config_invalid", field, "入站 JSON 配置无效: "+err.Error())
	}
	return fields, nil
}

func (c Config) validateOuter() error {
	if c.Backend != "ebpf" && c.Backend != "tun" {
		return validationError("inbound.backend_invalid", "backend", "入站后端只能是 ebpf 或 tun")
	}
	if c.RootPolicy != "default" && c.RootPolicy != "include" && c.RootPolicy != "exclude" {
		return validationError("inbound.root_policy_invalid", "root_policy", "Root 进程策略只能是 default、include 或 exclude")
	}
	for _, section := range []struct {
		name string
		raw  jsontext.Value
	}{{"ebpf", c.EBPF}, {"tun", c.TUN}} {
		fields, err := objectFields(section.raw, section.name)
		if err != nil {
			return err
		}
		var kind, tag string
		if err := json.Unmarshal(fields["type"], &kind); err != nil {
			return validationError("inbound.type_invalid", section.name+".type", "入站 type 必须匹配分区: "+section.name)
		}
		if err := json.Unmarshal(fields["tag"], &tag); err != nil {
			return validationError("inbound.tag_invalid", section.name+".tag", "受管入站 tag 必须为 "+Tag)
		}
		if err := validateIdentity(kind, tag, section.name); err != nil {
			return err
		}
	}
	if c.App.Mode != "blacklist" && c.App.Mode != "whitelist" {
		return validationError("inbound.app_mode_invalid", "app.mode", "分应用代理模式只能是 blacklist 或 whitelist")
	}
	for _, values := range [][]string{c.App.ProxyApps, c.App.BypassApps} {
		for _, value := range values {
			if _, err := ParsePackageRef(value); err != nil {
				return err
			}
		}
	}
	return nil
}

type ebpfInbound struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
	option.EBPFInboundOptions
}

type tunInbound struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
	option.TunInboundOptions
}

func (c Config) EBPFOptions() (option.EBPFInboundOptions, error) {
	fields, err := objectFields(c.EBPF, "ebpf")
	if err != nil {
		return option.EBPFInboundOptions{}, err
	}
	for _, field := range []string{"local", "shared"} {
		if raw, exists := fields[field]; exists {
			if _, err := objectFields(raw, "ebpf."+field); err != nil {
				return option.EBPFInboundOptions{}, err
			}
		}
	}
	var native ebpfInbound
	if err := unmarshalNative(c.EBPF, &native); err != nil {
		return option.EBPFInboundOptions{}, validationError("inbound.native_invalid", "ebpf", "eBPF 原生配置无效: "+err.Error())
	}
	if err := validateIdentity(native.Type, native.Tag, "ebpf"); err != nil {
		return option.EBPFInboundOptions{}, err
	}
	if err := validateEBPF(native.EBPFInboundOptions); err != nil {
		return option.EBPFInboundOptions{}, err
	}
	return native.EBPFInboundOptions, nil
}

func (c Config) TUNOptions() (option.TunInboundOptions, error) {
	fields, err := objectFields(c.TUN, "tun")
	if err != nil {
		return option.TunInboundOptions{}, err
	}
	// 上游结构仍留有已移除字段，不能因具体 options 能解码就重新接受旧别名。
	for _, field := range []string{"stack", "gso", "inet4_address", "inet6_address", "inet4_route_address", "inet6_route_address", "inet4_route_exclude_address", "inet6_route_exclude_address", "endpoint_independent_nat", "sniff", "sniff_override_destination", "sniff_timeout", "domain_strategy", "udp_disable_domain_unmapping"} {
		if _, exists := fields[field]; exists {
			return option.TunInboundOptions{}, validationError("tun.legacy_field", "tun."+field, "TUN 不支持已移除的原生字段: "+field)
		}
	}
	var native tunInbound
	if err := unmarshalNative(c.TUN, &native); err != nil {
		return option.TunInboundOptions{}, validationError("inbound.native_invalid", "tun", "TUN 原生配置无效: "+err.Error())
	}
	if err := validateIdentity(native.Type, native.Tag, "tun"); err != nil {
		return option.TunInboundOptions{}, err
	}
	if err := validateTUN(native.TunInboundOptions); err != nil {
		return option.TunInboundOptions{}, err
	}
	return native.TunInboundOptions, nil
}

func validateIdentity(kind, tag, section string) error {
	if kind != section {
		return validationError("inbound.type_invalid", section+".type", "入站 type 必须匹配分区: "+section)
	}
	if tag != Tag {
		return validationError("inbound.tag_invalid", section+".tag", "受管入站 tag 必须为 "+Tag)
	}
	return nil
}

func (c Config) validateSection(section string) error {
	switch section {
	case "ebpf":
		native, err := c.EBPFOptions()
		if err != nil {
			return err
		}
		local, _ := native.EffectiveEnablement()
		if local {
			if err := c.validateRootUsers(native.Local.IncludeAndroidUser); err != nil {
				return err
			}
		}
		if local && c.App.Enabled {
			return validateAppFilters(native.Local.IncludeAndroidUser, native.Local.IncludePackage, native.Local.ExcludePackage)
		}
	case "tun":
		native, err := c.TUNOptions()
		if err != nil {
			return err
		}
		if err := c.validateRootUsers(native.IncludeAndroidUser); err != nil {
			return err
		}
		if c.App.Enabled {
			return validateAppFilters(native.IncludeAndroidUser, native.IncludePackage, native.ExcludePackage)
		}
	}
	return nil
}

func (c Config) validateRootUsers(users []int) error {
	if c.RootPolicy == "include" && len(users) > 0 && !slices.Contains(users, 0) {
		return validationError("inbound.root_user_conflict", "root_policy", "接管 Root 进程需在原生 include_android_user 中包含 Android 用户 0")
	}
	return nil
}

func unmarshalNative(content []byte, value any) error {
	return json.Unmarshal(content, value, json.RejectUnknownMembers(true), json.WithUnmarshalers(nativeUnmarshalers))
}

func marshalNative(value any) ([]byte, error) {
	// 上游 omitempty 按 Go 零值省略自定义标量；v2 默认按编码后的 JSON 判断，会凭空输出零值 mark 等字段。
	return json.Marshal(value, json.Deterministic(true), json.OmitZeroStructFields(true), json.WithMarshalers(nativeMarshalers))
}

// Listable 的上下文方法不属于标准 JSON v2 接口；适配时仍由 v2 严格解码每个元素。
var nativeUnmarshalers *json.Unmarshalers

func init() {
	nativeUnmarshalers = json.JoinUnmarshalers(
		json.UnmarshalFunc(func(content []byte, value *option.NetworkList) error {
			var networks badoption.Listable[string]
			if err := unmarshalNative(content, &networks); err != nil {
				return err
			}
			for _, network := range networks {
				if network != "tcp" && network != "udp" {
					return validationError("ebpf.network_invalid", "ebpf.network", "代理协议只能是 tcp 或 udp")
				}
			}
			slices.Sort(networks)
			*value = option.NetworkList(strings.Join(slices.Compact(networks), "\n"))
			return nil
		}),
		listUnmarshaler[string](), listUnmarshaler[uint32](), listUnmarshaler[uint16](),
		listUnmarshaler[int](), listUnmarshaler[netip.Prefix](), listUnmarshaler[netip.Addr](),
	)
}

var nativeMarshalers = json.JoinMarshalers(
	json.MarshalFunc(func(value option.NetworkList) ([]byte, error) {
		return json.Marshal(value.Build(), json.Deterministic(true))
	}),
	listMarshaler[string](), listMarshaler[uint32](), listMarshaler[uint16](),
	listMarshaler[int](), listMarshaler[netip.Prefix](), listMarshaler[netip.Addr](),
)

func listUnmarshaler[T any]() *json.Unmarshalers {
	return json.UnmarshalFunc(func(content []byte, value *badoption.Listable[T]) error {
		if len(bytes.TrimSpace(content)) > 0 && bytes.TrimSpace(content)[0] == '[' {
			return json.Unmarshal(content, (*[]T)(value), json.RejectUnknownMembers(true), json.WithUnmarshalers(nativeUnmarshalers))
		}
		if bytes.Equal(bytes.TrimSpace(content), []byte("null")) {
			*value = nil
			return nil
		}
		var item T
		if err := json.Unmarshal(content, &item, json.RejectUnknownMembers(true), json.WithUnmarshalers(nativeUnmarshalers)); err != nil {
			return err
		}
		*value = []T{item}
		return nil
	})
}

func listMarshaler[T any]() *json.Marshalers {
	return json.MarshalFunc(func(value badoption.Listable[T]) ([]byte, error) {
		return json.Marshal([]T(value), json.Deterministic(true))
	})
}
