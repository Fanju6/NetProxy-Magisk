package httpapi

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/androidplatform"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

type platformFunc func(context.Context, androidplatform.Request) (jsontext.Value, error)

func (f platformFunc) Query(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
	return f(ctx, request)
}

func androidResourceRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = s.config.Listen
	r.Header.Set("Content-Type", "application/json")
	if path != "/api/v1/auth/provision" {
		r.Header.Set("Authorization", "Bearer "+s.config.Secret)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestProvisionOnlyDeliversPrivatelyAndIsLimited(t *testing.T) {
	s := resourceFixture(t)
	calls := 0
	s.platform = platformFunc(func(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
		calls++
		if request.Operation != "provision" || request.Connection == nil || *request.Connection != s.config.Connection() {
			t.Fatal("交付未使用服务端固定凭据", request.Operation)
		}
		return jsontext.Value(`{"delivered":true}`), nil
	})
	for _, tc := range []struct {
		host, origin, path, body string
		status                   int
	}{
		{"evil.test", "", "/api/v1/auth/provision", `{}`, 403},
		{s.config.Listen, "https://" + s.config.Listen, "/api/v1/auth/provision", `{}`, 403},
		{s.config.Listen, "", "/api/v1/auth/provision?user_id=0", `{}`, 403},
		{s.config.Listen, "", "/api/v1/auth/provision", `{"package":"evil"}`, 400},
		{s.config.Listen, "", "/api/v1/auth/provision", `{}`, 200},
		{s.config.Listen, "", "/api/v1/auth/provision", `{}`, 409},
	} {
		r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		r.Host = tc.host
		r.Header.Set("Content-Type", "application/json")
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), s.config.Secret) || strings.Contains(w.Body.String(), "PRIVATE KEY") {
			t.Fatal("交付响应泄漏秘密")
		}
	}
	if calls != 1 {
		t.Fatal("无效请求进入平台查询", calls)
	}
}

func TestProvisionCooldownBackoffAndRecovery(t *testing.T) {
	s := resourceFixture(t)
	calls := 0
	failing := true
	s.platform = platformFunc(func(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 15*time.Second {
			t.Fatal("主动交付没有有界时限")
		}
		if failing {
			return nil, errors.New("平台查询失败 fixture")
		}
		return jsontext.Value(`{"delivered":true}`), nil
	})
	for _, delay := range []time.Duration{3, 6, 12, 24, 30, 30} {
		// 无正文的客户端交付请求与后续请求共享退避状态。
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/provision", nil)
		if _, err := s.provisionClient(t.Context(), r); err == nil {
			t.Fatal("交付失败被当作成功")
		}
		want := delay * time.Second
		if s.provisionBackoff != want || time.Until(s.provisionAfter) < want-time.Second || time.Until(s.provisionAfter) > want {
			t.Fatalf("退避: %v, 剩余: %v, 期望: %v", s.provisionBackoff, time.Until(s.provisionAfter), want)
		}
		after := s.provisionAfter
		before := calls
		w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", `{}`)
		if w.Code != 409 || calls != before || !s.provisionAfter.Equal(after) {
			t.Fatal("冷却请求启动进程或延长窗口", w.Code, calls)
		}
		s.provisionAfter = time.Now().Add(-time.Second)
	}
	failing = false
	w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", `{}`)
	if w.Code != 200 || s.provisionBackoff != 0 || time.Until(s.provisionAfter) < 29*time.Second || time.Until(s.provisionAfter) > 30*time.Second {
		t.Fatal("成功没有重置退避并进入 30 秒冷却", w.Code, s.provisionBackoff)
	}
	before := calls
	if w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", `{}`); w.Code != 409 || calls != before {
		t.Fatal("成功后的冷却失效", w.Code, calls)
	}
	s.provisionAfter = time.Now().Add(-time.Second)
	if w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", `{}`); w.Code != 200 || calls != before+1 {
		t.Fatal("冷却后不能重新交付", w.Code, calls)
	}
	s.provisionAfter = time.Now().Add(-time.Second)
	failing = true
	androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", `{}`)
	if s.provisionBackoff != 3*time.Second {
		t.Fatal("成功后的首次失败沿用了旧退避", s.provisionBackoff)
	}
}

