package traffic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

type trustStore struct {
	name   string
	target string
}

var systemStores = []trustStore{
	{"system", "/system/etc/security/cacerts"},
	{"conscrypt", "/apex/com.android.conscrypt/cacerts"},
}

var androidCAFilename = regexp.MustCompile(`^[a-f0-9]{8}\.[0-9]+$`)

type trustAction int

const (
	trustBoot trustAction = iota
	trustDisable
	trustUninstall
)

func RestoreSystemTrust(ctx context.Context, layout paths.Layout, trigger string) {
	if runtime.GOOS != "android" || !layout.IsLive() {
		return
	}
	log := func(level, result, message string) {
		_ = logfile.AppendEntry(layout.ServiceLog(), logfile.Entry{
			Level: level, Component: "service", Event: "traffic.trust", Result: result,
			Message: trigger + ": " + message,
		})
	}
	log("INFO", "started", "检查并恢复抓包 CA 系统信任")
	if err := reconcileSystemTrust(ctx, layout, trustBoot); err != nil {
		log("WARN", "failed", "抓包 CA 未完成系统挂载: "+err.Error())
		return
	}
	log("INFO", "success", "系统 CA 挂载状态已与抓包配置同步")
}

func RemoveDisabledSystemTrust(ctx context.Context, layout paths.Layout) error {
	return reconcileSystemTrust(ctx, layout, trustDisable)
}

func RemoveSystemTrust(ctx context.Context, layout paths.Layout) error {
	return reconcileSystemTrust(ctx, layout, trustUninstall)
}

func reconcileSystemTrust(ctx context.Context, layout paths.Layout, action trustAction) error {
	if runtime.GOOS != "android" || !layout.IsLive() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	lock, err := processlock.Acquire(ctx, layout.TrafficTrust()+".lock")
	if err != nil {
		return errors.New("无法锁定系统 CA 挂载")
	}
	defer lock.Release()
	if action != trustUninstall {
		policy, err := loadPolicy(layout)
		if err != nil {
			return err
		}
		if policy.SystemTrust {
			if action == trustDisable {
				return nil
			}
			ca, err := loadCA(layout.TrafficCA())
			if err != nil {
				return errors.New("无法读取待安装的公开抓包 CA")
			}
			if err := installStores(ctx, layout, ca.public); err != nil {
				cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				defer done()
				return errors.Join(err, detachStores(cleanup, layout))
			}
			return nil
		}
	}
	return detachStores(ctx, layout)
}

func installStores(ctx context.Context, layout paths.Layout, cert Certificate) error {
	targets, err := mountTargets()
	if err != nil {
		return err
	}
	if err := detachStores(ctx, layout); err != nil {
		return err
	}
	for _, store := range systemStores {
		base := procRoot(1, store.target)
		if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) && store.name == "conscrypt" {
			continue
		} else if err != nil {
			return fmt.Errorf("读取 init 的 CA 存储 %s: %w", store.target, err)
		}
		stage := filepath.Join(layout.TrafficTrust(), store.name)
		if err := prepareStore(ctx, base, stage, caMountSource(store), cert); err != nil {
			return err
		}
		for _, target := range targets {
			if err := installStoreMount(ctx, target, stage, store.target, cert); err != nil {
				return err
			}
		}
	}
	return nil
}

func caMountSource(store trustStore) string { return "netproxy-traffic-ca-" + store.name }

func prepareStore(ctx context.Context, base, stage, name string, cert Certificate) error {
	source := procRoot(1, stage)
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		return fmt.Errorf("创建系统 CA 挂载目录: %w", err)
	}
	if err := os.Mkdir(source, 0o700); err != nil {
		return fmt.Errorf("创建系统 CA 候选目录: %w", err)
	}
	init, err := mountTargetFor(1)
	if err != nil {
		return err
	}
	if err := namespaceCommand(ctx, init, "/system/bin/mount", "-t", "tmpfs", "-o", "mode=0700,nodev,nosuid,noexec", name, stage); err != nil {
		return err
	}
	if err := populateStore(base, source, cert); err != nil {
		return err
	}
	label, err := namespaceCommandOutput(ctx, init, "/system/bin/ls", "-Zd", base)
	if err != nil {
		return err
	}
	context, _, _ := strings.Cut(strings.TrimSpace(string(label)), " ")
	if !strings.HasPrefix(context, "u:object_r:") {
		return errors.New("无法识别原系统 CA 存储的 SELinux 标签")
	}
	return namespaceCommand(ctx, init, "/system/bin/chcon", "-R", context, stage)
}

