// Package telemetry 记录模块设备的活跃与核心启动结果，不接收配置或节点数据。
package telemetry

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json/v2"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

// ProjectToken 与 IngestionHost 由正式构建注入；Host 测试不连接真实项目。
var ProjectToken, IngestionHost string

const (
	maxEvents = 64
	maxBytes  = 64 << 10
	retention = 7 * 24 * time.Hour
)

var beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

type properties struct {
	DistinctID     string `json:"distinct_id,omitzero"`
	Version        string `json:"module_version"`
	VersionCode    string `json:"module_version_code"`
	RootFramework  string `json:"root_framework,omitzero"`
	AndroidAPI     string `json:"android_api,omitzero"`
	KernelSeries   string `json:"kernel_series,omitzero"`
	Manufacturer   string `json:"manufacturer,omitzero"`
	PersonProfile  bool   `json:"$process_person_profile"`
	DisableGeoIP   bool   `json:"$geoip_disable"`
	Result         string `json:"result,omitzero"`
	FailureStage   string `json:"failure_stage,omitzero"`
	DurationMillis int64  `json:"duration_ms,omitzero"`
}

type event struct {
	UUID       string     `json:"uuid"`
	Name       string     `json:"event"`
	Timestamp  time.Time  `json:"timestamp"`
	Properties properties `json:"properties"`
}

type state struct {
	ActiveDay   string    `json:"active_day"`
	Version     string    `json:"version"`
	VersionCode string    `json:"version_code"`
	Pending     []event   `json:"pending"`
	RetryCount  int       `json:"retry_count"`
	NextAttempt time.Time `json:"next_attempt"`
}

// compatibility 是有限桶的设备兼容性摘要，不保存或上传机型、完整内核版本等可识别信息。
type compatibility struct {
	RootFramework string
	AndroidAPI    string
	KernelSeries  string
	Manufacturer  string
}

// Reporter 由 CLI 与唯一 Worker 共用同一状态文件；HTTP 只在 Worker 中执行。
type Reporter struct {
	statePath, lockPath, token, endpoint string
	version, versionCode                 string
	client                               *http.Client
	wake                                 chan struct{}
	deviceID                             string
	identify                             func(context.Context) (string, error)
	inspect                              func(context.Context, compatibility) compatibility
	compatibility                        compatibility
}

// New 只为当前 live 模块的 Android 正式构建创建上报器，排除安装暂存与 Host 测试。
func New(layout paths.Layout) *Reporter {
	if runtime.GOOS != "android" || !layout.IsLive() || !ValidCredentials(ProjectToken, IngestionHost) {
		return nil
	}
	version, code := "unknown", "unknown"
	content, err := os.ReadFile(layout.ModuleProp())
	if err == nil {
		for line := range strings.SplitSeq(string(content), "\n") {
			key, value, _ := strings.Cut(strings.TrimSpace(line), "=")
			if key == "version" && validVersion(value) {
				version = value
			}
			if key == "versionCode" {
				if _, err := strconv.ParseUint(value, 10, 64); err == nil {
					code = value
				}
			}
		}
	}
	return &Reporter{
		statePath: layout.TelemetryState(), lockPath: layout.TelemetryLock(),
		token: ProjectToken, endpoint: strings.TrimRight(IngestionHost, "/") + "/batch/",
		version: version, versionCode: code, wake: make(chan struct{}, 1),
		identify: readDeviceID,
		inspect:  readCompatibility,
		client:   newUploadClient(),
	}
}

func newUploadClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = fetch.DialContext
	return &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func readDeviceID(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// 固定用户 0 并从 Root 查询，避免应用签名或当前用户改变统计身份。
	output, err := exec.CommandContext(ctx, "/system/bin/settings", "--user", "0", "get", "secure", "android_id").Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", errors.New("统计设备标识暂不可读取")
	}
	return hashDeviceID(string(output))
}

func hashDeviceID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 16 || strings.IndexFunc(value, func(c rune) bool {
		return !strings.ContainsRune("0123456789abcdefABCDEF", c)
	}) >= 0 {
		return "", errors.New("统计设备标识无效")
	}
	id, err := strconv.ParseUint(value, 16, 64)
	if err != nil || id == 0 {
		return "", errors.New("统计设备标识无效")
	}
	hash := sha256.Sum256(fmt.Appendf(nil, "NetProxy:telemetry:%016x", id))
	return fmt.Sprintf("%x", hash), nil
}

