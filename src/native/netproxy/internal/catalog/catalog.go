package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"uuid"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	SJSON "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badoption"
)

var validGroupID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// isValidGroupID 统一拒绝事务目录和可能改变路径语义的分组 ID。
func isValidGroupID(value string) bool {
	return validGroupID.MatchString(value) && value != stagingDirName && !strings.Contains(value, "..")
}

// isGroupDir 判断 Catalog 根目录下的条目是否为可用分组目录。
// 所有分组扫描都必须经过本函数，避免多处副本各自演化导致校验漂移。
func isGroupDir(entry os.DirEntry) bool {
	name := entry.Name()
	return entry.IsDir() && isValidGroupID(name)
}

type GroupSummary struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	RuntimeTag        string         `json:"runtime_tag"`
	Type              string         `json:"type"`
	NodeCount         int            `json:"node_count"`
	Revision          int64          `json:"revision"`
	AutoUpdate        bool           `json:"auto_update"`
	UpdateInterval    int64          `json:"update_interval"`
	UpdateViaProxy    string         `json:"update_via_proxy"`
	Usage             jsontext.Value `json:"usage"`
	ProfileTitle      string         `json:"profile_title"`
	ProfileWebPageURL string         `json:"profile_web_page_url"`
	LastAttemptAt     string         `json:"last_attempt_at"`
	LastSuccessAt     string         `json:"last_success_at"`
	NextUpdateAt      string         `json:"next_update_at"`
	LastError         string         `json:"last_error"`
	UpdatedAt         string         `json:"updated_at"`
	Progress          jsontext.Value `json:"progress"`
}

type GroupSnapshot struct {
	Group GroupSummary           `json:"group"`
	Nodes []provider.NodeSummary `json:"nodes"`
}

type ScanOptions struct {
	Root        string
	ProgressDir string
	Type        string
	WithNodes   bool
	GroupID     string
}

type RuntimeResult struct {
	GroupCount int              `json:"group_count"`
	NodeCount  int              `json:"node_count"`
	Providers  []jsontext.Value `json:"-"`
}

type ScheduleResult struct {
	Nearest     int64            `json:"nearest"`
	Due         []string         `json:"due"`
	NextByGroup map[string]int64 `json:"-"`
}

func Scan(ctx context.Context, options ScanOptions) ([]GroupSnapshot, error) {
	release, err := acquireCatalogRootAndRecover(ctx, options.Root)
	if err != nil {
		return nil, err
	}
	defer release()
	groups, err := loadGroups(ctx, options.Root, true)
	if err != nil {
		return nil, err
	}
	result := make([]GroupSnapshot, 0, len(groups))
	for _, group := range groups {
		if options.Type != "" && options.Type != "all" && group.Metadata.Type != options.Type {
			continue
		}
		if options.GroupID != "" && group.ID != options.GroupID {
			continue
		}
		if options.WithNodes {
			nodes, err := provider.InspectFile(ctx, group.ProviderPath)
			if err != nil {
				return nil, fmt.Errorf("读取分组 %s Provider: %w", group.ID, err)
			}
			group.Nodes = nodes
		}
		summary := summaryFor(group, options.ProgressDir)
		nodes := []provider.NodeSummary{}
		if options.WithNodes {
			nodes = group.Nodes
		}
		result = append(result, GroupSnapshot{Group: summary, Nodes: nodes})
	}
	return result, nil
}

func Schedule(ctx context.Context, root string, now int64) (ScheduleResult, error) {
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return ScheduleResult{}, err
	}
	defer release()
	entries, err := os.ReadDir(root)
	if err != nil {
		return ScheduleResult{}, err
	}
	result := ScheduleResult{Due: []string{}, NextByGroup: make(map[string]int64)}
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
		result.NextByGroup[entry.Name()] = epoch
		if result.Nearest == 0 || epoch < result.Nearest {
			result.Nearest = epoch
		}
		if epoch <= now {
			result.Due = append(result.Due, entry.Name())
		}
	}
	return result, nil
}

func RuntimeTag(ctx context.Context, root, groupID string) (string, error) {
	if !isValidGroupID(groupID) {
		return "", fmt.Errorf("非法分组 ID: %s", groupID)
	}
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := loadMetadata(filepath.Join(root, groupID, "meta.json"), groupID); err != nil {
		return "", fmt.Errorf("读取分组 %s 元数据: %w", groupID, err)
	}
	return ProviderTag(groupID), nil
}

