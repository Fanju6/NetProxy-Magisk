package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
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
	StateFile      string
	ProgressDir    string
	DelayDir       string
	WorkerPIDFile  string
	SingBoxPath    string
	ServiceAddress string
	ServiceSecret  string
	RequestTimeout time.Duration
}

// Status 是仪表盘使用的服务状态快照，字段与 netproxyctl schema=1 保持一致。
type Status struct {
	State                  string `json:"state"`
	PID                    *int   `json:"pid"`
	StartedAt              int64  `json:"started_at"`
	ReadyAt                int64  `json:"ready_at"`
	UptimeSeconds          int64  `json:"uptime_seconds"`
	Error                  string `json:"error"`
	OutboundMode           string `json:"outbound_mode"`
	ConfiguredOutboundMode string `json:"configured_outbound_mode"`
	SelectorMode           string `json:"selector_mode"`
	ActiveGroupID          string `json:"active_group_id"`
	ActiveGroupName        string `json:"active_group_name"`
	ActiveGroupNodeCount   int    `json:"active_group_node_count"`
	SelectedNodeRef        string `json:"selected_node_ref"`
	RuntimeSelected        string `json:"runtime_selected"`
	MemoryBytes            uint64 `json:"memory_bytes"`
	ProcessCPUTicks        uint64 `json:"process_cpu_ticks"`
	SystemCPUTicks         uint64 `json:"system_cpu_ticks"`
	CPUCount               int    `json:"cpu_count"`
	ConnectionsIn          int32  `json:"connections_in"`
	ConnectionsOut         int32  `json:"connections_out"`
	UploadTotal            int64  `json:"upload_total"`
	DownloadTotal          int64  `json:"download_total"`
	WorkerState            string `json:"worker_state"`
	WorkerPID              *int   `json:"worker_pid"`
}

// DelayResult 是一次节点测速请求及其最新分组状态。
type DelayResult struct {
	Target string             `json:"target"`
	Groups []serviceapi.Group `json:"groups"`
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
	module := readModuleConfig(options.ModuleConfig)
	status := Status{
		State:                  state.State,
		StartedAt:              state.StartedAt,
		ReadyAt:                state.ReadyAt,
		Error:                  state.Error,
		OutboundMode:           unknownOutboundMode,
		ConfiguredOutboundMode: module.OutboundMode,
		SelectorMode:           module.SelectorMode,
		ActiveGroupID:          module.ActiveGroupID,
		SelectedNodeRef:        module.SelectedNodeRef,
		CPUCount:               1,
		WorkerState:            "stopped",
	}

	active, activeErr := readActiveGroup(ctx, options, module.ActiveGroupID)
	if active != nil {
		status.ActiveGroupName = active.Group.Name
		status.ActiveGroupNodeCount = active.Group.NodeCount
	}
	if activeErr != nil {
		if status.Error == "" {
			status.Error = fmt.Sprintf("Catalog 活动分组状态不可用: %v", activeErr)
		} else {
			status.Error += "; Catalog 活动分组状态不可用: " + activeErr.Error()
		}
	}
	if status.ActiveGroupName == "" {
		status.ActiveGroupName = status.ActiveGroupID
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
		status.ProcessCPUTicks = processCPUTicks(pid)
	}
	status.SystemCPUTicks, status.CPUCount = systemCPUTicks()

	if status.State == "ready" && status.ReadyAt > 0 {
		if elapsed := time.Now().Unix() - status.ReadyAt; elapsed >= 0 {
			status.UptimeSeconds = elapsed
		}
	}

	if worker, err := readWorkerStatus(options); err == nil {
		status.WorkerState = worker.State
		if worker.State == "running" && worker.PID > 0 {
			pid := worker.PID
			status.WorkerPID = &pid
		}
	}

	if status.State == "ready" {
		mergeRuntimeStatus(ctx, options, &status, active)
	} else if status.State == "stopped" {
		// 服务未运行时没有核心实时模式，展示持久化配置作为下一次启动模式。
		status.OutboundMode = status.ConfiguredOutboundMode
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
	module := readModuleConfig(options.ModuleConfig)
	return readNodes(ctx, options, module, groupID, true)
}

func readNodes(ctx context.Context, options Options, module moduleconfig.ModuleConfig, groupID string, withNodes bool) ([]catalog.GroupSnapshot, error) {
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
		ActiveGroup: module.ActiveGroupID,
		ProgressDir: options.ProgressDir, WithNodes: withNodes,
	})
}

