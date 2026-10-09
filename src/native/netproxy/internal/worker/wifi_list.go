package worker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

var savedWiFiSecurity = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_/^-]*$`)

// SavedWiFiNetworks 只查询系统保存的名称，不读取密码或主动扫描网络。
func SavedWiFiNetworks(ctx context.Context) ([]string, error) {
	output, err := androidCommand(ctx, "cmd", "wifi", "list-networks")
	if err != nil {
		return nil, fmt.Errorf("读取已保存 Wi-Fi 失败: %w", err)
	}
	return parseSavedWiFiNetworks(output)
}

func parseSavedWiFiNetworks(output string) ([]string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	ssids := make([]string, 0)
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "No networks" {
		return ssids, nil
	}
	if !strings.HasPrefix(lines[0], "Network Id") || !strings.Contains(lines[0], "SSID") || !strings.HasSuffix(strings.TrimSpace(lines[0]), "Security type") {
		return nil, errors.New("系统未返回有效的已保存 Wi-Fi 列表")
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) <= 13 {
			return nil, errors.New("系统返回的 Wi-Fi 名称列表格式无效")
		}
		_, idErr := strconv.ParseUint(strings.TrimSpace(line[:12]), 10, 32)
		fields := strings.TrimRight(line[13:], " \r")
		separator := strings.LastIndexByte(fields, ' ')
		if idErr != nil || line[12] != ' ' || separator < 0 || !savedWiFiSecurity.MatchString(fields[separator+1:]) {
			return nil, errors.New("系统返回的 Wi-Fi 名称列表格式无效")
		}
		ssid := strings.TrimRight(fields[:separator], " ")
		if err := moduleconfig.ValidateSSID(ssid); err != nil {
			continue
		}
		if !slices.Contains(ssids, ssid) {
			ssids = append(ssids, ssid)
		}
	}
	slices.Sort(ssids)
	return ssids, nil
}