func populateStore(base, pending string, cert Certificate) error {
	if !androidCAFilename.MatchString(cert.Filename) {
		return errors.New("抓包 CA 文件名无效")
	}
	files, err := os.ReadDir(base)
	if err != nil || len(files) > 512 {
		return errors.New("无法读取原系统 CA 列表")
	}
	count := 0
	for _, file := range files {
		if !androidCAFilename.MatchString(file.Name()) {
			continue
		}
		content, err := readStockCA(filepath.Join(base, file.Name()))
		if err != nil {
			return err
		}
		if file.Name() == cert.Filename && !bytes.Equal(content, []byte(cert.PEM)) {
			return errors.New("抓包 CA 文件名与系统证书冲突，未覆盖系统证书")
		}
		if err := writePublicCA(filepath.Join(pending, file.Name()), content); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		return errors.New("拒绝挂载没有原系统证书的 CA 目录")
	}
	if err := writePublicCA(filepath.Join(pending, cert.Filename), []byte(cert.PEM)); err != nil {
		return err
	}
	if err := os.Chmod(pending, 0o755); err != nil {
		return errors.New("无法设置系统 CA 目录权限")
	}
	return nil
}

func readStockCA(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("无法复制原系统 CA")
	}
	defer file.Close()
	const maxSize = 64 << 10
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSize {
		return nil, errors.New("原系统 CA 文件无效")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil || len(content) == 0 || len(content) > maxSize {
		return nil, errors.New("原系统 CA 文件无效")
	}
	return content, nil
}

func writePublicCA(path string, content []byte) error {
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return errors.New("无法写入公开系统 CA")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return errors.New("无法设置公开系统 CA 权限")
	}
	return nil
}

func detachStores(ctx context.Context, layout paths.Layout) error {
	targets, err := mountTargets()
	if err != nil {
		return err
	}
	var failures error
	for _, store := range systemStores {
		stage := filepath.Join(layout.TrafficTrust(), store.name)
		info, err := readMountInfo(targets[0])
		if err != nil {
			return errors.Join(failures, err)
		}
		legacy := make(map[string]bool)
		for _, mount := range mountsAt(info, stage) {
			if mount.filesystem == "tmpfs" {
				legacy[mount.device] = true
			}
		}
		var detachErr error
		for _, target := range targets {
			if err := detachStoreLayers(ctx, target, store, stage, legacy); err != nil {
				detachErr = errors.Join(detachErr, err)
			}
		}
		if detachErr == nil {
			detachErr = removeStage(ctx, targets[0], stage)
		}
		failures = errors.Join(failures, detachErr)
	}
	return failures
}

func removeStage(ctx context.Context, init mountTarget, stage string) error {
	info, err := os.ReadFile("/proc/1/mountinfo")
	if err != nil {
		return fmt.Errorf("读取 init 挂载表: %w", err)
	}
	if listedMount(string(info), stage) {
		if err := namespaceCommand(ctx, init, "/system/bin/umount", "-l", stage); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(procRoot(1, stage)); err != nil {
		return fmt.Errorf("清理系统 CA 候选目录: %w", err)
	}
	return nil
}

func listedMount(info, path string) bool {
	return len(mountsAt(info, path)) != 0
}

type caMount struct {
	id, parent, device, root, filesystem, source string
}

func mountsAt(info, path string) []caMount {
	escaped := strings.NewReplacer("\\", "\\134", " ", "\\040", "\t", "\\011", "\n", "\\012").Replace(path)
	var result []caMount
	for line := range strings.SplitSeq(info, "\n") {
		before, after, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		fields, fs := strings.Fields(before), strings.Fields(after)
		if len(fields) > 4 && len(fs) >= 2 && fields[4] == escaped {
			result = append(result, caMount{fields[0], fields[1], fields[2], fields[3], fs[0], fs[1]})
		}
	}
	return result
}

func ownedMountTop(info string, store trustStore, stage string, legacy map[string]bool) (caMount, bool, error) {
	mounts := mountsAt(info, store.target)
	parents := make(map[string]bool, len(mounts))
	owned := false
	for _, mount := range mounts {
		parents[mount.parent] = true
		owned = owned || ownsStoreMount(mount, store, stage, legacy)
	}
	if !owned {
		return caMount{}, false, nil
	}
	var top caMount
	for _, mount := range mounts {
		if !parents[mount.id] {
			if top.id != "" {
				return caMount{}, false, fmt.Errorf("无法确定 CA 挂载顶层 %s", store.target)
			}
			top = mount
		}
	}
	if !ownsStoreMount(top, store, stage, legacy) {
		return caMount{}, false, fmt.Errorf("另一个挂载遮住了 NetProxy CA 层 %s", store.target)
	}
	return top, true, nil
}

func ownsStoreMount(mount caMount, store trustStore, stage string, legacy map[string]bool) bool {
	legacyRoot, underDev := strings.CutPrefix(stage, "/dev/")
	return mount.filesystem == "tmpfs" &&
		(mount.root == "/" && (mount.source == caMountSource(store) || legacy[mount.device]) ||
			underDev && mount.root == "/"+legacyRoot)
}

func readMountInfo(target mountTarget) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", target.pid))
	if err != nil {
		return "", fmt.Errorf("读取挂载表 (PID %d, %s): %w", target.pid, target.namespace, err)
	}
	return string(data), nil
}

