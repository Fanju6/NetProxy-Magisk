package module

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

const googleFirewallChain = "zte_fw_gms"

// RepairGoogleConnectivity applies the vendor-specific repair once. Only cold
// boot needs a delay, because the vendor may populate its rules after boot.
func RepairGoogleConnectivity(ctx context.Context, options Options, delay time.Duration) {
	if runtime.GOOS != "android" || !googleFirewallModuleActive(options.ModuleDir) {
		return
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	brand, brandErr := exec.CommandContext(probe, "/system/bin/getprop", "ro.product.brand").Output()
	rom, romErr := exec.CommandContext(probe, "/system/bin/getprop", "ro.build.version.oplusrom").Output()
	cancel()
	colorOS := romErr == nil && colorOS16GoogleFirewall(string(brand), string(rom))
	if brandErr != nil || !colorOS && !googleFirewallBrand(string(brand)) {
		return
	}
	if delay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
	if ctx.Err() != nil || !googleFirewallModuleActive(options.ModuleDir) {
		return
	}
	ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	packages := []string{"com.google.android.gms", "com.android.vending"}
	if colorOS {
		packages = append(packages, "com.google.android.configupdater")
	}
	uids, err := inbound.ResolveExclusivePackageUIDs(ctx, 0, packages...)
	if err != nil {
		logService(options, "WARN", "module.google_firewall", "failed", "无法确认 Google 应用 UID，未修改防火墙")
		return
	}
	if len(uids) == 0 {
		return
	}
	var removed int
	if colorOS {
		if !canRepairColorOSGoogle(ctx) {
			return
		}
		removed, err = repairColorOSGoogleFirewall(uids)
	} else {
		removed, err = repairGoogleFirewall(uids, func(tool string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, filepath.Join("/system/bin", tool), args...).Output()
		})
	}
	if err != nil {
		logService(options, "WARN", "module.google_firewall", "failed", "Google 联网修复未完全应用，已移除 %d 项匹配拦截: %v", removed, err)
	} else if removed != 0 {
		logService(options, "INFO", "module.google_firewall", "success", "已移除 %d 项厂商 Google 联网拦截", removed)
	}
}

func colorOS16GoogleFirewall(brand, rom string) bool {
	brand = strings.ToLower(strings.TrimSpace(brand))
	rom = strings.TrimSpace(rom)
	return (brand == "oneplus" || brand == "oppo") && (rom == "V16" || strings.HasPrefix(rom, "V16."))
}

func canRepairColorOSGoogle(ctx context.Context) bool {
	currentUser, err := exec.CommandContext(ctx, "/system/bin/am", "get-current-user").Output()
	if err != nil || strings.TrimSpace(string(currentUser)) != "0" {
		return false
	}
	blocked, err := exec.CommandContext(ctx, "/system/bin/settings", "--user", "0", "get", "secure", "google_restric_info").Output()
	if err != nil || strings.TrimSpace(string(blocked)) != "1" {
		return false
	}
	for _, name := range []string{"oplus_user_change_gms_network_control", "oplus_comm_trafficmonitor_gms_network_control"} {
		value, err := exec.CommandContext(ctx, "/system/bin/settings", "get", "global", name).Output()
		if err != nil || !colorOSGooglePolicyUnset(string(value)) {
			return false
		}
	}
	return true
}

func colorOSGooglePolicyUnset(value string) bool {
	value = strings.TrimSpace(value)
	return value == "null" || value == "0"
}

func googleFirewallBrand(brand string) bool {
	switch strings.ToLower(strings.TrimSpace(brand)) {
	case "redmagic", "nubia", "zte":
		return true
	default:
		return false
	}
}

func googleFirewallModuleActive(dir string) bool {
	for _, marker := range []string{"disable", "remove"} {
		if _, err := os.Lstat(filepath.Join(dir, marker)); !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	_, err := os.Stat(paths.New(dir).Executable())
	return err == nil
}

func repairGoogleFirewall(uids map[uint32]bool, run func(string, ...string) ([]byte, error)) (int, error) {
	var failures error
	removed := 0
	for _, tool := range []string{"iptables", "ip6tables"} {
		rules, err := run(tool, "-w", "2", "-t", "filter", "-S")
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failures = errors.Join(failures, fmt.Errorf("%s: 读取规则失败: %w", tool, err))
			continue
		}
		for line := range strings.SplitSeq(string(rules), "\n") {
			rule := strings.Fields(line)
			if !isGoogleFirewallRule(rule, uids) {
				continue
			}
			if _, err := run(tool, append([]string{"-w", "2", "-t", "filter", "-D"}, rule[1:]...)...); err != nil {
				failures = errors.Join(failures, fmt.Errorf("%s: 删除匹配规则失败: %w", tool, err))
				break
			}
			removed++
		}
	}
	return removed, failures
}

// Only the vendor's dedicated GMS chain and single-UID owner rules are eligible.
// Generic Android firewall chains and unfamiliar rule shapes must remain untouched.
func isGoogleFirewallRule(rule []string, uids map[uint32]bool) bool {
	if len(rule) != 8 && len(rule) != 10 {
		return false
	}
	if rule[0] != "-A" || rule[1] != googleFirewallChain || rule[2] != "-m" || rule[3] != "owner" ||
		rule[4] != "--uid-owner" || rule[6] != "-j" {
		return false
	}
	uid, err := strconv.ParseUint(rule[5], 10, 32)
	if err != nil || !uids[uint32(uid)] {
		return false
	}
	return (rule[7] == "DROP" && len(rule) == 8) ||
		(rule[7] == "REJECT" && (len(rule) == 8 || rule[8] == "--reject-with" && !strings.HasPrefix(rule[9], "-")))
}
