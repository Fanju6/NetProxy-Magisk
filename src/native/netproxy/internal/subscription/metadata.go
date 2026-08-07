package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

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

var metadataStateFields = []string{
	"schema", "id", "name", "type", "url", "user_agent", "hwid", "custom_headers",
	"auto_update", "update_interval", "interval_source", "update_via_proxy", "include", "exclude",
	"allow_insecure", "timeout", "usage", "node_count", "revision", "etag", "last_modified",
	"profile_title", "profile_web_page_url", "content_disposition", "file_name", "last_status_code",
	"last_diagnostics", "last_attempt_at", "last_success_at", "next_update_at", "next_update_epoch",
	"last_error", "created_at", "updated_at",
}

// EncodeMetadataState 将元数据编码为 Shell 临时使用的单行字段记录。
func EncodeMetadataState(metadata Metadata) (string, error) {
	metadata = normalizeMetadata(metadata)
	customHeaders, err := json.Marshal(metadata.CustomHeaders)
	if err != nil {
		return "", err
	}
	usage := metadata.Usage
	if len(usage) == 0 {
		usage = json.RawMessage("null")
	}
	if !json.Valid(usage) {
		return "", errors.New("usage 不是有效 JSON")
	}
	var compactUsage bytes.Buffer
	if err := json.Compact(&compactUsage, usage); err != nil {
		return "", err
	}
	diagnostics, err := json.Marshal(metadata.LastDiagnostics)
	if err != nil {
		return "", err
	}
	values := []string{
		strconv.Itoa(metadata.Schema), metadata.ID, metadata.Name, metadata.Type, metadata.URL,
		metadata.UserAgent, metadata.HWID, string(customHeaders), strconv.FormatBool(metadata.AutoUpdate),
		strconv.FormatInt(metadata.UpdateInterval, 10), metadata.IntervalSource, metadata.UpdateViaProxy,
		metadata.Include, metadata.Exclude, strconv.FormatBool(metadata.AllowInsecure),
		strconv.FormatInt(metadata.Timeout, 10), compactUsage.String(), strconv.Itoa(metadata.NodeCount),
		strconv.FormatInt(metadata.Revision, 10), metadata.ETag, metadata.LastModified, metadata.ProfileTitle,
		metadata.ProfileWebPageURL, metadata.ContentDisposition, metadata.FileName,
		strconv.Itoa(metadata.LastStatusCode), string(diagnostics), metadata.LastAttemptAt,
		metadata.LastSuccessAt, metadata.NextUpdateAt, strconv.FormatInt(metadata.NextUpdateEpoch, 10),
		metadata.LastError, metadata.CreatedAt, metadata.UpdatedAt,
	}
	for _, value := range values {
		if strings.ContainsAny(value, "\r\n\t") {
			return "", errors.New("元数据字段不能包含换行或制表符")
		}
	}
	return strings.Join(values, "\x1f") + "\n", nil
}

// DecodeMetadataState 解码 Shell 生成的临时元数据记录。
func DecodeMetadataState(content string) (Metadata, error) {
	line := strings.TrimSuffix(content, "\n")
	line = strings.TrimSuffix(line, "\r")
	values := strings.Split(line, "\x1f")
	if len(values) != len(metadataStateFields) {
		return Metadata{}, fmt.Errorf("元数据字段数量错误: %d", len(values))
	}
	metadata := Metadata{}
	parseInt := func(index int, target *int64) error {
		value, err := strconv.ParseInt(values[index], 10, 64)
		if err != nil {
			return fmt.Errorf("字段 %s 无效", metadataStateFields[index])
		}
		*target = value
		return nil
	}
	parseBool := func(index int, target *bool) error {
		value, err := strconv.ParseBool(values[index])
		if err != nil {
			return fmt.Errorf("字段 %s 无效", metadataStateFields[index])
		}
		*target = value
		return nil
	}
	var schema, updateInterval, timeout, nodeCount, revision, statusCode, nextEpoch int64
	if err := parseInt(0, &schema); err != nil {
		return Metadata{}, err
	}
	if err := parseInt(9, &updateInterval); err != nil {
		return Metadata{}, err
	}
	if err := parseInt(15, &timeout); err != nil {
		return Metadata{}, err
	}
	if err := parseInt(17, &nodeCount); err != nil {
		return Metadata{}, err
	}
	if err := parseInt(18, &revision); err != nil {
		return Metadata{}, err
	}
	if err := parseInt(25, &statusCode); err != nil {
		return Metadata{}, err
	}
	if err := parseInt(30, &nextEpoch); err != nil {
		return Metadata{}, err
	}
	var autoUpdate, allowInsecure bool
	if err := parseBool(8, &autoUpdate); err != nil {
		return Metadata{}, err
	}
	if err := parseBool(14, &allowInsecure); err != nil {
		return Metadata{}, err
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(values[7]), &headers); err != nil {
		return Metadata{}, fmt.Errorf("custom_headers 无效: %w", err)
	}
	usage := json.RawMessage(values[16])
	if !json.Valid(usage) {
		return Metadata{}, errors.New("usage 不是有效 JSON")
	}
	var diagnostics []provider.Diagnostic
	if err := json.Unmarshal([]byte(values[26]), &diagnostics); err != nil {
		return Metadata{}, fmt.Errorf("last_diagnostics 无效: %w", err)
	}
	metadata = Metadata{
		Schema: int(schema), ID: values[1], Name: values[2], Type: values[3], URL: values[4],
		UserAgent: values[5], HWID: values[6], CustomHeaders: headers, AutoUpdate: autoUpdate,
		UpdateInterval: updateInterval, IntervalSource: values[10], UpdateViaProxy: values[11],
		Include: values[12], Exclude: values[13], AllowInsecure: allowInsecure, Timeout: timeout,
		Usage: usage, NodeCount: int(nodeCount), Revision: revision, ETag: values[19],
		LastModified: values[20], ProfileTitle: values[21], ProfileWebPageURL: values[22],
		ContentDisposition: values[23], FileName: values[24], LastStatusCode: int(statusCode),
		LastDiagnostics: diagnostics, LastAttemptAt: values[27], LastSuccessAt: values[28],
		NextUpdateAt: values[29], NextUpdateEpoch: nextEpoch, LastError: values[31],
		CreatedAt: values[32], UpdatedAt: values[33],
	}
	return normalizeMetadata(metadata), nil
}