func ProviderTag(groupID string) string { return "np/" + groupID }

func GroupIDs(ctx context.Context, root, groupType string) ([]string, error) {
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return nil, err
	}
	defer release()
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

// NewSubscriptionGroupID 为新订阅生成不冲突的稳定 ID。
func NewSubscriptionGroupID(ctx context.Context, root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("Catalog 根目录不能为空")
	}
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return "", err
	}
	defer release()
	for range 16 {
		candidate := uuid.NewV4().String()
		if _, err := os.Stat(filepath.Join(root, candidate)); err == nil {
			continue
		} else if os.IsNotExist(err) {
			return candidate, nil
		} else {
			return "", err
		}
	}
	return "", errors.New("无法生成不冲突的订阅分组 ID")
}

func BuildRuntime(ctx context.Context, root string) (RuntimeResult, error) {
	if root == "" {
		return RuntimeResult{}, errors.New("Catalog 根目录不能为空")
	}
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return RuntimeResult{}, err
	}
	defer release()
	groups, err := loadGroups(ctx, root, false)
	if err != nil {
		return RuntimeResult{}, err
	}
	result := RuntimeResult{GroupCount: len(groups)}
	for _, group := range groups {
		result.NodeCount += group.Metadata.NodeCount
	}
	content, err := SJSON.MarshalContext(provider.RuntimeContext(ctx), runtimeProviders(groups))
	if err != nil {
		return RuntimeResult{}, err
	}
	if err := json.Unmarshal(content, &result.Providers); err != nil {
		return RuntimeResult{}, err
	}
	return result, nil
}

// HasOtherProviderLocked reports whether a nonempty resource besides id exists.
// Callers hold the Catalog root lock when validating a candidate removal.
func HasOtherProviderLocked(root, id string) (bool, error) {
	groups, err := loadGroups(context.Background(), root, false)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if group.ID != id {
			return true, nil
		}
	}
	return false, nil
}

type loadedGroup struct {
	ID           string
	Metadata     Metadata
	ProviderPath string
	Nodes        []provider.NodeSummary
	hasNodes     bool
}

func loadGroups(ctx context.Context, root string, includeEmpty bool) ([]*loadedGroup, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	groups := make([]*loadedGroup, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isGroupDir(entry) {
			continue
		}
		groupDir := filepath.Join(root, entry.Name())
		metadata, err := loadMetadata(filepath.Join(groupDir, "meta.json"), entry.Name())
		if err != nil {
			return nil, fmt.Errorf("读取分组 %s 元数据: %w", entry.Name(), err)
		}
		providerPath := filepath.Join(groupDir, "provider.json")
		if !includeEmpty && metadata.NodeCount <= 0 {
			continue
		}
		groups = append(groups, &loadedGroup{
			ID: entry.Name(), Metadata: metadata, ProviderPath: providerPath,
			hasNodes: metadata.NodeCount > 0,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups, nil
}

func loadMetadata(path, fallbackID string) (Metadata, error) {
	return LoadMetadataLocked(path, fallbackID)
}

func summaryFor(group *loadedGroup, progressDir string) GroupSummary {
	nodeCount := group.Metadata.NodeCount
	if group.Nodes != nil {
		nodeCount = len(group.Nodes)
	}
	return summaryForMetadata(group.Metadata, ProviderTag(group.ID), progressDir, nodeCount)
}

func runtimeProviders(groups []*loadedGroup) []option.Provider {
	items := make([]option.Provider, 0, len(groups))
	for _, group := range groups {
		items = append(items, option.Provider{
			Type: C.ProviderTypeLocal,
			Tag:  ProviderTag(group.ID),
			Options: &option.ProviderLocalOptions{
				Path: group.ProviderPath,
				HealthCheck: option.ProviderHealthCheckOptions{
					Enabled:  true,
					URL:      "https://www.gstatic.com/generate_204",
					Interval: badoption.Duration(10 * time.Minute),
					Timeout:  badoption.Duration(5 * time.Second),
				},
			},
		})
	}
	return items
}