func detachStoreLayers(ctx context.Context, target mountTarget, store trustStore, stage string, legacy map[string]bool) error {
	seen := make(map[string]bool)
	for target.active() {
		info, err := readMountInfo(target)
		if err != nil || ctx.Err() != nil {
			if !target.active() {
				return nil
			}
			return errors.Join(err, ctx.Err())
		}
		top, owned, err := ownedMountTop(info, store, stage, legacy)
		if err != nil || !owned {
			return err
		}
		if seen[top.id] {
			return fmt.Errorf("未能卸载 CA 层 %s (PID %d, %s, mount %s)", store.target, target.pid, target.namespace, top.id)
		}
		seen[top.id] = true
		if err := namespaceCommand(ctx, target, "/system/bin/umount", "-l", store.target); err != nil && target.active() {
			return err
		}
	}
	return nil
}

type mountTarget struct {
	pid       int
	namespace string
}

func mountTargetFor(pid int) (mountTarget, error) {
	namespace, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/mnt", pid))
	return mountTarget{pid, namespace}, err
}

func (target mountTarget) active() bool {
	current, err := mountTargetFor(target.pid)
	return err == nil && current == target
}

func mountTargets() ([]mountTarget, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, errors.New("无法查询 Android 挂载命名空间")
	}
	pids := make([]int, 0, 8)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		pids = append(pids, pid)
	}
	targets := uniqueMountTargets(pids, mountTargetFor)
	if len(targets) == 0 || targets[0].pid != 1 {
		return nil, errors.New("无法访问 Android 初始挂载命名空间")
	}
	return targets, nil
}

func uniqueMountTargets(pids []int, lookup func(int) (mountTarget, error)) []mountTarget {
	slices.Sort(pids)
	seen := make(map[string]bool)
	var targets []mountTarget
	for _, pid := range pids {
		target, err := lookup(pid)
		if err == nil && !seen[target.namespace] {
			seen[target.namespace] = true
			targets = append(targets, target)
		}
	}
	return targets
}

func procRoot(pid int, path string) string {
	return filepath.Join("/proc", strconv.Itoa(pid), "root", path)
}

func sameMount(stage, target string) bool {
	source, srcErr := os.Stat(stage)
	mounted, dstErr := os.Stat(target)
	return srcErr == nil && dstErr == nil && os.SameFile(source, mounted)
}

func installStoreMount(ctx context.Context, target mountTarget, stage, path string, cert Certificate) error {
	source := procRoot(1, stage)
	dest := procRoot(target.pid, path)
	if !target.active() {
		return nil
	}
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) && target.pid != 1 {
		return nil
	} else if err != nil {
		return fmt.Errorf("读取 CA 存储 %s (PID %d, %s): %w", path, target.pid, target.namespace, err)
	}
	if !sameMount(source, dest) {
		if !sameMount(source, procRoot(target.pid, stage)) {
			return fmt.Errorf("CA 候选挂载 %s 在 PID %d (%s) 不可见", stage, target.pid, target.namespace)
		}
		if err := namespaceCommand(ctx, target, "/system/bin/mount", "-o", "bind", stage, path); err != nil && target.active() {
			return err
		}
	}
	if !target.active() {
		return nil
	}
	if !sameMount(source, dest) {
		return fmt.Errorf("CA 挂载状态验证失败 %s (PID %d, %s)", path, target.pid, target.namespace)
	}
	content, err := readStockCA(filepath.Join(dest, cert.Filename))
	if err != nil || string(content) != cert.PEM {
		return fmt.Errorf("CA 挂载缺少匹配证书 %s/%s (PID %d, %s)", path, cert.Filename, target.pid, target.namespace)
	}
	return nil
}

func namespaceArgs(pid int, command []string) []string {
	return append([]string{fmt.Sprintf("--mount=/proc/%d/ns/mnt", pid), "--"}, command...)
}

func namespaceCommand(ctx context.Context, target mountTarget, command ...string) error {
	_, err := namespaceCommandOutput(ctx, target, command...)
	return err
}

func namespaceCommandOutput(ctx context.Context, target mountTarget, command ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "/system/bin/nsenter", namespaceArgs(target.pid, command)...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s (PID %d, %s): %w: %.400s", strings.Join(command, " "), target.pid, target.namespace, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
