package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

const defaultServiceSecret = "singbox"

// logWorker 统一标识 Worker 写入服务日志的记录。
func logWorker(logger *log.Logger, format string, args ...any) {
	if logger != nil {
		logger.Printf("[Worker] "+format, args...)
	}
}

// Timer 描述 Worker 调度所需的最小定时器接口。
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// TimerFactory 创建 Worker 调度定时器；测试可以用虚拟时钟替换系统时间。
type TimerFactory func(time.Duration) Timer

// Options 描述订阅 Worker 的运行环境。
type Options struct {
	Root                string
	ProgressDir         string
	PIDFile             string
	LogFile             string
	ModuleConf          string
	NativePath          string
	SingBoxPath         string
	ServiceAddress      string
	ServiceSecret       string
	NetworkWatchEnabled bool
	NetworkEvaluate     func(context.Context, string, string) error
	NetworkTablesPath   string
	Now                 func() time.Time
	NewTimer            TimerFactory
}

// Summary 是一次调度轮次的结果。
type Summary struct {
	Updated []string `json:"updated"`
	Failed  []string `json:"failed"`
	Nearest int64    `json:"nearest"`
}

// Status 描述 Worker 的进程和下一次任务。
type Status struct {
	State   string `json:"state"`
	PID     int    `json:"pid,omitempty"`
	Nearest int64  `json:"nearest"`
}

// NewOptions 返回模块默认的 Worker 配置。
func NewOptions(root string) Options {
	return Options{
		Root:              root,
		ProgressDir:       "/dev/netproxy/subscriptions",
		PIDFile:           "/dev/netproxy/subworker.pid",
		ServiceAddress:    "127.0.0.1:9090",
		ServiceSecret:     defaultServiceSecret,
		NetworkTablesPath: "/data/misc/net/rt_tables",
		Now:               time.Now,
	}
}

// Run 执行订阅调度循环。wake 通道收到信号后立即重新计算任务。
func Run(ctx context.Context, options Options, wake <-chan struct{}, logger *log.Logger) error {
	if err := validateOptions(options); err != nil {
		return err
	}
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	if err := acquirePID(options.PIDFile); err != nil {
		return err
	}
	defer releasePID(options.PIDFile)

	networkWatchEnabled := options.NetworkWatchEnabled && options.NetworkEvaluate != nil
	var networkDone chan struct{}
	if networkWatchEnabled {
		networkDone = make(chan struct{})
		go func() {
			defer close(networkDone)
			runNetworkWatcher(ctx, options, logger)
		}()
		logWorker(logger, "Android 网络事件监听已启动")
	}
	defer func() {
		if networkDone != nil {
			<-networkDone
		}
	}()

	logWorker(logger, "订阅自动更新 Worker 已启动")
	for {
		now := options.Now()
		summary, err := RunDue(ctx, options, now, logger)
		if err != nil {
			logWorker(logger, "读取订阅调度失败: %v", err)
		}
		nearest := summary.Nearest
		if err != nil {
			nearest = now.Unix() + 60
		} else {
			nearest, err = nextUpdate(options.Root, now.Unix())
			if err != nil {
				logWorker(logger, "计算下一次订阅更新时间失败: %v", err)
				nearest = now.Unix() + 60
			}
		}
		if nearest == 0 && !networkWatchEnabled {
			logWorker(logger, "没有启用自动更新的订阅，Worker 退出")
			return nil
		}
		if nearest == 0 {
			nearest = now.Unix() + int64((24*time.Hour)/time.Second)
		}
		delay := time.Duration(nearest-now.Unix()) * time.Second
		if delay < time.Second {
			delay = time.Second
		}
		timer := newTimer(options, delay)
		select {
		case <-ctx.Done():
			stopTimer(timer)
			logWorker(logger, "订阅自动更新 Worker 已停止")
			return nil
		case <-wake:
			stopTimer(timer)
		case <-timer.C():
		}
	}
}

type systemTimer struct {
	timer *time.Timer
}

func (timer systemTimer) C() <-chan time.Time {
	return timer.timer.C
}

