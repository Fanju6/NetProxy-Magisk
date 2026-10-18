package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

const unknownOutboundMode = "unknown"

var serviceFindProcess = FindProcess

// Options 描述控制面访问所需的模块路径和 Service API 参数。
type Options struct {
	CatalogRoot    string
	ModuleConfig   string
	SingBoxConfig  string
	InboundConfig  string
	StateFile      string
	ProgressDir    string
	WorkerPIDFile  string
	SingBoxPath    string
	ServiceAddress string
	ServiceSecret  string
	RequestTimeout time.Duration
}

// Status 是仪表盘使用的服务状态快照，字段与 netproxyctl schema=1 保持一致。
type Status struct {
	State                  string   `json:"state"`
	ConfiguredBackend      string   `json:"configured_backend"`
	ActiveBackend          *string  `json:"active_backend"`
	PID                    *int     `json:"pid"`
	StartedAt              int64    `json:"started_at"`
	CoreStartedAtMillis    int64    `json:"core_started_at_millis"`
	ReadyAt                int64    `json:"ready_at"`
	UptimeSeconds          int64    `json:"uptime_seconds"`
	Error                  string   `json:"error"`
	OutboundMode           string   `json:"outbound_mode"`
	ConfiguredOutboundMode string   `json:"configured_outbound_mode"`
	AvailableOutboundModes []string `json:"available_outbound_modes"`
	DefaultOutbound        string   `json:"default_outbound"`
	RuntimeSelected        string   `json:"runtime_selected"`
	MemoryBytes            uint64   `json:"memory_bytes"`
	ProcessCPUTicks        uint64   `json:"process_cpu_ticks"`
	SystemCPUTicks         uint64   `json:"system_cpu_ticks"`
	CPUCount               int      `json:"cpu_count"`
	ConnectionsIn          int32    `json:"connections_in"`
	ConnectionsOut         int32    `json:"connections_out"`
	UploadTotal            int64    `json:"upload_total"`
	DownloadTotal          int64    `json:"download_total"`
	WorkerState            string   `json:"worker_state"`
	WorkerPID              *int     `json:"worker_pid"`
}

// Error 是控制操作可以直接返回给 netproxyctl 的结构化错误。
type Error struct {
	Code    string
	Message string
	Data    any
}

func (e *Error) Error() string {
	return e.Message
}

// Snapshot contains only the persistent Catalog nodes.
type Snapshot struct {
	Groups []catalog.GroupSnapshot `json:"groups"`
}

// ModeState 描述持久化出站模式以及核心当前模式。
type ModeState struct {
	Mode        string   `json:"mode"`
	RuntimeMode string   `json:"runtime_mode,omitempty"`
	Available   []string `json:"available"`
}

type stateFile struct {
	State               string `json:"state"`
	PID                 int    `json:"pid"`
	StartedAt           int64  `json:"started_at"`
	ReadyAt             int64  `json:"ready_at"`
	Error               string `json:"error"`
	ActiveBackend       string `json:"active_backend"`
	CoreStartedAtMillis int64  `json:"core_started_at_millis"`
}

// ReadStatus 读取模块状态并在服务就绪时合并 Service API 快照。
func ReadStatus(ctx context.Context, options Options) (Status, error) {
	options = normalizeOptions(options)
	state := readState(options.StateFile)
	modes, err := moduleconfig.LoadModes(options.SingBoxConfig)
	if err != nil {
		return Status{}, err
	}
	defaultOutbound, err := ReadDefaultOutbound(options.SingBoxConfig)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		State:                  state.State,
		StartedAt:              state.StartedAt,
		ReadyAt:                state.ReadyAt,
		Error:                  state.Error,
		OutboundMode:           unknownOutboundMode,
		ConfiguredOutboundMode: modes.Mode,
		AvailableOutboundModes: modes.Available,
		DefaultOutbound:        defaultOutbound,
		CPUCount:               1,
		WorkerState:            "stopped",
	}
	if options.InboundConfig != "" {
		config, err := inbound.Load(options.InboundConfig)
		if err != nil {
			return Status{}, err
		}
		status.ConfiguredBackend = config.Backend
	}

	pid := serviceFindProcess(options.SingBoxPath, state.PID)
	if pid <= 0 {
		status.PID = nil
		if state.State == "preparing" || state.State == "starting" || state.State == "ready" || state.State == "stopping" {
			status.State = "failed"
			if status.Error == "" {
				status.Error = "sing-box 进程已退出"
			}
		}
	} else {
		status.PID = &pid
		if status.State == "stopped" || state.PID != pid {
			status.State = "starting"
		}
	}
	status.ProcessCPUTicks, status.SystemCPUTicks, status.CPUCount = CPUUsage(pid)

	if status.State == "ready" && status.ReadyAt > 0 {
		if elapsed := time.Now().Unix() - status.ReadyAt; elapsed >= 0 {
			status.UptimeSeconds = elapsed
		}
	}

	if worker, err := readWorkerStatus(ctx, options); err == nil {
		status.WorkerState = worker.State
		if worker.State == "running" && worker.PID > 0 {
			pid := worker.PID
			status.WorkerPID = &pid
		}
	}

	if status.State == "ready" {
		mergeRuntimeStatus(ctx, options, &status, state)
	} else if status.State == "stopped" {
		// 服务未运行时没有核心实时模式，展示持久化配置作为下一次启动模式。
		status.OutboundMode = status.ConfiguredOutboundMode
	}
	return status, ctx.Err()
}