// MetadataRawField 返回指定字段的原始 JSON 值。
func MetadataRawField(metadata Metadata, field string) ([]byte, bool) {
	var value any
	switch field {
	case "schema":
		value = metadata.Schema
	case "id":
		value = metadata.ID
	case "name":
		value = metadata.Name
	case "type":
		value = metadata.Type
	case "url":
		value = metadata.URL
	case "user_agent":
		value = metadata.UserAgent
	case "hwid":
		value = metadata.HWID
	case "custom_headers":
		value = metadata.CustomHeaders
	case "auto_update":
		value = metadata.AutoUpdate
	case "update_interval":
		value = metadata.UpdateInterval
	case "interval_source":
		value = metadata.IntervalSource
	case "update_via_proxy":
		value = metadata.UpdateViaProxy
	case "include":
		value = metadata.Include
	case "exclude":
		value = metadata.Exclude
	case "allow_insecure":
		value = metadata.AllowInsecure
	case "timeout":
		value = metadata.Timeout
	case "usage":
		return append([]byte(nil), metadata.Usage...), true
	case "node_count":
		value = metadata.NodeCount
	case "revision":
		value = metadata.Revision
	case "etag":
		value = metadata.ETag
	case "last_modified":
		value = metadata.LastModified
	case "profile_title":
		value = metadata.ProfileTitle
	case "profile_web_page_url":
		value = metadata.ProfileWebPageURL
	case "content_disposition":
		value = metadata.ContentDisposition
	case "file_name":
		value = metadata.FileName
	case "last_status_code":
		value = metadata.LastStatusCode
	case "last_diagnostics":
		value = metadata.LastDiagnostics
	case "last_attempt_at":
		value = metadata.LastAttemptAt
	case "last_success_at":
		value = metadata.LastSuccessAt
	case "next_update_at":
		value = metadata.NextUpdateAt
	case "next_update_epoch":
		value = metadata.NextUpdateEpoch
	case "last_error":
		value = metadata.LastError
	case "created_at":
		value = metadata.CreatedAt
	case "updated_at":
		value = metadata.UpdatedAt
	default:
		return nil, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// MetadataStringField 返回指定字段的字符串值。
func MetadataStringField(metadata Metadata, field string) (string, bool) {
	switch field {
	case "id":
		return metadata.ID, true
	case "name":
		return metadata.Name, true
	case "type":
		return metadata.Type, true
	case "url":
		return metadata.URL, true
	case "user_agent":
		return metadata.UserAgent, true
	case "hwid":
		return metadata.HWID, true
	case "interval_source":
		return metadata.IntervalSource, true
	case "update_via_proxy":
		return metadata.UpdateViaProxy, true
	case "include":
		return metadata.Include, true
	case "exclude":
		return metadata.Exclude, true
	case "etag":
		return metadata.ETag, true
	case "last_modified":
		return metadata.LastModified, true
	case "profile_title":
		return metadata.ProfileTitle, true
	case "profile_web_page_url":
		return metadata.ProfileWebPageURL, true
	case "content_disposition":
		return metadata.ContentDisposition, true
	case "file_name":
		return metadata.FileName, true
	case "last_attempt_at":
		return metadata.LastAttemptAt, true
	case "last_success_at":
		return metadata.LastSuccessAt, true
	case "next_update_at":
		return metadata.NextUpdateAt, true
	case "last_error":
		return metadata.LastError, true
	case "created_at":
		return metadata.CreatedAt, true
	case "updated_at":
		return metadata.UpdatedAt, true
	default:
		raw, ok := MetadataRawField(metadata, field)
		if !ok {
			return "", false
		}
		return string(raw), true
	}
}

func normalizeMetadata(metadata Metadata) Metadata {
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
