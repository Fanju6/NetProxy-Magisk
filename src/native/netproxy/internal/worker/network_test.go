package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
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

func TestGetActiveNetworkInterface(t *testing.T) {
	tests := []struct {
		name      string
		route     string
		routeErr  error
		procRoute string
		procErr   error
		wantIface string
		wantErr   bool
	}{
		{
			name:      "ip route",
			route:     "1.1.1.1 via 192.168.1.1 dev wlan0 src 192.168.1.100",
			wantIface: "wlan0",
		},
		{
			name:      "proc route fallback",
			routeErr:  errors.New("ip unavailable"),
			procRoute: "Iface\tDestination\tGateway\nrmnet_data0\t00000000\t0100000A\n",
			wantIface: "rmnet_data0",
		},
		{
			name:      "no default route",
			routeErr:  errors.New("ip unavailable"),
			procRoute: "Iface\tDestination\tGateway\nwlan0\t00000001\t0100000A\n",
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, got, err := getActiveNetworkRouteWith(
				context.Background(),
				func(context.Context, string, ...string) (string, error) {
					return test.route, test.routeErr
				},
				func(string) ([]byte, error) {
					return []byte(test.procRoute), test.procErr
				},
			)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if got != test.wantIface {
				t.Fatalf("interface = %q, want %q", got, test.wantIface)
			}
		})
	}
}

func TestWiFiSnapshotUsesActiveInterface(t *testing.T) {
	tests := []struct {
		name        string
		activeRoute string
		wantNetwork string
		wantSSID    string
	}{
		{
			name:        "wifi carries the default route",
			activeRoute: "1.1.1.1 dev wlan0 src 192.168.1.100",
			wantNetwork: "wifi",
			wantSSID:    "Home WiFi",
		},
		{
			name:        "mobile data carries the default route",
			activeRoute: "1.1.1.1 dev rmnet0 src 10.0.0.2",
			wantNetwork: "not_wifi",
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
						return "mSoftApState=11", nil
					case "ip":
						if len(args) >= 3 && args[0] == "route" && args[1] == "get" {
							return test.activeRoute, nil
						}
						if len(args) >= 3 && args[0] == "-o" && args[1] == "link" && args[2] == "show" {
							iface := "wlan0"
							if strings.Contains(test.activeRoute, "rmnet0") {
								iface = "rmnet0"
							}
							return "2: " + iface + ": <BROADCAST,UP,LOWER_UP> mtu 1500 state UP mode DEFAULT", nil
						}
						if len(args) >= 3 && args[0] == "route" && args[1] == "show" && args[2] == "default" {
							iface := "wlan0"
							if strings.Contains(test.activeRoute, "rmnet0") {
								iface = "rmnet0"
							}
							return "default via 192.168.1.1 dev " + iface + " metric 100", nil
						}
						return "", errors.New("unexpected ip command")
					default:
						return "", errors.New("unexpected command")
					}
				},
				func(string) ([]byte, error) { return nil, errors.New("not needed") },
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

func TestNetworkStateFingerprintIncludesRouteInterfaceAndHotspot(t *testing.T) {
	base := NetworkState{
		NetworkType:     "wifi",
		SSID:            "Home WiFi",
		DefaultRoute:    "default dev wlan0 metric 100",
		ActiveInterface: "wlan0",
		InterfaceStatus: "wlan0=up<BROADCAST,UP,LOWER_UP>",
		HotspotState:    "disabled",
	}
	for name, changed := range map[string]NetworkState{
		"ssid":             func() NetworkState { value := base; value.SSID = "Office"; return value }(),
		"default route":    func() NetworkState { value := base; value.DefaultRoute = "default dev rmnet0 metric 100"; return value }(),
		"active interface": func() NetworkState { value := base; value.ActiveInterface = "rmnet0"; return value }(),
		"interface status": func() NetworkState { value := base; value.InterfaceStatus = "wlan0=down<BROADCAST>"; return value }(),
		"hotspot":          func() NetworkState { value := base; value.HotspotState = "enabled"; return value }(),
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
				return "mSoftApState=11", nil
			case "ip":
				switch {
				case len(args) >= 2 && args[0] == "route" && args[1] == "get":
					return "1.1.1.1 via 10.0.0.1 dev rmnet_data0 src 10.0.0.2", nil
				case len(args) >= 3 && args[0] == "route" && args[1] == "show" && args[2] == "default":
					return "default via 192.168.1.1 dev wlan0 metric 600\ndefault via 10.0.0.1 dev rmnet_data0 metric 100", nil
				case len(args) >= 3 && args[0] == "-o" && args[1] == "link" && args[2] == "show":
					return "2: wlan0: <BROADCAST,UP,LOWER_UP> mtu 1500 state UP mode DEFAULT\n3: rmnet_data0: <UP,LOWER_UP> mtu 1500 state UNKNOWN mode DEFAULT", nil
				}
			}
			return "", errors.New("unexpected command")
		},
		func(string) ([]byte, error) { return nil, errors.New("proc route fallback not expected") },
	)
	if err != nil {
		t.Fatal(err)
	}
	if state.NetworkType != "not_wifi" || state.SSID != "" {
		t.Fatalf("双连接时不应按 Wi-Fi 评估: %+v", state)
	}
	if state.ActiveInterface != "rmnet_data0" ||
		!strings.Contains(state.DefaultRoute, "dev wlan0") ||
		!strings.Contains(state.DefaultRoute, "dev rmnet_data0") {
		t.Fatalf("网络路径指纹不完整: %+v", state)
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
		func(string) ([]byte, error) { return nil, errors.New("proc route read failed") },
	)
	if err == nil {
		t.Fatal("网络状态读取失败时不应生成快照")
	}
}

