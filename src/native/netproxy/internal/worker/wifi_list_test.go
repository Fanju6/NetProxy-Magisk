package worker

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSavedWiFiNames(t *testing.T) {
	header := "Network Id SSID Security type\n"
	row := func(id int, name, security string) string {
		return fmt.Sprintf("%-12d %-32s %-4s\n", id, name, security)
	}
	got, err := parseSavedWiFiNetworks(header + row(1, "Home Wi-Fi", "PSK/SAE^") + row(2, " 办公,Wi-Fi \"A\"", "OPEN") +
		row(3, "Home Wi-Fi", "PSK") + row(4, "Home Wi-Fi", "wpa2-psk") +
		row(5, " 办公,Wi-Fi \"A\"", "wpa3-sae^") + row(6, "Home Wi-Fi", "open/owe^"))
	if err != nil || !reflect.DeepEqual(got, []string{" 办公,Wi-Fi \"A\"", "Home Wi-Fi"}) {
		t.Fatalf("%q %v", got, err)
	}
	for _, input := range []string{"No networks\n", header} {
		got, err := parseSavedWiFiNetworks(input)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("%q %v", got, err)
		}
	}
	for _, input := range []string{"", "Permission denied", header + "broken row", header + row(2, "Office", "password:secret")} {
		if _, err := parseSavedWiFiNetworks(input); err == nil {
			t.Fatalf("无效输出被识别为候选: %q", input)
		}
	}
}
