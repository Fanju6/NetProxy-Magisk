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
	"sort"
	"strings"
	"time"
)

const (
	defaultNetworkTablesPath = "/data/misc/net/rt_tables"
	networkPollInterval      = 3 * time.Second
	networkDebounceInterval  = 2 * time.Second
	networkCommandTimeout    = 3 * time.Second
	networkEvaluateTimeout   = 8 * time.Second
	networkErrorRepeatEvery  = 100
)

var errNetworkUnavailable = errors.New("Android 网络尚未就绪")

func networkUnavailable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errNetworkUnavailable, fmt.Sprintf(format, args...))
}

type repeatedNetworkError struct {
	key   string
	count int
}

func (state *repeatedNetworkError) record(logger *log.Logger, message string, err error) {
	key := message + "\x00" + err.Error()
	if state.key != key {
		state.key = key
		state.count = 0
	}
	state.count++
	if state.count == 1 || state.count%networkErrorRepeatEvery == 0 {
		logWorker(logger, "ERROR", "network.read", "failed", "%s: %v (连续 %d 次)", message, err, state.count)
	}
}

func (state *repeatedNetworkError) recovered(logger *log.Logger) {
	if state.count > 1 {
		logWorker(logger, "INFO", "network.read", "recovered", "Android 网络状态读取已恢复，期间抑制 %d 条重复错误", state.count-1)
	}
	state.key = ""
	state.count = 0
}

var (
	connectedSSIDPattern  = regexp.MustCompile(`(?i)wifi is connected to\s+(.+?)(?:,\s*bssid:|$)`)
	infoSSIDPattern       = regexp.MustCompile(`(?i)(?:^|[\s,=:])ssid:\s*([^,\r\n]+)`)
	activeDevicePattern   = regexp.MustCompile(`(?m)\bdev\s+(\S+)`)
	interfaceLinePattern  = regexp.MustCompile(`(?m)^\d+:\s+([^:\s@]+)(?:@\S+)?:\s+<([^>]*)>.*?\bstate\s+(\S+)`)
	hotspotStatePattern   = regexp.MustCompile(`(?i)(?:softap|wifi[ _-]*ap|hotspot|tethering).*?(?:state|status)?\s*[:=]?\s*(enabled|disabled|enabling|disabling|started|stopped|up|down)`)
	hotspotNumericPattern = regexp.MustCompile(`(?i)(?:msoftapstate|mwifiapstate|wifiapstate)\s*[:=]\s*(\d+)`)
)

// NetworkState 描述一次 Android 网络采集结果，也是 Worker 的网络变化指纹输入。
type NetworkState struct {
	NetworkType     string
	SSID            string
	DefaultRoute    string
	ActiveInterface string
	InterfaceStatus string
	HotspotState    string
}

// Fingerprint 返回包含网络路径和接口状态的稳定指纹。
func (state NetworkState) Fingerprint() string {
	return strings.Join([]string{
		state.NetworkType,
		state.SSID,
		state.DefaultRoute,
		state.ActiveInterface,
		state.InterfaceStatus,
		state.HotspotState,
	}, "\x00")
}

// NetworkStateReader 读取一次完整网络状态，测试可注入确定性的状态序列。
type NetworkStateReader func(context.Context) (NetworkState, error)

type networkFileState struct {
	exists  bool
	modTime int64
	size    int64
	content string
}

// runNetworkWatcher 轮询完整 Android 网络状态，并在网络状态稳定后评估 Wi-Fi 策略。
func runNetworkWatcher(ctx context.Context, options Options, logger *log.Logger) {
	path := strings.TrimSpace(options.NetworkTablesPath)
	if path == "" {
		path = defaultNetworkTablesPath
	}
	reader := options.NetworkStateReader
	if reader == nil {
		reader = getNetworkState
	}
	pollInterval := options.NetworkPollInterval
	if pollInterval <= 0 {
		pollInterval = networkPollInterval
	}
	debounceInterval := options.NetworkDebounceInterval
	if debounceInterval <= 0 {
		debounceInterval = networkDebounceInterval
	}

	lastFileState := readNetworkFileState(path)
	initialState, err := readNetworkState(ctx, reader)
	var repeatedError repeatedNetworkError
	if err != nil {
		logNetworkReadFailure(logger, &repeatedError, "读取 Android 网络状态失败", err)
	} else {
		evaluateNetworkState(ctx, options, logger, initialState)
	}
	lastEvaluatedState := initialState
	haveEvaluatedState := err == nil

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var debounceTimer *time.Timer
	var debounce <-chan time.Time
	defer func() { stopNetworkTimer(debounceTimer) }()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentFileState := readNetworkFileState(path)
			if currentFileState != lastFileState {
				lastFileState = currentFileState
				stopNetworkTimer(debounceTimer)
				debounceTimer = time.NewTimer(debounceInterval)
				debounce = debounceTimer.C
			}
		case <-debounce:
			debounce = nil
			stableState, readErr := readNetworkState(ctx, reader)
			if readErr != nil {
				stopNetworkTimer(debounceTimer)
				debounceTimer = nil
				logNetworkReadFailure(logger, &repeatedError, "确认 Android 网络状态失败", readErr)
				continue
			}
			repeatedError.recovered(logger)
			if haveEvaluatedState && stableState.Fingerprint() == lastEvaluatedState.Fingerprint() {
				continue
			}
			lastEvaluatedState = stableState
			haveEvaluatedState = true
			evaluateNetworkState(ctx, options, logger, stableState)
		}
	}
}

