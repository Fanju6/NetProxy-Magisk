package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/ebpf"
)

func runConfig(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少配置操作: module-get|module-set|ebpf-get|ebpf-set")
	}
	action := args[0]
	flags := newFlagSet("config " + action)
	path := flags.String("path", "", "配置文件路径")
	key := flags.String("key", "", "读取的配置键")
	format := flags.String("format", "json", "输出格式")
	assignments := make([]string, 0)
	flags.Func("set", "设置 KEY=value，可重复使用", func(value string) error {
		assignments = append(assignments, value)
		return nil
	})
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" {
		return errors.New("配置操作需要 --path")
	}

	switch action {
	case "module-get":
		config, err := moduleconfig.LoadModule(*path)
		if err != nil {
			return &resultError{Code: "config.invalid", Message: "module.conf 配置无效", Data: map[string]string{"error": err.Error()}}
		}
		if *key != "" {
			value, err := moduleConfigValue(config, *key)
			if err != nil {
				return err
			}
			switch *format {
			case "text":
				fmt.Fprintln(os.Stdout, value)
			case "tsv":
				fmt.Fprintf(os.Stdout, "%s\t%s\n", *key, value)
			case "json":
				writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.module_read", Message: "模块配置已读取", Data: map[string]string{*key: value}})
			default:
				return fmt.Errorf("不支持的配置输出格式: %s", *format)
			}
			return nil
		}
		if *format == "tsv" {
			writeModuleConfigTSV(config)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("读取完整 module.conf 只支持 json 或 tsv")
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.module_read", Message: "模块配置已读取", Data: config})
		return nil
	case "module-set":
		updates, err := parseAssignments(assignments)
		if err != nil {
			return err
		}
		if err := moduleconfig.UpdateModule(*path, updates); err != nil {
			return &resultError{Code: "config.invalid", Message: "module.conf 修改未通过校验", Data: map[string]string{"error": err.Error()}}
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.module_updated", Message: "模块配置已更新", Data: map[string]string{"path": *path}})
		return nil
	case "ebpf-get":
		config, err := ebpf.Load(*path)
		if err != nil {
			return &resultError{Code: "config.invalid", Message: "ebpf.conf 配置无效", Data: map[string]string{"error": err.Error()}}
		}
		if *key == "" {
			if *format != "tsv" {
				return fmt.Errorf("读取完整 ebpf.conf 只支持 tsv")
			}
			writeEBPFConfigTSV(config)
			return nil
		}
		value, err := ebpfConfigValue(config, *key)
		if err != nil {
			return err
		}
		switch *format {
		case "text":
			fmt.Fprintln(os.Stdout, value)
		case "tsv":
			fmt.Fprintf(os.Stdout, "%s\t%s\n", *key, value)
		case "json":
			writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.ebpf_read", Message: "eBPF 配置已读取", Data: map[string]string{*key: value}})
		default:
			return fmt.Errorf("不支持的配置输出格式: %s", *format)
		}
		return nil
	case "ebpf-set":
		updates, err := parseAssignments(assignments)
		if err != nil {
			return err
		}
		err = moduleconfig.UpdateValidated(*path, updates, func(candidate string) error {
			_, validateErr := ebpf.Load(candidate)
			return validateErr
		})
		if err != nil {
			return &resultError{Code: "config.invalid", Message: "ebpf.conf 修改未通过校验", Data: map[string]string{"error": err.Error()}}
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "config.ebpf_updated", Message: "eBPF 配置已更新", Data: map[string]string{"path": *path}})
		return nil
	default:
		return fmt.Errorf("未知配置操作 %q", action)
	}
}

func parseAssignments(values []string) (map[string]string, error) {
	updates := make(map[string]string, len(values))
	for _, assignment := range values {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("配置修改必须使用 KEY=value: %s", assignment)
		}
		if _, exists := updates[key]; exists {
			return nil, fmt.Errorf("配置键重复: %s", key)
		}
		updates[key] = value
	}
	if len(updates) == 0 {
		return nil, errors.New("配置修改至少需要一个 --set KEY=value")
	}
	return updates, nil
}