// ReadSelection 读取当前分组、选择模式和运行时实际节点。
func ReadSelection(ctx context.Context, options Options) (Selection, error) {
	options = normalizeOptions(options)
	module := readModuleConfig(options.ModuleConfig)
	groups, err := readNodes(ctx, options, module, "", false)
	if err != nil {
		return Selection{}, err
	}
	runtimeGroups, _ := readRuntimeGroups(ctx, options)
	return selectionFromRuntimeGroups(module, groups, runtimeGroups), nil
}

// ReadSnapshot 读取持久化节点并尽力合并运行时 Service API 状态。
func ReadSnapshot(ctx context.Context, options Options, groupID string) (Snapshot, error) {
	options = normalizeOptions(options)
	module := readModuleConfig(options.ModuleConfig)
	allGroups, err := readNodes(ctx, options, module, "", true)
	if err != nil {
		return Snapshot{}, err
	}
	runtimeGroups, _ := readRuntimeGroups(ctx, options)
	selection := selectionFromRuntimeGroups(module, allGroups, runtimeGroups)
	groups := allGroups
	if strings.TrimSpace(groupID) != "" {
		resolved, resolveErr := resolveSnapshotGroup(allGroups, groupID)
		if resolveErr != nil {
			return Snapshot{}, resolveErr
		}
		groups = groups[:0]
		for _, candidate := range allGroups {
			if candidate.Group.ID == resolved {
				groups = append(groups, candidate)
				break
			}
		}
	}
	return Snapshot{Groups: groups, Selection: selection, RuntimeGroups: runtimeGroups}, nil
}

func resolveSnapshotGroup(groups []catalog.GroupSnapshot, query string) (string, error) {
	for _, group := range groups {
		if group.Group.ID == query {
			return group.Group.ID, nil
		}
	}
	match := ""
	for _, group := range groups {
		if group.Group.Name != query {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("分组名称不唯一: %s", query)
		}
		match = group.Group.ID
	}
	if match == "" {
		return "", fmt.Errorf("分组不存在: %s", query)
	}
	return match, nil
}

