package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

// Options 描述控制面访问所需的模块路径和 Service API 参数。
type Options struct {
	CatalogRoot    string
	ModuleConfig   string
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
	State                 string `json:"state"`
	PID                   *int   `json:"pid"`
	StartedAt             int64  `json:"started_at"`
	ReadyAt               int64  `json:"ready_at"`
	UptimeSeconds         int64  `json:"uptime_seconds"`
	Error                 string `json:"error"`
	OutboundMode          string `json:"outbound_mode"`
	SelectorMode          string `json:"selector_mode"`
	ActiveGroupID         string `json:"active_group_id"`
	ActiveGroupName       string `json:"active_group_name"`
	ActiveGroupNodeCount  int    `json:"active_group_node_count"`
	SelectedNodeRef       string `json:"selected_node_ref"`
	RuntimeSelected       string `json:"runtime_selected"`
	MemoryBytes           uint64 `json:"memory_bytes"`
	ProcessCPUTicks       uint64 `json:"process_cpu_ticks"`
	SystemCPUTicks        uint64 `json:"system_cpu_ticks"`
	CPUCount              int    `json:"cpu_count"`
	ConnectionsIn         int32  `json:"connections_in"`
	ConnectionsOut        int32  `json:"connections_out"`
	UploadTotal           int64  `json:"upload_total"`
	DownloadTotal         int64  `json:"download_total"`
	SubscriptionWorker    string `json:"subscription_worker"`
	SubscriptionWorkerPID *int   `json:"subscription_worker_pid"`
}

// DelayResult 是一次节点测速请求及其最新分组状态。
type DelayResult struct {
	Target string             `json:"target"`
	Groups []serviceapi.Group `json:"groups"`
}

// Selection 描述持久化选择与运行时实际选择。
type Selection struct {
	ActiveGroupID         string `json:"active_group_id"`
	ActiveGroupName       string `json:"active_group_name"`
	ActiveGroupRuntimeTag string `json:"active_group_runtime_tag"`
	ActiveGroupNodeCount  int    `json:"active_group_node_count"`
	SelectorMode          string `json:"selector_mode"`
	SelectedNodeRef       string `json:"selected_node_ref"`
	Selected              string `json:"selected"`
	RuntimeSelected       string `json:"runtime_selected"`
}

// Snapshot 描述持久化节点与运行时节点组的合并快照。
type Snapshot struct {
	Groups        []catalog.GroupSnapshot `json:"groups"`
	Selection     Selection               `json:"selection"`
	RuntimeGroups []serviceapi.Group      `json:"runtime_groups,omitempty"`
}

// ModeState 描述持久化出站模式以及核心当前模式。
type ModeState struct {
	Mode        string   `json:"mode"`
	RuntimeMode string   `json:"runtime_mode,omitempty"`
	Available   []string `json:"available"`
}

type stateFile struct {
	State     string `json:"state"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"started_at"`
	ReadyAt   int64  `json:"ready_at"`
	Error     string `json:"error"`
}

// ReadStatus 读取模块状态并在服务就绪时合并 Service API 快照。
func ReadStatus(ctx context.Context, options Options) (Status, error) {
	options = normalizeOptions(options)
	state := readState(options.StateFile)
	status := Status{
		State:              state.State,
		StartedAt:          state.StartedAt,
		ReadyAt:            state.ReadyAt,
		Error:              state.Error,
		OutboundMode:       readConfig(options.ModuleConfig, "OUTBOUND_MODE", "rule"),
		SelectorMode:       readConfig(options.ModuleConfig, "SELECTOR_MODE", "urltest"),
		ActiveGroupID:      readConfig(options.ModuleConfig, "ACTIVE_GROUP_ID", ""),
		SelectedNodeRef:    readConfig(options.ModuleConfig, "SELECTED_NODE_REF", ""),
		CPUCount:           1,
		SubscriptionWorker: "stopped",
	}

	_, active := readActiveGroup(ctx, options)
	if active != nil {
		status.ActiveGroupName = active.Group.Name
		status.ActiveGroupNodeCount = active.Group.NodeCount
	}
	if status.ActiveGroupName == "" {
		status.ActiveGroupName = status.ActiveGroupID
	}

	pid := findProcess(options.SingBoxPath, state.PID)
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
		status.ProcessCPUTicks = processCPUTicks(pid)
	}
	status.SystemCPUTicks, status.CPUCount = systemCPUTicks()

	if status.State == "ready" && status.ReadyAt > 0 {
		if elapsed := time.Now().Unix() - status.ReadyAt; elapsed >= 0 {
			status.UptimeSeconds = elapsed
		}
	}

	if worker, err := readWorkerStatus(options); err == nil {
		status.SubscriptionWorker = worker.State
		if worker.State == "running" && worker.PID > 0 {
			pid := worker.PID
			status.SubscriptionWorkerPID = &pid
		}
	}

	if status.State == "ready" {
		mergeRuntimeStatus(ctx, options, &status, active)
	}
	return status, nil
}

