package traffic

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

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

func RestoreSystemTrust(ctx context.Context, layout paths.Layout) error {
	return reconcileSystemTrust(ctx, layout, trustBoot)
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
	pids, err := mountTargets(true)
	if err != nil {
		return err
	}
	for _, store := range systemStores {
		base := procRoot(1, store.target)
		if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) && store.name == "conscrypt" {
			continue
		} else if err != nil {
			return errors.New("无法读取系统 CA 存储")
		}
		stage := filepath.Join(layout.TrafficTrust(), store.name)
		if !sameMount(stage, base) {
			if err := prepareStore(ctx, base, stage, cert); err != nil {
				return err
			}
		} else if content, err := os.ReadFile(filepath.Join(stage, cert.Filename)); err != nil || string(content) != cert.PEM {
			return errors.New("已挂载的抓包 CA 与当前证书不符，请重启设备")
		}
		for _, pid := range pids {
			if sameMount(stage, procRoot(pid, store.target)) {
				continue
			}
			if err := namespaceCommand(ctx, pid, "/system/bin/mount", "-o", "bind", stage, store.target); err != nil ||
				!sameMount(stage, procRoot(pid, store.target)) {
				return errors.New("绑定系统 CA 存储失败")
			}
		}
	}
	return nil
}

func prepareStore(ctx context.Context, base, stage string, cert Certificate) error {
	parent := filepath.Dir(stage)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return errors.New("无法创建系统 CA 挂载目录")
	}
	pending, err := os.MkdirTemp(parent, ".cacerts-*")
	if err != nil {
		return errors.New("无法创建系统 CA 候选目录")
	}
	defer os.RemoveAll(pending)
	if err := populateStore(base, pending, cert); err != nil {
		return err
	}
	if err := exec.CommandContext(ctx, "/system/bin/chcon", "-R", "u:object_r:system_file:s0", pending).Run(); err != nil {
		return errors.New("无法设置系统 CA 目录的 SELinux 标签")
	}
	if _, err := os.Stat(stage); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(pending, stage); err == nil {
			return nil
		}
	}
	return errors.New("系统 CA 挂载目录已存在或无法提交，请重启设备")
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
	if _, err := os.Stat(layout.TrafficTrust()); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	pids, err := mountTargets(false)
	if err != nil {
		return err
	}
	for _, store := range systemStores {
		stage := filepath.Join(layout.TrafficTrust(), store.name)
		failed := false
		for _, pid := range pids {
			if !sameMount(stage, procRoot(pid, store.target)) {
				continue
			}
			if err := namespaceCommand(ctx, pid, "/system/bin/umount", "-l", store.target); err != nil &&
				sameMount(stage, procRoot(pid, store.target)) {
				failed = true
			}
		}
		if failed {
			return errors.New("部分系统 CA 挂载无法移除，请重启设备")
		}
		if err := os.RemoveAll(stage); err != nil {
			return errors.New("无法清理系统 CA 挂载目录，请重启设备")
		}
	}
	return nil
}

func mountTargets(start bool) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, errors.New("无法查询 Android 挂载命名空间")
	}
	seen := make(map[string]bool)
	pids := make([]int, 0, 8)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		proc := filepath.Join("/proc", entry.Name())
		if start && pid != 1 {
			name, _ := os.ReadFile(filepath.Join(proc, "comm"))
			switch strings.TrimSpace(string(name)) {
			case "zygote", "zygote64", "webview_zygote":
			default:
				continue
			}
		}
		namespace, err := os.Readlink(filepath.Join(proc, "ns/mnt"))
		if err == nil && !seen[namespace] {
			seen[namespace] = true
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 || pids[0] != 1 {
		return nil, errors.New("无法访问 Android 初始挂载命名空间")
	}
	return pids, nil
}

func procRoot(pid int, path string) string {
	return filepath.Join("/proc", strconv.Itoa(pid), "root", path)
}

func sameMount(stage, target string) bool {
	source, srcErr := os.Stat(stage)
	mounted, dstErr := os.Stat(target)
	return srcErr == nil && dstErr == nil && os.SameFile(source, mounted)
}

func namespaceCommand(ctx context.Context, pid int, command ...string) error {
	args := append([]string{"-t", strconv.Itoa(pid), "-m", "--"}, command...)
	return exec.CommandContext(ctx, "/system/bin/nsenter", args...).Run()
}