func moduleConfigValue(config moduleconfig.ModuleConfig, key string) (string, error) {
	switch key {
	case "AUTO_START":
		return boolString(config.AutoStart), nil
	case "OUTBOUND_MODE":
		return config.OutboundMode, nil
	case "SELECTOR_MODE":
		return config.SelectorMode, nil
	case "ACTIVE_GROUP_ID":
		return config.ActiveGroupID, nil
	case "SELECTED_NODE_REF":
		return config.SelectedNodeRef, nil
	case "WIFI_AUTO_SWITCH":
		return boolString(config.WiFiAutoSwitch), nil
	case "WIFI_SSID_MODE":
		return config.WiFiSSIDMode, nil
	case "WIFI_SSID_LIST":
		return config.WiFiSSIDList, nil
	case "PROXY_ON_CELLULAR":
		return boolString(config.ProxyOnCellular), nil
	default:
		return "", fmt.Errorf("不支持的 module.conf 配置键: %s", key)
	}
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func writeModuleConfigTSV(config moduleconfig.ModuleConfig) {
	keys := []string{"AUTO_START", "OUTBOUND_MODE", "SELECTOR_MODE", "ACTIVE_GROUP_ID", "SELECTED_NODE_REF", "WIFI_AUTO_SWITCH", "WIFI_SSID_MODE", "WIFI_SSID_LIST", "PROXY_ON_CELLULAR"}
	for _, key := range keys {
		value, _ := moduleConfigValue(config, key)
		fmt.Fprintf(os.Stdout, "%s\t%s\n", key, value)
	}
}

func ebpfConfigValue(config ebpf.Config, key string) (string, error) {
	switch key {
	case "EBPF_NETWORK":
		return config.Network, nil
	case "EBPF_UDP_TIMEOUT":
		return config.UDPTimeout, nil
	case "EBPF_DNS_MODE":
		return config.DNSMode, nil
	case "EBPF_CGROUP_ENABLED":
		return boolString(config.CgroupEnabled), nil
	case "EBPF_CGROUP_PATH":
		return config.CgroupPath, nil
	case "EBPF_CGROUP_IPV6_MODE":
		return config.CgroupIPv6Mode, nil
	case "EBPF_BYPASS_PRIVATE_ADDRESS":
		return boolString(config.BypassPrivateAddress), nil
	case "EBPF_BYPASS_RULE_SETS":
		return strings.Join(config.BypassRuleSets, ","), nil
	case "APP_PROXY_ENABLE":
		return boolString(config.AppProxyEnable), nil
	case "APP_PROXY_MODE":
		return config.AppProxyMode, nil
	case "APP_ANDROID_USERS":
		return joinUintValues(config.AndroidUsers), nil
	case "PROXY_APPS_LIST":
		return strings.Join(config.ProxyPackages, ","), nil
	case "BYPASS_APPS_LIST":
		return strings.Join(config.BypassPackages, ","), nil
	case "EBPF_SHARED_NETWORK":
		return boolString(config.SharedNetwork), nil
	case "EBPF_SHARED_INTERFACES":
		return strings.Join(config.SharedInterfaces, ","), nil
	case "EBPF_SHARED_INCLUDE_SOURCE_CIDRS":
		return strings.Join(config.SharedIncludeSourceCIDRs, ","), nil
	case "EBPF_SHARED_EXCLUDE_SOURCE_CIDRS":
		return strings.Join(config.SharedExcludeSourceCIDRs, ","), nil
	case "EBPF_SHARED_INCLUDE_MAC_ADDRESSES":
		return strings.Join(config.SharedIncludeMACAddresses, ","), nil
	case "EBPF_SHARED_EXCLUDE_MAC_ADDRESSES":
		return strings.Join(config.SharedExcludeMACAddresses, ","), nil
	case "EBPF_TCP_MAP_CAPACITY":
		return strconv.FormatUint(config.TCPMapCapacity, 10), nil
	case "EBPF_UDP_MAP_CAPACITY":
		return strconv.FormatUint(config.UDPMapCapacity, 10), nil
	case "EBPF_SOCKET_MAP_CAPACITY":
		return strconv.FormatUint(config.SocketMapCapacity, 10), nil
	case "EBPF_SHARED_PROXY_MAP_CAPACITY":
		return strconv.FormatUint(config.SharedProxyMapCapacity, 10), nil
	case "EBPF_SHARED_BYPASS_MAP_CAPACITY":
		return strconv.FormatUint(config.SharedBypassMapCapacity, 10), nil
	case "EBPF_SHARED_FRAGMENT_MAP_CAPACITY":
		return strconv.FormatUint(config.SharedFragmentMapCapacity, 10), nil
	default:
		return "", fmt.Errorf("不支持的 ebpf.conf 配置键: %s", key)
	}
}

func joinUintValues(values []uint64) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, strconv.FormatUint(value, 10))
	}
	return strings.Join(items, ",")
}

func writeEBPFConfigTSV(config ebpf.Config) {
	keys := []string{
		"EBPF_NETWORK", "EBPF_UDP_TIMEOUT", "EBPF_DNS_MODE", "EBPF_CGROUP_ENABLED",
		"EBPF_CGROUP_PATH", "EBPF_CGROUP_IPV6_MODE", "EBPF_BYPASS_PRIVATE_ADDRESS",
		"EBPF_BYPASS_RULE_SETS", "APP_PROXY_ENABLE",
		"APP_PROXY_MODE", "APP_ANDROID_USERS", "PROXY_APPS_LIST", "BYPASS_APPS_LIST",
		"EBPF_SHARED_NETWORK", "EBPF_SHARED_INTERFACES", "EBPF_SHARED_INCLUDE_SOURCE_CIDRS",
		"EBPF_SHARED_EXCLUDE_SOURCE_CIDRS", "EBPF_SHARED_INCLUDE_MAC_ADDRESSES",
		"EBPF_SHARED_EXCLUDE_MAC_ADDRESSES", "EBPF_TCP_MAP_CAPACITY", "EBPF_UDP_MAP_CAPACITY",
		"EBPF_SOCKET_MAP_CAPACITY", "EBPF_SHARED_PROXY_MAP_CAPACITY",
		"EBPF_SHARED_BYPASS_MAP_CAPACITY", "EBPF_SHARED_FRAGMENT_MAP_CAPACITY",
	}
	for _, key := range keys {
		value, _ := ebpfConfigValue(config, key)
		fmt.Fprintf(os.Stdout, "%s\t%s\n", key, value)
	}
}
