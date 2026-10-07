package telemetry

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

func testReporter(t testing.TB) *Reporter {
	t.Helper()
	root := t.TempDir()
	return &Reporter{statePath: filepath.Join(root, "config", "telemetry", "state.json"),
		lockPath: filepath.Join(root, "dev", "telemetry.lock.flock"), version: "v8.2.1", versionCode: "42",
		identify: func(context.Context) (string, error) { return hashDeviceID("0123456789abcdef") },
		inspect: func(context.Context, compatibility) compatibility {
			return compatibility{RootFramework: "kernelsu", AndroidAPI: "36", KernelSeries: "6.6", Manufacturer: "oneplus"}
		},
		token: "phc_fixture", wake: make(chan struct{}, 1), client: newUploadClient()}
}

func TestUploadClientUsesSharedDNSAndCancellation(t *testing.T) {
	client := newUploadClient()
	transport := client.Transport.(*http.Transport)
	defer client.CloseIdleConnections()
	if transport == http.DefaultTransport || reflect.ValueOf(transport.DialContext).Pointer() != reflect.ValueOf(fetch.DialContext).Pointer() {
		t.Fatal("统计请求未复用共享 DNS 拨号，或修改了全局 Transport")
	}
	if client.Timeout != 5*time.Second || client.CheckRedirect(&http.Request{}, nil) != http.ErrUseLastResponse {
		t.Fatal("统计请求超时或重定向策略改变")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := transport.DialContext(ctx, "tcp", "telemetry.invalid:443"); !errors.Is(err, context.Canceled) {
		t.Fatalf("DNS 拨号未传播取消: %v", err)
	}
}

func loadState(t testing.TB, r *Reporter) state {
	t.Helper()
	s, err := r.read()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestActivityDayVersionWithoutDeviceLookup(t *testing.T) {
	r := testReporter(t)
	r.identify = func(context.Context) (string, error) {
		t.Fatal("本地记录不能查询设备标识")
		return "", nil
	}
	first := time.Date(2026, 10, 5, 15, 59, 0, 0, time.UTC)
	if added, err := r.RecordActive(first); !added || err != nil {
		t.Fatalf("首次活跃未记录: %v %v", added, err)
	}
	s := loadState(t, r)
	info := s.Pending[0]
	if info.Properties.DistinctID != "" || info.Properties.RootFramework != "" || info.Properties.AndroidAPI != "" || info.Properties.KernelSeries != "" || info.Properties.Manufacturer != "" || info.Timestamp.Location() != time.UTC || info.Properties.PersonProfile || !info.Properties.DisableGeoIP {
		t.Fatalf("统计身份或隐私属性错误: %+v", info)
	}
	if added, err := r.RecordActive(first.Add(30 * time.Second)); added || err != nil {
		t.Fatalf("同日重复记录: %v %v", added, err)
	}
	if added, err := r.RecordActive(first.Add(time.Minute)); !added || err != nil {
		t.Fatalf("北京时间跨日未记录: %v %v", added, err)
	}
	r.version, r.versionCode = "v8.2.2", "43"
	if added, err := r.RecordActive(first.Add(2 * time.Minute)); !added || err != nil {
		t.Fatalf("同日升级未记录: %v %v", added, err)
	}
	s = loadState(t, r)
	if len(s.Pending) != 3 || s.ActiveDay != "2026-10-06" {
		t.Fatalf("活跃去重错误: %+v", s)
	}
	for _, e := range s.Pending {
		if e.Properties.DistinctID != "" || e.Properties.RootFramework != "" || e.Properties.AndroidAPI != "" || e.Properties.KernelSeries != "" || e.Properties.Manufacturer != "" || len(e.UUID) != 36 {
			t.Fatal("事件包含设备摘要或未生成独立 UUID")
		}
	}
}

func TestCompatibilityNormalization(t *testing.T) {
	if rootFramework(func(path string) bool { return path == "/data/adb/ksud" }) != "kernelsu" {
		t.Fatal("KernelSU 未正确归类")
	}
	if rootFramework(func(path string) bool { return path == "/data/adb/apd" }) != "apatch" {
		t.Fatal("APatch 未正确归类")
	}
	if rootFramework(func(path string) bool { return path == "/data/adb/magisk/busybox" }) != "magisk" {
		t.Fatal("Magisk 未正确归类")
	}
	if rootFramework(func(path string) bool { return path == "/data/adb/ksud" || path == "/data/adb/magisk/busybox" }) != "multiple" {
		t.Fatal("多框架特征未保守归类")
	}
	if rootFramework(func(path string) bool { return path == "/sbin/.magisk" }) != "unknown" {
		t.Fatal("临时 Magisk 路径不应作为框架特征")
	}
	if rootFramework(func(string) bool { return false }) != "unknown" {
		t.Fatal("未知 Root 方案未正确归类")
	}
	for input, want := range map[string]string{"36\n": "36", "0": "unknown", "x": "unknown"} {
		if got := androidAPI(input); got != want {
			t.Fatalf("Android API 归类错误: %q = %q", input, got)
		}
	}
	for input, want := range map[string]string{"6.6.47-gki": "6.6", "5.10": "5.10", "broken": "unknown"} {
		if got := kernelSeries(input); got != want {
			t.Fatalf("内核版本归类错误: %q = %q", input, got)
		}
	}
	for input, want := range map[string]string{"OnePlus": "oneplus", "Redmi": "xiaomi", "Samsung Electronics": "samsung", "": "unknown", "Fairphone": "other"} {
		if got := manufacturer(input); got != want {
			t.Fatalf("厂商归类错误: %q = %q", input, got)
		}
	}
}

func TestBoundedQueueTTLAndStartProperties(t *testing.T) {
	r := testReporter(t)
	now := time.Now()
	_, _ = r.RecordActive(now)
	for range 80 {
		if err := r.RecordStart(now, 2*time.Second, "ready", false); err != nil {
			t.Fatal(err)
		}
	}
	s := loadState(t, r)
	if len(s.Pending) != maxEvents || s.Pending[0].Name != "module_active" {
		t.Fatal("队列未优先保留活跃事件")
	}
	e := s.Pending[1]
	if e.Properties.FailureStage != "ready" || e.Properties.DurationMillis != 2000 || e.Properties.Result != "failure" {
		t.Fatalf("启动失败属性错误: %+v", e)
	}
	if err := r.RecordStart(now, time.Second, "raw secret error", false); err == nil {
		t.Fatal("接受了任意错误文本")
	}
	if err := r.RecordStart(now.Add(retention+time.Hour), time.Second, "state", true); err != nil {
		t.Fatal(err)
	}
	s = loadState(t, r)
	if len(s.Pending) != 1 || s.Pending[0].Properties.Result != "success" || s.Pending[0].Properties.FailureStage != "" {
		t.Fatal("过期队列未清理或成功结果携带失败阶段")
	}
	content, err := os.ReadFile(r.statePath)
	if err != nil || len(content) > maxBytes || strings.Contains(string(content), r.token) || strings.Contains(string(content), "distinct_id") {
		t.Fatal("统计状态超过容量或包含项目 Token、设备身份")
	}
}

func TestCorruptStateAndBusyLockNeverOverwriteQueue(t *testing.T) {
	r := testReporter(t)
	_, _ = r.RecordActive(time.Now())
	for _, content := range []string{`{"unknown":true}`, `{`, strings.Repeat("x", maxBytes+1)} {
		if err := os.WriteFile(r.statePath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if added, err := r.RecordActive(time.Now()); added || err == nil {
			t.Fatal("损坏状态被静默重建")
		}
		got, _ := os.ReadFile(r.statePath)
		if string(got) != content {
			t.Fatal("损坏状态被覆盖")
		}
	}
	lock, err := processlock.TryAcquire(r.lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	start := time.Now()
	if added, err := r.RecordActive(start); added || !errors.Is(err, processlock.ErrBusy) || time.Since(start) > time.Second {
		t.Fatalf("活跃记录阻塞安装锁: %v %v", added, err)
	}
}

func TestStartDoesNotEvictActiveEventsWhenQueueFull(t *testing.T) {
	r := testReporter(t)
	now := time.Now()
	for i := range maxEvents {
		r.versionCode = fmt.Sprint(i)
		if _, err := r.RecordActive(now); err != nil {
			t.Fatal(err)
		}
	}
	before := loadState(t, r).Pending
	if err := r.RecordStart(now, time.Second, "ready", false); err != nil {
		t.Fatal(err)
	}
	after := loadState(t, r).Pending
	if len(after) != maxEvents || before[0].UUID != after[0].UUID || after[maxEvents-1].Name != "module_active" {
		t.Fatal("满队列的活跃事件被启动结果挤出")
	}
}

func TestUploadSnapshotDoesNotOverwriteConcurrentCLI(t *testing.T) {
	r := testReporter(t)
	now := time.Now()
	_, _ = r.RecordActive(now)
	original := loadState(t, r).Pending[0]
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var batch struct {
			Token string  `json:"token"`
			Batch []event `json:"batch"`
		}
		content, _ := io.ReadAll(req.Body)
		if req.Method != http.MethodPost || req.URL.Path != "/batch/" || req.Header.Get("Content-Type") != "application/json" {
			t.Error("PostHog API 请求格式错误")
		}
		if err := json.Unmarshal(content, &batch); err != nil || batch.Token != r.token || len(batch.Batch) != 1 || batch.Batch[0].UUID != original.UUID || batch.Batch[0].Properties.DistinctID == "" || batch.Batch[0].Properties.RootFramework != "kernelsu" || batch.Batch[0].Properties.AndroidAPI != "36" || batch.Batch[0].Properties.KernelSeries != "6.6" || batch.Batch[0].Properties.Manufacturer != "oneplus" {
			t.Errorf("事件快照或 UUID 改变: %v", err)
		}
		if err := r.RecordStart(now, time.Second, "ready", true); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r.endpoint, r.client = server.URL+"/batch/", server.Client()
	if _, err := r.flush(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if id, _ := hashDeviceID("0123456789abcdef"); r.deviceID != id || loadState(t, r).Pending[0].Properties.DistinctID != "" {
		t.Fatal("Worker 设备身份错误或被写回本地队列")
	}
	s := loadState(t, r)
	if len(s.Pending) != 1 || s.Pending[0].Name != "service_start_result" || s.Pending[0].UUID == original.UUID {
		t.Fatal("HTTP 期间的并发事件被确认删除")
	}
}

func TestDeviceIDValidationAndCancellation(t *testing.T) {
	id, err := hashDeviceID("0123456789abcdef")
	if err != nil || len(id) != 64 || strings.Contains(id, "0123456789abcdef") {
		t.Fatal("设备身份未使用独立 SHA-256 摘要")
	}
	for _, value := range []string{"0123456789ABCDEF", "123456789abcdef", " 0123456789abcdef\n"} {
		got, err := hashDeviceID(value)
		if err != nil || got != id {
			t.Fatal("同一 Android ID 的大小写或前导零改变了身份")
		}
	}
	other, _ := hashDeviceID("fedcba9876543210")
	if other == id {
		t.Fatal("不同设备得到同一身份")
	}
	for _, value := range []string{"", "null", "undefined", "0", "0000000000000000", "-1", "0123456789abcdef0", "permission denied", "0123\n4567"} {
		if _, err := hashDeviceID(value); err == nil || strings.Contains(err.Error(), value) && value != "" {
			t.Fatalf("无效设备标识被接受或进入错误消息: %q", value)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readDeviceID(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("设备查询未传播取消: %v", err)
	}
}

func TestDeviceIdentitySurvivesFreshInstallAndDifferentDevices(t *testing.T) {
	var received []string
	var mu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var batch struct {
			Batch []event `json:"batch"`
		}
		content, _ := io.ReadAll(req.Body)
		if strings.Contains(string(content), "0123456789abcdef") || strings.Contains(string(content), "fedcba9876543210") {
			t.Error("原始 Android ID 进入网络请求")
		}
		if err := json.Unmarshal(content, &batch); err != nil || len(batch.Batch) != 1 {
			t.Error("设备统计批次无效")
		} else {
			mu.Lock()
			received = append(received, batch.Batch[0].Properties.DistinctID)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r := testReporter(t)
	var lookups int
	attach := func(r *Reporter, source string) {
		r.endpoint, r.client = server.URL, server.Client()
		r.identify = func(ctx context.Context) (string, error) {
			lookups++
			lock, err := processlock.TryAcquire(r.lockPath)
			if err != nil {
				t.Fatal("设备查询期间仍持有状态锁")
			}
			lock.Release()
			return hashDeviceID(source)
		}
	}
	attach(r, "0123456789abcdef")
	now := time.Now()
	for i := range 2 {
		_, _ = r.RecordActive(now.Add(time.Duration(i) * 24 * time.Hour))
		if _, err := r.flush(context.Background(), now.Add(time.Duration(i)*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if lookups != 1 {
		t.Fatal("同一 Worker 重复查询设备标识")
	}
	if err := os.Remove(r.statePath); err != nil {
		t.Fatal(err)
	}
	reinstalled := testReporter(t)
	reinstalled.statePath, reinstalled.lockPath = r.statePath, r.lockPath
	attach(reinstalled, "0123456789abcdef")
	_, _ = reinstalled.RecordActive(now)
	if _, err := reinstalled.flush(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	other := testReporter(t)
	attach(other, "fedcba9876543210")
	_, _ = other.RecordActive(now)
	if _, err := other.flush(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 4 || received[0] == "" || received[0] != received[1] || received[0] != received[2] || received[0] == received[3] {
		t.Fatalf("全新安装改变了身份或不同设备混为一台: %v", received)
	}
}

func TestUnavailableDeviceIdentityNeverUploadsRandomID(t *testing.T) {
	r := testReporter(t)
	var sent atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { sent.Store(true) }))
	defer server.Close()
	r.endpoint, r.client = server.URL, server.Client()
	r.identify = func(context.Context) (string, error) { return "", errors.New("统计设备标识暂不可读取") }
	now := time.Now()
	if _, err := r.RecordActive(now); err != nil {
		t.Fatal(err)
	}
	if next, err := r.flush(context.Background(), now); err == nil || !next.Equal(now.Add(15*time.Minute)) || sent.Load() {
		t.Fatal("设备查询失败未退避或上传了替代身份")
	}
	if r.deviceID != "" || len(loadState(t, r).Pending) != 1 {
		t.Fatal("设备查询失败生成身份或丢失待发送事件")
	}
	r.identify = func(context.Context) (string, error) { return hashDeviceID("0123456789abcdef") }
	if _, err := r.flush(context.Background(), now.Add(15*time.Minute)); err != nil || !sent.Load() {
		t.Fatal("设备标识恢复后未发送原事件")
	}
}

func TestRetryAfterBackoffStableUUIDAndTTL(t *testing.T) {
	r := testReporter(t)
	now := time.Now()
	_, _ = r.RecordActive(now)
	original := loadState(t, r).Pending[0].UUID
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "7200")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	r.endpoint, r.client = server.URL, server.Client()
	next, err := r.flush(context.Background(), now)
	if err == nil || !next.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("未遵循 Retry-After: %v %v", next, err)
	}
	for range 5 {
		if _, err := r.flush(context.Background(), now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 || loadState(t, r).Pending[0].UUID != original {
		t.Fatal("轮询绕过了退避或重试 UUID 改变")
	}
	if _, err := r.flush(context.Background(), next); err == nil {
		t.Fatal("失败重试被返回成功")
	}
	next, err = r.flush(context.Background(), now.Add(retention+time.Hour))
	if err != nil || !next.IsZero() || len(loadState(t, r).Pending) != 0 || calls.Load() != 2 {
		t.Fatal("过期队列继续上传或空队列留下过期调度")
	}
}

func TestUploadRejectsRedirectAndCancels(t *testing.T) {
	r := testReporter(t)
	now := time.Now()
	_, _ = r.RecordActive(now)
	var leaked atomic.Bool
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { leaked.Store(true) }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	r.endpoint, r.client = redirect.URL, redirect.Client()
	r.client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	if _, err := r.flush(context.Background(), now); err == nil || leaked.Load() {
		t.Fatal("接受重定向或向其他目标泄露 Token")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.flush(ctx, now.Add(24*time.Hour)); !errors.Is(err, context.Canceled) {
		t.Fatalf("上传取消未传播: %v", err)
	}
	if len(loadState(t, r).Pending) != 1 {
		t.Fatal("取消删除了待发送事件")
	}
}

func TestCanceledUploadDoesNotCacheCompatibility(t *testing.T) {
	r := testReporter(t)
	var inspected atomic.Int32
	r.inspect = func(context.Context, compatibility) compatibility {
		inspected.Add(1)
		return compatibility{RootFramework: "magisk", AndroidAPI: "35", KernelSeries: "6.1", Manufacturer: "xiaomi"}
	}
	received := make(chan properties, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var batch struct {
			Batch []event `json:"batch"`
		}
		content, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(content, &batch); err != nil || len(batch.Batch) != 1 {
			t.Error("未收到完整统计批次")
		} else {
			received <- batch.Batch[0].Properties
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r.endpoint, r.client = server.URL, server.Client()
	events := []event{{UUID: "fixture", Name: "module_active", Timestamp: time.Now()}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.send(ctx, events, time.Now()); err == nil || inspected.Load() != 0 {
		t.Fatal("取消上传仍采集或缓存了兼容性摘要")
	}
	if _, err := r.send(context.Background(), events, time.Now()); err != nil || inspected.Load() != 1 {
		t.Fatalf("后续正常上传未采集兼容性摘要: %v", err)
	}
	properties := <-received
	if properties.RootFramework != "magisk" || properties.AndroidAPI != "35" || properties.KernelSeries != "6.1" || properties.Manufacturer != "xiaomi" {
		t.Fatalf("上传缺少兼容性摘要: %+v", properties)
	}
}

func TestCompatibilityRetriesFailedFieldsAndCachesSuccessfulUnknown(t *testing.T) {
	r := testReporter(t)
	calls := 0
	r.inspect = func(_ context.Context, current compatibility) compatibility {
		calls++
		if calls == 1 {
			return compatibility{RootFramework: "kernelsu", Manufacturer: "unknown"}
		}
		if current.RootFramework != "kernelsu" || current.Manufacturer != "unknown" || current.AndroidAPI != "" {
			t.Fatalf("成功字段未缓存或失败字段被缓存: %+v", current)
		}
		current.AndroidAPI, current.KernelSeries = "36", "6.6"
		return current
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r.endpoint, r.client = server.URL, server.Client()
	for index := range 3 {
		events := []event{{UUID: "fixture", Name: "module_active", Timestamp: time.Now()}}
		if _, err := r.send(context.Background(), events, time.Now()); err != nil {
			t.Fatal(err)
		}
		expectedAPI := "36"
		if index == 0 {
			expectedAPI = "unknown"
		}
		if events[0].Properties.AndroidAPI != expectedAPI || events[0].Properties.Manufacturer != "unknown" {
			t.Fatalf("上传摘要异常: %+v", events[0].Properties)
		}
	}
	if calls != 2 {
		t.Fatalf("完整摘要仍重复采集: %d", calls)
	}
}

func TestCompatibilityCancellationPreservesSuccessfulFields(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	current := compatibility{RootFramework: "kernelsu", AndroidAPI: "36", Manufacturer: "other"}
	if got := readCompatibility(ctx, current); got != current {
		t.Fatalf("取消后成功字段丢失或失败查询不可重试: %+v", got)
	}
}

func TestConcurrentActivityDeduplication(t *testing.T) {
	r := testReporter(t)
	now := time.Now()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := r.RecordActive(now)
			if err != nil && !errors.Is(err, processlock.ErrBusy) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(loadState(t, r).Pending) != 1 {
		t.Fatal("并发 CLI 重复记录活跃")
	}
}

func TestProductionCredentialsAndHostIsolation(t *testing.T) {
	if !ValidCredentials("phc_fixture", "https://us.i.posthog.com") {
		t.Fatal("有效项目接入参数被拒绝")
	}
	for _, pair := range [][2]string{{"phx_personal", "https://us.i.posthog.com"}, {"phc_fixture", "http://us.i.posthog.com"},
		{"phc_fixture", "https://user:pass@us.i.posthog.com"}, {"phc_fixture", "https://us.i.posthog.com/path"},
		{"phc_bad key", "https://us.i.posthog.com"}} {
		if ValidCredentials(pair[0], pair[1]) {
			t.Fatalf("接受无效接入参数: %q", pair)
		}
	}
	if New(paths.New(t.TempDir())) != nil || New(paths.New("/data/adb/modules_update/netproxy")) != nil {
		t.Fatal("测试或暂存模块启用了真实上报")
	}
}

func TestRunCancellationAndDailyRunningObservation(t *testing.T) {
	r := testReporter(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		started <- struct{}{}
		<-release
	}))
	defer server.Close()
	defer close(release)
	r.endpoint, r.client = server.URL, server.Client()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); r.Run(ctx, func() bool { return true }, nil) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("运行中的核心未产生每日活跃")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("统计上传阻塞 Worker 退出")
	}
	if len(loadState(t, r).Pending) != 1 {
		t.Fatal("取消丢失待上传队列")
	}
}

func BenchmarkDeduplicatedActivity(b *testing.B) {
	r := testReporter(b)
	now := time.Now()
	_, _ = r.RecordActive(now)
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.RecordActive(now); err != nil {
			b.Fatal(err)
		}
	}
}