// ReadGroups 读取 Service API 当前的节点组和测速状态。
func ReadGroups(ctx context.Context, options Options) ([]serviceapi.Group, error) {
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer client.Close()
	return client.Groups(requestContext)
}

// ReadNodes 读取 Catalog 节点组，不依赖 sing-box 是否运行。
func ReadNodes(ctx context.Context, options Options, groupID string) ([]catalog.GroupSnapshot, error) {
	options = normalizeOptions(options)
	if strings.TrimSpace(options.CatalogRoot) == "" {
		return nil, errors.New("Catalog 根目录不能为空")
	}
	if strings.TrimSpace(groupID) != "" {
		resolved, err := catalog.ResolveGroup(options.CatalogRoot, groupID)
		if err != nil {
			return nil, err
		}
		groupID = resolved
	}
	return catalog.Scan(ctx, catalog.ScanOptions{
		Root: options.CatalogRoot, GroupID: groupID,
		ActiveGroup: readConfig(options.ModuleConfig, "ACTIVE_GROUP_ID", ""),
		ProgressDir: options.ProgressDir, WithNodes: true,
	})
}

// ReadSelection 读取当前分组、选择模式和运行时实际节点。
func ReadSelection(ctx context.Context, options Options) (Selection, error) {
	options = normalizeOptions(options)
	groups, err := catalog.Scan(ctx, catalog.ScanOptions{
		Root:        options.CatalogRoot,
		ActiveGroup: readConfig(options.ModuleConfig, "ACTIVE_GROUP_ID", ""),
		ProgressDir: options.ProgressDir, WithNodes: true,
	})
	if err != nil {
		return Selection{}, err
	}
	return selectionFromGroups(ctx, options, groups), nil
}

// ReadSnapshot 读取持久化节点并尽力合并运行时 Service API 状态。
func ReadSnapshot(ctx context.Context, options Options, groupID string) (Snapshot, error) {
	options = normalizeOptions(options)
	allGroups, err := ReadNodes(ctx, options, "")
	if err != nil {
		return Snapshot{}, err
	}
	runtimeGroups, _ := readRuntimeGroups(ctx, options)
	selection := selectionFromRuntimeGroups(options, allGroups, runtimeGroups)
	groups := allGroups
	if strings.TrimSpace(groupID) != "" {
		groups, err = ReadNodes(ctx, options, groupID)
		if err != nil {
			return Snapshot{}, err
		}
	}
	return Snapshot{Groups: groups, Selection: selection, RuntimeGroups: runtimeGroups}, nil
}

// ReadMode 读取模块模式，并在核心运行时补充当前 Service API 模式。
func ReadMode(ctx context.Context, options Options) (ModeState, error) {
	options = normalizeOptions(options)
	state := ModeState{
		Mode:      normalizeModuleMode(readConfig(options.ModuleConfig, "OUTBOUND_MODE", "rule")),
		Available: []string{"rule", "global", "direct", "AllowAds"},
	}
	runtimeMode, err := readRuntimeMode(ctx, options)
	if err == nil {
		state.RuntimeMode = runtimeMode
	}
	return state, nil
}

// ReadRuntimeMode 读取 Service API 当前出站模式。
func ReadRuntimeMode(ctx context.Context, options Options) (string, error) {
	return readRuntimeMode(ctx, normalizeOptions(options))
}

// SetMode 将模块模式映射为 Service API 模式并提交。
func SetMode(ctx context.Context, options Options, mode string) error {
	runtimeMode, err := moduleModeToServiceMode(mode)
	if err != nil {
		return err
	}
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return err
	}
	defer cancel()
	defer client.Close()
	return client.SetMode(requestContext, runtimeMode)
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

// Delay 发起测速并返回最新的节点组状态。
func Delay(ctx context.Context, options Options, target, group string) (DelayResult, error) {
	options = normalizeOptions(options)
	resolved, err := resolveDelayTarget(ctx, options, target, group)
	if err != nil {
		return DelayResult{}, err
	}
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return DelayResult{}, err
	}
	defer cancel()
	defer client.Close()
	if err := client.URLTest(requestContext, resolved); err != nil {
		return DelayResult{}, fmt.Errorf("节点测速请求失败: %w", err)
	}
	// URLTest 是异步请求，给 Service API 一个很短的窗口写入最新延迟。
	select {
	case <-ctx.Done():
		return DelayResult{}, ctx.Err()
	case <-time.After(300 * time.Millisecond):
	}
	groups, err := client.Groups(requestContext)
	if err != nil {
		return DelayResult{}, fmt.Errorf("读取测速结果失败: %w", err)
	}
	return DelayResult{Target: resolved, Groups: groups}, nil
}

