package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRepeatedNetworkErrorSuppressesDuplicatesAndReportsRecovery(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	var state repeatedNetworkError
	err := errors.New("network unavailable")
	for index := 0; index < networkErrorRepeatEvery+1; index++ {
		state.record(logger, "读取 Android 网络状态失败", err)
	}
	state.recovered(logger)
	content := output.String()
	if count := strings.Count(content, "读取 Android 网络状态失败"); count != 2 {
		t.Fatalf("重复网络错误未按首条和周期聚合: %d\n%s", count, content)
	}
	if !strings.Contains(content, "连续 100 次") || !strings.Contains(content, "抑制 100 条重复错误") {
		t.Fatalf("聚合日志缺少重复次数或恢复摘要: %s", content)
	}
}

func TestNetworkUnavailableIsReportedAsWaiting(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	var repeated repeatedNetworkError

	logNetworkReadFailure(logger, &repeated, "network read failed", networkUnavailable("no default route"))

	content := output.String()
	if strings.Contains(content, "[ERROR]") {
		t.Fatalf("网络未就绪不应记录为 ERROR: %s", content)
	}
	if !strings.Contains(content, "[INFO] [worker] [network.read] [waiting]") {
		t.Fatalf("网络未就绪应记录 waiting 日志: %s", content)
	}
}

func TestParseWiFiSnapshot(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		network string
		ssid    string
	}{
		{
			name:    "cmd wifi status",
			input:   `Wifi is connected to "Home WiFi", BSSID: 00:11:22:33:44:55`,
			network: "wifi",
			ssid:    "Home WiFi",
		},
		{
			name:    "dumpsys wifi",
			input:   "mWifiInfo SSID: Office, BSSID: 00:11:22:33:44:55\ndetailed state: CONNECTED",
			network: "wifi",
			ssid:    "Office",
		},
		{
			name:    "disabled",
			input:   "Wifi is disabled",
			network: "not_wifi",
		},
		{
			name:    "not connected",
			input:   "Wifi is enabled\nstate: DISCONNECTED",
			network: "not_wifi",
		},
		{
			name:    "unknown ssid",
			input:   `Wifi is connected to "<unknown ssid>", BSSID: 00:11:22:33:44:55`,
			network: "wifi",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			network, ssid := parseWiFiSnapshot(test.input)
			if network != test.network || ssid != test.ssid {
				t.Fatalf("parseWiFiSnapshot() = (%q, %q), want (%q, %q)", network, ssid, test.network, test.ssid)
			}
		})
	}
}

func TestWiFiSnapshotUsesActiveInterface(t *testing.T) {
	tests := []struct {
		name            string
		activeInterface string
		wantNetwork     string
		wantSSID        string
	}{
		{
			name:            "wifi carries the default route",
			activeInterface: "wlan0",
			wantNetwork:     "wifi",
			wantSSID:        "Home WiFi",
		},
		{
			name:            "mobile data carries the default route",
			activeInterface: "rmnet0",
			wantNetwork:     "not_wifi",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, err := getNetworkStateWith(
				context.Background(),
				func(_ context.Context, name string, args ...string) (string, error) {
					switch name {
					case "cmd":
						return `Wifi is connected to "Home WiFi", BSSID: 00:11:22:33:44:55`, nil
					case "dumpsys":
						return "", nil
					default:
						return "", errors.New("unexpected command")
					}
				},
				func(context.Context) (string, error) { return test.activeInterface, nil },
			)
			if err != nil {
				t.Fatal(err)
			}
			gotNetwork, gotSSID := state.NetworkType, state.SSID
			if gotNetwork != test.wantNetwork || gotSSID != test.wantSSID {
				t.Fatalf("snapshot = (%q, %q), want (%q, %q)", gotNetwork, gotSSID, test.wantNetwork, test.wantSSID)
			}
		})
	}
}

func TestIsWiFiInterface(t *testing.T) {
	for _, test := range []struct {
		iface string
		want  bool
	}{
		{iface: "wlan0", want: true},
		{iface: "AP0", want: true},
		{iface: "wifi0", want: true},
		{iface: "rmnet_data0", want: false},
		{iface: "eth0", want: false},
	} {
		if got := isWiFiInterface(test.iface); got != test.want {
			t.Errorf("isWiFiInterface(%q) = %v, want %v", test.iface, got, test.want)
		}
	}
}

func TestNetworkStateFingerprintIncludesPolicyInputs(t *testing.T) {
	base := NetworkState{
		NetworkType:     "wifi",
		SSID:            "Home WiFi",
		ActiveInterface: "wlan0",
	}
	for name, changed := range map[string]NetworkState{
		"ssid":             func() NetworkState { value := base; value.SSID = "Office"; return value }(),
		"active interface": func() NetworkState { value := base; value.ActiveInterface = "rmnet0"; return value }(),
	} {
		if base.Fingerprint() == changed.Fingerprint() {
			t.Fatalf("%s did not change the network fingerprint", name)
		}
	}
}

