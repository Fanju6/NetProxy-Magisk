package httpapi

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

const (
	resourceReadTimeout         = 30 * time.Second
	resourceWriteTimeout        = 120 * time.Second
	resourceSubscriptionTimeout = 10 * time.Minute
	resourceMaxLogLines         = 2000
)

func (s *Server) registerResources() {
	s.route("GET /api/v1/service", resourceReadTimeout, false, s.readService)
	for _, action := range []string{"start", "stop", "restart", "reload", "check"} {
		s.route("POST /api/v1/service/"+action, resourceWriteTimeout, action != "stop", func(ctx context.Context, r *http.Request) (contract.Result, error) {
			if err := resourceEmptyRequest(r); err != nil {
				return contract.Result{}, err
			}
			data, err := module.ManageService(ctx, s.options, action)
			return contract.Success("service."+action, "服务操作完成", data), err
		})
	}
	s.route("GET /api/v1/catalog", resourceReadTimeout, false, s.readCatalog)
	s.route("GET /api/v1/nodes", resourceReadTimeout, false, s.readNodes)
	s.route("GET /api/v1/nodes/snapshot", resourceReadTimeout, false, s.readNodeSnapshot)
	s.route("GET /api/v1/nodes/get", resourceReadTimeout, false, s.readNode)
	s.route("GET /api/v1/nodes/export", resourceReadTimeout, false, s.exportNode)
	s.route("POST /api/v1/nodes", resourceWriteTimeout, true, s.addNode)
	s.route("PUT /api/v1/nodes", resourceWriteTimeout, true, s.editNode)
	s.route("DELETE /api/v1/nodes", resourceWriteTimeout, true, s.removeNode)
	s.route("POST /api/v1/nodes/import", resourceWriteTimeout, true, s.importNodes)
	s.route("POST /api/v1/nodes/delay", resourceWriteTimeout, true, s.delayNodes)
	s.route("GET /api/v1/selection", resourceReadTimeout, false, s.readSelection)
	s.route("PUT /api/v1/selection", resourceWriteTimeout, false, s.saveSelection)
	s.route("GET /api/v1/mode", resourceReadTimeout, false, s.readMode)
	s.route("PUT /api/v1/mode", resourceWriteTimeout, true, s.saveMode)
	s.route("GET /api/v1/subscriptions", resourceReadTimeout, false, s.listSubscriptions)
	s.route("POST /api/v1/subscriptions", resourceSubscriptionTimeout, true, s.addSubscription)
	s.route("GET /api/v1/subscriptions/{id}", resourceReadTimeout, false, s.showSubscription)
	s.route("PUT /api/v1/subscriptions/{id}", resourceSubscriptionTimeout, true, s.editSubscription)
	s.route("DELETE /api/v1/subscriptions/{id}", resourceWriteTimeout, true, s.removeSubscription)
	s.route("POST /api/v1/subscriptions/{id}/update", resourceSubscriptionTimeout, true, s.updateSubscription)
	s.route("POST /api/v1/subscriptions/update-all", resourceSubscriptionTimeout, true, s.updateAllSubscriptions)
	s.route("GET /api/v1/subscriptions/{id}/history", resourceReadTimeout, false, s.subscriptionHistory)
	s.route("GET /api/v1/subscriptions/{id}/progress", resourceReadTimeout, false, s.subscriptionProgress)
	s.route("POST /api/v1/subscriptions/{id}/cancel", resourceReadTimeout, false, s.cancelSubscription)
	s.route("GET /api/v1/configs", resourceReadTimeout, false, s.listConfigs)
	s.route("GET /api/v1/config", resourceReadTimeout, false, s.readConfig)
	s.route("PUT /api/v1/config", resourceWriteTimeout, true, s.saveConfig)
	s.route("POST /api/v1/config/validate", resourceWriteTimeout, true, s.validateConfig)
	s.route("GET /api/v1/network/wifi-list", resourceReadTimeout, false, s.listWiFi)
	s.route("GET /api/v1/app-policy", resourceReadTimeout, false, s.readAppPolicy)
	s.route("POST /api/v1/diagnostics/ebpf", resourceWriteTimeout, true, s.probeEBPF)
	s.route("GET /api/v1/logs", resourceReadTimeout, false, s.readLogs)
	s.route("DELETE /api/v1/logs", resourceReadTimeout, false, s.clearLogs)
}