func normalizeOptions(options Options) Options {
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
		options.ProgressDir = "/dev/netproxy/subscriptions"
	}
	if options.WorkerPIDFile == "" {
		options.WorkerPIDFile = "/dev/netproxy/worker.pid"
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

func readConfig(path, key, fallback string) string {
	if path == "" {
		return fallback
	}
	config, err := moduleconfig.LoadModule(path)
	if err != nil {
		return fallback
	}
	switch key {
	case "AUTO_START":
		return boolString(config.AutoStart)
	case "OUTBOUND_MODE":
		return config.OutboundMode
	case "SELECTOR_MODE":
		return config.SelectorMode
	case "ACTIVE_GROUP_ID":
		return config.ActiveGroupID
	case "SELECTED_NODE_REF":
		return config.SelectedNodeRef
	case "WIFI_AUTO_SWITCH":
		return boolString(config.WiFiAutoSwitch)
	case "WIFI_SSID_MODE":
		return config.WiFiSSIDMode
	case "WIFI_SSID_LIST":
		return config.WiFiSSIDList
	case "PROXY_ON_CELLULAR":
		return boolString(config.ProxyOnCellular)
	default:
		return fallback
	}
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func selectionFromGroups(ctx context.Context, options Options, groups []catalog.GroupSnapshot) Selection {
	runtimeGroups, _ := readRuntimeGroups(ctx, options)
	return selectionFromRuntimeGroups(options, groups, runtimeGroups)
}

func selectionFromRuntimeGroups(options Options, groups []catalog.GroupSnapshot, runtimeGroups []serviceapi.Group) Selection {
	activeID := readConfig(options.ModuleConfig, "ACTIVE_GROUP_ID", "")
	selector := normalizeModuleSelector(readConfig(options.ModuleConfig, "SELECTOR_MODE", "urltest"))
	selection := Selection{
		ActiveGroupID:   activeID,
		SelectorMode:    selector,
		SelectedNodeRef: readConfig(options.ModuleConfig, "SELECTED_NODE_REF", ""),
	}
	for _, group := range groups {
		if group.Group.ID != activeID {
			continue
		}
		selection.ActiveGroupName = group.Group.Name
		selection.ActiveGroupRuntimeTag = group.Group.RuntimeTag
		selection.ActiveGroupNodeCount = group.Group.NodeCount
		if group.Group.NodeCount == 0 {
			selection.Selected = ""
		} else if selector == "urltest" {
			selection.Selected = "Auto/" + group.Group.RuntimeTag
		} else {
			selection.Selected = selection.SelectedNodeRef
		}
		break
	}
	if selection.ActiveGroupName == "" {
		selection.ActiveGroupName = activeID
	}
	if selection.ActiveGroupRuntimeTag != "" {
		runtimeGroup := "Auto/" + selection.ActiveGroupRuntimeTag
		if selector == "manual" {
			runtimeGroup = "Select/" + selection.ActiveGroupRuntimeTag
		}
		for _, group := range runtimeGroups {
			if group.Tag == runtimeGroup {
				selection.RuntimeSelected = group.Selected
				break
			}
		}
	}
	return selection
}

func readRuntimeGroups(ctx context.Context, options Options) ([]serviceapi.Group, error) {
	options.RequestTimeout = 500 * time.Millisecond
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer client.Close()
	return client.Groups(requestContext)
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
	return serviceModeToModuleMode(mode.Current)
}

func normalizeModuleSelector(value string) string {
	if value == "manual" || value == "selector" {
		return "manual"
	}
	return "urltest"
}

func normalizeModuleMode(value string) string {
	switch value {
	case "rule", "global", "direct", "AllowAds":
		return value
	default:
		return "rule"
	}
}

func moduleModeToServiceMode(value string) (string, error) {
	switch value {
	case "rule":
		return "Rule", nil
	case "global":
		return "Global", nil
	case "direct":
		return "Direct", nil
	case "AllowAds":
		return "AllowAds", nil
	default:
		return "", fmt.Errorf("未知出站模式: %s", value)
	}
}

func serviceModeToModuleMode(value string) (string, error) {
	switch value {
	case "Rule":
		return "rule", nil
	case "Global":
		return "global", nil
	case "Direct":
		return "direct", nil
	case "AllowAds":
		return "AllowAds", nil
	default:
		return "", fmt.Errorf("未知 Service API 模式: %s", value)
	}
}

func readActiveGroup(ctx context.Context, options Options) ([]catalog.GroupSnapshot, *catalog.GroupSnapshot) {
	if options.CatalogRoot == "" {
		return nil, nil
	}
	groups, err := catalog.Scan(ctx, catalog.ScanOptions{
		Root: options.CatalogRoot, ActiveGroup: readConfig(options.ModuleConfig, "ACTIVE_GROUP_ID", ""),
		ProgressDir: options.ProgressDir, WithNodes: true,
	})
	if err != nil {
		return nil, nil
	}
	activeID := readConfig(options.ModuleConfig, "ACTIVE_GROUP_ID", "")
	for index := range groups {
		if groups[index].Group.ID == activeID {
			return groups, &groups[index]
		}
	}
	return groups, nil
}

func mergeRuntimeStatus(ctx context.Context, options Options, status *Status, active *catalog.GroupSnapshot) {
	if active == nil {
		return
	}
	runtimeTag := active.Group.RuntimeTag
	if runtimeTag == "" {
		return
	}
	selector := status.SelectorMode
	if selector == "auto" {
		selector = "urltest"
	}
	runtimeGroup := "Auto/" + runtimeTag
	if selector == "manual" {
		runtimeGroup = "Select/" + runtimeTag
	}
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return
	}
	defer cancel()
	defer client.Close()
	apiStatus, err := client.Status(requestContext)
	if err != nil {
		return
	}
	status.MemoryBytes = apiStatus.Memory
	status.ConnectionsIn = apiStatus.ConnectionsIn
	status.ConnectionsOut = apiStatus.ConnectionsOut
	status.UploadTotal = apiStatus.UplinkTotal
	status.DownloadTotal = apiStatus.DownlinkTotal
	groups, err := client.Groups(requestContext)
	if err != nil {
		return
	}
	for _, group := range groups {
		if group.Tag != runtimeGroup {
			continue
		}
		status.RuntimeSelected = group.Selected
		if status.RuntimeSelected == "" && len(active.Nodes) == 1 {
			status.RuntimeSelected = active.Group.RuntimeTag + "/" + active.Nodes[0].Tag
		}
		break
	}
}

func resolveDelayTarget(ctx context.Context, options Options, target, group string) (string, error) {
	activeID := readConfig(options.ModuleConfig, "ACTIVE_GROUP_ID", "")
	selector := readConfig(options.ModuleConfig, "SELECTOR_MODE", "urltest")
	selected := readConfig(options.ModuleConfig, "SELECTED_NODE_REF", "")
	if target == "" {
		group = activeID
		if selector == "manual" {
			return runtimeNodeRef(ctx, options.CatalogRoot, selected)
		}
		target = "auto"
	}
	if target == "all" {
		target = "auto"
	}
	if target == "auto" {
		if group == "" {
			group = activeID
		}
		resolvedGroup, err := catalog.ResolveGroup(options.CatalogRoot, group)
		if err != nil {
			return "", err
		}
		runtimeTag, err := catalog.RuntimeTag(options.CatalogRoot, resolvedGroup)
		if err != nil {
			return "", err
		}
		return "Auto/" + runtimeTag, nil
	}
	return runtimeNodeRef(ctx, options.CatalogRoot, target)
}

func runtimeNodeRef(ctx context.Context, root, reference string) (string, error) {
	groupID, tag, found := strings.Cut(reference, "/")
	if !found || groupID == "" || tag == "" {
		return "", errors.New("节点引用格式应为 <group-id>/<tag>")
	}
	runtimeTag, err := catalog.RuntimeTag(root, groupID)
	if err != nil {
		return "", err
	}
	present, err := catalog.GroupContainsTag(ctx, root, groupID, tag)
	if err != nil {
		return "", err
	}
	if !present {
		return "", fmt.Errorf("未找到节点: %s", reference)
	}
	return runtimeTag + "/" + tag, nil
}

func readWorkerStatus(options Options) (worker.Status, error) {
	workerOptions := worker.NewOptions(options.CatalogRoot)
	workerOptions.ProgressDir = options.ProgressDir
	workerOptions.PIDFile = options.WorkerPIDFile
	workerOptions.ModuleConf = options.ModuleConfig
	return worker.ReadStatus(workerOptions)
}

func findProcess(executable string, statePID int) int {
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
	return findProcess(executable, 0) > 0
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
	command := strings.SplitN(string(content), "\x00", 2)[0]
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
	return filepath.Base(candidate) == filepath.Base(target)
}

func processCPUTicks(pid int) uint64 {
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	end := strings.LastIndexByte(string(content), ')')
	if end < 0 || end+2 >= len(content) {
		return 0
	}
	fields := strings.Fields(string(content)[end+2:])
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
	for _, line := range strings.Split(string(content), "\n") {
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
		if strings.HasPrefix(fields[0], "cpu") {
			if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu")); err == nil {
				cpuCount++
			}
		}
	}
	if cpuCount == 0 {
		cpuCount = 1
	}
	return total, cpuCount
}