// ReadMode 读取模块模式，并在核心运行时补充当前 Service API 模式。
func ReadMode(ctx context.Context, options Options) (ModeState, error) {
	options = normalizeOptions(options)
	module := readModuleConfig(options.ModuleConfig)
	state := ModeState{
		Mode:      normalizeModuleMode(module.OutboundMode),
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

// Delay 通过 Service API 发起异步测速，并返回当前可用的延迟快照。
func Delay(ctx context.Context, options Options, target, group string) (DelayResult, error) {
	options = normalizeOptions(options)
	request, err := resolveDelayRequest(ctx, options, target, group)
	if err != nil {
		return DelayResult{}, err
	}
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return DelayResult{}, delayAPIError(request.Target, err)
	}
	result, requestErr := delayWithClient(requestContext, client, request.Target, false)
	cancel()
	_ = client.Close()
	if requestErr == nil {
		return result, nil
	}
	if errors.Is(requestErr, context.DeadlineExceeded) || errors.Is(requestErr, context.Canceled) {
		return DelayResult{}, mapDelayRequestError(request.Target, requestErr)
	}
	if serviceAPIUnavailable(requestErr) && offlineDelayAllowed(options) {
		return offlineDelayRunner(ctx, options, request)
	}
	return DelayResult{}, mapDelayRequestError(request.Target, requestErr)
}

func delayWithClient(ctx context.Context, client *serviceapi.Client, target string, waitForResults bool) (DelayResult, error) {
	if err := client.URLTest(ctx, target); err != nil {
		return DelayResult{}, err
	}
	var outbounds []serviceapi.GroupItem
	var err error
	if waitForResults {
		outbounds, err = waitDelayResults(ctx, client, target)
	} else {
		// 正式核心持续维护测速缓存，短暂观察后即可读取本轮或最近一次有效结果。
		select {
		case <-ctx.Done():
			return DelayResult{}, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		outbounds, err = client.Outbounds(ctx)
	}
	if err != nil {
		return DelayResult{}, err
	}

	if strings.HasPrefix(target, "Auto/") || strings.HasPrefix(target, "Select/") {
		groupType := "urltest"
		if strings.HasPrefix(target, "Select/") {
			groupType = "selector"
		}
		runtimeTag := strings.TrimPrefix(strings.TrimPrefix(target, "Auto/"), "Select/")
		prefix := runtimeTag + "/"
		items := make([]serviceapi.GroupItem, 0)
		groupFound := false
		for _, item := range outbounds {
			switch {
			case item.Tag == target:
				groupFound = true
				if item.Type != "" {
					groupType = item.Type
				}
			case strings.HasPrefix(item.Tag, prefix):
				items = append(items, item)
			}
		}
		if !groupFound || len(items) == 0 {
			return DelayResult{}, delayTargetError(target)
		}
		return DelayResult{Target: target, Groups: []serviceapi.Group{{
			Tag: target, Type: groupType, Selectable: groupType == "selector", Items: items,
		}}}, nil
	}

	runtimeGroup, _, found := strings.Cut(target, "/")
	if !found {
		return DelayResult{}, delayTargetError(target)
	}
	items := make([]serviceapi.GroupItem, 0, 1)
	for _, item := range outbounds {
		if item.Tag == target {
			items = append(items, item)
			break
		}
	}
	if len(items) == 0 {
		return DelayResult{}, delayTargetError(target)
	}
	return DelayResult{
		Target: target,
		Groups: []serviceapi.Group{{Tag: "Auto/" + runtimeGroup, Items: items}},
	}, nil
}

func waitDelayResults(ctx context.Context, client *serviceapi.Client, target string) ([]serviceapi.GroupItem, error) {
	// Service API 最快每 250ms 推送一次测速变化，更高频轮询只会重复读取同一快照。
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	limit := time.NewTimer(15 * time.Second)
	defer limit.Stop()
	var latest []serviceapi.GroupItem
	completedCount := 0
	lastProgress := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-limit.C:
			return latest, nil
		case <-poll.C:
		}
		current, err := client.Outbounds(ctx)
		if err != nil {
			return nil, err
		}
		latest = current
		expected, completed := delayResultProgress(target, current)
		if completed > completedCount {
			completedCount = completed
			lastProgress = time.Now()
		}
		if expected > 0 && completedCount >= expected {
			return latest, nil
		}
		if expected > 0 && completedCount*2 >= expected && !lastProgress.IsZero() && time.Since(lastProgress) >= 2*time.Second {
			return latest, nil
		}
	}
}

func delayResultProgress(target string, outbounds []serviceapi.GroupItem) (expected int, completed int) {
	groupTarget := strings.HasPrefix(target, "Auto/") || strings.HasPrefix(target, "Select/")
	prefix := strings.TrimPrefix(strings.TrimPrefix(target, "Auto/"), "Select/") + "/"
	for _, item := range outbounds {
		if (!groupTarget && item.Tag == target) || (groupTarget && strings.HasPrefix(item.Tag, prefix)) {
			expected++
			if item.URLTestTime > 0 {
				completed++
			}
		}
	}
	return
}

func normalizeOptions(options Options) Options {
	layout := paths.Default()
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
	if options.DelayDir == "" {
		options.DelayDir = layout.DelayDir()
	}
	if options.WorkerPIDFile == "" {
		options.WorkerPIDFile = layout.WorkerPID()
	}
	return options
}

func serviceAPIUnavailable(err error) bool {
	var requestError *url.Error
	return errors.As(err, &requestError)
}

func offlineDelayAllowed(options Options) bool {
	state := readState(options.StateFile).State
	return state == "stopped" || state == "failed"
}

func delayAPIError(target string, cause error) error {
	return &Error{
		Code:    "node.delay_api_failed",
		Message: fmt.Sprintf("节点测速 Service API 失败: %v", cause),
		Data: map[string]any{
			"target": target,
			"cause":  cause.Error(),
		},
	}
}

