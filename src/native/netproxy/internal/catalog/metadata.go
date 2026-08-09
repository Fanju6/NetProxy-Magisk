package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const (
	defaultInterval = 24 * time.Hour
	minimumInterval = 15 * time.Minute
)

// Metadata 是 Catalog 分组的持久元数据。
// 订阅更新必须由 Go 统一读写，避免 Shell 通过 awk/sed 解析 JSON。
type Metadata struct {
	Schema             int                   `json:"schema"`
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	Type               string                `json:"type"`
	URL                string                `json:"url"`
	UserAgent          string                `json:"user_agent"`
	HWID               string                `json:"hwid"`
	CustomHeaders      map[string]string     `json:"custom_headers"`
	AutoUpdate         bool                  `json:"auto_update"`
	UpdateInterval     int64                 `json:"update_interval"`
	IntervalSource     string                `json:"interval_source"`
	UpdateViaProxy     string                `json:"update_via_proxy"`
	Include            string                `json:"include"`
	Exclude            string                `json:"exclude"`
	AllowInsecure      bool                  `json:"allow_insecure"`
	Timeout            int64                 `json:"timeout"`
	Usage              json.RawMessage       `json:"usage"`
	NodeCount          int                   `json:"node_count"`
	Revision           int64                 `json:"revision"`
	ETag               string                `json:"etag"`
	LastModified       string                `json:"last_modified"`
	ProfileTitle       string                `json:"profile_title"`
	ProfileWebPageURL  string                `json:"profile_web_page_url"`
	ContentDisposition string                `json:"content_disposition"`
	FileName           string                `json:"file_name"`
	LastStatusCode     int                   `json:"last_status_code"`
	LastDiagnostics    []provider.Diagnostic `json:"last_diagnostics"`
	LastAttemptAt      string                `json:"last_attempt_at"`
	LastSuccessAt      string                `json:"last_success_at"`
	NextUpdateAt       string                `json:"next_update_at"`
	NextUpdateEpoch    int64                 `json:"next_update_epoch"`
	LastError          string                `json:"last_error"`
	CreatedAt          string                `json:"created_at"`
	UpdatedAt          string                `json:"updated_at"`
}

// LoadMetadata 读取并补齐旧字段缺省值。
func LoadMetadata(path, fallbackID string) (Metadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return Metadata{}, err
	}
	if metadata.ID == "" {
		metadata.ID = fallbackID
	}
	if metadata.Name == "" {
		metadata.Name = metadata.ID
	}
	return NormalizeMetadata(metadata), nil
}

// SaveMetadataAtomic 以 0600 权限原子保存 Catalog 元数据。
func SaveMetadataAtomic(path string, metadata Metadata) error {
	metadata = NormalizeMetadata(metadata)
	content, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return provider.WriteAtomic(path, content, 0o600)
}

// NewMetadata 创建一份可直接写入 Catalog 的默认元数据。
func NewMetadata(id, name, metadataType, rawURL string, now time.Time) Metadata {
	if now.IsZero() {
		now = time.Now()
	}
	nowText := FormatEpochUTC(now.Unix())
	return Metadata{
		Schema:          1,
		ID:              id,
		Name:            name,
		Type:            metadataType,
		URL:             rawURL,
		CustomHeaders:   map[string]string{},
		UpdateInterval:  int64(defaultInterval / time.Second),
		IntervalSource:  "default",
		UpdateViaProxy:  "auto",
		Timeout:         60,
		Usage:           json.RawMessage("null"),
		LastDiagnostics: []provider.Diagnostic{},
		CreatedAt:       nowText,
		UpdatedAt:       nowText,
	}
}

// DurationToSeconds 将 15m、4h、1d 或纯秒数转换为秒。
func DurationToSeconds(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("更新周期不能为空")
	}
	multiplier := int64(1)
	number := value
	switch value[len(value)-1] {
	case 'm':
		multiplier = 60
		number = value[:len(value)-1]
	case 'h':
		multiplier = 3600
		number = value[:len(value)-1]
	case 'd':
		multiplier = 86400
		number = value[:len(value)-1]
	}
	parsed, err := strconv.ParseInt(number, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("更新周期无效")
	}
	seconds := parsed * multiplier
	if seconds < int64(minimumInterval/time.Second) {
		return 0, fmt.Errorf("更新周期不能小于 %s", minimumInterval)
	}
	return seconds, nil
}

// FormatEpochUTC 将 Unix 时间转换为稳定的 UTC 时间文本。
func FormatEpochUTC(epoch int64) string {
	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

// ScheduleAt 根据元数据的更新周期计算下一次更新。
func ScheduleAt(metadata *Metadata, now time.Time) {
	if now.IsZero() {
		now = time.Now()
	}
	if metadata.UpdateInterval < int64(minimumInterval/time.Second) {
		metadata.UpdateInterval = int64(minimumInterval / time.Second)
	}
	metadata.NextUpdateEpoch = now.Unix() + metadata.UpdateInterval
	metadata.NextUpdateAt = FormatEpochUTC(metadata.NextUpdateEpoch)
}

func NormalizeMetadata(metadata Metadata) Metadata {
	if metadata.Schema == 0 {
		metadata.Schema = 1
	}
	if metadata.Type == "" {
		metadata.Type = "local"
	}
	if metadata.UpdateInterval <= 0 {
		metadata.UpdateInterval = int64(defaultInterval / time.Second)
	}
	if metadata.IntervalSource == "" {
		metadata.IntervalSource = "default"
	}
	if metadata.UpdateViaProxy == "" {
		metadata.UpdateViaProxy = "auto"
	}
	if metadata.Timeout <= 0 {
		metadata.Timeout = 60
	}
	if metadata.CustomHeaders == nil {
		metadata.CustomHeaders = map[string]string{}
	}
	if len(metadata.Usage) == 0 {
		metadata.Usage = json.RawMessage("null")
	}
	if metadata.LastDiagnostics == nil {
		metadata.LastDiagnostics = []provider.Diagnostic{}
	}
	return metadata
}
