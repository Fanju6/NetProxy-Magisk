package catalog

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

var validGroupID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// isGroupDir 判断 Catalog 根目录下的条目是否为可用分组目录。
// staging 跳过与路径穿越校验只在此处实现，所有分组扫描都必须经过本函数，
// 避免多处副本各自演化导致校验漂移。
func isGroupDir(entry os.DirEntry) bool {
	name := entry.Name()
	return entry.IsDir() && name != "staging" && validGroupID.MatchString(name) &&
		!strings.Contains(name, "..")
}

type GroupSummary struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	RuntimeTag        string          `json:"runtime_tag"`
	Type              string          `json:"type"`
	Active            bool            `json:"active"`
	NodeCount         int             `json:"node_count"`
	Revision          int64           `json:"revision"`
	AutoUpdate        bool            `json:"auto_update"`
	UpdateInterval    int64           `json:"update_interval"`
	UpdateViaProxy    string          `json:"update_via_proxy"`
	Usage             json.RawMessage `json:"usage"`
	ProfileTitle      string          `json:"profile_title"`
	ProfileWebPageURL string          `json:"profile_web_page_url"`
	LastAttemptAt     string          `json:"last_attempt_at"`
	LastSuccessAt     string          `json:"last_success_at"`
	NextUpdateAt      string          `json:"next_update_at"`
	LastError         string          `json:"last_error"`
	UpdatedAt         string          `json:"updated_at"`
	Progress          json.RawMessage `json:"progress"`
}

type GroupSnapshot struct {
	Group GroupSummary           `json:"group"`
	Nodes []provider.NodeSummary `json:"nodes"`
}

type ScanOptions struct {
	Root        string
	ActiveGroup string
	ProgressDir string
	Type        string
	WithNodes   bool
	GroupID     string
}

type runtimeGroup struct {
	ID           string
	Name         string
	RuntimeTag   string
	ProviderPath string
	Nodes        []provider.NodeSummary
}

type RuntimeOptions struct {
	Root            string
	ModuleConfig    string
	ProvidersOutput string
	OutboundsOutput string
	StateOutput     string
	ActiveGroup     string
	SelectorMode    string
	SelectedNodeRef string
	AllowEmpty      bool
}

type RuntimeResult struct {
	ActiveGroup     string `json:"active_group_id"`
	ActiveGroupTag  string `json:"active_group_tag"`
	SelectorMode    string `json:"selector_mode"`
	SelectedNodeRef string `json:"selected_node_ref"`
	OutboundMode    string `json:"outbound_mode,omitempty"`
	GroupCount      int    `json:"group_count"`
	NodeCount       int    `json:"node_count"`
}

type ScheduleResult struct {
	Nearest int64    `json:"nearest"`
	Due     []string `json:"due"`
}

func Scan(ctx context.Context, options ScanOptions) ([]GroupSnapshot, error) {
	if err := recoverTransactions(options.Root); err != nil {
		return nil, err
	}
	groups, err := loadGroups(ctx, options.Root, true)
	if err != nil {
		return nil, err
	}
	assignRuntimeTags(groups)
	result := make([]GroupSnapshot, 0, len(groups))
	for _, group := range groups {
		if options.Type != "" && options.Type != "all" && group.Metadata.Type != options.Type {
			continue
		}
		if options.GroupID != "" && group.ID != options.GroupID {
			continue
		}
		summary := summaryFor(group, options.ActiveGroup, options.ProgressDir)
		nodes := []provider.NodeSummary{}
		if options.WithNodes {
			nodes = group.Nodes
		}
		result = append(result, GroupSnapshot{Group: summary, Nodes: nodes})
	}
	return result, nil
}

func Schedule(root string, now int64) (ScheduleResult, error) {
	if err := recoverTransactions(root); err != nil {
		return ScheduleResult{}, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ScheduleResult{}, err
	}
	result := ScheduleResult{Due: []string{}}
	for _, entry := range entries {
		if !isGroupDir(entry) {
			continue
		}
		metadata, err := loadMetadata(filepath.Join(root, entry.Name(), "meta.json"), entry.Name())
		if err != nil {
			return ScheduleResult{}, fmt.Errorf("读取分组 %s 元数据: %w", entry.Name(), err)
		}
		if metadata.Type != "subscription" || !metadata.AutoUpdate {
			continue
		}
		epoch := metadata.NextUpdateEpoch
		if epoch <= 0 {
			epoch = now
		}
		if result.Nearest == 0 || epoch < result.Nearest {
			result.Nearest = epoch
		}
		if epoch <= now {
			result.Due = append(result.Due, entry.Name())
		}
	}
	return result, nil
}