// CPUUsage 采集进程与系统 CPU 计数，供普通快照和共享事件流使用。
func CPUUsage(pid int) (uint64, uint64, int) {
	system, count := systemCPUTicks()
	var process uint64
	if pid > 0 {
		process = processCPUTicks(pid)
	}
	return process, system, count
}

// ReadNodes 读取 Catalog 节点组，不依赖 sing-box 是否运行。
func ReadNodes(ctx context.Context, options Options, groupID string) ([]catalog.GroupSnapshot, error) {
	options = normalizeOptions(options)
	return readNodes(ctx, options, groupID)
}

func readNodes(ctx context.Context, options Options, groupID string) ([]catalog.GroupSnapshot, error) {
	if strings.TrimSpace(options.CatalogRoot) == "" {
		return nil, errors.New("Catalog 根目录不能为空")
	}
	if strings.TrimSpace(groupID) != "" {
		resolved, err := catalog.ResolveGroup(ctx, options.CatalogRoot, groupID)
		if err != nil {
			return nil, err
		}
		groupID = resolved
	}
	return catalog.Scan(ctx, catalog.ScanOptions{
		Root: options.CatalogRoot, GroupID: groupID,
		ProgressDir: options.ProgressDir, WithNodes: true,
	})
}

// ReadSnapshot reads the persistent Catalog without runtime or selection state.
func ReadSnapshot(ctx context.Context, options Options, groupID string) (Snapshot, error) {
	groups, err := ReadNodes(ctx, options, groupID)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Groups: groups}, nil
}

// ReadMode 读取主配置模式，并在核心运行时补充当前 Service API 模式。
func ReadMode(ctx context.Context, options Options) (ModeState, error) {
	options = normalizeOptions(options)
	modes, err := moduleconfig.LoadModes(options.SingBoxConfig)
	if err != nil {
		return ModeState{}, err
	}
	state := ModeState{Mode: modes.Mode, Available: modes.Available}
	if ProcessRunning(options.SingBoxPath) {
		state.RuntimeMode, _ = readRuntimeMode(ctx, options)
	}
	return state, ctx.Err()
}

// ReadRuntimeMode 读取 Service API 当前出站模式。
func ReadRuntimeMode(ctx context.Context, options Options) (string, error) {
	return readRuntimeMode(ctx, normalizeOptions(options))
}

// SetMode 提交内核原生模式，并确认内核没有静默拒绝该模式。
func SetMode(ctx context.Context, options Options, mode string) error {
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return err
	}
	defer cancel()
	defer client.Close()
	if err := client.SetMode(requestContext, mode); err != nil {
		return err
	}
	actual, err := client.Mode(requestContext)
	if err != nil {
		return err
	}
	if actual.Current != mode {
		return fmt.Errorf("核心模式未生效: 期望 %s，实际 %s", mode, actual.Current)
	}
	return nil
}

// CloseAllConnections 关闭核心当前维护的全部连接。
func CloseAllConnections(ctx context.Context, options Options) error {
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return err
	}
	defer cancel()
	defer client.Close()
	return client.CloseAllConnections(requestContext)
}