func (s *Server) readService(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := module.ManageService(ctx, s.options, "status")
	return contract.Success("service.status", "服务状态", data.Status), err
}

func (s *Server) catalogSummaries(ctx context.Context) ([]catalog.GroupSummary, error) {
	active := ""
	config, err := moduleconfig.LoadModule(s.options.ModuleConfig)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		active = config.ActiveGroupID
	}
	groups, err := catalog.Scan(ctx, catalog.ScanOptions{
		Root: s.options.CatalogRoot, Type: "all", ActiveGroup: active, ProgressDir: s.options.ProgressDir,
	})
	if err != nil {
		return nil, err
	}
	data := make([]catalog.GroupSummary, 0, len(groups))
	for _, group := range groups {
		data = append(data, group.Group)
	}
	return data, nil
}

func (s *Server) readCatalog(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := s.catalogSummaries(ctx)
	return contract.Success("catalog.groups", "Catalog 分组快照", data), err
}

func (s *Server) readNodes(ctx context.Context, r *http.Request) (contract.Result, error) {
	group, err := resourceGroupQuery(r)
	if err != nil {
		return contract.Result{}, err
	}
	data, err := service.ReadNodes(ctx, s.controlOptions(), group)
	return contract.Success("node.list", "节点列表", data), err
}

func (s *Server) readNodeSnapshot(ctx context.Context, r *http.Request) (contract.Result, error) {
	group, err := resourceGroupQuery(r)
	if err != nil {
		return contract.Result{}, err
	}
	data, err := service.ReadSnapshot(ctx, s.controlOptions(), group)
	return contract.Success("node.snapshot", "节点快照", data), err
}

func (s *Server) readNode(ctx context.Context, r *http.Request) (contract.Result, error) {
	group, tag, err := resourceNodeQuery(r)
	if err != nil {
		return contract.Result{}, err
	}
	document, err := catalog.GroupNode(ctx, s.options.CatalogRoot, group, tag)
	if err != nil {
		return contract.Result{}, err
	}
	content, err := provider.Marshal(ctx, document)
	return contract.Success("node.loaded", "节点配置已读取", jsontext.Value(content)), err
}

func (s *Server) exportNode(ctx context.Context, r *http.Request) (contract.Result, error) {
	group, tag, err := resourceNodeQuery(r)
	if err != nil {
		return contract.Result{}, err
	}
	data, err := catalog.ExportGroupNode(ctx, s.options.CatalogRoot, group, tag)
	return contract.Success("node.exported", "节点分享链接已生成", data), err
}

type resourceNodeContent struct {
	GroupID       string `json:"group_id"`
	Content       string `json:"content"`
	AllowInsecure bool   `json:"allow_insecure"`
}

func (s *Server) addNode(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input resourceNodeContent
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	if input.GroupID == "" {
		input.GroupID = "default"
	}
	if err := resourceGroupID(input.GroupID, false); err != nil {
		return contract.Result{}, err
	}
	if strings.TrimSpace(input.Content) == "" {
		return contract.Result{}, invalid("节点内容不能为空")
	}
	data, err := module.NodeAppendContent(ctx, s.options, input.GroupID, []byte(input.Content), input.AllowInsecure)
	return contract.Success("node.added", "节点已加入本地配置", data), err
}

func (s *Server) editNode(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input struct {
		resourceNodeContent
		NodeTag string `json:"node_tag"`
	}
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	if err := resourceNodeTarget(input.GroupID, input.NodeTag); err != nil {
		return contract.Result{}, err
	}
	if strings.TrimSpace(input.Content) == "" {
		return contract.Result{}, invalid("节点内容不能为空")
	}
	data, err := module.NodeEditContent(ctx, s.options, input.GroupID+"/"+input.NodeTag, []byte(input.Content), input.AllowInsecure)
	return contract.Success("node.edited", "节点已更新", data), err
}

