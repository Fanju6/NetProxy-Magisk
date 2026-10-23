package logfile

import (
	"regexp"
	"strings"

	"encoding/json/jsontext"
	json "encoding/json/v2"
)

const maxMessageRunes = 1024

var (
	urlCredentialPattern = regexp.MustCompile(`(https?://)[^/@\s]+@`)
	querySecretPattern   = regexp.MustCompile(`(?i)([?&](?:token|key|secret|password|auth|uuid|hwid)=)[^&\s]+`)
	authorizationPattern = regexp.MustCompile(`(?i)((?:authorization|proxy-authorization)\s*:\s*(?:bearer|basic)\s+)[^\r\n\s,;，；。]+`)
	lineSecretPattern    = regexp.MustCompile(`(?i)((?:["']?(?:x-hwid|hwid|uuid|token|password|secret|auth(?:[_-]?(?:str|key))?|p(?:re)?[_-]?shared[_-]?key|psk|private[_-]?key|public[_-]?key|short[_-]?id|custom[_-]?headers)["']?\s*[:=]\s*["']?))[^"'\r\n\s,;}，；。]+`)
	httpURLPattern       = regexp.MustCompile(`(?i)\bhttps?://[^\s\p{Cc}<>"'，；。]+`)
	otherURLPattern      = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s\p{Cc}<>"'，；。]+`)
	privateKeyPattern    = regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |ENCRYPTED |OPENSSH )?PRIVATE KEY-----.*?(?:-----END (?:RSA |EC |ENCRYPTED |OPENSSH )?PRIVATE KEY-----|$)`)
	tlsSecretJSONKeys    = map[string]struct{}{
		"key": {}, "client_key": {}, "mca_key": {}, "client_key_password": {}, "mca_key_password": {},
	}
	sensitiveJSONKeys = map[string]struct{}{
		"access_token": {}, "api_key": {}, "auth": {}, "auth_key": {}, "auth_str": {},
		"authorization": {}, "client_secret": {}, "custom_headers": {}, "headers": {},
		"hwid": {}, "password": {}, "pre_shared_key": {}, "private_key": {},
		"private_key_passphrase": {}, "private_key_path": {}, "proxy_authorization": {},
		"psk": {}, "public_key": {}, "secret": {}, "short_id": {}, "token": {},
		"url": {}, "user_agent": {}, "username": {}, "uuid": {},
		"proxy_apps": {}, "bypass_apps": {}, "include_package": {}, "exclude_package": {},
	}
)

// RedactText 对日志、配置和诊断文本使用统一的凭据脱敏规则。
func RedactText(value string) string {
	var document any
	if json.Unmarshal([]byte(value), &document) == nil {
		switch document.(type) {
		case map[string]any, []any:
			redactJSON(document, false)
			if encoded, err := json.Marshal(document, json.Deterministic(true), jsontext.WithIndent("  ")); err == nil {
				value = string(encoded) + "\n"
			}
		}
	}
	value = privateKeyPattern.ReplaceAllString(value, "***")
	value = urlCredentialPattern.ReplaceAllString(value, `$1***@`)
	value = querySecretPattern.ReplaceAllString(value, `${1}***`)
	value = authorizationPattern.ReplaceAllString(value, `${1}***`)
	value = lineSecretPattern.ReplaceAllString(value, `${1}***`)
	value = httpURLPattern.ReplaceAllString(value, "[订阅链接已隐藏]")
	value = otherURLPattern.ReplaceAllString(value, "[节点链接已隐藏]")
	return value
}

// RedactMessage 返回适合单行落盘的脱敏短消息。
func RedactMessage(value string) string {
	value = strings.Join(strings.Fields(RedactText(value)), " ")
	runes := []rune(value)
	if len(runes) <= maxMessageRunes {
		return value
	}
	return string(runes[:maxMessageRunes]) + "…"
}

func redactJSON(value any, tls bool) any {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			normalized := strings.ReplaceAll(strings.ToLower(key), "-", "_")
			if normalized == "wifi" {
				if policy, ok := child.(map[string]any); ok {
					for _, name := range []string{"blacklist", "whitelist"} {
						if _, exists := policy[name]; exists {
							policy[name] = "***"
						}
					}
				}
			}
			_, sensitive := sensitiveJSONKeys[normalized]
			_, tlsSecret := tlsSecretJSONKeys[normalized]
			if sensitive || tls && tlsSecret {
				item[key] = "***"
				continue
			}
			// TLS 密钥列表可能逐行存 PEM；仅沿已知 TLS 子结构传播上下文。
			childTLS := normalized == "tls" || tls && (normalized == "ech" || normalized == "control_wrap")
			item[key] = redactJSON(child, childTLS)
		}
	case []any:
		for index, child := range item {
			item[index] = redactJSON(child, tls)
		}
	case string:
		return privateKeyPattern.ReplaceAllString(item, "***")
	}
	return value
}