func TestParseHotspotState(t *testing.T) {
	for input, want := range map[string]string{
		"SoftApManager: state=enabled": "enabled",
		"mSoftApState=13":              "enabled",
		"mWifiApState: 11":             "disabled",
		"Wifi is enabled":              "unknown",
	} {
		if got := parseHotspotState(input); got != want {
			t.Errorf("parseHotspotState(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNetworkWatcherDebouncesStateChanges(t *testing.T) {
	routeTable, err := os.CreateTemp(t.TempDir(), "rt_tables-")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routeTable.WriteString("initial\n"); err != nil {
		t.Fatal(err)
	}
	if err := routeTable.Close(); err != nil {
		t.Fatal(err)
	}
	initial := NetworkState{NetworkType: "wifi", SSID: "A", DefaultRoute: "wlan0", ActiveInterface: "wlan0", InterfaceStatus: "wlan0=up", HotspotState: "disabled"}
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
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNetworkWatcher(ctx, Options{
			NetworkTablesPath:       routeTable.Name(),
			NetworkStateReader:      read,
			NetworkPollInterval:     5 * time.Millisecond,
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
		t.Fatalf("route table stable state should not reread network state, count=%d", gotReads)
	}
	if err := os.WriteFile(routeTable.Name(), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("route table change should trigger one network reread, count=%d", gotReads)
	}
	cancel()
	<-done
}

func TestNetworkWatcherDoesNotReadStateWhileRouteTableIsStable(t *testing.T) {
	routeTable, err := os.CreateTemp(t.TempDir(), "rt_tables-")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routeTable.WriteString("stable\n"); err != nil {
		t.Fatal(err)
	}
	if err := routeTable.Close(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	reads := 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNetworkWatcher(ctx, Options{
			NetworkTablesPath:   routeTable.Name(),
			NetworkPollInterval: 5 * time.Millisecond,
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
		t.Fatalf("stable route table should not reread network state, count=%d", reads)
	}
}

func TestNetworkWatcherSkipsEvaluationWhenStateReadFails(t *testing.T) {
	evaluated := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runNetworkWatcher(ctx, Options{
			NetworkStateReader: func(context.Context) (NetworkState, error) {
				return NetworkState{}, errors.New("unavailable")
			},
			NetworkPollInterval:     5 * time.Millisecond,
			NetworkDebounceInterval: 5 * time.Millisecond,
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