func (s *Server) removeNode(ctx context.Context, r *http.Request) (contract.Result, error) {
	if err := resourceEmptyBody(r); err != nil {
		return contract.Result{}, err
	}
	group, tag, err := resourceNodeQuery(r)
	if err != nil {
		return contract.Result{}, err
	}
	data, err := module.NodeRemove(ctx, s.options, group+"/"+tag)
	return contract.Success("node.removed", "节点已删除", data), err
}

func (s *Server) importNodes(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input struct {
		Content       string `json:"content"`
		AllowInsecure bool   `json:"allow_insecure"`
	}
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	if err := decodeRequest(r, &input); err != nil {
		return contract.Result{}, err
	}
	if strings.TrimSpace(input.Content) == "" {
		return contract.Result{}, invalid("导入内容不能为空")
	}
	data, err := module.NodeImportContent(ctx, s.options, []byte(input.Content), input.AllowInsecure)
	return contract.Success("node.imported", "文件节点已加入本地配置", data), err
}

func (s *Server) delayNodes(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input struct {
		GroupID string `json:"group_id"`
		NodeTag string `json:"node_tag"`
	}
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	if err := resourceGroupID(input.GroupID, false); err != nil {
		return contract.Result{}, err
	}
	target := "auto"
	if input.NodeTag != "" {
		target = input.GroupID + "/" + input.NodeTag
	}
	data, err := service.Delay(ctx, s.controlOptions(), target, input.GroupID)
	return contract.Success("node.delay", "节点测速完成", data), err
}

func (s *Server) readSelection(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := service.ReadSelection(ctx, s.controlOptions())
	return contract.Success("node.current", "当前节点选择", data), err
}

func (s *Server) saveSelection(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input struct {
		GroupID string `json:"group_id"`
		NodeTag string `json:"node_tag"`
	}
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	if err := resourceGroupID(input.GroupID, false); err != nil {
		return contract.Result{}, err
	}
	target := "auto"
	if input.NodeTag != "" {
		target = input.GroupID + "/" + input.NodeTag
	}
	data, err := module.SelectNode(ctx, s.options, target, input.GroupID)
	return contract.Success("node.selected", "节点选择已更新", data), err
}

func (s *Server) readMode(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := module.ReadMode(ctx, s.options)
	return contract.Success("mode.current", "当前出站模式", data), err
}

func (s *Server) saveMode(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input struct {
		Mode string `json:"mode"`
	}
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	if strings.TrimSpace(input.Mode) == "" {
		return contract.Result{}, invalid("出站模式不能为空")
	}
	data, err := module.ApplyMode(ctx, s.options, input.Mode)
	return contract.Success("mode.changed", "默认出站模式已保存", data), err
}

func (s *Server) listSubscriptions(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := s.catalogSummaries(ctx)
	return contract.Success("subscription.list", "订阅列表", data), err
}

type resourceSubscriptionEdit struct {
	Name           *string            `json:"name"`
	URL            *string            `json:"url"`
	UserAgent      *string            `json:"user_agent"`
	HWID           *string            `json:"hwid"`
	CustomHeaders  *map[string]string `json:"custom_headers"`
	AutoUpdate     *bool              `json:"auto_update"`
	UpdateInterval *int64             `json:"update_interval"`
	UpdateViaProxy *string            `json:"update_via_proxy"`
	Include        *string            `json:"include"`
	Exclude        *string            `json:"exclude"`
	AllowInsecure  *bool              `json:"allow_insecure"`
	Timeout        *int64             `json:"download_timeout"`
}