func readCompatibility(ctx context.Context, current compatibility) compatibility {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if current.RootFramework == "" {
		current.RootFramework = rootFramework(func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		})
	}
	if current.AndroidAPI == "" {
		current.AndroidAPI = readSystemValue(ctx, androidAPI, "/system/bin/getprop", "ro.build.version.sdk")
	}
	if current.KernelSeries == "" {
		current.KernelSeries = readSystemValue(ctx, kernelSeries, "/system/bin/uname", "-r")
	}
	if current.Manufacturer == "" {
		current.Manufacturer = readSystemValue(ctx, manufacturer, "/system/bin/getprop", "ro.product.manufacturer")
	}
	return current
}

func readSystemValue(ctx context.Context, normalize func(string) string, command string, args ...string) string {
	if ctx.Err() != nil {
		return ""
	}
	output, err := exec.CommandContext(ctx, command, args...).Output()
	if err != nil || ctx.Err() != nil {
		return ""
	}
	return normalize(string(output))
}

func rootFramework(exists func(string) bool) string {
	detected := ""
	for _, candidate := range []struct{ name, path string }{
		{"kernelsu", "/data/adb/ksud"},
		{"apatch", "/data/adb/apd"},
		{"magisk", "/data/adb/magisk/busybox"},
	} {
		if !exists(candidate.path) {
			continue
		}
		if detected != "" {
			return "multiple"
		}
		detected = candidate.name
	}
	if detected == "" {
		return "unknown"
	}
	return detected
}

func androidAPI(value string) string {
	api, err := strconv.ParseUint(strings.TrimSpace(value), 10, 8)
	if err != nil || api == 0 {
		return "unknown"
	}
	return strconv.FormatUint(api, 10)
}

func kernelSeries(value string) string {
	major, minor, _ := strings.Cut(strings.TrimSpace(value), ".")
	minor, _, _ = strings.Cut(minor, ".")
	if _, err := strconv.ParseUint(major, 10, 8); err != nil {
		return "unknown"
	}
	if _, err := strconv.ParseUint(minor, 10, 8); err != nil {
		return "unknown"
	}
	return major + "." + minor
}

func manufacturer(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "":
		return "unknown"
	case strings.Contains(value, "oneplus"):
		return "oneplus"
	case strings.Contains(value, "xiaomi"), strings.Contains(value, "redmi"), strings.Contains(value, "poco"):
		return "xiaomi"
	case strings.Contains(value, "samsung"):
		return "samsung"
	case strings.Contains(value, "google"):
		return "google"
	case strings.Contains(value, "huawei"):
		return "huawei"
	case strings.Contains(value, "honor"):
		return "honor"
	case strings.Contains(value, "oppo"):
		return "oppo"
	case strings.Contains(value, "vivo"):
		return "vivo"
	case strings.Contains(value, "realme"):
		return "realme"
	default:
		return "other"
	}
}