func normalizeOptions(options Options) Options {
	layout := paths.Default()
	if options.SingBoxConfig == "" {
		options.SingBoxConfig = paths.SingBoxConfig(layout.SingBoxDir())
	}
	if options.ServiceAddress == "" {
		options.ServiceAddress = "127.0.0.1:9090"
	}
	if options.ServiceSecret == "" {
		options.ServiceSecret = "singbox"
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 8 * time.Second
	}
	if options.ProgressDir == "" {
		options.ProgressDir = layout.ProgressDir()
	}
	if options.WorkerPIDFile == "" {
		options.WorkerPIDFile = layout.WorkerPID()
	}
	return options
}

func newClient(ctx context.Context, options Options) (*serviceapi.Client, context.Context, context.CancelFunc, error) {
	options = normalizeOptions(options)
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err != nil {
		return nil, nil, nil, err
	}
	requestContext, cancel := context.WithTimeout(ctx, options.RequestTimeout)
	return client, requestContext, cancel, nil
}

func readState(path string) stateFile {
	state := stateFile{State: "stopped"}
	if path == "" {
		return state
	}
	content, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(content, &state) != nil {
		return stateFile{State: "stopped"}
	}
	if state.State == "" {
		state.State = "stopped"
	}
	return state
}

func readRuntimeMode(ctx context.Context, options Options) (string, error) {
	options.RequestTimeout = 500 * time.Millisecond
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return "", err
	}
	defer cancel()
	defer client.Close()
	mode, err := client.Mode(requestContext)
	if err != nil {
		return "", err
	}
	if mode.Current == "" {
		return "", errors.New("核心未返回当前模式")
	}
	return mode.Current, nil
}

func mergeRuntimeStatus(ctx context.Context, options Options, status *Status, state stateFile) {
	status.OutboundMode = unknownOutboundMode
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return
	}
	defer cancel()
	defer client.Close()
	var (
		mode        serviceapi.Mode
		modeErr     error
		apiStatus   serviceapi.Status
		statusErr   error
		groups      []serviceapi.Group
		groupsErr   error
		waitGroup   sync.WaitGroup
		started     serviceapi.StartedAt
		identityErr error
	)
	confirmedIdentity := status.PID != nil && state.PID == *status.PID && state.CoreStartedAtMillis > 0 && (state.ActiveBackend == "ebpf" || state.ActiveBackend == "tun")
	if confirmedIdentity {
		waitGroup.Go(func() { started, identityErr = client.StartedAt(requestContext) })
	}
	waitGroup.Go(func() {
		modeContext, modeCancel := context.WithTimeout(requestContext, 500*time.Millisecond)
		defer modeCancel()
		mode, modeErr = client.Mode(modeContext)
	})
	waitGroup.Go(func() {
		apiStatus, statusErr = client.Status(requestContext)
	})
	if status.DefaultOutbound != "" {
		waitGroup.Go(func() {
			groups, groupsErr = client.Groups(requestContext)
		})
	}
	waitGroup.Wait()
	if confirmedIdentity && identityErr == nil && started.UnixMilli == state.CoreStartedAtMillis {
		status.CoreStartedAtMillis = started.UnixMilli
		backend := state.ActiveBackend
		status.ActiveBackend = &backend
	}

	if modeErr == nil && mode.Current != "" {
		status.OutboundMode = mode.Current
	}
	if statusErr != nil {
		return
	}
	status.MemoryBytes = apiStatus.Memory
	status.ConnectionsIn = apiStatus.ConnectionsIn
	status.ConnectionsOut = apiStatus.ConnectionsOut
	status.UploadTotal = apiStatus.UplinkTotal
	status.DownloadTotal = apiStatus.DownlinkTotal
	if status.DefaultOutbound == "" || !confirmedIdentity || identityErr != nil || started.UnixMilli != state.CoreStartedAtMillis || groupsErr != nil {
		return
	}
	current, err := client.StartedAt(requestContext)
	if err != nil || current.UnixMilli != state.CoreStartedAtMillis || readState(options.StateFile) != state {
		return
	}
	status.RuntimeSelected = RuntimeSelected(status.DefaultOutbound, groups)
}

