package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

// MutationResult 是 Catalog 节点变更提交后的最小结果。
type MutationResult struct {
	GroupID          string `json:"group_id"`
	NodeCount        int    `json:"node_count"`
	Revision         int64  `json:"revision"`
	StructureChanged bool   `json:"structure_changed"`
}

// MutationOptions 描述一次本地节点变更。
type MutationOptions struct {
	GroupDir      string
	GroupID       string
	Name          string
	Type          string
	Input         string
	Tag           string
	AllowInsecure bool
	Now           time.Time
}

// ImportOptions 描述一次新的本地分组导入。
type ImportOptions struct {
	Root          string
	GroupID       string
	Name          string
	Input         string
	AllowInsecure bool
	Now           time.Time
}

// GroupOptions 描述一个 Catalog 分组的初始化配置。
type GroupOptions struct {
	Root           string
	GroupID        string
	Name           string
	Type           string
	URL            string
	UserAgent      string
	HWID           string
	CustomHeaders  map[string]string
	AutoUpdate     bool
	UpdateInterval int64
	IntervalSource string
	UpdateViaProxy string
	Include        string
	Exclude        string
	AllowInsecure  bool
	Timeout        int64
	Now            time.Time
}

// InitializeGroup 创建一个新的空 Catalog 分组。
func InitializeGroup(ctx context.Context, options GroupOptions) error {
	if err := validateGroupOptions(options); err != nil {
		return err
	}
	release := lockGroup(options.GroupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(options.Root); err != nil {
		return err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	groupDir := filepath.Join(options.Root, options.GroupID)
	if fileExists(filepath.Join(groupDir, "meta.json")) || fileExists(filepath.Join(groupDir, "provider.json")) {
		return fmt.Errorf("Catalog 分组已存在: %s", options.GroupID)
	}
	metadata, err := buildGroupMetadata(options)
	if err != nil {
		return err
	}
	return commitPair(ctx, groupDir, provider.Document{}, metadata)
}

// EnsureGroup 确保已有 Catalog 根目录包含指定的本地分组。
func EnsureGroup(ctx context.Context, options GroupOptions) error {
	if err := validateGroupOptions(options); err != nil {
		return err
	}
	release := lockGroup(options.GroupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(options.Root); err != nil {
		return err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	groupDir := filepath.Join(options.Root, options.GroupID)
	providerPath := filepath.Join(groupDir, "provider.json")
	metadataPath := filepath.Join(groupDir, "meta.json")
	providerExists := fileExists(providerPath)
	metadataExists := fileExists(metadataPath)
	if providerExists && metadataExists {
		return nil
	}
	document := provider.Document{}
	var err error
	if providerExists {
		document, err = provider.LoadAllowEmpty(ctx, providerPath)
		if err != nil {
			return err
		}
	}
	metadata, err := buildGroupMetadata(options)
	if metadataExists {
		metadata, err = LoadMetadata(metadataPath, options.GroupID)
	}
	if err != nil {
		return err
	}
	return commitPair(ctx, groupDir, document, metadata)
}

// SetGroupName 更新 Catalog 分组的显示名称并保留其余元数据。
func SetGroupName(ctx context.Context, root, groupID, name string, now time.Time) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("Catalog 根目录不能为空")
	}
	if !validGroupID.MatchString(groupID) {
		return fmt.Errorf("非法分组 ID: %s", groupID)
	}
	release := lockGroup(groupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(root); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("分组名称不能为空")
	}
	if now.IsZero() {
		now = time.Now()
	}
	metadataPath := filepath.Join(root, groupID, "meta.json")
	metadata, err := LoadMetadata(metadataPath, groupID)
	if err != nil {
		return err
	}
	metadata.Name = name
	metadata.UpdatedAt = FormatEpochUTC(now.Unix())
	return SaveMetadataAtomic(metadataPath, metadata)
}

func validateGroupOptions(options GroupOptions) error {
	if strings.TrimSpace(options.Root) == "" {
		return errors.New("Catalog 根目录不能为空")
	}
	if !validGroupID.MatchString(options.GroupID) {
		return fmt.Errorf("非法分组 ID: %s", options.GroupID)
	}
	if options.Type == "" || options.Type == "all" {
		return errors.New("Catalog 分组类型不能为空")
	}
	switch options.Type {
	case "local", "file", "subscription":
	default:
		return fmt.Errorf("未知 Catalog 分组类型: %s", options.Type)
	}
	return nil
}

func buildGroupMetadata(options GroupOptions) (Metadata, error) {
	metadata := NewMetadata(options.GroupID, options.Name, options.Type, options.URL, options.Now)
	if options.Type != "subscription" {
		// 本地分组只保存节点，不应因为 group-ensure 的默认参数进入订阅调度。
		options.AutoUpdate = false
		options.UpdateViaProxy = "never"
	}
	metadata.UserAgent = options.UserAgent
	metadata.HWID = options.HWID
	if options.CustomHeaders == nil {
		metadata.CustomHeaders = map[string]string{}
	} else {
		metadata.CustomHeaders = options.CustomHeaders
	}
	metadata.AutoUpdate = options.AutoUpdate
	if options.UpdateInterval > 0 {
		metadata.UpdateInterval = options.UpdateInterval
	}
	if options.IntervalSource != "" {
		metadata.IntervalSource = options.IntervalSource
	}
	metadata.UpdateViaProxy = options.UpdateViaProxy
	metadata.Include = options.Include
	metadata.Exclude = options.Exclude
	metadata.AllowInsecure = options.AllowInsecure
	if options.Timeout > 0 {
		metadata.Timeout = options.Timeout
	}
	if metadata.AutoUpdate {
		ScheduleAt(&metadata, options.Now)
	}
	return metadata, nil
}

// AppendNode 将链接或文件节点加入现有 Catalog 分组。
func AppendNode(ctx context.Context, options MutationOptions) (MutationResult, error) {
	if err := validateMutationOptions(options, false); err != nil {
		return MutationResult{}, err
	}
	release := lockGroup(options.GroupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(filepath.Dir(options.GroupDir)); err != nil {
		return MutationResult{}, err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	document, err := loadProvider(ctx, options.GroupDir)
	if err != nil {
		return MutationResult{}, err
	}
	oldNodeCount := len(document.Outbounds) + len(document.Endpoints)
	source, err := convert.Input(ctx, options.Input, options.AllowInsecure)
	if err != nil {
		return MutationResult{}, err
	}
	if options.Tag != "" {
		selected, found := provider.Select(source.Document, options.Tag)
		if !found {
			return MutationResult{}, fmt.Errorf("输入内容中未找到节点标签 %q", options.Tag)
		}
		source.Document = selected
	}
	provider.Append(&document, source.Document)
	metadata, err := loadMutationMetadata(options)
	if err != nil {
		return MutationResult{}, err
	}
	result := mutationResult(options, metadata, document, oldNodeCount == 0)
	return commitMutation(ctx, options.GroupDir, document, metadata, result, options.Now)
}

// RemoveNode 从 Catalog 分组中删除指定标签。
func RemoveNode(ctx context.Context, options MutationOptions) (MutationResult, error) {
	if err := validateMutationOptions(options, true); err != nil {
		return MutationResult{}, err
	}
	release := lockGroup(options.GroupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(filepath.Dir(options.GroupDir)); err != nil {
		return MutationResult{}, err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	document, err := loadProvider(ctx, options.GroupDir)
	if err != nil {
		return MutationResult{}, err
	}
	if !provider.Remove(&document, options.Tag) {
		return MutationResult{}, fmt.Errorf("未找到节点标签 %q", options.Tag)
	}
	metadata, err := loadMutationMetadata(options)
	if err != nil {
		return MutationResult{}, err
	}
	result := mutationResult(options, metadata, document, len(document.Outbounds)+len(document.Endpoints) == 0)
	return commitMutation(ctx, options.GroupDir, document, metadata, result, options.Now)
}

// EditNode 原子替换 Catalog 分组中的一个节点。
func EditNode(ctx context.Context, options MutationOptions) (MutationResult, error) {
	if err := validateMutationOptions(options, true); err != nil {
		return MutationResult{}, err
	}
	release := lockGroup(options.GroupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(filepath.Dir(options.GroupDir)); err != nil {
		return MutationResult{}, err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	document, err := loadProvider(ctx, options.GroupDir)
	if err != nil {
		return MutationResult{}, err
	}
	if !provider.Remove(&document, options.Tag) {
		return MutationResult{}, fmt.Errorf("未找到节点标签 %q", options.Tag)
	}
	source, err := convert.Input(ctx, options.Input, options.AllowInsecure)
	if err != nil {
		return MutationResult{}, err
	}
	provider.Append(&document, source.Document)
	metadata, err := loadMutationMetadata(options)
	if err != nil {
		return MutationResult{}, err
	}
	result := mutationResult(options, metadata, document, false)
	return commitMutation(ctx, options.GroupDir, document, metadata, result, options.Now)
}

// ImportGroup 创建本地分组并原子写入 Provider 与元数据。
func ImportGroup(ctx context.Context, options ImportOptions) (MutationResult, error) {
	if !validGroupID.MatchString(options.GroupID) {
		return MutationResult{}, fmt.Errorf("非法分组 ID: %s", options.GroupID)
	}
	if strings.TrimSpace(options.Root) == "" || strings.TrimSpace(options.Input) == "" {
		return MutationResult{}, errors.New("Catalog 根目录和输入内容不能为空")
	}
	release := lockGroup(options.GroupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(options.Root); err != nil {
		return MutationResult{}, err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	document, err := convertInput(ctx, options.Input, options.AllowInsecure)
	if err != nil {
		return MutationResult{}, err
	}
	if len(document.Outbounds)+len(document.Endpoints) == 0 {
		return MutationResult{}, errors.New("输入内容中没有可用节点")
	}
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = options.GroupID
	}
	metadata := NewMetadata(options.GroupID, name, "file", "", options.Now)
	result := mutationResultFor(options.GroupID, metadata, document, true)
	return commitMutation(ctx, filepath.Join(options.Root, options.GroupID), document, metadata, result, options.Now)
}

func convertInput(ctx context.Context, input string, allowInsecure bool) (provider.Document, error) {
	parsed, err := convert.Input(ctx, input, allowInsecure)
	if err != nil {
		return provider.Document{}, err
	}
	return parsed.Document, nil
}

func validateMutationOptions(options MutationOptions, requireTag bool) error {
	if strings.TrimSpace(options.GroupDir) == "" {
		return errors.New("Catalog 分组目录不能为空")
	}
	if strings.TrimSpace(options.GroupID) == "" {
		options.GroupID = filepath.Base(options.GroupDir)
	}
	if !validGroupID.MatchString(options.GroupID) {
		return fmt.Errorf("非法分组 ID: %s", options.GroupID)
	}
	if requireTag && strings.TrimSpace(options.Tag) == "" {
		return errors.New("节点标签不能为空")
	}
	if !requireTag && strings.TrimSpace(options.Input) == "" {
		return errors.New("输入内容不能为空")
	}
	return nil
}

func loadProvider(ctx context.Context, groupDir string) (provider.Document, error) {
	path := filepath.Join(groupDir, "provider.json")
	document, err := provider.LoadAllowEmpty(ctx, path)
	if os.IsNotExist(err) {
		return provider.Document{}, nil
	}
	return document, err
}

func loadMutationMetadata(options MutationOptions) (Metadata, error) {
	path := filepath.Join(options.GroupDir, "meta.json")
	metadata, err := LoadMetadata(path, options.GroupID)
	if os.IsNotExist(err) {
		metadata = NewMetadata(options.GroupID, options.Name, options.Type, "", options.Now)
	}
	if err != nil && !os.IsNotExist(err) {
		return Metadata{}, err
	}
	if metadata.ID == "" {
		metadata.ID = options.GroupID
	}
	if metadata.Name == "" {
		metadata.Name = options.Name
	}
	if metadata.Type == "" {
		metadata.Type = options.Type
	}
	return metadata, nil
}

func mutationResult(options MutationOptions, metadata Metadata, document provider.Document, structureChanged bool) MutationResult {
	return mutationResultFor(options.GroupID, metadata, document, structureChanged)
}

func mutationResultFor(groupID string, metadata Metadata, document provider.Document, structureChanged bool) MutationResult {
	metadata.NodeCount = len(document.Outbounds) + len(document.Endpoints)
	return MutationResult{
		GroupID:          groupID,
		NodeCount:        metadata.NodeCount,
		Revision:         metadata.Revision + 1,
		StructureChanged: structureChanged,
	}
}

func commitMutation(ctx context.Context, groupDir string, document provider.Document, metadata Metadata, result MutationResult, now time.Time) (MutationResult, error) {
	if result.Revision <= 0 {
		result.Revision = 1
	}
	if now.IsZero() {
		now = time.Now()
	}
	metadata.NodeCount = result.NodeCount
	metadata.Revision = result.Revision
	metadata.UpdatedAt = FormatEpochUTC(now.Unix())
	if err := commitPair(ctx, groupDir, document, metadata); err != nil {
		return MutationResult{}, err
	}
	return result, nil
}

func commitPair(ctx context.Context, groupDir string, document provider.Document, metadata Metadata) error {
	parent := filepath.Dir(groupDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	providerContent, err := provider.MarshalAllowEmpty(ctx, document)
	if err != nil {
		return err
	}
	metadataContent, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	metadataContent = append(metadataContent, '\n')
	return CommitPair(parent, groupDir, providerContent, metadataContent)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