func TestProvisionConcurrentRequestsDoNotStartDuplicateProcesses(t *testing.T) {
	s := resourceFixture(t)
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	s.platform = platformFunc(func(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
			return jsontext.Value(`{"delivered":true}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", `{}`) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("首次交付未启动")
	}
	var concurrent sync.WaitGroup
	for range 32 {
		concurrent.Go(func() {
			if w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", `{}`); w.Code != 409 {
				t.Error("并发交付未拒绝", w.Code)
			}
		})
	}
	concurrent.Wait()
	once.Do(func() { close(release) })
	if w := <-done; w.Code != 200 || calls.Load() != 1 {
		t.Fatal("并发重复启动或首次交付失败", w.Code, calls.Load())
	}
}

func TestProvisionInvalidAndCancelledRequestsDoNotConsumeCooldown(t *testing.T) {
	s := resourceFixture(t)
	s.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
		t.Fatal("无效请求启动平台进程")
		return nil, nil
	})
	for _, body := range []string{`{"connection":{}}`, `{"user_id":0}`, `{"package":"example"}`, `[]`, `null`, `{"x":1,"x":2}`} {
		if w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", body); w.Code != 400 {
			t.Fatal("无效正文未拒绝", w.Code)
		}
	}
	if w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", strings.Repeat(" ", 1025)); w.Code != 413 {
		t.Fatal("超长正文未拒绝", w.Code)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.provisionClient(ctx, httptest.NewRequest(http.MethodPost, "/api/v1/auth/provision", nil)); !errors.Is(err, context.Canceled) {
		t.Fatal("交付吞掉取消", err)
	}
	if !s.provisionAfter.IsZero() || s.provisionBackoff != 0 {
		t.Fatal("无效或取消请求消耗冷却")
	}
}

func TestAndroidAppsPreparePrivateCacheAndRemainCancellable(t *testing.T) {
	s := resourceFixture(t)
	calls := 0
	s.platform = platformFunc(func(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
		calls++
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if request.Operation != "apps" || request.IconCache != paths.New(s.options.ModuleDir).AndroidIcons() {
			t.Fatal(request)
		}
		info, err := os.Stat(request.IconCache)
		if err != nil || !info.IsDir() {
			t.Fatal("未准备图标缓存目录", err)
		}
		return jsontext.Value(`{"users":[{"id":0,"name":"Owner","revision":"owner","apps":[]},{"id":10,"name":"Work","revision":"work","apps":[]}]}`), nil
	})
	for range cap(s.writeSlots) {
		s.writeSlots <- struct{}{}
	}
	w := androidResourceRequest(s, http.MethodGet, "/api/v1/android/apps", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"users"`) || !strings.Contains(w.Body.String(), `"Work"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, query := range []string{"user_id=0", "size=128", "user_id=10&size=96", "icon_cache=/secret", "size=96&size=128"} {
		if w := androidResourceRequest(s, http.MethodGet, "/api/v1/android/apps?"+query, ""); w.Code != 400 {
			t.Fatal("旧参数未拒绝", query, w.Code, w.Body.String())
		}
	}
	if w := androidResourceRequest(s, http.MethodGet, "/api/v1/android/users", ""); w.Code != 404 {
		t.Fatal("旧用户端点仍然可用", w.Code)
	}
	if calls != 1 {
		t.Fatal("无效请求进入平台查询", calls)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.queryApps(cancelled, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("平台查询吞掉取消", err)
	}
	connection, err := json.Marshal(s.config.Connection())
	if err != nil || strings.Contains(string(connection), "private_key") || strings.Contains(string(connection), "PRIVATE KEY") {
		t.Fatal("客户端连接资料包含私钥")
	}
}

func TestAndroidAppsPlatformFailureIsNotAnEmptySnapshot(t *testing.T) {
	s := resourceFixture(t)
	s.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
		return nil, &contract.Error{Code: "android.query_failed", Message: "fixture"}
	})
	w := androidResourceRequest(s, http.MethodGet, "/api/v1/android/apps", "")
	if w.Code == 200 || !strings.Contains(w.Body.String(), "android.query_failed") {
		t.Fatal("平台失败被转换为空清单", w.Code, w.Body.String())
	}
}

func TestAndroidAppsWaitingForPlatformRemainsCancellable(t *testing.T) {
	s := resourceFixture(t)
	for range cap(s.appSlots) {
		s.appSlots <- struct{}{}
	}
	s.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
		t.Error("取消后启动了平台进程")
		return nil, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/android/apps", nil).WithContext(ctx)
	r.Host = s.config.Listen
	r.Header.Set("Authorization", "Bearer "+s.config.Secret)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 504 {
		t.Fatal("等待平台查询没有遵循取消", w.Code, w.Body.String())
	}
}

func TestAndroidAppsQueriesShareOneWriter(t *testing.T) {
	s := resourceFixture(t)
	var active atomic.Int32
	var calls atomic.Int32
	s.platform = platformFunc(func(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
		if active.Add(1) != 1 {
			t.Error("图标生成与清理发生并行写入")
		}
		defer active.Add(-1)
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return jsontext.Value(`{"users":[]}`), nil
	})
	var parallel sync.WaitGroup
	for range 8 {
		parallel.Go(func() {
			if _, err := s.queryApps(t.Context(), ""); err != nil {
				t.Error(err)
			}
		})
	}
	parallel.Wait()
	if calls.Load() != 8 {
		t.Fatal("平台查询丢失", calls.Load())
	}
}

func TestProvisionDoesNotWaitForAppsButKeepsOwnCooldown(t *testing.T) {
	s := resourceFixture(t)
	s.appSlots <- struct{}{}
	defer func() { <-s.appSlots }()
	var calls atomic.Int32
	s.platform = platformFunc(func(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
		if request.Operation != "provision" {
			t.Error("交付错误进入应用查询", request.Operation)
		}
		calls.Add(1)
		return jsontext.Value(`{"delivered":true}`), nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/provision", nil).WithContext(ctx)
	r.Host = s.config.Listen
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 || calls.Load() != 1 || len(s.appSlots) != 1 {
		t.Fatal("交付等待或消耗了应用查询槽位", w.Code, calls.Load(), len(s.appSlots))
	}
	if w := androidResourceRequest(s, http.MethodPost, "/api/v1/auth/provision", ""); w.Code != 409 || calls.Load() != 1 {
		t.Fatal("独立交付绕过了冷却", w.Code, calls.Load())
	}
}

func TestAndroidIconReadsCachedFileWithoutPlatformAndSupportsETag(t *testing.T) {
	s := resourceFixture(t)
	s.platform = platformFunc(func(context.Context, androidplatform.Request) (jsontext.Value, error) {
		t.Fatal("缓存读取不应启动 Android 进程")
		return nil, nil
	})
	key := strings.Repeat("a", 64)
	cache := paths.New(s.options.ModuleDir).AndroidIcons()
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	png := "\x89PNG\r\n\x1a\nfixture"
	if err := os.WriteFile(filepath.Join(cache, key+".png"), []byte(png), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, method, token, etag string
		status                    int
	}{
		{"/api/v1/android/icons/" + key, "GET", s.config.Secret, "", 200},
		{"/api/v1/android/icons/" + key, "GET", s.config.Secret, `"` + key + `"`, 304},
		{"/api/v1/android/icons/" + key, "HEAD", s.config.Secret, "", 405},
		{"/api/v1/android/icons/" + key, "GET", "wrong", "", 401},
		{"/api/v1/android/icons/" + strings.Repeat("b", 64), "GET", s.config.Secret, "", 404},
		{"/api/v1/android/icons/0/96/" + key, "GET", s.config.Secret, "", 404},
		{"/api/v1/android/icons/..%2Fsecret", "GET", s.config.Secret, "", 400},
		{"/api/v1/android/icons/" + strings.Repeat("A", 64), "GET", s.config.Secret, "", 400},
		{"/api/v1/android/icons/" + key + "?file=secret", "GET", s.config.Secret, "", 400},
		{"/api/v1/android/apps/icons", "POST", s.config.Secret, "", 404},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = s.config.Listen
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("If-None-Match", tc.etag)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if tc.status == 200 && (w.Body.String() != png || w.Header().Get("Content-Type") != "image/png" ||
			w.Header().Get("Cache-Control") != "private, max-age=604800, immutable" || w.Header().Get("ETag") != `"`+key+`"`) {
			t.Fatal("图标未按私有二进制资源返回", w.Header())
		}
	}
}

func TestAndroidAppsIconKeyCanBeReadUnchanged(t *testing.T) {
	s := resourceFixture(t)
	key := strings.Repeat("a", 64)
	png := "\x89PNG\r\n\x1a\nfixture"
	s.platform = platformFunc(func(ctx context.Context, request androidplatform.Request) (jsontext.Value, error) {
		if request.Operation != "apps" || request.IconCache != paths.New(s.options.ModuleDir).AndroidIcons() {
			t.Fatal(request)
		}
		if err := os.WriteFile(filepath.Join(request.IconCache, key+".png"), []byte(png), 0o600); err != nil {
			t.Fatal(err)
		}
		return json.Marshal(map[string]any{"users": []map[string]any{
			{"id": 10, "name": "Work", "revision": "fixture", "apps": []map[string]string{{"icon_key": key}}},
		}}, json.Deterministic(true))
	})
	w := androidResourceRequest(s, http.MethodGet, "/api/v1/android/apps", "")
	var listing struct {
		Data struct {
			Users []struct {
				Apps []struct {
					IconKey string `json:"icon_key"`
				} `json:"apps"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil || w.Code != 200 || len(listing.Data.Users) != 1 || len(listing.Data.Users[0].Apps) != 1 {
		t.Fatal("应用清单无效", w.Code, err)
	}
	if listing.Data.Users[0].Apps[0].IconKey != key {
		t.Fatal("服务端改写了图标键")
	}
	w = androidResourceRequest(s, http.MethodGet, "/api/v1/android/icons/"+listing.Data.Users[0].Apps[0].IconKey, "")
	if w.Code != 200 || w.Body.String() != png {
		t.Fatal("图标键不能直接读取图标", w.Code)
	}
}
