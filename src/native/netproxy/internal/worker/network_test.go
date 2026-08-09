package worker

import (
	"context"
	"errors"
	"os"
	"testing"
)

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

func TestNetworkFileStateDetectsChanges(t *testing.T) {
	temporary := t.TempDir() + "/rt_tables"
	first := readNetworkFileState(temporary)
	if first.exists {
		t.Fatal("missing network file reported as present")
	}

	if err := os.WriteFile(temporary, []byte("1000 netproxy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := readNetworkFileState(temporary)
	if !second.exists || second == first {
		t.Fatalf("network file state did not change: first=%+v second=%+v", first, second)
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
			got, err := getActiveNetworkInterfaceWith(
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
			gotNetwork, gotSSID, err := getWiFiSnapshotWith(
				context.Background(),
				func(_ context.Context, name string, _ ...string) (string, error) {
					switch name {
					case "cmd":
						return `Wifi is connected to "Home WiFi", BSSID: 00:11:22:33:44:55`, nil
					case "ip":
						return test.activeRoute, nil
					default:
						return "", errors.New("unexpected command")
					}
				},
				func(string) ([]byte, error) { return nil, errors.New("not needed") },
			)
			if err != nil {
				t.Fatal(err)
			}
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