func (s *Server) addSubscription(ctx context.Context, r *http.Request) (contract.Result, error) {
	input := resourceSubscriptionEdit{}
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	if input.URL == nil || strings.TrimSpace(*input.URL) == "" {
		return contract.Result{}, invalid("订阅 URL 不能为空")
	}
	options := module.SubscriptionOptions{
		Options: s.options, AutoUpdate: true, UpdateInterval: 86400, IntervalSource: "user", UpdateViaProxy: "auto", Timeout: 60,
	}
	if input.Name != nil {
		options.Name = *input.Name
	}
	options.URL = *input.URL
	if input.UserAgent != nil {
		options.UserAgent = *input.UserAgent
	}
	if input.HWID != nil {
		options.HWID = *input.HWID
	}
	if input.CustomHeaders != nil {
		options.Headers = *input.CustomHeaders
	}
	if input.AutoUpdate != nil {
		options.AutoUpdate = *input.AutoUpdate
	}
	if input.UpdateInterval != nil {
		options.UpdateInterval = *input.UpdateInterval
	}
	if input.UpdateViaProxy != nil {
		options.UpdateViaProxy = *input.UpdateViaProxy
	}
	if input.Include != nil {
		options.Include = *input.Include
	}
	if input.Exclude != nil {
		options.Exclude = *input.Exclude
	}
	if input.AllowInsecure != nil {
		options.AllowInsecure = *input.AllowInsecure
	}
	if input.Timeout != nil {
		options.Timeout = *input.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, resourceDownloadBudget(options.Timeout))
	defer cancel()
	data, err := module.AddSubscription(ctx, options)
	return contract.Success("subscription.added", "订阅已添加", data), err
}

func (s *Server) subscriptionMetadata(ctx context.Context, r *http.Request) (catalog.Metadata, error) {
	id := r.PathValue("id")
	if err := resourceGroupID(id, false); err != nil {
		return catalog.Metadata{}, err
	}
	data, err := catalog.PrivateMetadata(ctx, s.options.CatalogRoot, id)
	if err != nil {
		return catalog.Metadata{}, err
	}
	if data.Type != "subscription" {
		return catalog.Metadata{}, &subscription.Error{Code: "subscription.read_only", Message: "目标不是 URL 订阅"}
	}
	if data.ID != id {
		return catalog.Metadata{}, &subscription.Error{Code: "subscription.metadata_read_failed", Message: "订阅元数据 ID 与分组不一致"}
	}
	return data, nil
}

func (s *Server) showSubscription(ctx context.Context, r *http.Request) (contract.Result, error) {
	query, err := resourceQuery(r, "private")
	if err != nil {
		return contract.Result{}, err
	}
	private := false
	if query.Has("private") {
		if query.Get("private") != "true" && query.Get("private") != "false" {
			return contract.Result{}, invalid("private 必须为 true 或 false")
		}
		private = query.Get("private") == "true"
	}
	if private {
		metadata, err := s.subscriptionMetadata(ctx, r)
		return contract.Success("subscription.show", "订阅详情", metadata), err
	}
	id := r.PathValue("id")
	if err := resourceGroupID(id, false); err != nil {
		return contract.Result{}, err
	}
	settings, err := moduleconfig.LoadModule(s.options.ModuleConfig)
	if err != nil {
		return contract.Result{}, err
	}
	groups, err := catalog.Scan(ctx, catalog.ScanOptions{Root: s.options.CatalogRoot, GroupID: id, ActiveGroup: settings.ActiveGroupID, ProgressDir: s.options.ProgressDir, WithNodes: true})
	if err != nil {
		return contract.Result{}, err
	}
	if len(groups) == 0 {
		return contract.Result{}, &contract.Error{Code: "subscription.not_found", Message: "订阅不存在", HTTPStatus: http.StatusNotFound}
	}
	return contract.Success("subscription.show", "订阅详情", groups[0]), nil
}