func (timer systemTimer) Stop() bool {
	return timer.timer.Stop()
}

func newTimer(options Options, duration time.Duration) Timer {
	if options.NewTimer != nil {
		return options.NewTimer(duration)
	}
	return systemTimer{timer: time.NewTimer(duration)}
}

func stopTimer(timer Timer) {
	if timer == nil || timer.Stop() {
		return
	}
	select {
	case <-timer.C():
	default:
	}
}

// RunDue 顺序执行当前已经到期的订阅更新。
func RunDue(ctx context.Context, options Options, now time.Time, logger *log.Logger) (Summary, error) {
	if err := validateOptions(options); err != nil {
		return Summary{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	schedule, err := catalog.Schedule(options.Root, now.Unix())
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Updated: []string{}, Failed: []string{}, Nearest: schedule.Nearest}
	for _, groupID := range schedule.Due {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if logger != nil {
			logWorker(logger, "自动更新到期订阅: %s", groupID)
		}
		_, updateErr := UpdateGroup(ctx, options, groupID, now, logger)
		if updateErr != nil {
			summary.Failed = append(summary.Failed, groupID)
			if logger != nil {
				logWorker(logger, "订阅更新失败: %s: %v", groupID, updateErr)
			}
			continue
		}
		summary.Updated = append(summary.Updated, groupID)
	}
	return summary, nil
}

// UpdateGroup 执行单个订阅更新，并统一处理更新后的运行时状态。
func UpdateGroup(ctx context.Context, options Options, groupID string, now time.Time, logger *log.Logger) (subscription.Result, error) {
	result, err := subscription.Update(ctx, subscription.UpdateOptions{
		Root: options.Root, GroupID: groupID, ProgressDir: options.ProgressDir,
		UseConfiguredProxy: true, Now: now,
	})
	if err != nil {
		return subscription.Result{}, err
	}
	if effectErr := applyUpdateEffects(ctx, options, result, groupID, logger); effectErr != nil && logger != nil {
		logWorker(logger, "订阅更新后的运行时同步失败: %s: %v", groupID, effectErr)
	}
	return result, nil
}

// NextUpdate 返回下一次自动更新时间；没有自动订阅时返回 0。
func NextUpdate(root string, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return nextUpdate(root, now.Unix())
}

func nextUpdate(root string, now int64) (int64, error) {
	schedule, err := catalog.Schedule(root, now)
	if err != nil {
		return 0, err
	}
	return schedule.Nearest, nil
}

func applyUpdateEffects(ctx context.Context, options Options, result subscription.Result, groupID string, logger *log.Logger) error {
	activeChanged, err := activateGroupIfNeeded(ctx, options, groupID)
	if err != nil {
		return err
	}
	if err := fallbackMissingNode(ctx, options, groupID, logger); err != nil {
		return err
	}
	if (result.StructureChanged || activeChanged) && isProcessRunning(options.SingBoxPath) {
		return reloadService(ctx, options)
	}
	return nil
}

func activateGroupIfNeeded(ctx context.Context, options Options, groupID string) (bool, error) {
	module, err := moduleconfig.LoadModule(options.ModuleConf)
	if err != nil {
		return false, err
	}
	active := module.ActiveGroupID
	if active != "" {
		hasNodes, hasErr := catalog.GroupHasNodes(ctx, options.Root, active)
		if hasErr != nil {
			return false, hasErr
		}
		if hasNodes {
			return false, nil
		}
	}
	hasNodes, err := catalog.GroupHasNodes(ctx, options.Root, groupID)
	if err != nil || !hasNodes {
		return false, err
	}
	if err := moduleconfig.UpdateModule(options.ModuleConf, map[string]string{
		"ACTIVE_GROUP_ID":   moduleconfig.Quote(groupID),
		"SELECTOR_MODE":     "urltest",
		"SELECTED_NODE_REF": moduleconfig.Quote(""),
	}); err != nil {
		return false, err
	}
	return true, nil
}

func fallbackMissingNode(ctx context.Context, options Options, groupID string, logger *log.Logger) error {
	module, err := moduleconfig.LoadModule(options.ModuleConf)
	if err != nil || module.SelectorMode != "manual" {
		return err
	}
	selected := module.SelectedNodeRef
	if selected == "" {
		return err
	}
	selectedGroup, selectedTag, found := strings.Cut(selected, "/")
	if !found || selectedGroup != groupID || selectedTag == "" {
		return nil
	}
	present, err := catalog.GroupContainsTag(ctx, options.Root, groupID, selectedTag)
	if err != nil || present {
		return err
	}
	if err := moduleconfig.UpdateModule(options.ModuleConf, map[string]string{
		"SELECTOR_MODE":     "urltest",
		"SELECTED_NODE_REF": moduleconfig.Quote(""),
	}); err != nil {
		return err
	}
	runtimeTag, err := catalog.RuntimeTag(options.Root, groupID)
	if err != nil {
		return err
	}
	if logger != nil {
		logWorker(logger, "手动节点已从 Provider 移除，回退到 Auto/%s", runtimeTag)
	}
	if !isProcessRunning(options.SingBoxPath) {
		return nil
	}
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err != nil {
		return err
	}
	defer client.Close()
	requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.Select(requestContext, "Proxy", "Auto/"+runtimeTag)
}

func reloadService(ctx context.Context, options Options) error {
	if strings.TrimSpace(options.NativePath) == "" {
		return errors.New("未配置 NetProxy 原生组件路径")
	}
	moduleDir := filepath.Dir(filepath.Dir(options.Root))
	command := exec.CommandContext(ctx, options.NativePath,
		"module", "service", "reload",
		"--module-dir", moduleDir,
		"--catalog-root", options.Root,
		"--module-config", options.ModuleConf,
		"--sing-box", options.SingBoxPath,
		"--service-address", options.ServiceAddress,
		"--service-secret", options.ServiceSecret,
	)
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Run(); err != nil {
		return fmt.Errorf("服务 reload 失败: %w", err)
	}
	return nil
}

func validateOptions(options Options) error {
	for name, value := range map[string]string{"Catalog 根目录": options.Root, "PID 文件": options.PIDFile, "模块配置": options.ModuleConf} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s不能为空", name)
		}
	}
	if options.Now == nil {
		return errors.New("Worker 时钟不能为空")
	}
	return nil
}

