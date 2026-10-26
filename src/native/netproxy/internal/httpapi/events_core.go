package httpapi

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
)

// 状态文件由同目录原子替换；缓存共享解析结果，但每次发送仍核对当前路径。
type eventStateCache struct {
	mu    sync.Mutex
	info  os.FileInfo
	state moduleapp.ServiceState
}

func sameEventFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) &&
		a.ModTime() == b.ModTime() && a.Size() == b.Size() && a.Mode() == b.Mode()
}

func (c *eventStateCache) read(path string, read func(string) (moduleapp.ServiceState, error)) (moduleapp.ServiceState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	before, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		c.info = nil
		return moduleapp.ServiceState{Schema: 1, State: "stopped"}, nil
	}
	if err != nil {
		c.info = nil
		return moduleapp.ServiceState{}, err
	}
	if sameEventFile(before, c.info) {
		return c.state, nil
	}
	c.info = nil
	// Windows 的 Stat 延迟读取文件 ID，必须在读内容前固定身份，不能留到后置比较。
	if !os.SameFile(before, before) {
		return moduleapp.ServiceState{}, errors.New("无法确认服务状态文件身份")
	}
	state, err := read(path)
	if err != nil {
		return state, err
	}
	after, err := os.Stat(path)
	if err != nil {
		return moduleapp.ServiceState{}, err
	}
	// 不能把读到的旧 inode 内容绑定到替换后的文件，也不能缓存读取期间的写入。
	if !sameEventFile(before, after) {
		return moduleapp.ServiceState{}, errors.New("读取期间服务状态文件已变化")
	}
	c.info, c.state = after, state
	return state, nil
}

func (h *eventHub) readNative() (moduleapp.ServiceState, error) {
	return h.stateCache.read(h.server.options.StateFile, moduleapp.ReadServiceState)
}

// 仅验证记录中的 PID；不能让失败路径扫描 /proc 并误认另一实例。
func eventProcessMatches(executable string, pid int64) bool {
	if pid <= 0 || pid == int64(os.Getpid()) || int64(int(pid)) != pid || executable == "" {
		return false
	}
	proc := filepath.Join("/proc", strconv.FormatInt(pid, 10))
	target, err := os.Readlink(filepath.Join(proc, "exe"))
	if err != nil {
		content, err := os.ReadFile(filepath.Join(proc, "cmdline"))
		if err != nil {
			return false
		}
		target, _, _ = strings.Cut(string(content), "\x00")
	}
	return filepath.Clean(strings.TrimSuffix(target, " (deleted)")) == filepath.Clean(executable)
}

type coreFeed struct {
	h             *eventHub
	messages      chan coreEvent
	cancel        context.CancelFunc
	done          chan struct{}
	identity      int64
	generation    uint64
	saved         moduleapp.ServiceState
	groups        []serviceapi.Group
	groupsReady   bool
	snapshot      service.PolicySnapshot
	snapshotReady bool
	status        service.Status
	statusReady   bool
	groupTimer    *time.Timer
	groupFlush    <-chan time.Time
}

func newCoreFeed(h *eventHub) *coreFeed {
	return &coreFeed{h: h, messages: make(chan coreEvent, 8)}
}

func (f *coreFeed) stopStream() {
	if f.cancel != nil {
		f.cancel()
		<-f.done
		f.cancel, f.done = nil, nil
	}
}

func (f *coreFeed) clearGroupWindow() {
	if f.groupTimer != nil {
		f.groupTimer.Stop()
	}
	f.groupFlush = nil
}

func (f *coreFeed) close() {
	f.stopStream()
	f.clearGroupWindow()
}

func (f *coreFeed) reset(state moduleapp.ServiceState) {
	f.stopStream()
	f.clearGroupWindow()
	f.generation++
	f.identity, f.groups, f.groupsReady = 0, nil, false
	f.snapshotReady, f.statusReady = false, false
	f.saved = state
	f.h.epoch.Add(1)
	f.h.native.Store(&state)
	f.h.mu.Lock()
	for name, event := range f.h.latest {
		if event.topic != topicSubscriptions {
			delete(f.h.latest, name)
		}
	}
	f.h.mu.Unlock()
}

func (f *coreFeed) unavailable(message string) {
	f.h.emit(topicStatus, "traffic", contract.Success("api.runtime_status", message, map[string]any{"available": false}))
}