func (s *Server) editSubscription(ctx context.Context, r *http.Request) (contract.Result, error) {
	var input resourceSubscriptionEdit
	if err := resourceDecode(r, &input); err != nil {
		return contract.Result{}, err
	}
	metadata, err := s.subscriptionMetadata(ctx, r)
	if err != nil {
		return contract.Result{}, err
	}
	timeout := metadata.Timeout
	if input.Timeout != nil {
		timeout = *input.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, resourceDownloadBudget(timeout))
	defer cancel()
	data, err := module.EditSubscription(ctx, s.options, metadata.ID, subscription.EditOptions{
		Name: input.Name, URL: input.URL, UserAgent: input.UserAgent, HWID: input.HWID, CustomHeaders: input.CustomHeaders,
		AutoUpdate: input.AutoUpdate, UpdateInterval: input.UpdateInterval, UpdateViaProxy: input.UpdateViaProxy,
		Include: input.Include, Exclude: input.Exclude, AllowInsecure: input.AllowInsecure, Timeout: input.Timeout,
	})
	return contract.Success("subscription.edited", "订阅设置已更新", data), err
}

func (s *Server) removeSubscription(ctx context.Context, r *http.Request) (contract.Result, error) {
	if err := resourceEmptyBody(r); err != nil {
		return contract.Result{}, err
	}
	query, err := resourceQuery(r, "replacement_group_id")
	if err != nil {
		return contract.Result{}, err
	}
	if err := resourceGroupID(query.Get("replacement_group_id"), true); err != nil {
		return contract.Result{}, err
	}
	metadata, err := s.subscriptionMetadata(ctx, r)
	if err != nil {
		return contract.Result{}, err
	}
	err = module.RemoveSubscription(ctx, s.options, metadata.ID, query.Get("replacement_group_id"))
	return contract.Success("subscription.removed", "订阅已删除", map[string]string{"id": metadata.ID}), err
}

func (s *Server) updateSubscription(ctx context.Context, r *http.Request) (contract.Result, error) {
	if err := resourceEmptyRequest(r); err != nil {
		return contract.Result{}, err
	}
	metadata, err := s.subscriptionMetadata(ctx, r)
	if err != nil {
		return contract.Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, resourceDownloadBudget(metadata.Timeout))
	defer cancel()
	data, err := module.UpdateSubscription(ctx, s.options, metadata.ID)
	return contract.Success("subscription.updated", "订阅更新完成", data), err
}

func (s *Server) updateAllSubscriptions(ctx context.Context, r *http.Request) (contract.Result, error) {
	if err := resourceEmptyRequest(r); err != nil {
		return contract.Result{}, err
	}
	data, err := module.UpdateAllSubscriptions(ctx, s.options)
	if err != nil {
		// Keep the completed per-group outcomes when the ordinary error path discards response.Data.
		failure, _ := contract.Failure(ctx, err)
		partial := map[string]any{"updated": append([]string{}, data.Updated...), "failed": append([]string{}, data.Failed...), "first_error": failure.Data}
		if fields, ok := failure.Data.(map[string]any); ok {
			if persisted, ok := fields["persisted"].(bool); ok {
				partial["persisted"] = persisted
			}
		}
		return contract.Result{}, &contract.Error{Code: failure.Code, Message: failure.Message,
			HTTPStatus: failureStatus(failure.Code, err), Data: partial}
	}
	return contract.Success("subscription.updated_all", "全部订阅更新完成", data), err
}

func (s *Server) subscriptionHistory(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	metadata, err := s.subscriptionMetadata(ctx, r)
	if err != nil {
		return contract.Result{}, err
	}
	data, err := subscription.LoadHistory(filepath.Join(s.options.CatalogRoot, metadata.ID, "history.jsonl"))
	return contract.Success("subscription.history", "订阅更新历史", data), err
}

func (s *Server) subscriptionProgress(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	metadata, err := s.subscriptionMetadata(ctx, r)
	if err != nil {
		return contract.Result{}, err
	}
	summary, err := catalog.ReadGroupSummary(ctx, s.options.CatalogRoot, metadata.ID, s.options.ProgressDir)
	return contract.Success("subscription.progress", "订阅更新进度", map[string]any{"id": metadata.ID, "progress": summary.Progress}), err
}

