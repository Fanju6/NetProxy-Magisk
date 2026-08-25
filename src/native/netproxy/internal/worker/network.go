package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	networkDebounceInterval   = time.Second
	networkEventRetryInterval = 30 * time.Second
	networkCommandTimeout     = 3 * time.Second
	networkEvaluateTimeout    = 8 * time.Second
	networkErrorRepeatEvery   = 100
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
	connectedSSIDPattern = regexp.MustCompile(`(?i)wifi is connected to\s+(.+?)(?:,\s*bssid:|$)`)
	infoSSIDPattern      = regexp.MustCompile(`(?i)(?:^|[\s,=:])ssid:\s*([^,\r\n]+)`)
)

// NetworkState 描述一次 Android 网络采集结果，也是 Worker 的网络变化指纹输入。
type NetworkState struct {
	NetworkType     string
	SSID            string
	ActiveInterface string
}

// Fingerprint 返回会影响 Wi-Fi 策略选择的稳定指纹。
func (state NetworkState) Fingerprint() string {
	return strings.Join([]string{
		state.NetworkType,
		state.SSID,
		state.ActiveInterface,
	}, "\x00")
}

// NetworkStateReader 读取一次完整网络状态，测试可注入确定性的状态序列。
type NetworkStateReader func(context.Context) (NetworkState, error)

// NetworkEventSource 阻塞监听网络变化，并通过 notify 合并通知控制器。
type NetworkEventSource func(context.Context, func()) error

type networkEvaluationResult struct {
	state       NetworkState
	readErr     error
	evaluateErr error
	skipped     bool
}

// runNetworkWatcher 监听 Android 网络事件，并在网络状态稳定后评估 Wi-Fi 策略。
func runNetworkWatcher(ctx context.Context, options Options, logger *log.Logger) {
	reader := options.NetworkStateReader
	if reader == nil {
		reader = getNetworkState
	}
	eventSource := options.NetworkEventSource
	if eventSource == nil {
		eventSource = defaultNetworkEventSource
	}
	debounceInterval := options.NetworkDebounceInterval
	if debounceInterval <= 0 {
		debounceInterval = networkDebounceInterval
	}
	events := make(chan struct{}, 1)
	notify := func() {
		select {
		case events <- struct{}{}:
		default:
		}
	}
	sourceContext, cancelSource := context.WithCancel(ctx)
	var sourceWait sync.WaitGroup
	sourceWait.Go(func() {
		runNetworkEventSource(sourceContext, eventSource, notify, logger)
	})

	results := make(chan networkEvaluationResult, 1)
	var evaluationWait sync.WaitGroup
	var evaluationCancel context.CancelFunc
	running := false
	pending := false
	var lastEvaluatedState NetworkState
	haveEvaluatedState := false
	var repeatedError repeatedNetworkError
	var debounceTimer *time.Timer
	var debounce <-chan time.Time

	startEvaluation := func() {
		if running {
			pending = true
			return
		}
		pending = false
		running = true
		evaluationContext, cancel := context.WithCancel(ctx)
		evaluationCancel = cancel
		previousState := lastEvaluatedState
		havePreviousState := haveEvaluatedState
		evaluationWait.Go(func() {
			results <- readAndEvaluateNetworkState(evaluationContext, options, reader, previousState, havePreviousState)
		})
	}

	scheduleEvaluation := func(delay time.Duration) {
		pending = false
		if evaluationCancel != nil {
			evaluationCancel()
		}
		stopNetworkTimer(debounceTimer)
		debounceTimer = time.NewTimer(delay)
		debounce = debounceTimer.C
	}

	defer func() {
		stopNetworkTimer(debounceTimer)
		if evaluationCancel != nil {
			evaluationCancel()
		}
		cancelSource()
		evaluationWait.Wait()
		sourceWait.Wait()
	}()

	// 首次状态不等待事件，确保开机已有网络时立即应用策略。
	startEvaluation()

	for {
		select {
		case <-ctx.Done():
			return
		case <-events:
			scheduleEvaluation(debounceInterval)
		case <-debounce:
			debounce = nil
			debounceTimer = nil
			startEvaluation()
		case result := <-results:
			running = false
			evaluationCancel = nil
			if errors.Is(result.readErr, context.Canceled) || errors.Is(result.evaluateErr, context.Canceled) {
				if pending {
					startEvaluation()
				}
				continue
			}
			if result.readErr != nil {
				logNetworkReadFailure(logger, &repeatedError, "确认 Android 网络状态失败", result.readErr)
			} else {
				repeatedError.recovered(logger)
			}
			if result.evaluateErr != nil {
				logWorker(logger, "ERROR", "network.policy", "failed", "网络策略评估失败 (network_type=%s): %v", result.state.NetworkType, result.evaluateErr)
			} else if result.readErr == nil && !result.skipped {
				lastEvaluatedState = result.state
				haveEvaluatedState = true
			}
			if pending {
				startEvaluation()
			}
		}
	}
}

func runNetworkEventSource(ctx context.Context, source NetworkEventSource, notify func(), logger *log.Logger) {
	for {
		err := source(ctx, notify)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logWorker(logger, "ERROR", "network.watch", "failed", "监听 Android 网络事件失败: %v", err)
		} else {
			logWorker(logger, "ERROR", "network.watch", "failed", "Android 网络事件监听意外结束")
		}
		timer := time.NewTimer(networkEventRetryInterval)
		select {
		case <-ctx.Done():
			stopNetworkTimer(timer)
			return
		case <-timer.C:
		}
	}
}

func readAndEvaluateNetworkState(
	ctx context.Context,
	options Options,
	reader NetworkStateReader,
	previous NetworkState,
	havePrevious bool,
) networkEvaluationResult {
	state, err := readNetworkState(ctx, reader)
	if err != nil {
		return networkEvaluationResult{readErr: err}
	}
	if havePrevious && state.Fingerprint() == previous.Fingerprint() {
		return networkEvaluationResult{state: state, skipped: true}
	}
	return networkEvaluationResult{
		state:       state,
		evaluateErr: evaluateNetworkState(ctx, options, state),
	}
}

func logNetworkReadFailure(logger *log.Logger, repeated *repeatedNetworkError, message string, err error) {
	if errors.Is(err, errNetworkUnavailable) {
		logWorker(logger, "INFO", "network.read", "waiting", "网络尚未就绪：等待 Android 默认路由")
		return
	}
	repeated.record(logger, message, err)
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

func evaluateNetworkState(parent context.Context, options Options, state NetworkState) error {
	if options.NetworkEvaluate == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, networkEvaluateTimeout)
	defer cancel()
	return options.NetworkEvaluate(ctx, state.NetworkType, state.SSID)
}

type networkCommandFunc func(context.Context, string, ...string) (string, error)
type activeNetworkReader func(context.Context) (string, error)

func getNetworkState(ctx context.Context) (NetworkState, error) {
	return getNetworkStateWith(ctx, androidCommand, readActiveNetworkInterface)
}

func getNetworkStateWith(
	ctx context.Context,
	command networkCommandFunc,
	readActiveInterface activeNetworkReader,
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

	activeInterface, err := readActiveInterface(ctx)
	if err != nil {
		return NetworkState{}, err
	}
	if networkType == "wifi" && !isWiFiInterface(activeInterface) {
		// Android 仍可能报告 Wi-Fi 已连接，但 policy routing 已将真实出口切到蜂窝网络。
		networkType = "not_wifi"
		ssid = ""
	}

	return NetworkState{
		NetworkType:     networkType,
		SSID:            ssid,
		ActiveInterface: activeInterface,
	}, nil
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