func (f *coreFeed) sync() bool {
	if f.h.interests()&(topicStatus|topicGroups) == 0 {
		if f.cancel != nil || f.identity != 0 {
			f.reset(f.saved)
		}
		return false
	}
	state, err := f.h.readNative()
	if err != nil {
		// 解析失败也必须撤销旧流门禁，不能用 stopped 默认值掩盖损坏文件。
		if f.h.native.Load() == nil || f.saved != (moduleapp.ServiceState{}) || f.cancel != nil {
			f.reset(moduleapp.ServiceState{})
			return true
		}
		return false
	}
	next := int64(0)
	if state.State == "ready" && state.CoreStartedAtMillis > 0 && eventProcessMatches(f.h.server.options.SingBoxPath, state.PID) {
		next = state.CoreStartedAtMillis
	}
	changed := state != f.saved || f.h.native.Load() == nil || next != f.identity
	if state != f.saved || f.h.native.Load() == nil || (f.identity != 0 && next != f.identity) {
		f.reset(state)
	}
	if next == f.identity {
		if changed && next == 0 {
			f.unavailable("核心未运行")
		}
		return changed
	}
	client, err := serviceapi.New(f.h.server.options.ServiceAddress, f.h.server.options.ServiceSecret)
	if err != nil {
		return changed
	}
	ctx, cancel := context.WithTimeout(f.h.ctx, time.Second)
	started, err := client.Ready(ctx)
	cancel()
	if err != nil || started.UnixMilli != next {
		client.Close()
		return changed
	}
	// Ready 也是异步读取：建立流前再次核对状态文件，拒绝校准期间的替换。
	current, err := f.h.readNative()
	if err != nil || current != state || f.h.ctx.Err() != nil {
		client.Close()
		return changed
	}
	f.reset(state)
	f.identity = next
	f.unavailable("核心控制接口正在校准")
	f.start(client)
	return true
}