// ValidCredentials 验证构建接入参数，不允许 Personal API Key 或非 HTTPS 地址。
func ValidCredentials(token, host string) bool {
	parsed, err := url.Parse(host)
	return strings.HasPrefix(token, "phc_") && len(token) <= 256 && !strings.ContainsAny(token, " \t\r\n\"'\\") &&
		err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil &&
		(parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validVersion(value string) bool {
	return value != "" && len(value) <= 64 && strings.IndexFunc(value, func(c rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-+_", c)
	}) == -1
}

// RecordActive 按北京时间日期和模块版本去重，只写本地文件，不发起网络请求。
func (r *Reporter) RecordActive(now time.Time) (bool, error) {
	if r == nil {
		return false, nil
	}
	added := false
	err := r.modify(func(s *state) (bool, error) {
		day := now.In(beijing).Format(time.DateOnly)
		if s.ActiveDay == day && s.Version == r.version && s.VersionCode == r.versionCode {
			return false, nil
		}
		r.append(s, "module_active", now, properties{})
		s.ActiveDay, s.Version, s.VersionCode = day, r.version, r.versionCode
		added = true
		return true, nil
	})
	return added && err == nil, err
}

// RecordStart 仅接收真实启动尝试的结果和固定阶段，不接收错误文本。
func (r *Reporter) RecordStart(now time.Time, duration time.Duration, stage string, success bool) error {
	if r == nil {
		return nil
	}
	if !slices.Contains([]string{"launch", "cgroup", "state", "ready", "selection", "mode"}, stage) {
		return errors.New("无效的统计启动阶段")
	}
	result := "failure"
	if success {
		result, stage = "success", ""
	}
	return r.modify(func(s *state) (bool, error) {
		r.append(s, "service_start_result", now, properties{Result: result, FailureStage: stage, DurationMillis: max(0, duration.Milliseconds())})
		return true, nil
	})
}

func (r *Reporter) append(s *state, name string, now time.Time, p properties) {
	p.Version, p.VersionCode = r.version, r.versionCode
	p.DisableGeoIP = true
	bytes := [16]byte{}
	_, _ = rand.Read(bytes[:])
	bytes[6], bytes[8] = bytes[6]&0x0f|0x40, bytes[8]&0x3f|0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", bytes[:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
	s.Pending = slices.DeleteFunc(s.Pending, func(e event) bool { return now.Sub(e.Timestamp) > retention })
	for len(s.Pending) >= maxEvents {
		index := slices.IndexFunc(s.Pending, func(e event) bool { return e.Name != "module_active" })
		if index < 0 {
			if name != "module_active" {
				return
			}
			index = 0
		}
		s.Pending = slices.Delete(s.Pending, index, index+1)
	}
	s.Pending = append(s.Pending, event{UUID: id, Name: name, Timestamp: now.UTC(), Properties: p})
}

func (r *Reporter) read() (state, error) {
	file, err := os.Open(r.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, errors.New("统计状态不可读取")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(content) > maxBytes {
		return state{}, errors.New("统计状态不可读取或超过容量")
	}
	var s state
	if err := json.Unmarshal(content, &s, json.RejectUnknownMembers(true)); err != nil || len(s.Pending) > maxEvents || s.RetryCount < 0 || s.RetryCount > 3 {
		return state{}, errors.New("统计状态损坏，保留原文件")
	}
	return s, nil
}

func (r *Reporter) modify(change func(*state) (bool, error)) error {
	lock, err := processlock.TryAcquire(r.lockPath)
	if err != nil {
		return err
	}
	defer lock.Release()
	s, err := r.read()
	if err != nil {
		return err
	}
	changed, err := change(&s)
	if err != nil || !changed {
		return err
	}
	content, err := json.Marshal(s, json.Deterministic(true))
	if err != nil || len(content) > maxBytes {
		return errors.New("统计状态超过容量")
	}
	return provider.WriteAtomic(r.statePath, content, 0o600)
}

// Notify 唤醒当前 Worker 的统计任务，不改变订阅失败退避。
func (r *Reporter) Notify() {
	if r != nil {
		select {
		case r.wake <- struct{}{}:
		default:
		}
	}
}

// Run 复用 Worker 生命周期，核心运行时每日记录活跃，离线队列按独立退避上传。
func (r *Reporter) Run(ctx context.Context, running func() bool, warn func()) {
	for ctx.Err() == nil {
		now := time.Now()
		var recordErr error
		if running != nil && running() {
			_, recordErr = r.RecordActive(now)
		}
		next, flushErr := r.flush(ctx, now)
		if recordErr != nil || flushErr != nil {
			if warn != nil && !errors.Is(flushErr, context.Canceled) {
				warn()
			}
			// 状态无法写入时仍限制唤醒频率，不能被仪表盘轮询变成密集重试。
			if next.Before(now.Add(15 * time.Minute)) {
				next = now.Add(15 * time.Minute)
			}
		}
		day := now.In(beijing)
		midnight := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, beijing)
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(r.statePath))
		// 抖动按设备身份计算，不能用所有设备相同的固定模块路径。
		_, _ = hash.Write([]byte(r.deviceID))
		midnight = midnight.Add(time.Duration(hash.Sum32()%300) * time.Second)
		if next.IsZero() || midnight.Before(next) {
			next = midnight
		}
		timer := time.NewTimer(max(time.Second, time.Until(next)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-r.wake:
			timer.Stop()
			if (recordErr != nil || flushErr != nil) && next.After(time.Now()) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Until(next)):
				}
			}
		}
	}
}