func (s *Server) cancelSubscription(ctx context.Context, r *http.Request) (contract.Result, error) {
	if err := resourceEmptyRequest(r); err != nil {
		return contract.Result{}, err
	}
	metadata, err := s.subscriptionMetadata(ctx, r)
	if err != nil {
		return contract.Result{}, err
	}
	_, err = subscription.RequestCancel(s.options.CatalogRoot, metadata.ID, s.options.ProgressDir)
	return contract.Success("subscription.cancelled", "已请求取消订阅更新", map[string]string{"id": metadata.ID}), err
}

// 代理下载失败可再直连一次，随后仍需给校验与事务收尾保留预算。
func resourceDownloadBudget(seconds int64) time.Duration {
	if seconds <= 0 {
		seconds = 60
	}
	if seconds >= int64((resourceSubscriptionTimeout-resourceWriteTimeout)/(2*time.Second)) {
		return resourceSubscriptionTimeout
	}
	return 2*time.Duration(seconds)*time.Second + resourceWriteTimeout
}

func (s *Server) listConfigs(_ context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := module.ListConfigs(s.options)
	return contract.Success("config.list", "sing-box 配置列表", data), err
}

func (s *Server) configTarget(r *http.Request) (string, error) {
	query, err := resourceQuery(r, "target")
	if err != nil {
		return "", err
	}
	target := query.Get("target")
	if target == "" {
		return "", invalid("配置目标不能为空")
	}
	if _, err := module.ResolveConfig(s.options, target); err != nil {
		return "", invalid("配置目标无效")
	}
	return target, nil
}

func (s *Server) readConfig(_ context.Context, r *http.Request) (contract.Result, error) {
	target, err := s.configTarget(r)
	if err != nil {
		return contract.Result{}, err
	}
	data, err := module.ReadConfig(s.options, target)
	return contract.Success("config.read", "配置内容", data), err
}

func (s *Server) saveConfig(ctx context.Context, r *http.Request) (contract.Result, error) {
	return s.applyConfigContent(ctx, r, false)
}

func (s *Server) validateConfig(ctx context.Context, r *http.Request) (contract.Result, error) {
	return s.applyConfigContent(ctx, r, true)
}

func (s *Server) applyConfigContent(ctx context.Context, r *http.Request, validateOnly bool) (contract.Result, error) {
	target, err := s.configTarget(r)
	if err != nil {
		return contract.Result{}, err
	}
	var input struct {
		Revision string  `json:"revision"`
		Content  *string `json:"content"`
	}
	if err := decodeRequest(r, &input); err != nil {
		return contract.Result{}, err
	}
	if strings.TrimSpace(input.Revision) == "" || input.Content == nil {
		return contract.Result{}, invalid("必须提供读取时的 revision 与 content 字符串")
	}
	revision, err := module.ApplyConfigContent(ctx, s.options, target, []byte(*input.Content), validateOnly, input.Revision)
	action := "apply"
	if validateOnly {
		action = "validate"
	}
	return contract.Success("config."+action, "配置检查通过", map[string]string{"target": target, "revision": revision}), err
}

func (s *Server) listWiFi(ctx context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := worker.SavedWiFiNetworks(ctx)
	return contract.Success("network.wifi_list", "已保存 Wi-Fi", map[string]any{"ssids": data}), err
}

func (s *Server) readAppPolicy(_ context.Context, r *http.Request) (contract.Result, error) {
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	data, err := module.LoadAppPolicy(s.options.InboundConfig)
	return contract.Success("app.list", "分应用代理配置", data), err
}