func RuntimeTag(root, groupID string) (string, error) {
	if !validGroupID.MatchString(groupID) || strings.Contains(groupID, "..") {
		return "", fmt.Errorf("非法分组 ID: %s", groupID)
	}
	if err := recoverTransactions(root); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	targetName := ""
	duplicateCount := 0
	names := make(map[string]string, len(entries))
	for _, entry := range entries {
		if !isGroupDir(entry) {
			continue
		}
		metadata, err := loadMetadata(filepath.Join(root, entry.Name(), "meta.json"), entry.Name())
		if err != nil {
			return "", fmt.Errorf("读取分组 %s 元数据: %w", entry.Name(), err)
		}
		names[entry.Name()] = metadata.Name
		if entry.Name() == groupID {
			targetName = metadata.Name
		}
	}
	if targetName == "" {
		return "", fmt.Errorf("分组不存在: %s", groupID)
	}
	for _, name := range names {
		if name == targetName {
			duplicateCount++
		}
	}
	if duplicateCount > 1 {
		return fmt.Sprintf("%s [%s]", targetName, groupID), nil
	}
	return targetName, nil
}

func GroupIDs(root, groupType string) ([]string, error) {
	if err := recoverTransactions(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !isGroupDir(entry) {
			continue
		}
		metadata, err := loadMetadata(filepath.Join(root, entry.Name(), "meta.json"), entry.Name())
		if err != nil {
			return nil, fmt.Errorf("读取分组 %s 元数据: %w", entry.Name(), err)
		}
		if groupType == "" || groupType == "all" || metadata.Type == groupType {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

// NewGroupID 为新订阅或本地文件分组生成不冲突的 ID。
func NewGroupID(root, kind, source string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("Catalog 根目录不能为空")
	}
	if err := recoverTransactions(root); err != nil {
		return "", err
	}
	switch kind {
	case "subscription":
		for attempt := 0; attempt < 16; attempt++ {
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err != nil {
				return "", err
			}
			raw[6] = (raw[6] & 0x0f) | 0x40
			raw[8] = (raw[8] & 0x3f) | 0x80
			candidate := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
				raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
			if _, err := os.Stat(filepath.Join(root, candidate)); err == nil {
				continue
			} else if os.IsNotExist(err) {
				return candidate, nil
			} else {
				return "", err
			}
		}
		return "", errors.New("无法生成不冲突的订阅分组 ID")
	case "file", "local":
		name := filepath.Base(strings.TrimSpace(source))
		if name == "." || name == string(filepath.Separator) || name == "" {
			name = fmt.Sprintf("%d", time.Now().Unix())
		}
		if extension := filepath.Ext(name); extension != "" {
			name = strings.TrimSuffix(name, extension)
		}
		var builder strings.Builder
		lastDash := false
		for _, char := range strings.ToLower(name) {
			valid := (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-'
			if valid {
				builder.WriteRune(char)
				lastDash = false
			} else if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
		slug := strings.Trim(builder.String(), ".-")
		if slug == "" {
			slug = fmt.Sprintf("%d", time.Now().Unix())
		}
		base := "local-" + slug
		candidate := base
		for suffix := 2; ; suffix++ {
			if _, err := os.Stat(filepath.Join(root, candidate)); err == nil {
				candidate = fmt.Sprintf("%s-%d", base, suffix)
				continue
			} else if os.IsNotExist(err) {
				return candidate, nil
			} else {
				return "", err
			}
		}
	default:
		return "", fmt.Errorf("未知 Catalog 分组 ID 类型: %s", kind)
	}
}

func BuildRuntime(ctx context.Context, options RuntimeOptions) (RuntimeResult, error) {
	if options.Root == "" || options.ProvidersOutput == "" || options.OutboundsOutput == "" {
		return RuntimeResult{}, errors.New("Catalog 根目录和运行时输出路径不能为空")
	}
	if err := recoverTransactions(options.Root); err != nil {
		return RuntimeResult{}, err
	}
	outboundMode := ""
	if options.ModuleConfig != "" {
		module, err := moduleconfig.LoadModule(options.ModuleConfig)
		if err != nil {
			return RuntimeResult{}, fmt.Errorf("读取 module.conf 失败: %w", err)
		}
		options.ActiveGroup = module.ActiveGroupID
		options.SelectorMode = module.SelectorMode
		options.SelectedNodeRef = module.SelectedNodeRef
		outboundMode = module.OutboundMode
	}
	groups, err := loadGroups(ctx, options.Root, false)
	if err != nil {
		return RuntimeResult{}, err
	}
	if len(groups) == 0 {
		if !options.AllowEmpty {
			return RuntimeResult{}, errors.New("Catalog 中没有可用节点，请先导入单节点、文件或订阅")
		}
		if err := writeEmptyRuntime(options); err != nil {
			return RuntimeResult{}, err
		}
		return RuntimeResult{SelectorMode: "urltest", OutboundMode: outboundMode}, nil
	}

	assignRuntimeTags(groups)
	active := options.ActiveGroup
	activeIndex := findGroup(groups, active)
	if activeIndex < 0 {
		activeIndex = 0
		active = groups[0].ID
	}
	selector := normalizeSelector(options.SelectorMode)
	selected := options.SelectedNodeRef
	if selector == "manual" && !containsNode(groups[activeIndex], selected) {
		selector = "urltest"
		selected = ""
	}
	if selector == "urltest" {
		selected = ""
	}

	if err := writeRuntimeProviders(options.ProvidersOutput, groups); err != nil {
		return RuntimeResult{}, err
	}
	if err := writeRuntimeOutbounds(options.OutboundsOutput, groups, groups[activeIndex].RuntimeTag, selector); err != nil {
		return RuntimeResult{}, err
	}

	nodeCount := 0
	for _, group := range groups {
		nodeCount += len(group.Nodes)
	}
	result := RuntimeResult{
		ActiveGroup:     active,
		ActiveGroupTag:  groups[activeIndex].RuntimeTag,
		SelectorMode:    selector,
		SelectedNodeRef: selected,
		OutboundMode:    outboundMode,
		GroupCount:      len(groups),
		NodeCount:       nodeCount,
	}
	if options.StateOutput != "" {
		if err := writeRuntimeState(options.StateOutput, result); err != nil {
			return RuntimeResult{}, err
		}
	}
	return result, nil
}

type loadedGroup struct {
	ID           string
	Metadata     Metadata
	ProviderPath string
	Nodes        []provider.NodeSummary
	RuntimeTag   string
}

func loadGroups(ctx context.Context, root string, includeEmpty bool) ([]*loadedGroup, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	groups := make([]*loadedGroup, 0, len(entries))
	for _, entry := range entries {
		if !isGroupDir(entry) {
			continue
		}
		groupDir := filepath.Join(root, entry.Name())
		metadata, err := loadMetadata(filepath.Join(groupDir, "meta.json"), entry.Name())
		if err != nil {
			return nil, fmt.Errorf("读取分组 %s 元数据: %w", entry.Name(), err)
		}
		providerPath := filepath.Join(groupDir, "provider.json")
		document, err := provider.LoadAllowEmpty(ctx, providerPath)
		if err != nil {
			return nil, fmt.Errorf("读取分组 %s Provider: %w", entry.Name(), err)
		}
		nodes := provider.Inspect(document)
		if !includeEmpty && len(nodes) == 0 {
			continue
		}
		groups = append(groups, &loadedGroup{ID: entry.Name(), Metadata: metadata, ProviderPath: providerPath, Nodes: nodes})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups, nil
}

func loadMetadata(path, fallbackID string) (Metadata, error) {
	return LoadMetadata(path, fallbackID)
}

func summaryFor(group *loadedGroup, activeGroup, progressDir string) GroupSummary {
	metadata := group.Metadata
	progress := json.RawMessage("null")
	if progressDir != "" {
		if content, err := os.ReadFile(filepath.Join(progressDir, group.ID+".progress.json")); err == nil {
			var state struct {
				Stage string `json:"stage"`
			}
			if json.Unmarshal(content, &state) == nil {
				switch state.Stage {
				case "download", "convert", "validate", "apply":
					progress = content
				}
			}
		}
	}
	return GroupSummary{
		ID: metadata.ID, Name: metadata.Name, RuntimeTag: group.RuntimeTag, Type: metadata.Type,
		Active: group.ID == activeGroup, NodeCount: len(group.Nodes), Revision: metadata.Revision,
		AutoUpdate: metadata.AutoUpdate, UpdateInterval: metadata.UpdateInterval,
		UpdateViaProxy: metadata.UpdateViaProxy, Usage: metadata.Usage,
		ProfileTitle: metadata.ProfileTitle, ProfileWebPageURL: metadata.ProfileWebPageURL,
		LastAttemptAt: metadata.LastAttemptAt, LastSuccessAt: metadata.LastSuccessAt,
		NextUpdateAt: metadata.NextUpdateAt, LastError: metadata.LastError,
		UpdatedAt: metadata.UpdatedAt, Progress: progress,
	}
}

func assignRuntimeTags(groups []*loadedGroup) {
	counts := make(map[string]int, len(groups))
	for _, group := range groups {
		counts[group.Metadata.Name]++
	}
	for _, group := range groups {
		group.RuntimeTag = group.Metadata.Name
		if counts[group.Metadata.Name] > 1 {
			group.RuntimeTag = fmt.Sprintf("%s [%s]", group.Metadata.Name, group.ID)
		}
	}
}

func findGroup(groups []*loadedGroup, id string) int {
	for index, group := range groups {
		if group.ID == id {
			return index
		}
	}
	return -1
}

func normalizeSelector(mode string) string {
	switch mode {
	case "manual", "selector":
		return "manual"
	default:
		return "urltest"
	}
}

func containsNode(group *loadedGroup, reference string) bool {
	groupID, tag, found := strings.Cut(reference, "/")
	if !found || groupID != group.ID || tag == "" {
		return false
	}
	for _, node := range group.Nodes {
		if node.Tag == tag {
			return true
		}
	}
	return false
}

func writeRuntimeProviders(path string, groups []*loadedGroup) error {
	items := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		items = append(items, map[string]any{
			"type": "local", "tag": group.RuntimeTag, "path": group.ProviderPath,
			"health_check": map[string]any{
				"enabled": true, "url": "https://www.gstatic.com/generate_204",
				"interval": "10m", "timeout": "5s",
			},
		})
	}
	return writeJSONAtomic(path, map[string]any{"providers": items})
}

func writeRuntimeOutbounds(path string, groups []*loadedGroup, activeTag, selector string) error {
	outbounds := []map[string]any{
		{"type": "direct", "tag": "direct"},
		{"type": "block", "tag": "block"},
	}
	options := make([]string, 0, len(groups)*2)
	for _, group := range groups {
		autoTag := "Auto/" + group.RuntimeTag
		selectTag := "Select/" + group.RuntimeTag
		outbounds = append(outbounds,
			map[string]any{
				"type": "urltest", "tag": autoTag, "providers": []string{group.RuntimeTag},
				"url": "https://www.gstatic.com/generate_204", "interval": "3m", "tolerance": 50,
				"interrupt_exist_connections": true,
			},
			map[string]any{
				"type": "selector", "tag": selectTag, "providers": []string{group.RuntimeTag},
				"interrupt_exist_connections": true,
			},
		)
		options = append(options, autoTag, selectTag)
	}
	defaultTag := "Auto/" + activeTag
	if selector == "manual" {
		defaultTag = "Select/" + activeTag
	}
	outbounds = append(outbounds, map[string]any{
		"type": "selector", "tag": "Proxy", "outbounds": options, "default": defaultTag,
		"interrupt_exist_connections": true,
	})
	return writeJSONAtomic(path, map[string]any{"outbounds": outbounds})
}

func writeEmptyRuntime(options RuntimeOptions) error {
	if err := writeJSONAtomic(options.ProvidersOutput, map[string]any{"providers": []any{}}); err != nil {
		return err
	}
	if err := writeJSONAtomic(options.OutboundsOutput, map[string]any{"outbounds": []map[string]any{
		{"type": "direct", "tag": "direct"},
		{"type": "block", "tag": "block"},
		{"type": "direct", "tag": "Proxy"},
	}}); err != nil {
		return err
	}
	if options.StateOutput != "" {
		return writeRuntimeState(options.StateOutput, RuntimeResult{SelectorMode: "urltest"})
	}
	return nil
}

func writeRuntimeState(path string, result RuntimeResult) error {
	content := fmt.Sprintf(
		"active_group_id\t%s\nactive_group_tag\t%s\nselector_mode\t%s\nselected_node_ref\t%s\noutbound_mode\t%s\ngroup_count\t%d\nnode_count\t%d\n",
		result.ActiveGroup, result.ActiveGroupTag, result.SelectorMode,
		result.SelectedNodeRef, result.OutboundMode, result.GroupCount, result.NodeCount,
	)
	return provider.WriteAtomic(path, []byte(content), 0o600)
}

func writeJSONAtomic(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return provider.WriteAtomic(path, content, 0o600)
}
