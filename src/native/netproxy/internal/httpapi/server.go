package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

const (
	bodyLimit        = 8 << 20
	maxOperationTime = 10 * time.Minute
)

type Server struct {
	options     moduleapp.Options
	config      apiconfig.Config
	mux         *http.ServeMux
	instance    string
	expensive   chan struct{}
	writeSlots  chan struct{}
	stopping    atomic.Bool
	eventMu     sync.Mutex
	events      *eventHub
	admissionMu sync.Mutex
	inFlight    sync.WaitGroup
	lifetime    context.Context
	stopEvents  context.CancelFunc
}

func New(options moduleapp.Options, config apiconfig.Config) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return nil, err
	}
	s := &Server{options: options, config: config, mux: http.NewServeMux(),
		instance: hex.EncodeToString(identity[:]), expensive: make(chan struct{}, 4), writeSlots: make(chan struct{}, 8)}
	s.lifetime, s.stopEvents = context.WithCancel(context.Background())
	s.route("GET /api/v1/info", 5*time.Second, false, s.info)
	s.registerResources()
	s.mux.HandleFunc("GET /api/v1/events", s.streamEvents)
	s.mux.HandleFunc("GET /api/v1/diagnostics/export", s.exportDiagnostics)
	// WebUI 与 HTTP API 共享同一个受限 loopback Origin，不放开跨域策略。
	static := http.FileServer(http.Dir(filepath.Join(options.ModuleDir, "webroot", "netproxy", "control")))
	s.mux.Handle("GET /ui/", http.StripPrefix("/ui/", static))
	s.mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusPermanentRedirect)
	})
	s.mux.HandleFunc("/", s.unmatched)
	return s, nil
}

func (s *Server) unmatched(w http.ResponseWriter, r *http.Request) {
	var allowed []string
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		probe := r.Clone(r.Context())
		probe.Method = method
		_, pattern := s.mux.Handler(probe)
		if pattern != "/" && pattern != "" {
			allowed = append(allowed, method)
		}
	}
	if len(allowed) != 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		writeResult(w, http.StatusMethodNotAllowed, contract.Result{Schema: 1, Code: "api.method_not_allowed", Message: "此接口不支持该请求方法"})
		return
	}
	writeResult(w, http.StatusNotFound, contract.Result{Schema: 1, Code: "api.not_found", Message: "管理接口不存在"})
}

func (s *Server) Serve(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return err
	}
	return s.serveListener(ctx, listener)
}

func (s *Server) serveListener(ctx context.Context, listener net.Listener) error {
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	var serveErr error
	finished := false
	select {
	case serveErr = <-done:
		finished = true
	case <-ctx.Done():
	}
	s.admissionMu.Lock()
	s.stopping.Store(true)
	s.admissionMu.Unlock()
	s.stopEvents()
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), maxOperationTime+15*time.Second)
	defer cancel()
	err := server.Shutdown(drain)
	if err != nil {
		// 关闭连接不等于领域写入完成，PID 必须保留到所有处理器真正退出。
		_ = server.Close()
	}
	if !finished {
		serveErr = <-done
	}
	s.inFlight.Wait()
	s.eventMu.Lock()
	hub := s.events
	s.eventMu.Unlock()
	if hub != nil {
		<-hub.done
	}
	if !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
	}
	return err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(15 * time.Second))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	if r.Host != s.config.Listen {
		writeResult(w, http.StatusForbidden, contract.Result{Schema: 1, Code: "api.host_invalid", Message: "管理接口 Host 无效"})
		return
	}
	if len(r.Header.Values("Origin")) > 1 {
		writeResult(w, http.StatusForbidden, contract.Result{Schema: 1, Code: "api.origin_invalid", Message: "此网页不能访问管理接口"})
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "http" || parsed.Host != s.config.Listen || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			writeResult(w, http.StatusForbidden, contract.Result{Schema: 1, Code: "api.origin_invalid", Message: "此网页不能访问管理接口"})
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	query, queryErr := url.ParseQuery(r.URL.RawQuery)
	if queryErr != nil {
		writeResult(w, http.StatusBadRequest, contract.Result{Schema: 1, Code: "api.query_invalid", Message: "查询参数无效"})
		return
	}
	for key, values := range query {
		if len(values) != 1 || slices.Contains([]string{"token", "secret", "access_token"}, strings.ToLower(key)) {
			writeResult(w, http.StatusBadRequest, contract.Result{Schema: 1, Code: "api.query_invalid", Message: "查询参数无效，凭据只能通过 Authorization 传递"})
			return
		}
	}
	// 仅公开静态页面，所有 /api/v1/* 资源及 SSE 仍强制 Bearer 鉴权。
	// Host、Origin 和查询参数检查已在上方完成，未放宽安全边界。
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		(r.URL.Path == "/ui" || strings.HasPrefix(r.URL.Path, "/ui/")) {
		s.mux.ServeHTTP(w, r)
		return
	}
	authorization := r.Header.Values("Authorization")
	if len(authorization) != 1 || subtle.ConstantTimeCompare([]byte(authorization[0]), []byte("Bearer "+s.config.Secret)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeResult(w, http.StatusUnauthorized, contract.Result{Schema: 1, Code: "api.unauthorized", Message: "需要有效的管理接口凭据"})
		return
	}
	// ServeMux 的 GET 同时匹配 HEAD；管理 API 不允许它进入业务或持续订阅。
	if r.Method == http.MethodHead {
		s.unmatched(w, r)
		return
	}
	s.admissionMu.Lock()
	if s.stopping.Load() {
		s.admissionMu.Unlock()
		writeResult(w, http.StatusServiceUnavailable, contract.Result{Schema: 1, Code: "api.stopping", Message: "后台管理服务正在停止"})
		return
	}
	s.inFlight.Add(1)
	s.admissionMu.Unlock()
	defer s.inFlight.Done()
	s.mux.ServeHTTP(w, r)
}