func acquirePID(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock := path + ".lock"
	if err := os.Mkdir(lock, 0o700); err != nil {
		if os.IsExist(err) {
			owner := readPID(filepath.Join(lock, "pid"))
			if owner > 0 && isProcessRunningPID(owner) {
				return errors.New("订阅 Worker 已在运行")
			}
			_ = os.RemoveAll(lock)
			if err = os.Mkdir(lock, 0o700); err != nil {
				return err
			}
		}
		if !os.IsExist(err) && err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		_ = os.RemoveAll(lock)
		return err
	}
	if pid := readPID(path); pid > 0 && pid != os.Getpid() && isWorkerProcessPID(pid) {
		_ = os.RemoveAll(lock)
		return fmt.Errorf("订阅 Worker 已在运行: %d", pid)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		_ = os.RemoveAll(lock)
		return err
	}
	return nil
}

func releasePID(path string) {
	if readPID(path) == os.Getpid() {
		_ = os.Remove(path)
	}
	_ = os.RemoveAll(path + ".lock")
}

func readPID(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	_, _ = fmt.Sscanf(string(content), "%d", &pid)
	return pid
}

// ReadStatus 返回 Worker 当前状态，不会启动 Worker。
func ReadStatus(options Options) (Status, error) {
	if err := validateOptions(options); err != nil {
		return Status{}, err
	}
	pid := readPID(options.PIDFile)
	if pid <= 0 || !isWorkerProcessPID(pid) {
		return Status{State: "stopped"}, nil
	}
	nearest, err := NextUpdate(options.Root, options.Now())
	if err != nil {
		return Status{}, err
	}
	return Status{State: "running", PID: pid, Nearest: nearest}, nil
}