// ReadDefaultOutbound reads the native route.final without assuming a policy tag.
func ReadDefaultOutbound(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var document *struct {
		Route struct {
			Final string `json:"final"`
		} `json:"route"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return "", err
	}
	if document == nil {
		return "", errors.New("native configuration must be an object")
	}
	return document.Route.Final, nil
}

// RuntimeSelected follows only confirmed members of the native default group.
func RuntimeSelected(defaultOutbound string, groups []serviceapi.Group) string {
	byTag := make(map[string]serviceapi.Group, len(groups))
	for _, group := range groups {
		if _, exists := byTag[group.Tag]; exists {
			return ""
		}
		byTag[group.Tag] = group
	}
	seen := make(map[string]bool)
	tag := defaultOutbound
	for {
		group, exists := byTag[tag]
		if !exists || seen[tag] || group.Selected == "" {
			return ""
		}
		seen[tag] = true
		var member *serviceapi.GroupItem
		for i := range group.Items {
			if group.Items[i].Tag == group.Selected {
				if member != nil {
					return ""
				}
				member = &group.Items[i]
			}
		}
		if member == nil {
			return ""
		}
		if _, nested := byTag[member.Tag]; nested {
			tag = member.Tag
			continue
		}
		if member.Type == "" || member.Type == "selector" || member.Type == "urltest" {
			return ""
		}
		return member.Tag
	}
}

func readWorkerStatus(ctx context.Context, options Options) (worker.Status, error) {
	workerOptions := worker.NewOptions(options.CatalogRoot)
	workerOptions.ProgressDir = options.ProgressDir
	workerOptions.PIDFile = options.WorkerPIDFile
	workerOptions.ModuleConfig = options.ModuleConfig
	return worker.ReadStatus(ctx, workerOptions)
}

// FindProcess 返回与指定可执行文件匹配的进程；statePID 可减少 /proc 扫描。
func FindProcess(executable string, statePID int) int {
	selfPID := os.Getpid()
	if statePID > 0 && statePID != selfPID && processExists(statePID) && (executable == "" || processMatches(statePID, executable)) {
		return statePID
	}
	if executable == "" {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || pid == selfPID {
			continue
		}
		if processMatches(pid, executable) {
			return pid
		}
	}
	return 0
}

// ProcessRunning 判断指定可执行文件是否正在运行。
func ProcessRunning(executable string) bool {
	return FindProcess(executable, 0) > 0
}

func processExists(pid int) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil
}

func processMatches(pid int, executable string) bool {
	if pid <= 0 || pid == os.Getpid() || executable == "" {
		return false
	}
	procPath := filepath.Join("/proc", strconv.Itoa(pid))
	if target, err := os.Readlink(filepath.Join(procPath, "exe")); err == nil {
		return executableMatches(target, executable)
	}
	content, err := os.ReadFile(filepath.Join(procPath, "cmdline"))
	if err != nil {
		return false
	}
	command, _, _ := strings.Cut(string(content), "\x00")
	return executableMatches(command, executable)
}

func executableMatches(candidate, target string) bool {
	candidate = strings.TrimSuffix(candidate, " (deleted)")
	candidate = filepath.Clean(candidate)
	target = filepath.Clean(target)
	if candidate == "." || target == "." || candidate == "" || target == "" {
		return false
	}
	if candidate == target {
		return true
	}
	return false
}

func processCPUTicks(pid int) uint64 {
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	_, processFields, found := strings.CutLast(string(content), ")")
	if !found {
		return 0
	}
	fields := strings.Fields(processFields)
	if len(fields) <= 12 {
		return 0
	}
	user, _ := strconv.ParseUint(fields[11], 10, 64)
	system, _ := strconv.ParseUint(fields[12], 10, 64)
	return user + system
}

func systemCPUTicks() (uint64, int) {
	content, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 1
	}
	var total uint64
	cpuCount := 0
	for line := range strings.SplitSeq(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "cpu" {
			if len(fields) > 1 {
				for _, value := range fields[1:] {
					part, parseErr := strconv.ParseUint(value, 10, 64)
					if parseErr == nil {
						total += part
					}
				}
			}
			continue
		}
		if after, ok := strings.CutPrefix(fields[0], "cpu"); ok {
			if _, err := strconv.Atoi(after); err == nil {
				cpuCount++
			}
		}
	}
	if cpuCount == 0 {
		cpuCount = 1
	}
	return total, cpuCount
}