func (f *coreFeed) start(client *serviceapi.Client) {
	ctx, cancel := context.WithCancel(f.h.ctx)
	f.cancel, f.done = cancel, make(chan struct{})
	generation, done := f.generation, f.done
	go func() {
		defer close(done)
		defer client.Close()
		send := func(value coreEvent) error {
			value.generation = generation
			select {
			case f.messages <- value:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		finish := func(err error) {
			if ctx.Err() == nil {
				if err == nil {
					err = io.EOF
				}
				_ = send(coreEvent{err: err})
				cancel()
			}
		}
		var tasks sync.WaitGroup
		tasks.Go(func() {
			finish(client.SubscribeStatus(ctx, time.Second, func(value serviceapi.Status) error {
				return send(coreEvent{status: &value})
			}))
		})
		tasks.Go(func() {
			finish(client.SubscribeGroups(ctx, func(value []serviceapi.Group) error {
				return send(coreEvent{groups: &value})
			}))
		})
		tasks.Go(func() {
			finish(client.SubscribeClashMode(ctx, func(value string) error {
				return send(coreEvent{mode: &value})
			}))
		})
		tasks.Wait()
	}()
}

func (f *coreFeed) refresh(topics int) {
	epoch := f.h.epoch.Load()
	ctx, cancel := context.WithTimeout(f.h.ctx, 3*time.Second)
	defer cancel()
	if topics&topicStatus != 0 {
		options := f.h.server.controlOptions()
		// 身份由 feed 验证；领域读取仅检查记录 PID，禁止 FindProcess 兜底扫描。
		options.SingBoxPath = ""
		data, err := service.ReadStatus(ctx, options)
		f.statusReady = err == nil
		if err != nil {
			response, _ := contract.Failure(ctx, err)
			f.h.emitAt(topicStatus, "status", response, epoch)
		} else {
			f.status = data
			if data.PID != nil && !eventProcessMatches(f.h.server.options.SingBoxPath, int64(*data.PID)) {
				f.status.PID, f.status.ProcessCPUTicks = nil, 0
				if data.State == "preparing" || data.State == "starting" || data.State == "ready" || data.State == "stopping" {
					f.status.State = "failed"
					if f.status.Error == "" {
						f.status.Error = "sing-box 进程已退出"
					}
				}
			}
			if f.identity == 0 {
				// 领域快照可读到尚未校准的 API，不能把旧实例的实时字段带入新身份。
				f.status.ActiveBackend = nil
				f.status.CoreStartedAtMillis = 0
				f.status.RuntimeSelected = ""
				f.status.MemoryBytes, f.status.ConnectionsIn, f.status.ConnectionsOut = 0, 0, 0
				f.status.UploadTotal, f.status.DownloadTotal = 0, 0
				if f.status.State != "stopped" {
					f.status.OutboundMode = "unknown"
				}
			}
			f.h.emitAt(topicStatus, "status", contract.Success("service.status", "服务状态", f.status), epoch)
		}
	}
	if topics&(topicStatus|topicGroups) != 0 {
		var snapshot service.PolicySnapshot
		var err error
		if f.identity > 0 {
			snapshot.DefaultOutbound, err = service.ReadDefaultOutbound(f.h.server.controlOptions().SingBoxConfig)
		} else {
			snapshot, err = service.ReadConfiguredPolicyGroups(ctx, f.h.server.controlOptions())
		}
		f.snapshotReady = err == nil
		if err != nil {
			if topics&topicGroups != 0 {
				response, _ := contract.Failure(ctx, err)
				f.h.emitAt(topicGroups, "groups", response, epoch)
			}
		} else {
			f.snapshot = snapshot
			f.publishGroups(epoch)
		}
	}
	if topics&topicSubscriptions != 0 {
		groups, err := f.h.server.catalogSummaries(ctx)
		if err != nil {
			response, _ := contract.Failure(ctx, err)
			f.h.emit(topicSubscriptions, "subscriptions", response)
		} else {
			f.h.emit(topicSubscriptions, "subscriptions", contract.Success("subscription.list", "订阅状态", groups))
		}
	}
}

func (f *coreFeed) publishGroups(epoch uint64) {
	if !f.snapshotReady {
		return
	}
	// A bound core must deliver its complete first groups frame before publication.
	if f.identity > 0 && !f.groupsReady {
		return
	}
	snapshot := f.snapshot
	if f.identity > 0 {
		snapshot.Groups = f.groups
		if snapshot.Groups == nil {
			snapshot.Groups = []serviceapi.Group{}
		}
		snapshot.Live = true
		snapshot.Source = ""
		snapshot.CoreStartedAtMillis = f.identity
	}
	topics := f.h.interests()
	if topics&topicGroups != 0 {
		f.h.emitAt(topicGroups, "groups", contract.Success("policy.groups", "策略组快照", snapshot), epoch)
	}
	if topics&topicStatus != 0 && f.statusReady {
		f.status.DefaultOutbound = snapshot.DefaultOutbound
		f.status.RuntimeSelected = ""
		if snapshot.Live {
			f.status.RuntimeSelected = service.RuntimeSelected(snapshot.DefaultOutbound, snapshot.Groups)
		}
		f.h.emitAt(topicStatus, "status", contract.Success("service.status", "服务状态", f.status), epoch)
	}
}

func (f *coreFeed) flushGroups() {
	f.groupFlush = nil
	if f.identity != 0 {
		f.publishGroups(f.h.epoch.Load())
	}
}

func (f *coreFeed) consume(event coreEvent) bool {
	if event.generation != f.generation || f.identity == 0 {
		return false
	}
	current, err := f.h.readNative()
	if err != nil || current != f.saved {
		return f.sync()
	}
	if event.err != nil {
		f.reset(current)
		f.unavailable("核心控制接口暂不可用")
		return true
	}
	if event.mode != nil {
		mode := *event.mode
		if mode == "" {
			mode = "unknown"
		}
		if f.statusReady {
			f.status.OutboundMode = mode
			f.h.emit(topicStatus, "status", contract.Success("service.status", "服务状态", f.status))
		}
	}
	if event.status != nil {
		value := *event.status
		f.status.MemoryBytes, f.status.ConnectionsIn, f.status.ConnectionsOut = value.Memory, value.ConnectionsIn, value.ConnectionsOut
		f.status.UploadTotal, f.status.DownloadTotal = value.UplinkTotal, value.DownlinkTotal
		process, system, count := service.CPUUsage(int(f.saved.PID))
		f.status.ProcessCPUTicks, f.status.SystemCPUTicks, f.status.CPUCount = process, system, count
		f.h.emit(topicStatus, "traffic", contract.Success("api.runtime_status", "核心实时状态", map[string]any{
			"available": true, "started_at_millis": f.identity, "status": value,
			"process_cpu_ticks": process, "system_cpu_ticks": system, "cpu_count": count,
		}))
	}
	if event.groups != nil {
		f.groups, f.groupsReady = *event.groups, true
		if f.groupFlush == nil {
			// 首帧启动固定窗口，后续帧只替换最新状态，不延长截止时间。
			if f.groupTimer == nil {
				f.groupTimer = time.NewTimer(250 * time.Millisecond)
			} else {
				f.groupTimer.Reset(250 * time.Millisecond)
			}
			f.groupFlush = f.groupTimer.C
		}
	}
	return false
}
