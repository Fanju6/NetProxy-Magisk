package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"runtime/metrics"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/androidplatform"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

const (
	bodyLimit        = 4 << 20
	maxOperationTime = 10 * time.Minute
)

type Server struct {
	options          moduleapp.Options
	config           apiconfig.Config
	mux              *http.ServeMux
	instance         string
	version          string
	versionCode      string
	expensive        chan struct{}
	writeSlots       chan struct{}
	stopping         atomic.Bool
	eventMu          sync.Mutex
	events           *eventHub
	admissionMu      sync.Mutex
	inFlight         sync.WaitGroup
	lifetime         context.Context
	stopEvents       context.CancelFunc
	platform         platformQuerier
	appSlots         chan struct{}
	provisionMu      sync.Mutex
	provisionAfter   time.Time
	provisionBackoff time.Duration
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
		instance: hex.EncodeToString(identity[:]), expensive: make(chan struct{}, 4), writeSlots: make(chan struct{}, 8),
		platform: androidplatform.Client{DEX: paths.New(options.ModuleDir).AndroidPlatformDEX()}, appSlots: make(chan struct{}, 1)}
	s.lifetime, s.stopEvents = context.WithCancel(context.Background())
	s.options.TransferContext = s.lifetime
	s.version, s.versionCode = readModuleVersion(paths.New(options.ModuleDir).ModuleProp())
	s.route("GET /api/v1/info", 5*time.Second, false, s.info)
	s.registerResources()
	s.registerAndroid()
	s.mux.HandleFunc("GET /api/v1/events", s.streamEvents)
	s.mux.HandleFunc("GET /api/v1/diagnostics/export", s.exportDiagnostics)
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
	tlsConfig, err := s.config.ServerTLS()
	if err != nil {
		listener.Close()
		return err
	}
	listener = tls.NewListener(listener, tlsConfig)
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
	err = server.Shutdown(drain)
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
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
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
		if err != nil || parsed.Scheme != "https" || parsed.Host != s.config.Listen || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
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
	authorization := r.Header.Values("Authorization")
	provision := r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/provision"
	if provision && (r.Header.Get("Origin") != "" || len(query) != 0 || len(authorization) != 0) {
		writeResult(w, http.StatusForbidden, contract.Result{Schema: 1, Code: "api.provision_invalid", Message: "自动连接请求无效"})
		return
	}
	if !provision && (len(authorization) != 1 || subtle.ConstantTimeCompare([]byte(authorization[0]), []byte("Bearer "+s.config.Secret)) != 1) {
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
	s.registerRoute(pattern, budget, expensive, !strings.HasPrefix(pattern, "GET "), handler)
}

func (s *Server) registerRoute(pattern string, budget time.Duration, expensive, mutation bool, handler func(context.Context, *http.Request) (contract.Result, error)) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
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
		if pattern != "GET /api/v1/info" && pattern != "POST /api/v1/auth/provision" && !strings.Contains(pattern, "/android/") {
			moduleapp.RecordActivity(ctx, s.options, false)
		}
		writeResult(w, http.StatusOK, response)
	})
}

func (s *Server) fail(w http.ResponseWriter, ctx context.Context, err error) {
	response, _ := contract.Failure(ctx, err)
	response = resourcePublicFailure(response)
	writeResult(w, failureStatus(response.Code, err), response)
}