func logNetworkReadFailure(logger *log.Logger, repeated *repeatedNetworkError, message string, err error) {
	if errors.Is(err, errNetworkUnavailable) {
		logWorker(logger, "INFO", "network.read", "waiting", "网络尚未就绪：等待 Android 默认路由")
		return
	}
	repeated.record(logger, message, err)
}

func readNetworkFileState(path string) networkFileState {
	content, err := os.ReadFile(path)
	if err != nil {
		return networkFileState{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return networkFileState{}
	}
	return networkFileState{
		exists:  true,
		modTime: info.ModTime().UnixNano(),
		size:    info.Size(),
		content: string(content),
	}
}

func readNetworkState(parent context.Context, reader NetworkStateReader) (NetworkState, error) {
	ctx, cancel := context.WithTimeout(parent, networkEvaluateTimeout)
	defer cancel()
	return reader(ctx)
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

func evaluateNetworkState(parent context.Context, options Options, logger *log.Logger, state NetworkState) {
	if options.NetworkEvaluate == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, networkEvaluateTimeout)
	defer cancel()
	if err := options.NetworkEvaluate(ctx, state.NetworkType, state.SSID); err != nil {
		logWorker(logger, "ERROR", "network.policy", "failed", "网络策略评估失败 (network_type=%s): %v", state.NetworkType, err)
	}
}

type networkCommandFunc func(context.Context, string, ...string) (string, error)
type networkFileReader func(string) ([]byte, error)

func getNetworkState(ctx context.Context) (NetworkState, error) {
	return getNetworkStateWith(ctx, androidCommand, os.ReadFile)
}

func getNetworkStateWith(
	ctx context.Context,
	command networkCommandFunc,
	readFile networkFileReader,
) (NetworkState, error) {
	status, statusErr := command(ctx, "cmd", "wifi", "status")
	dumpsys, dumpsysErr := command(ctx, "dumpsys", "wifi")
	if statusErr != nil && dumpsysErr != nil {
		return NetworkState{}, fmt.Errorf("cmd wifi status: %v; dumpsys wifi: %v", statusErr, dumpsysErr)
	}
	parts := make([]string, 0, 2)
	if statusErr == nil {
		parts = append(parts, status)
	}
	if dumpsysErr == nil {
		parts = append(parts, dumpsys)
	}
	combined := strings.Join(parts, "\n")
	networkType, ssid := parseWiFiSnapshot(combined)

	activeRoute, activeInterface, activeErr := getActiveNetworkRouteWith(ctx, command, readFile)
	if activeErr != nil {
		return NetworkState{}, activeErr
	}
	interfaceStatusOutput, err := command(ctx, "ip", "-o", "link", "show")
	if err != nil {
		return NetworkState{}, fmt.Errorf("读取网络接口状态失败: %w", err)
	}
	interfaceStates, err := parseNetworkInterfaceStates(interfaceStatusOutput)
	if err != nil {
		return NetworkState{}, err
	}
	if !networkInterfaceIsUp(interfaceStates, activeInterface) {
		return NetworkState{}, networkUnavailable("活动网络接口 %s 不可用", activeInterface)
	}

	defaultRoute, err := readDefaultRoute(ctx, command, readFile)
	if err != nil {
		if activeRoute == "" {
			return NetworkState{}, err
		}
		// Android policy routing 可能不在 main table 暴露 default 行；route get
		// 已经给出本次真实流量路径时，用它作为有效默认路由指纹。
		defaultRoute = activeRoute
	}
	if activeRoute != "" {
		defaultRoute = activeRoute + "|" + defaultRoute
	}
	if networkType == "wifi" && !isWiFiInterface(activeInterface) {
		// Wi-Fi 框架仍显示 connected，但默认路由已经落到 rmnet/蜂窝接口时，
		// 该次评估必须按实际流量路径处理。
		networkType = "not_wifi"
		ssid = ""
	}

	return NetworkState{
		NetworkType:     networkType,
		SSID:            ssid,
		DefaultRoute:    defaultRoute,
		ActiveInterface: activeInterface,
		InterfaceStatus: canonicalNetworkInterfaceStates(interfaceStates),
		HotspotState:    parseHotspotState(combined),
	}, nil
}

func readDefaultRoute(
	ctx context.Context,
	command networkCommandFunc,
	readFile networkFileReader,
) (string, error) {
	output, routeErr := command(ctx, "ip", "route", "show", "default")
	if routeErr == nil {
		if value := canonicalDefaultRoutes(output); value != "" {
			return value, nil
		}
	}
	content, fileErr := readFile("/proc/net/route")
	if fileErr == nil {
		if value := canonicalProcDefaultRoutes(string(content)); value != "" {
			return value, nil
		}
		if routeErr == nil {
			return "none", nil
		}
	}
	if routeErr != nil {
		if fileErr != nil {
			return "", fmt.Errorf("读取默认路由失败: %v; /proc/net/route: %v", routeErr, fileErr)
		}
		return "", fmt.Errorf("默认路由为空: %w", routeErr)
	}
	return "", errors.New("无法读取默认路由")
}

func canonicalDefaultRoutes(content string) string {
	routes := make([]string, 0)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "default" {
			routes = append(routes, strings.Join(fields, " "))
		}
	}
	sort.Strings(routes)
	return strings.Join(routes, ";")
}

