package worker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	defaultNetworkTablesPath = "/data/misc/net/rt_tables"
	networkPollInterval      = 3 * time.Second
	networkDebounceInterval  = 2 * time.Second
	networkCommandTimeout    = 3 * time.Second
	networkEvaluateTimeout   = 8 * time.Second
)

var (
	connectedSSIDPattern = regexp.MustCompile(`(?i)wifi is connected to\s+(.+?)(?:,\s*bssid:|$)`)
	infoSSIDPattern      = regexp.MustCompile(`(?i)(?:^|[\s,=:])ssid:\s*([^,\r\n]+)`)
	activeDevicePattern  = regexp.MustCompile(`(?m)\bdev\s+(\S+)`)
)

// runNetworkWatcher 轮询 Android 路由表变化，并在网络状态稳定后评估 Wi-Fi 策略。
func runNetworkWatcher(ctx context.Context, options Options, logger *log.Logger) {
	path := options.NetworkTablesPath
	if strings.TrimSpace(path) == "" {
		path = defaultNetworkTablesPath
	}

	lastState := readNetworkFileState(path)
	evaluateNetwork(ctx, options, logger)

	ticker := time.NewTicker(networkPollInterval)
	defer ticker.Stop()

	var debounceTimer *time.Timer
	var debounce <-chan time.Time
	defer func() { stopNetworkTimer(debounceTimer) }()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentState := readNetworkFileState(path)
			if currentState != lastState {
				lastState = currentState
				stopNetworkTimer(debounceTimer)
				debounceTimer = time.NewTimer(networkDebounceInterval)
				debounce = debounceTimer.C
			}
		case <-debounce:
			debounce = nil
			evaluateNetwork(ctx, options, logger)
		}
	}
}

type networkFileState struct {
	exists  bool
	modTime int64
	size    int64
}

func readNetworkFileState(path string) networkFileState {
	info, err := os.Stat(path)
	if err != nil {
		return networkFileState{}
	}
	return networkFileState{exists: true, modTime: info.ModTime().UnixNano(), size: info.Size()}
}

func stopNetworkTimer(timer *time.Timer) {
	if timer == nil || timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func evaluateNetwork(parent context.Context, options Options, logger *log.Logger) {
	if options.NetworkEvaluate == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, networkEvaluateTimeout)
	defer cancel()

	networkType, ssid, err := getWiFiSnapshot(ctx)
	if err != nil {
		if logger != nil {
			logWorker(logger, "读取 Android 网络状态失败: %v", err)
		}
		return
	}
	if err := options.NetworkEvaluate(ctx, networkType, ssid); err != nil && logger != nil {
		logWorker(logger, "网络策略评估失败: %v", err)
	}
}

func getWiFiSnapshot(ctx context.Context) (string, string, error) {
	return getWiFiSnapshotWith(ctx, androidCommand, os.ReadFile)
}

type networkCommandFunc func(context.Context, string, ...string) (string, error)
type networkFileReader func(string) ([]byte, error)

func getWiFiSnapshotWith(
	ctx context.Context,
	command networkCommandFunc,
	readFile networkFileReader,
) (string, string, error) {
	status, statusErr := command(ctx, "cmd", "wifi", "status")
	if statusErr == nil && containsFold(status, "wifi is disabled") {
		return "not_wifi", "", nil
	}

	combined := status
	if statusErr != nil || !containsConnectedState(status) {
		fallback, fallbackErr := command(ctx, "dumpsys", "wifi")
		if fallbackErr == nil {
			combined = strings.TrimSpace(strings.Join([]string{status, fallback}, "\n"))
		} else if statusErr != nil {
			return "", "", fmt.Errorf("cmd wifi status: %w; dumpsys wifi: %v", statusErr, fallbackErr)
		}
	}

	networkType, ssid := parseWiFiSnapshot(combined)
	if networkType == "wifi" {
		activeInterface, routeErr := getActiveNetworkInterfaceWith(ctx, command, readFile)
		if routeErr == nil && !isWiFiInterface(activeInterface) {
			return "not_wifi", "", nil
		}
	}
	return networkType, ssid, nil
}

func getActiveNetworkInterfaceWith(
	ctx context.Context,
	command networkCommandFunc,
	readFile networkFileReader,
) (string, error) {
	if output, err := command(ctx, "ip", "route", "get", "1.1.1.1"); err == nil {
		if iface := parseRouteDevice(output); iface != "" {
			return iface, nil
		}
	}

	content, err := readFile("/proc/net/route")
	if err != nil {
		return "", fmt.Errorf("读取默认路由失败: %w", err)
	}
	if iface := parseProcRouteDevice(string(content)); iface != "" {
		return iface, nil
	}
	return "", errors.New("无法确定活跃网络接口")
}

func parseRouteDevice(output string) string {
	match := activeDevicePattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func parseProcRouteDevice(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	if scanner.Scan() {
		// 第一行是接口、目标地址和网关等字段的表头。
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == "00000000" {
			return fields[0]
		}
	}
	return ""
}

func isWiFiInterface(iface string) bool {
	lower := strings.ToLower(strings.TrimSpace(iface))
	return strings.HasPrefix(lower, "wlan") ||
		strings.HasPrefix(lower, "ap") ||
		strings.HasPrefix(lower, "wifi")
}

func androidCommand(parent context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, networkCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func parseWiFiSnapshot(output string) (string, string) {
	disabled := containsFold(output, "wifi is disabled")
	connected := containsConnectedState(output)
	ssid := ""

	if match := connectedSSIDPattern.FindStringSubmatch(output); len(match) > 1 {
		ssid = normalizeSSID(match[1])
	}
	if match := infoSSIDPattern.FindStringSubmatch(output); len(match) > 1 {
		if value := normalizeSSID(match[1]); value != "" {
			ssid = value
			connected = true
		}
	}
	if disabled {
		return "not_wifi", ""
	}
	if connected {
		return "wifi", ssid
	}
	return "not_wifi", ""
}

func containsConnectedState(output string) bool {
	return containsFold(output, "wifi is connected to") ||
		containsFold(output, "state: connected") ||
		containsFold(output, "detailed state: connected")
}

func containsFold(value, target string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(target))
}

func normalizeSSID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	switch strings.ToLower(value) {
	case "", "<unknown ssid>", "<none>":
		return ""
	default:
		return value
	}
}