func (r *Reporter) flush(ctx context.Context, now time.Time) (time.Time, error) {
	var snapshot []event
	var next time.Time
	err := r.modify(func(s *state) (bool, error) {
		next = s.NextAttempt
		before := len(s.Pending)
		s.Pending = slices.DeleteFunc(s.Pending, func(e event) bool { return now.Sub(e.Timestamp) > retention })
		if len(s.Pending) == 0 {
			changed := before != 0 || s.RetryCount != 0 || !s.NextAttempt.IsZero()
			s.RetryCount, s.NextAttempt, next = 0, time.Time{}, time.Time{}
			return changed, nil
		}
		if !next.After(now) {
			snapshot = slices.Clone(s.Pending)
		}
		return len(s.Pending) != before, nil
	})
	if err != nil || len(snapshot) == 0 {
		return next, err
	}
	wait, sendErr := r.send(ctx, snapshot, now)
	if ctx.Err() != nil {
		return next, ctx.Err()
	}
	err = r.modify(func(s *state) (bool, error) {
		if sendErr != nil {
			s.RetryCount = min(3, s.RetryCount+1)
			delays := [...]time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour}
			s.NextAttempt = now.Add(max(wait, delays[s.RetryCount-1]))
		} else {
			// 仅确认本次快照，HTTP 期间其他 CLI 新增的事件不得丢失。
			s.Pending = slices.DeleteFunc(s.Pending, func(e event) bool {
				return slices.ContainsFunc(snapshot, func(sent event) bool { return sent.UUID == e.UUID })
			})
			s.RetryCount, s.NextAttempt = 0, time.Time{}
		}
		next = s.NextAttempt
		if sendErr == nil && len(s.Pending) > 0 {
			next = now.Add(time.Minute)
		}
		return true, nil
	})
	return next, errors.Join(sendErr, err)
}

func (r *Reporter) send(ctx context.Context, events []event, now time.Time) (time.Duration, error) {
	defer r.client.CloseIdleConnections()
	if r.deviceID == "" {
		id, err := r.identify(ctx)
		if err != nil {
			return 0, err
		}
		if id == "" {
			return 0, errors.New("统计设备标识无效")
		}
		r.deviceID = id
	}
	if ctx.Err() == nil && r.inspect != nil && (r.compatibility.RootFramework == "" ||
		r.compatibility.AndroidAPI == "" || r.compatibility.KernelSeries == "" || r.compatibility.Manufacturer == "") {
		// 查询失败保留空值，下次既有上传机会只重试缺失字段；成功的 unknown 也可缓存。
		r.compatibility = r.inspect(ctx, r.compatibility)
	}
	// 身份只在 Worker 中派生，CLI 和离线队列均不保存设备标识。
	for i := range events {
		events[i].Properties.DistinctID = r.deviceID
		events[i].Properties.RootFramework = cmp.Or(r.compatibility.RootFramework, "unknown")
		events[i].Properties.AndroidAPI = cmp.Or(r.compatibility.AndroidAPI, "unknown")
		events[i].Properties.KernelSeries = cmp.Or(r.compatibility.KernelSeries, "unknown")
		events[i].Properties.Manufacturer = cmp.Or(r.compatibility.Manufacturer, "unknown")
	}
	content, err := json.Marshal(struct {
		Token string  `json:"token"`
		Batch []event `json:"batch"`
	}{r.token, events}, json.Deterministic(true))
	if err != nil {
		return 0, errors.New("统计请求无法编码")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(content))
	if err != nil {
		return 0, errors.New("统计请求无法创建")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "NetProxy")
	response, err := r.client.Do(request)
	if err != nil {
		return 0, errors.New("统计暂时无法上传")
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	closeErr := response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 && readErr == nil && closeErr == nil {
		return 0, nil
	}
	var wait time.Duration
	if response.StatusCode == http.StatusTooManyRequests {
		header := response.Header.Get("Retry-After")
		if seconds, err := strconv.ParseInt(header, 10, 32); err == nil && seconds > 0 {
			wait = time.Duration(seconds) * time.Second
		} else if date, err := http.ParseTime(header); err == nil {
			wait = max(0, date.Sub(now))
		}
	}
	return min(wait, 24*time.Hour), errors.New("统计暂时无法上传")
}