func canonicalProcDefaultRoutes(content string) string {
	routes := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(content))
	// 跳过 /proc/net/route 表头。
	_ = scanner.Scan()
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == "00000000" {
			routes = append(routes, strings.Join(fields, " "))
		}
	}
	sort.Strings(routes)
	return strings.Join(routes, ";")
}

type networkInterfaceState struct {
	Name  string
	Flags string
	State string
}

func parseNetworkInterfaceStates(content string) ([]networkInterfaceState, error) {
	matches := interfaceLinePattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, errors.New("无法解析网络接口状态")
	}
	result := make([]networkInterfaceState, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		result = append(result, networkInterfaceState{
			Name:  strings.TrimSpace(match[1]),
			Flags: strings.ToUpper(strings.TrimSpace(match[2])),
			State: strings.ToLower(strings.TrimSpace(match[3])),
		})
	}
	if len(result) == 0 {
		return nil, errors.New("无法解析网络接口状态")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func canonicalNetworkInterfaceStates(states []networkInterfaceState) string {
	values := make([]string, 0, len(states))
	for _, state := range states {
		values = append(values, state.Name+"="+state.State+"<"+state.Flags+">")
	}
	return strings.Join(values, ";")
}

func networkInterfaceIsUp(states []networkInterfaceState, name string) bool {
	for _, state := range states {
		if state.Name != name {
			continue
		}
		if state.State == "up" || strings.Contains(","+state.Flags+",", ",UP,") || strings.Contains(","+state.Flags+",", ",LOWER_UP,") {
			return true
		}
		return false
	}
	return false
}

func parseHotspotState(output string) string {
	if match := hotspotStatePattern.FindStringSubmatch(output); len(match) > 1 {
		return strings.ToLower(match[1])
	}
	if match := hotspotNumericPattern.FindStringSubmatch(output); len(match) > 1 {
		switch match[1] {
		case "10":
			return "disabling"
		case "11":
			return "disabled"
		case "12":
			return "enabling"
		case "13":
			return "enabled"
		case "14":
			return "failed"
		}
	}
	return "unknown"
}

func getActiveNetworkRouteWith(
	ctx context.Context,
	command networkCommandFunc,
	readFile networkFileReader,
) (string, string, error) {
	if output, err := command(ctx, "ip", "route", "get", "1.1.1.1"); err == nil {
		if iface := parseRouteDevice(output); iface != "" {
			return strings.Join(strings.Fields(output), " "), iface, nil
		}
	}

	content, err := readFile("/proc/net/route")
	if err != nil {
		return "", "", fmt.Errorf("%w: 读取默认路由失败: %w", errNetworkUnavailable, err)
	}
	if iface := parseProcRouteDevice(string(content)); iface != "" {
		return "proc:" + canonicalProcDefaultRoutes(string(content)), iface, nil
	}
	return "", "", networkUnavailable("无法确定活跃网络接口")
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
	// 跳过接口、目标地址和网关等字段的表头。
	_ = scanner.Scan()
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
