package httpapi

import (
	"context"
	"encoding/json/jsontext"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/androidplatform"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

type platformQuerier interface {
	Query(context.Context, androidplatform.Request) (jsontext.Value, error)
}

var androidIconKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

const provisionCooldown = 30 * time.Second

func (s *Server) registerAndroid() {
	s.registerRoute("POST /api/v1/auth/provision", 15*time.Second, false, false, s.provisionClient)
	s.registerRoute("GET /api/v1/android/apps", 45*time.Second, false, false, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if _, err := resourceQuery(r); err != nil {
			return contract.Result{}, err
		}
		cache := paths.New(s.options.ModuleDir).AndroidIcons()
		if err := os.MkdirAll(cache, 0o700); err != nil {
			return contract.Result{}, err
		}
		data, err := s.queryApps(ctx, cache)
		return contract.Success("android.apps", "Android 应用", data), err
	})
	s.mux.HandleFunc("GET /api/v1/android/icons/{key}", s.readAndroidIcon)
}

func (s *Server) readAndroidIcon(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, queryErr := resourceQuery(r); queryErr != nil || !androidIconKey.MatchString(key) {
		s.fail(w, r.Context(), invalid("图标资源标识无效"))
		return
	}
	file, err := os.Open(filepath.Join(paths.New(s.options.ModuleDir).AndroidIcons(), key+".png"))
	if err != nil {
		if os.IsNotExist(err) {
			err = &contract.Error{Code: "android.icon.not_found", Message: "图标缓存不存在，请刷新应用清单", HTTPStatus: http.StatusNotFound}
		}
		s.fail(w, r.Context(), err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 256<<10 {
		s.fail(w, r.Context(), invalid("图标缓存无效"))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	w.Header().Set("ETag", `"`+key+`"`)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
	http.ServeContent(w, r, key+".png", info.ModTime(), file)
}

func (s *Server) queryApps(ctx context.Context, cache string) (jsontext.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case s.appSlots <- struct{}{}:
		defer func() { <-s.appSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.platform.Query(ctx, androidplatform.Request{Operation: "apps", IconCache: cache})
}

func (s *Server) provisionClient(ctx context.Context, r *http.Request) (contract.Result, error) {
	if !s.provisionMu.TryLock() {
		return contract.Result{}, busy()
	}
	defer s.provisionMu.Unlock()
	if time.Now().Before(s.provisionAfter) {
		return contract.Result{}, busy()
	}
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	if r.Body != nil && r.ContentLength != 0 {
		var body struct{}
		if err := decodeRequestLimit(r, &body, 1024); err != nil {
			return contract.Result{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return contract.Result{}, err
	}
	// 私有 Provider 交付有界执行，失败不能立即占用下一次进程启动。
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	connection := s.config.Connection()
	_, err := s.platform.Query(ctx, androidplatform.Request{Operation: "provision", Connection: &connection})
	delay := provisionCooldown
	if err != nil {
		// 首次失败保留安装后的短重试窗口，连续失败最多退避到 30 秒。
		s.provisionBackoff = min(provisionCooldown, max(3*time.Second, 2*s.provisionBackoff))
		delay = s.provisionBackoff
	} else {
		s.provisionBackoff = 0
	}
	s.provisionAfter = time.Now().Add(delay)
	return contract.Success("api.provisioned", "连接资料已交付到官方管理器", map[string]bool{"delivered": true}), err
}
