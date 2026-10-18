package httpapi

import (
	"context"
	"net/http"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func (s *Server) registerCatalogGroups() {
	s.route("POST /api/v1/catalog", resourceWriteTimeout, true, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		var input struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := resourceDecode(r, &input); err != nil {
			return contract.Result{}, err
		}
		if err := resourceGroupID(input.ID, false); err != nil {
			return contract.Result{}, err
		}
		if err := module.CreateLocalGroup(ctx, s.options, input.ID, input.Name); err != nil {
			return contract.Result{}, err
		}
		return contract.Success("catalog.created", "本地资源已创建", map[string]string{"id": input.ID}), nil
	})
	s.route("PUT /api/v1/catalog/{id}", resourceWriteTimeout, true, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		var input struct {
			Name string `json:"name"`
		}
		if err := resourceDecode(r, &input); err != nil {
			return contract.Result{}, err
		}
		id := r.PathValue("id")
		if err := resourceGroupID(id, false); err != nil {
			return contract.Result{}, err
		}
		if err := module.RenameLocalGroup(ctx, s.options, id, input.Name); err != nil {
			return contract.Result{}, err
		}
		return contract.Success("catalog.renamed", "本地资源已重命名", map[string]string{"id": id}), nil
	})
	s.route("DELETE /api/v1/catalog/{id}", resourceWriteTimeout, true, func(ctx context.Context, r *http.Request) (contract.Result, error) {
		if err := resourceEmptyRequest(r); err != nil {
			return contract.Result{}, err
		}
		id := r.PathValue("id")
		if err := resourceGroupID(id, false); err != nil {
			return contract.Result{}, err
		}
		if id == "default" {
			return contract.Result{}, invalid("默认本地资源不能删除")
		}
		if err := module.RemoveLocalGroup(ctx, s.options, id); err != nil {
			return contract.Result{}, err
		}
		return contract.Success("catalog.removed", "本地资源已删除", map[string]string{"id": id}), nil
	})
}
