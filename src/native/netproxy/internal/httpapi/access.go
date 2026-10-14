package httpapi

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func Probe(ctx context.Context, config apiconfig.Config) (map[string]any, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, config.Connection().Address+"/api/v1/info", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Secret)
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true, TLSClientConfig: config.ClientTLS()}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("管理接口尚未就绪或凭据不匹配")
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	var result struct {
		Schema int            `json:"schema"`
		OK     bool           `json:"ok"`
		Code   string         `json:"code"`
		Data   map[string]any `json:"data"`
	}
	if err := json.Unmarshal(content, &result); err != nil || result.Schema != 1 || !result.OK || result.Code != "api.info" || result.Data["instance_id"] == nil || result.Data["pid"] == nil {
		return nil, errors.New("管理接口没有返回有效的就绪身份")
	}
	return result.Data, nil
}

func (s *Server) exportDiagnostics(w http.ResponseWriter, r *http.Request) {
	query, err := resourceQuery(r, "manager_version", "manager_version_code")
	if err != nil {
		s.fail(w, r.Context(), err)
		return
	}
	options := s.options
	if value := query.Get("manager_version"); value != "" {
		if len(value) > 64 || !diagnosticVersion.MatchString(value) {
			s.fail(w, r.Context(), invalid("管理器版本无效"))
			return
		}
		options.ManagerVersion = value
	}
	if value := query.Get("manager_version_code"); value != "" {
		if len(value) > 20 || !diagnosticBuild.MatchString(value) {
			s.fail(w, r.Context(), invalid("管理器版本号无效"))
			return
		}
		options.ManagerVersionCode = value
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	select {
	case s.expensive <- struct{}{}:
		defer func() { <-s.expensive }()
	default:
		s.fail(w, r.Context(), busy())
		return
	}
	directory := paths.New(s.options.ModuleDir).APITempDir()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		s.fail(w, r.Context(), err)
		return
	}
	file, err := os.CreateTemp(directory, "diagnostics-")
	if err != nil {
		s.fail(w, r.Context(), err)
		return
	}
	defer os.Remove(file.Name())
	err = moduleapp.ExportDiagnostics(options, contextWriter{ctx: ctx, Writer: file})
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		s.fail(w, r.Context(), err)
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="NetProxy_Diagnostics.tar.gz"`)
	w.Header().Set("Content-Type", "application/gzip")
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Minute))
	http.ServeFile(w, r, file.Name())
}

var diagnosticVersion = regexp.MustCompile(`^[A-Za-z0-9.+_-]+$`)
var diagnosticBuild = regexp.MustCompile(`^[0-9]+$`)

type contextWriter struct {
	ctx context.Context
	io.Writer
}

func (writer contextWriter) Write(content []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	return writer.Writer.Write(content)
}
