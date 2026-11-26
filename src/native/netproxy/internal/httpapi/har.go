package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

func (s *Server) registerHARRoutes() {
	s.mux.HandleFunc("GET "+trafficRoute+"export", s.exportHAR)
	s.route("POST "+trafficRoute+"import", 30*time.Second, true, s.importHAR)
}

func (s *Server) exportHAR(w http.ResponseWriter, r *http.Request) {
	select {
	case s.expensive <- struct{}{}:
		defer func() { <-s.expensive }()
	default:
		s.fail(w, r.Context(), busy())
		return
	}
	query, err := resourceQuery(r, "batch", "id", "host", "query", "method", "status_class", "tls", "rewritten", "app", "outbound", "response_types")
	if err != nil {
		s.fail(w, r.Context(), err)
		return
	}
	status := 0
	if value := query.Get("status_class"); value != "" {
		status, err = strconv.Atoi(value)
		if err != nil || status < 1 || status > 5 {
			s.fail(w, r.Context(), harAPIError(traffic.ErrHARFilter))
			return
		}
	}
	rewritten := query.Get("rewritten")
	if rewritten != "" && rewritten != "true" && rewritten != "false" {
		s.fail(w, r.Context(), harAPIError(traffic.ErrHARFilter))
		return
	}
	types, ok := traffic.ParseResponseTypes(query.Get("response_types"))
	if !ok || query.Has("response_types") && types == 0 {
		s.fail(w, r.Context(), harAPIError(traffic.ErrHARFilter))
		return
	}
	export, err := s.traffic.PrepareHARExport(traffic.HARFilter{Batch: query.Get("batch"), ID: query.Get("id"),
		Host: query.Get("host"), Query: query.Get("query"), Method: query.Get("method"), StatusClass: status,
		TLS: query.Get("tls"), Rewritten: rewritten == "true", App: query.Get("app"), Outbound: query.Get("outbound"), ResponseTypes: types})
	if err != nil {
		s.fail(w, r.Context(), harAPIError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="capture.har"`)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	if err := export.Write(r.Context(), w); err != nil {
		panic(http.ErrAbortHandler)
	}
}

func (s *Server) importHAR(ctx context.Context, r *http.Request) (contract.Result, error) {
	query, err := resourceQuery(r, "name")
	if err != nil {
		return contract.Result{}, err
	}
	if strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]) != "application/json" {
		return contract.Result{}, invalid("请求正文必须为 application/json")
	}
	limit := func() error {
		return &contract.Error{Code: "api.body_too_large", Message: "请求正文超出大小限制", HTTPStatus: http.StatusRequestEntityTooLarge}
	}
	if r.ContentLength > traffic.MaxHARImportBytes {
		return contract.Result{}, limit()
	}
	batch, err := s.traffic.ImportHAR(ctx, query.Get("name"), http.MaxBytesReader(nil, r.Body, traffic.MaxHARImportBytes))
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return contract.Result{}, limit()
	}
	if err != nil {
		return contract.Result{}, harAPIError(err)
	}
	return contract.Success("traffic.har_imported", "HAR 已导入", batch), nil
}

func harAPIError(err error) error {
	switch {
	case errors.Is(err, traffic.ErrHARMissing):
		return &contract.Error{Code: "traffic.har_not_found", Message: err.Error(), HTTPStatus: http.StatusNotFound}
	case errors.Is(err, traffic.ErrHARInvalid):
		return &contract.Error{Code: "traffic.har_invalid", Message: err.Error(), HTTPStatus: http.StatusUnprocessableEntity}
	case errors.Is(err, traffic.ErrHARFilter):
		return invalid(err.Error())
	default:
		return captureHistoryError()
	}
}
