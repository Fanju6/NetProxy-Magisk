package module

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestGoogleFirewallDeletesOnlyExactGoogleOwnerRules(t *testing.T) {
	v4 := "-A zte_fw_gms -m owner --uid-owner 10123 -j DROP"
	v6 := "-A zte_fw_gms -m owner --uid-owner 10456 -j REJECT --reject-with icmp6-adm-prohibited"
	untouched := []string{
		"-A fw_OUTPUT -m owner --uid-owner 10123 -j DROP",
		"-A fw_OUTPUT_oplus_dns -m owner --uid-owner 10123 -j DROP",
		"-A fw_INPUT -i wlan+ -m bpf --object-pinned /sys/fs/bpf/prog_oplus-netd_skfilter_reject_wlan_uid -j REJECT",
		"-A fw_OUTPUT -o wlan+ -m bpf --object-pinned /sys/fs/bpf/prog_oplus-netd_skfilter_reject_wlan_uid -j REJECT",
		"-A fw_OUTPUT_oplus_dns -p udp --dport 53 -m string --string example.invalid --algo bm -j REJECT",
		"-A fw_OUTPUT -s ::1/128 ! -o lo -j DROP",
		"-A zte_fw_gms -m string --string \"DROP -m owner --uid-owner 10123\" -j ACCEPT",
		"-A zte_fw_gms -m owner --uid-owner 10299 -j DROP",
		"-A zte_fw_gms -o wlan+ -m owner --uid-owner 10123 -j DROP",
		"-A zte_fw_gms -m owner --uid-owner 10123-10456 -j DROP",
		"-A zte_fw_gms -m owner ! --uid-owner 10123 -j REJECT",
		"-A zte_fw_gms -j DROP",
	}
	rules := map[string][]string{
		"iptables":  append(slices.Clone(untouched), v4),
		"ip6tables": append(slices.Clone(untouched), v6),
	}
	run := func(tool string, args ...string) ([]byte, error) {
		t.Helper()
		if len(args) < 5 || !slices.Equal(args[:4], []string{"-w", "2", "-t", "filter"}) {
			t.Fatalf("unexpected command: %s %v", tool, args)
		}
		switch args[4] {
		case "-S":
			return []byte(strings.Join(rules[tool], "\n")), nil
		case "-D":
			spec := "-A " + strings.Join(args[5:], " ")
			if want := map[string]string{"iptables": v4, "ip6tables": v6}[tool]; spec != want {
				t.Fatalf("unexpected deletion: %s %s", tool, spec)
			}
			i := slices.Index(rules[tool], spec)
			if i < 0 {
				t.Fatalf("duplicate deletion: %s %s", tool, spec)
			}
			rules[tool] = slices.Delete(rules[tool], i, i+1)
			return nil, nil
		default:
			t.Fatalf("unexpected operation: %v", args)
			return nil, nil
		}
	}
	uids := map[uint32]bool{10123: true, 10456: true}
	if count, err := repairGoogleFirewall(uids, run); count != 2 || err != nil {
		t.Fatalf("first pass: count=%d, err=%v", count, err)
	}
	for tool, current := range rules {
		if !slices.Equal(current, untouched) {
			t.Fatalf("%s changed unrelated rules: %v", tool, current)
		}
	}
	if count, err := repairGoogleFirewall(uids, run); count != 0 || err != nil {
		t.Fatalf("second pass: count=%d, err=%v", count, err)
	}
}

func TestGoogleFirewallDealsWithFamiliesIndependently(t *testing.T) {
	for _, failure := range []error{os.ErrNotExist, errors.New("lock busy")} {
		count, err := repairGoogleFirewall(map[uint32]bool{10123: true}, func(tool string, args ...string) ([]byte, error) {
			if tool == "iptables" {
				return nil, failure
			}
			if args[4] == "-S" {
				return []byte("-A zte_fw_gms -m owner --uid-owner 10123 -j DROP\n"), nil
			}
			return nil, nil
		})
		if count != 1 || (err != nil) == errors.Is(failure, os.ErrNotExist) {
			t.Fatalf("count=%d, err=%v, input=%v", count, err, failure)
		}
	}
}

func TestGoogleFirewallBootGuards(t *testing.T) {
	if !googleFirewallBrand(" REDMAGIC\n") || !googleFirewallBrand("nubia") ||
		googleFirewallBrand("OnePlus") || googleFirewallBrand("generic") {
		t.Fatal("vendor gate admitted an unexpected brand")
	}
	root := t.TempDir()
	executable := paths.New(root).Executable()
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if googleFirewallModuleActive(root) {
		t.Fatal("missing module executable accepted")
	}
	if err := os.WriteFile(executable, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	if !googleFirewallModuleActive(root) {
		t.Fatal("active module rejected")
	}
	for _, marker := range []string{"disable", "remove"} {
		path := filepath.Join(root, marker)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if googleFirewallModuleActive(root) {
			t.Fatalf("module with %s marker accepted", marker)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestColorOSGoogleFirewallGuards(t *testing.T) {
	if !colorOS16GoogleFirewall("OnePlus\n", "V16.0.0\n") ||
		!colorOS16GoogleFirewall("OPPO", "V16") ||
		colorOS16GoogleFirewall("OnePlus", "") ||
		colorOS16GoogleFirewall("OnePlus", "V17.0.0") ||
		colorOS16GoogleFirewall("other", "V16.0.0") {
		t.Fatal("ColorOS ROM and brand gate admitted an unverified configuration")
	}
	for _, value := range []string{"", "1", "4", "-1", "error"} {
		if colorOSGooglePolicyUnset(value) {
			t.Fatalf("allowed unknown or manually configured policy %q", value)
		}
	}
	if !colorOSGooglePolicyUnset("null\n") || !colorOSGooglePolicyUnset("0") {
		t.Fatal("known unset policy values rejected")
	}
}