func (s *Server) probeEBPF(ctx context.Context, r *http.Request) (contract.Result, error) {
	input := struct {
		Mode string `json:"mode"`
		Raw  bool   `json:"raw"`
	}{Mode: "configured"}
	if _, err := resourceQuery(r); err != nil {
		return contract.Result{}, err
	}
	if r.Body != nil && r.Body != http.NoBody {
		if err := decodeRequest(r, &input); err != nil {
			return contract.Result{}, err
		}
	}
	if !slices.Contains([]string{"configured", "all", "local", "shared"}, input.Mode) {
		return contract.Result{}, invalid("eBPF 检查模式应为 configured、all、local 或 shared")
	}
	data, err := module.ProbeEBPF(ctx, s.options, input.Mode, input.Raw)
	return contract.Success("ebpf.status", "eBPF 能力检查完成", data), err
}

func (s *Server) readLogs(_ context.Context, r *http.Request) (contract.Result, error) {
	query, err := resourceQuery(r, "kind", "lines")
	if err != nil {
		return contract.Result{}, err
	}
	kind, err := resourceLogKind(query)
	if err != nil {
		return contract.Result{}, err
	}
	lines := 200
	if query.Has("lines") {
		lines, err = strconv.Atoi(query.Get("lines"))
		if err != nil || lines <= 0 || lines > resourceMaxLogLines {
			return contract.Result{}, invalid("日志行数必须在 1 到 2000 之间")
		}
	}
	data, err := module.ReadLog(s.options, kind, lines)
	return contract.Success("logs.show", "日志内容", data), err
}

func (s *Server) clearLogs(_ context.Context, r *http.Request) (contract.Result, error) {
	if err := resourceEmptyBody(r); err != nil {
		return contract.Result{}, err
	}
	query, err := resourceQuery(r, "kind")
	if err != nil {
		return contract.Result{}, err
	}
	kind, err := resourceLogKind(query)
	if err != nil {
		return contract.Result{}, err
	}
	err = module.ClearLog(s.options, kind)
	return contract.Success("logs.cleared", "日志已清空", map[string]string{"kind": kind}), err
}

func resourceLogKind(query url.Values) (string, error) {
	kind := "service"
	if query.Has("kind") {
		kind = query.Get("kind")
	}
	if kind != "service" && kind != "core" {
		return "", invalid("日志类型应为 service 或 core")
	}
	return kind, nil
}

func resourceDecode(r *http.Request, target any) error {
	if _, err := resourceQuery(r); err != nil {
		return err
	}
	return decodeRequest(r, target)
}

func resourceEmptyRequest(r *http.Request) error {
	if _, err := resourceQuery(r); err != nil {
		return err
	}
	return resourceEmptyBody(r)
}

func resourceEmptyBody(r *http.Request) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	return decodeRequest(r, &struct{}{})
}

func resourceQuery(r *http.Request, allowed ...string) (url.Values, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, invalid("查询参数无效")
	}
	for key, values := range query {
		if !slices.Contains(allowed, key) || len(values) != 1 {
			return nil, invalid("包含未知或重复的查询参数")
		}
	}
	return query, nil
}

func resourceGroupQuery(r *http.Request) (string, error) {
	query, err := resourceQuery(r, "group_id")
	if err != nil {
		return "", err
	}
	group := query.Get("group_id")
	return group, resourceGroupID(group, true)
}

func resourceNodeQuery(r *http.Request) (string, string, error) {
	query, err := resourceQuery(r, "group_id", "node_tag")
	if err != nil {
		return "", "", err
	}
	group, tag := query.Get("group_id"), query.Get("node_tag")
	return group, tag, resourceNodeTarget(group, tag)
}

func resourceNodeTarget(group, tag string) error {
	if err := resourceGroupID(group, false); err != nil {
		return err
	}
	if strings.TrimSpace(tag) == "" {
		return invalid("节点标签不能为空")
	}
	return nil
}

func resourceGroupID(group string, optional bool) error {
	if group == "" && optional {
		return nil
	}
	if group == "" || group == "staging" || strings.Contains(group, "..") {
		return invalid("分组 ID 无效")
	}
	for index, char := range group {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		if index > 0 && (char == '.' || char == '_' || char == '-') {
			continue
		}
		return invalid("分组 ID 无效")
	}
	return nil
}
