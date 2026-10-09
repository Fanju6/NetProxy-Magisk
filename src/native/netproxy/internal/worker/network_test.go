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
	for range networkErrorRepeatEvery + 1 {
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

func TestNetworkSnapshotUsesOnlyActualInterface(t *testing.T) {
	for _, iface := range []string{"wlan1", "rmnet_data0", "eth0", "ap0"} {
		state, err := getNetworkStateWith(t.Context(), func(ctx context.Context, active string) (NetworkState, error) {
			if active != iface {
				t.Fatalf("Wi-Fi 查询接口=%s，实际出口=%s", active, iface)
			}
			if active == "wlan1" {
				return NetworkState{NetworkType: "wifi", SSID: " Home,Wi-Fi "}, nil
			}
			return NetworkState{NetworkType: "not_wifi"}, nil
		}, func(context.Context) (string, error) { return iface, nil })
		if err != nil || state.ActiveInterface != iface {
			t.Fatalf("%+v %v", state, err)
		}
		if iface == "wlan1" && state.SSID != " Home,Wi-Fi " {
			t.Fatal("SSID 被修改")
		}
		if iface != "wlan1" && (state.NetworkType != "not_wifi" || state.SSID != "") {
			t.Fatalf("非 station 被识别成 Wi-Fi: %+v", state)
		}
	}
}
func TestNetworkReadFailureAndCancellation(t *testing.T) {
	for _, failure := range []error{ErrNetworkUnavailable, context.Canceled} {
		called := false
		_, err := getNetworkStateWith(t.Context(), func(context.Context, string) (NetworkState, error) { called = true; return NetworkState{}, nil },
			func(context.Context) (string, error) { return "", failure })
		if !errors.Is(err, failure) || called {
			t.Fatalf("查询出口失败后仍读取 Wi-Fi: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := getNetworkStateWith(ctx, func(context.Context, string) (NetworkState, error) {
		t.Fatal("取消后仍读取 Wi-Fi")
		return NetworkState{}, nil
	},
		func(context.Context) (string, error) { return "wlan0", nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, err = getNetworkStateWith(t.Context(), func(context.Context, string) (NetworkState, error) { return NetworkState{NetworkType: "wifi"}, nil },
		func(context.Context) (string, error) { return "wlan0", nil })
	if !errors.Is(err, ErrNetworkUnavailable) {
		t.Fatal("未知 SSID 被当作有效网络")
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

func TestNetworkWatcherQueuesLatestWithoutCancellingApply(t *testing.T) {
	states := []NetworkState{
		{NetworkType: "wifi", SSID: "A"},
		{NetworkType: "wifi", SSID: "B"},
	}
	var mu sync.Mutex
	reads := 0
	firstStarted := make(chan struct{})
	finishFirst := make(chan struct{})
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
					select {
					case <-ctx.Done():
						t.Error("新事件取消了已开始的应用")
						return ctx.Err()
					case <-finishFirst:
						return nil
					}
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
	time.Sleep(20 * time.Millisecond)
	select {
	case got := <-latestEvaluated:
		t.Fatalf("并行应用了 %s", got)
	default:
	}
	close(finishFirst)
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