func failureStatus(code string, err error) int {
	if value, ok := errors.AsType[*contract.Error](err); ok && value.HTTPStatus != 0 {
		return value.HTTPStatus
	}
	if _, ok := errors.AsType[*inbound.ValidationError](err); ok {
		return http.StatusUnprocessableEntity
	}
	switch code {
	case "usage.invalid", "api.query_invalid", "policy.invalid":
		return http.StatusBadRequest
	case "config.conflict", "subscription.conflict", "subscription.busy", "api.busy", "node.delay_busy",
		"policy.core_not_ready", "catalog.provider_in_use", "catalog.provider_conflict", "catalog.outbound_conflict":
		return http.StatusConflict
	case "android.icon.not_found", "node.delay_target_missing", "policy.group_not_found", "policy.member_not_found":
		return http.StatusNotFound
	case "command.timeout", "node.delay_timeout":
		return http.StatusGatewayTimeout
	case "command.canceled", "subscription.cancelled":
		return http.StatusRequestTimeout
	case "node.runtime_sync_failed", "mode.runtime_sync_failed", "subscription.runtime_sync_failed", "node.delay_api_failed",
		"policy.runtime_sync_failed", "policy.api_failed":
		return http.StatusBadGateway
	case "subscription.invalid_target", "subscription.invalid_group", "subscription.read_only", "subscription.interval_too_short",
		"subscription.edit_invalid", "subscription.proxy_mode_invalid", "subscription.timeout_invalid", "subscription.text_invalid", "mode.invalid", "mode.unavailable",
		"ebpf.status_invalid", "ebpf.unsupported", "network.direct_route_required",
		"policy.not_selectable", "policy.config_invalid", "catalog.invalid", "catalog.read_only":
		return http.StatusUnprocessableEntity
	default:
		if errors.Is(err, os.ErrNotExist) {
			return http.StatusNotFound
		}
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
	tooLarge := func() error {
		return &contract.Error{Code: "api.body_too_large", Message: "请求正文超出大小限制", HTTPStatus: http.StatusRequestEntityTooLarge}
	}
	if r.ContentLength > limit {
		return tooLarge()
	}
	var body bytes.Buffer
	if r.ContentLength > 0 {
		body.Grow(int(r.ContentLength) + bytes.MinRead)
	}
	if _, err := body.ReadFrom(io.LimitReader(r.Body, limit+1)); err != nil {
		return invalid("请求正文读取失败")
	}
	content := body.Bytes()
	if int64(len(content)) > limit {
		return tooLarge()
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
	return &contract.Error{Code: "usage.invalid", Message: message, Status: 2, HTTPStatus: http.StatusBadRequest}
}
func busy() error {
	return &contract.Error{Code: "api.busy", Message: "操作正在进行，请稍后再试", HTTPStatus: http.StatusConflict}
}

func (s *Server) controlOptions() service.Options {
	return service.Options{CatalogRoot: s.options.CatalogRoot, ModuleConfig: s.options.ModuleConfig,
		SingBoxConfig: paths.SingBoxConfig(s.options.SingBoxDir), InboundConfig: s.options.InboundConfig,
		StateFile: s.options.StateFile, ProgressDir: s.options.ProgressDir,
		WorkerPIDFile: s.options.WorkerPIDFile, SingBoxPath: s.options.SingBoxPath,
		ServiceAddress: s.options.ServiceAddress, ServiceSecret: s.options.ServiceSecret, RequestTimeout: s.options.RequestTimeout}
}

func (s *Server) info(context.Context, *http.Request) (contract.Result, error) {
	memory := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/heap/unused:bytes"},
		{Name: "/memory/classes/heap/released:bytes"}, {Name: "/memory/classes/heap/stacks:bytes"}}
	metrics.Read(memory)
	return contract.Success("api.info", "模块管理接口", map[string]any{"api_version": 1, "version": s.version,
		"memory": map[string]uint64{"heap_alloc_bytes": memory[0].Value.Uint64(), "heap_inuse_bytes": memory[0].Value.Uint64() + memory[1].Value.Uint64(),
			"heap_released_bytes": memory[2].Value.Uint64(), "stack_inuse_bytes": memory[3].Value.Uint64()},
		"version_code": s.versionCode, "instance_id": s.instance, "pid": os.Getpid(), "platform": runtime.GOOS,
		"capabilities": []string{"module", "catalog", "subscriptions", "policy_groups", "events"}}), nil
}

func readModuleVersion(path string) (string, string) {
	version, build := "unknown", "unknown"
	if content, err := os.ReadFile(path); err == nil {
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
	return version, build
}