func delayTimeoutError(target string, cause error) error {
	return &Error{
		Code:    "node.delay_timeout",
		Message: "节点测速请求超时",
		Data: map[string]any{
			"target": target,
			"cause":  cause.Error(),
		},
	}
}

func delayTargetError(target string) error {
	return &Error{
		Code:    "node.delay_target_missing",
		Message: fmt.Sprintf("未找到测速目标 %s", target),
		Data:    map[string]any{"target": target},
	}
}

func mapDelayRequestError(target string, cause error) error {
	if structured, ok := errors.AsType[*Error](cause); ok {
		return structured
	}
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		return delayTimeoutError(target, cause)
	}
	return delayAPIError(target, cause)
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

func readModuleConfig(path string) moduleconfig.ModuleConfig {
	if path != "" {
		if config, err := moduleconfig.LoadModule(path); err == nil {
			return config
		}
	}
	return moduleconfig.DefaultModule()
}

func selectionFromRuntimeGroups(module moduleconfig.ModuleConfig, groups []catalog.GroupSnapshot, runtimeGroups []serviceapi.Group) Selection {
	selection := Selection{
		ActiveGroupID:   module.ActiveGroupID,
		SelectorMode:    module.SelectorMode,
		SelectedNodeRef: module.SelectedNodeRef,
	}
	for _, group := range groups {
		if group.Group.ID != module.ActiveGroupID {
			continue
		}
		selection.ActiveGroupName = group.Group.Name
		selection.ActiveGroupRuntimeTag = group.Group.RuntimeTag
		selection.ActiveGroupNodeCount = group.Group.NodeCount
		if group.Group.NodeCount == 0 {
			selection.Selected = ""
		} else if module.SelectorMode == "urltest" {
			selection.Selected = "Auto/" + group.Group.RuntimeTag
		} else {
			selection.Selected = selection.SelectedNodeRef
		}
		break
	}
	if selection.ActiveGroupName == "" {
		selection.ActiveGroupName = module.ActiveGroupID
	}
	if selection.ActiveGroupRuntimeTag != "" {
		runtimeGroup := "Auto/" + selection.ActiveGroupRuntimeTag
		if module.SelectorMode == "manual" {
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

func readActiveGroup(ctx context.Context, options Options, activeID string) (*catalog.GroupSnapshot, error) {
	if options.CatalogRoot == "" {
		return nil, nil
	}
	summary, err := catalog.ReadGroupSummary(ctx, options.CatalogRoot, activeID, options.ProgressDir)
	if err != nil {
		if summary.ID == "" {
			return nil, err
		}
		return &catalog.GroupSnapshot{Group: summary}, err
	}
	return &catalog.GroupSnapshot{Group: summary}, nil
}

func mergeRuntimeStatus(ctx context.Context, options Options, status *Status, active *catalog.GroupSnapshot) {
	status.OutboundMode = unknownOutboundMode
	client, requestContext, cancel, err := newClient(ctx, options)
	if err != nil {
		return
	}
	defer cancel()
	defer client.Close()
	var (
		mode      serviceapi.Mode
		modeErr   error
		apiStatus serviceapi.Status
		statusErr error
		groups    []serviceapi.Group
		groupsErr error
		waitGroup sync.WaitGroup
	)
	waitGroup.Go(func() {
		modeContext, modeCancel := context.WithTimeout(requestContext, 500*time.Millisecond)
		defer modeCancel()
		mode, modeErr = client.Mode(modeContext)
	})
	waitGroup.Go(func() {
		apiStatus, statusErr = client.Status(requestContext)
	})
	if active != nil && active.Group.RuntimeTag != "" {
		waitGroup.Go(func() {
			groups, groupsErr = client.Groups(requestContext)
		})
	}
	waitGroup.Wait()

	if modeErr == nil {
		if runtimeMode, mapErr := serviceModeToModuleMode(mode.Current); mapErr == nil {
			status.OutboundMode = runtimeMode
		}
	}
	if statusErr != nil {
		return
	}
	status.MemoryBytes = apiStatus.Memory
	status.ConnectionsIn = apiStatus.ConnectionsIn
	status.ConnectionsOut = apiStatus.ConnectionsOut
	status.UploadTotal = apiStatus.UplinkTotal
	status.DownloadTotal = apiStatus.DownlinkTotal
	if active == nil {
		return
	}
	runtimeTag := active.Group.RuntimeTag
	if runtimeTag == "" {
		return
	}
	runtimeGroup := "Auto/" + runtimeTag
	if status.SelectorMode == "manual" {
		runtimeGroup = "Select/" + runtimeTag
	}
	if groupsErr != nil {
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

type delayRequest struct {
	Target  string
	GroupID string
	NodeTag string
}

func resolveDelayRequest(ctx context.Context, options Options, target, group string) (delayRequest, error) {
	module := readModuleConfig(options.ModuleConfig)
	activeID := module.ActiveGroupID
	if target == "" {
		group = activeID
		if module.SelectorMode == "manual" {
			return runtimeNodeDelayRequest(ctx, options.CatalogRoot, module.SelectedNodeRef)
		}
		target = "auto"
	}
	if target == "auto" {
		if group == "" {
			group = activeID
		}
		resolvedGroup, err := resolveDelayGroup(ctx, options, group)
		if err != nil {
			return delayRequest{}, err
		}
		runtimeTag, err := catalog.RuntimeTag(options.CatalogRoot, resolvedGroup)
		if err != nil {
			return delayRequest{}, err
		}
		return delayRequest{Target: "Auto/" + runtimeTag, GroupID: resolvedGroup}, nil
	}
	if prefix, groupRef, found := strings.Cut(target, "/"); found && (prefix == "Auto" || prefix == "Select") {
		if groupRef == "" {
			return delayRequest{}, errors.New("测速分组引用格式应为 Auto/<group> 或 Select/<group>")
		}
		resolvedGroup, err := resolveDelayGroup(ctx, options, groupRef)
		if err != nil {
			return delayRequest{}, err
		}
		runtimeTag, err := catalog.RuntimeTag(options.CatalogRoot, resolvedGroup)
		if err != nil {
			return delayRequest{}, err
		}
		return delayRequest{Target: prefix + "/" + runtimeTag, GroupID: resolvedGroup}, nil
	}
	return runtimeNodeDelayRequest(ctx, options.CatalogRoot, target)
}

func resolveDelayGroup(ctx context.Context, options Options, query string) (string, error) {
	resolved, err := catalog.ResolveGroup(options.CatalogRoot, query)
	if err == nil {
		return resolved, nil
	}
	// 重名分组的运行时标签带有稳定 ID 后缀，ResolveGroup 只接受 ID 或显示名称，
	// 因此这里补充按已生成的 RuntimeTag 查找，确保 Auto/<group> 与实际标签一致。
	groups, scanErr := catalog.Scan(ctx, catalog.ScanOptions{Root: options.CatalogRoot})
	if scanErr != nil {
		return "", err
	}
	for _, group := range groups {
		if group.Group.RuntimeTag == query {
			return group.Group.ID, nil
		}
	}
	return "", err
}

func runtimeNodeDelayRequest(ctx context.Context, root, reference string) (delayRequest, error) {
	groupID, tag, found := strings.Cut(reference, "/")
	if !found || groupID == "" || tag == "" {
		return delayRequest{}, errors.New("节点引用格式应为 <group-id>/<tag>")
	}
	runtimeTag, err := catalog.RuntimeTag(root, groupID)
	if err != nil {
		return delayRequest{}, err
	}
	present, err := catalog.GroupContainsTag(ctx, root, groupID, tag)
	if err != nil {
		return delayRequest{}, err
	}
	if !present {
		return delayRequest{}, fmt.Errorf("未找到节点: %s", reference)
	}
	return delayRequest{Target: runtimeTag + "/" + tag, GroupID: groupID, NodeTag: tag}, nil
}

func readWorkerStatus(options Options) (worker.Status, error) {
	workerOptions := worker.NewOptions(options.CatalogRoot)
	workerOptions.ProgressDir = options.ProgressDir
	workerOptions.PIDFile = options.WorkerPIDFile
	workerOptions.ModuleConf = options.ModuleConfig
	return worker.ReadStatus(workerOptions)
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