func (s *Server) route(pattern string, budget time.Duration, expensive bool, handler func(context.Context, *http.Request) (contract.Result, error)) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		mutation := r.Method != http.MethodGet
		if mutation {
			select {
			case s.writeSlots <- struct{}{}:
				defer func() { <-s.writeSlots }()
			default:
				s.fail(w, r.Context(), busy())
				return
			}
		}
		if expensive {
			select {
			case s.expensive <- struct{}{}:
				defer func() { <-s.expensive }()
			default:
				s.fail(w, r.Context(), busy())
				return
			}
		}
		if err := r.Context().Err(); err != nil {
			s.fail(w, r.Context(), err)
			return
		}
		parent := r.Context()
		if mutation {
			parent = context.WithoutCancel(parent)
		}
		ctx, cancel := context.WithTimeout(parent, min(budget, maxOperationTime))
		defer cancel()
		response, err := handler(ctx, r)
		if err != nil {
			s.fail(w, ctx, err)
			return
		}
		if pattern != "GET /api/v1/info" {
			moduleapp.RecordActivity(ctx, s.options, false)
		}
		writeResult(w, http.StatusOK, response)
	})
}

func (s *Server) fail(w http.ResponseWriter, ctx context.Context, err error) {
	response, _ := contract.Failure(ctx, err)
	writeResult(w, failureStatus(response.Code, err), response)
}

func failureStatus(code string, err error) int {
	switch {
	case code == "usage.invalid", code == "api.query_invalid":
		return http.StatusBadRequest
	case strings.HasSuffix(code, ".conflict"), strings.HasSuffix(code, ".busy"):
		return http.StatusConflict
	case strings.HasSuffix(code, ".not_found"), errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case code == "command.timeout":
		return http.StatusGatewayTimeout
	case code == "command.canceled":
		return http.StatusRequestTimeout
	case strings.HasSuffix(code, ".runtime_sync_failed"):
		return http.StatusBadGateway
	case strings.HasSuffix(code, ".invalid"), strings.HasSuffix(code, "_invalid"), strings.HasPrefix(code, "inbound."), strings.HasPrefix(code, "tun."), strings.HasPrefix(code, "ebpf."):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func writeResult(w http.ResponseWriter, status int, response contract.Result) {
	if status >= 400 {
		w.Header().Set("Connection", "close")
		// 拒绝请求后不再等待正文，避免自动 drain 被未鉴权慢客户端占住。
		_ = http.NewResponseController(w).SetReadDeadline(time.Now())
	}
	content, err := json.Marshal(response, json.Deterministic(true))
	if err != nil {
		status = http.StatusInternalServerError
		content = []byte(`{"schema":1,"ok":false,"code":"api.encode_failed","message":"管理接口响应编码失败"}`)
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(content, '\n'))
}

func decodeRequest(r *http.Request, target any) error {
	return decodeRequestLimit(r, target, bodyLimit)
}

func decodeRequestLimit(r *http.Request, target any, limit int64) error {
	if media := strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]); media != "application/json" {
		return invalid("请求正文必须为 application/json")
	}
	content, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || len(content) > int(limit) {
		return invalid("请求正文读取失败或超出大小限制")
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(content))
	token, err := decoder.ReadToken()
	if err != nil || token.Kind() != '{' {
		return invalid("请求正文必须为 JSON 对象")
	}
	for {
		token, err := decoder.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil || token.Kind() == 'n' {
			return invalid("请求 JSON 无效，字段不能为 null")
		}
	}
	if err := json.Unmarshal(content, target, json.RejectUnknownMembers(true)); err != nil {
		return invalid("请求 JSON 无效、字段类型错误或包含未知字段")
	}
	return nil
}

func invalid(message string) error {
	return &contract.Error{Code: "usage.invalid", Message: message, Status: 2}
}
func busy() error {
	return &contract.Error{Code: "api.busy", Message: "操作正在进行，请稍后再试"}
}

func (s *Server) controlOptions() service.Options {
	layout := paths.New(s.options.ModuleDir)
	return service.Options{CatalogRoot: s.options.CatalogRoot, ModuleConfig: s.options.ModuleConfig,
		SingBoxConfig: paths.SingBoxConfig(s.options.SingBoxDir), InboundConfig: s.options.InboundConfig,
		StateFile: s.options.StateFile, ProgressDir: s.options.ProgressDir, DelayDir: layout.DelayDir(),
		WorkerPIDFile: s.options.WorkerPIDFile, SingBoxPath: s.options.SingBoxPath,
		ServiceAddress: s.options.ServiceAddress, ServiceSecret: s.options.ServiceSecret, RequestTimeout: s.options.RequestTimeout}
}

func (s *Server) info(context.Context, *http.Request) (contract.Result, error) {
	version, build := "unknown", "unknown"
	if content, err := os.ReadFile(paths.New(s.options.ModuleDir).ModuleProp()); err == nil {
		for line := range strings.SplitSeq(string(content), "\n") {
			key, value, _ := strings.Cut(strings.TrimSpace(line), "=")
			switch key {
			case "version":
				version = value
			case "versionCode":
				build = value
			}
		}
	}
	return contract.Success("api.info", "模块管理接口", map[string]any{"api_version": 1, "version": version,
		"version_code": build, "instance_id": s.instance, "pid": os.Getpid(), "platform": runtime.GOOS,
		"capabilities": []string{"module", "catalog", "subscriptions", "offline_delay", "events"}}), nil
}