func TestNetworkStateUsesActiveRouteForDualConnections(t *testing.T) {
	state, err := getNetworkStateWith(
		context.Background(),
		func(_ context.Context, name string, args ...string) (string, error) {
			switch name {
			case "cmd":
				return `Wifi is connected to "Home WiFi", BSSID: 00:11:22:33:44:55`, nil
			case "dumpsys":
				return "", nil
			}
			return "", errors.New("unexpected command")
		},
		func(context.Context) (string, error) { return "rmnet_data0", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if state.NetworkType != "not_wifi" || state.SSID != "" {
		t.Fatalf("双连接时不应按 Wi-Fi 评估: %+v", state)
	}
	if state.ActiveInterface != "rmnet_data0" {
		t.Fatalf("活动网络接口不正确: %+v", state)
	}
}

func TestNetworkStateReadFailureReturnsError(t *testing.T) {
	_, err := getNetworkStateWith(
		context.Background(),
		func(_ context.Context, name string, args ...string) (string, error) {
			if name == "cmd" {
				return `Wifi is connected to "Home WiFi"`, nil
			}
			if name == "dumpsys" {
				return "mSoftApState=11", nil
			}
			return "", errors.New("network read failed")
		},
		func(context.Context) (string, error) { return "", networkUnavailable("没有默认路由") },
	)
	if err == nil {
		t.Fatal("网络状态读取失败时不应生成快照")
	}
}

func TestNetworkWatcherDebouncesStateChanges(t *testing.T) {
	initial := NetworkState{NetworkType: "wifi", SSID: "A", ActiveInterface: "wlan0"}
	final := initial
	final.SSID = "C"

	var mu sync.Mutex
	readCount := 0
	read := func(context.Context) (NetworkState, error) {
		mu.Lock()
		defer mu.Unlock()
		readCount++
		switch readCount {
		case 1:
			return initial, nil
		default:
			return final, nil
		}
	}
	evaluated := make(chan string, 4)
	events := make(chan struct{}, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNetworkWatcher(ctx, Options{
			NetworkStateReader:      read,
			NetworkEventSource:      channelNetworkEventSource(events),
			NetworkDebounceInterval: 20 * time.Millisecond,
			NetworkEvaluate: func(_ context.Context, _, ssid string) error {
				evaluated <- ssid
				return nil
			},
		}, log.New(io.Discard, "", 0))
		close(done)
	}()

	select {
	case got := <-evaluated:
		if got != "A" {
			t.Fatalf("初始评估 SSID=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("初始网络评估未执行")
	}
	mu.Lock()
	gotReads := readCount
	mu.Unlock()
	if gotReads != 1 {
		t.Fatalf("没有网络事件时不应重复读取网络状态，count=%d", gotReads)
	}
	events <- struct{}{}
	events <- struct{}{}
	events <- struct{}{}
	select {
	case got := <-evaluated:
		if got != "C" {
			t.Fatalf("debounce 后评估 SSID=%q, 中间状态不应提交", got)
		}
	case <-time.After(time.Second):
		t.Fatal("debounce 后评估未执行")
	}
	select {
	case got := <-evaluated:
		t.Fatalf("重复评估了稳定状态 %q", got)
	case <-time.After(30 * time.Millisecond):
	}
	mu.Lock()
	gotReads = readCount
	mu.Unlock()
	if gotReads != 2 {
		t.Fatalf("连续网络事件应合并为一次状态读取，count=%d", gotReads)
	}
	cancel()
	<-done
}

func TestNetworkWatcherDoesNotReadStateWithoutEvents(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNetworkWatcher(ctx, Options{
			NetworkEventSource: channelNetworkEventSource(nil),
			NetworkStateReader: func(context.Context) (NetworkState, error) {
				mu.Lock()
				defer mu.Unlock()
				reads++
				return NetworkState{NetworkType: "not_wifi"}, nil
			},
			NetworkEvaluate: func(context.Context, string, string) error { return nil },
		}, log.New(io.Discard, "", 0))
		close(done)
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if reads != 1 {
		t.Fatalf("没有网络事件时不应重复读取网络状态，count=%d", reads)
	}
}

func TestNetworkWatcherCancelsStaleEvaluation(t *testing.T) {
	states := []NetworkState{
		{NetworkType: "wifi", SSID: "A"},
		{NetworkType: "wifi", SSID: "B"},
	}
	var mu sync.Mutex
	reads := 0
	firstStarted := make(chan struct{})
	latestEvaluated := make(chan string, 1)
	events := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNetworkWatcher(ctx, Options{
			NetworkEventSource: channelNetworkEventSource(events),
			NetworkStateReader: func(context.Context) (NetworkState, error) {
				mu.Lock()
				defer mu.Unlock()
				state := states[min(reads, len(states)-1)]
				reads++
				return state, nil
			},
			NetworkEvaluate: func(ctx context.Context, _, ssid string) error {
				if ssid == "A" {
					close(firstStarted)
					<-ctx.Done()
					return ctx.Err()
				}
				latestEvaluated <- ssid
				return nil
			},
			NetworkDebounceInterval: 5 * time.Millisecond,
		}, log.New(io.Discard, "", 0))
		close(done)
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("首个网络策略评估未启动")
	}
	events <- struct{}{}
	select {
	case got := <-latestEvaluated:
		if got != "B" {
			t.Fatalf("过期评估取消后应用了 %q, want B", got)
		}
	case <-time.After(time.Second):
		t.Fatal("新网络状态未在取消旧评估后应用")
	}
	cancel()
	<-done
}

func TestNetworkWatcherSkipsEvaluationWhenStateReadFails(t *testing.T) {
	evaluated := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNetworkWatcher(ctx, Options{
			NetworkEventSource: channelNetworkEventSource(nil),
			NetworkStateReader: func(context.Context) (NetworkState, error) {
				return NetworkState{}, errors.New("unavailable")
			},
			NetworkEvaluate: func(context.Context, string, string) error {
				evaluated <- struct{}{}
				return nil
			},
		}, log.New(io.Discard, "", 0))
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
	select {
	case <-evaluated:
		t.Fatal("网络状态读取失败时不应执行策略评估")
	default:
	}
}

func channelNetworkEventSource(events <-chan struct{}) NetworkEventSource {
	return func(ctx context.Context, notify func()) error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case _, open := <-events:
				if !open {
					<-ctx.Done()
					return nil
				}
				notify()
			}
		}
	}
}
