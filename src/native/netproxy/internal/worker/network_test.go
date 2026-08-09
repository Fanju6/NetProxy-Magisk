package worker

import (
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
