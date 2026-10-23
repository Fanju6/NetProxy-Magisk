package logfile

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestRedactTextTLSKeyContext(t *testing.T) {
	for _, test := range []struct {
		name string
		tls  map[string]any
	}{
		{"server-lines", map[string]any{"key": []string{"-----BEGIN PRIVATE KEY-----", "FAKE_TLS_MATERIAL", "-----END PRIVATE KEY-----"}}},
		{"client-lines", map[string]any{"client_key": []string{"-----BEGIN RSA PRIVATE KEY-----", "FAKE_TLS_MATERIAL", "-----END RSA PRIVATE KEY-----"}}},
		{"server-string", map[string]any{"key": "FAKE_TLS_MATERIAL"}},
		{"client-string", map[string]any{"client_key": "FAKE_TLS_MATERIAL"}},
		{"ech", map[string]any{"ech": map[string]any{"key": []string{"FAKE_TLS_MATERIAL"}}}},
		{"control-wrap", map[string]any{"control_wrap": map[string]any{"key": []string{"FAKE_TLS_MATERIAL"}}}},
		{"openconnect-mca", map[string]any{"mca_key": []string{"FAKE_TLS_MATERIAL"}, "client_key_password": "FAKE_TLS_MATERIAL", "mca_key_password": "FAKE_TLS_MATERIAL"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.tls["enabled"] = true
			test.tls["server_name"] = "example.test"
			test.tls["extension"] = map[string]any{"key": "ordinary-extension"}
			input, err := json.Marshal([]any{map[string]any{
				"key": "ordinary-business", "client_key": "ordinary-client-label",
				"nested": map[string]any{"tls": test.tls, "key": "ordinary-nested"},
			}}, json.Deterministic(true))
			if err != nil {
				t.Fatal(err)
			}
			output := RedactText(string(input))
			if strings.Contains(output, "FAKE_TLS_MATERIAL") {
				t.Fatal("TLS 私钥字段未脱敏")
			}
			for _, keep := range []string{"ordinary-business", "ordinary-client-label", "ordinary-nested", "ordinary-extension", "example.test"} {
				if !strings.Contains(output, keep) {
					t.Fatal("误脱敏普通字段或 TLS 诊断信息")
				}
			}
			var decoded []map[string]any
			if err := json.Unmarshal([]byte(output), &decoded); err != nil {
				t.Fatal("脱敏后的 JSON 无效")
			}
			if !strings.Contains(output, "***") {
				t.Fatal("缺少脱敏占位")
			}
		})
	}
}

func TestRedactTextPrivateKeyPEM(t *testing.T) {
	for _, label := range []string{"PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY", "ENCRYPTED PRIVATE KEY", "OPENSSH PRIVATE KEY"} {
		t.Run(label, func(t *testing.T) {
			pem := "-----BEGIN " + label + "-----\nFAKE_PEM_MATERIAL\n-----END " + label + "-----"
			for _, input := range []string{
				"before\n" + pem + "\nafter",
				"before " + strings.ReplaceAll(pem, "\n", `\n`) + " after",
				"before " + strings.ReplaceAll(pem, "\n", "\r\n") + " after",
				"before " + strings.ReplaceAll(pem, "\n", " ") + " after",
			} {
				for _, output := range []string{RedactText(input), RedactMessage(input)} {
					if strings.Contains(output, "FAKE_PEM_MATERIAL") || strings.Contains(output, "-----BEGIN") {
						t.Fatal("PEM 私钥日志未脱敏")
					}
					if !strings.Contains(output, "before") || !strings.Contains(output, "after") {
						t.Fatal("完整 PEM 块误吞相邻日志")
					}
				}
			}
			if strings.Contains(RedactText("-----BEGIN "+label+"-----\nFAKE_TRUNCATED_MATERIAL"), "FAKE_TRUNCATED_MATERIAL") {
				t.Fatal("缺少 END 的私钥日志未脱敏")
			}
		})
	}
}

func TestRedactTextPEMInJSONStringsAndOrdinaryKeys(t *testing.T) {
	input := `{"key":"business-value","nested":{"key":"-----BEGIN EC PRIVATE KEY-----\nFAKE_JSON_PEM\n-----END EC PRIVATE KEY-----"},"list":["-----BEGIN OPENSSH PRIVATE KEY-----\nFAKE_JSON_PEM\n-----END OPENSSH PRIVATE KEY-----"],"certificate":"-----BEGIN CERTIFICATE-----\nPUBLIC_CERTIFICATE\n-----END CERTIFICATE-----","private_key":"FAKE_EXISTING_SECRET","token":"FAKE_EXISTING_SECRET","password":"FAKE_EXISTING_SECRET"}`
	output := RedactText(input)
	for _, secret := range []string{"FAKE_JSON_PEM", "FAKE_EXISTING_SECRET"} {
		if strings.Contains(output, secret) {
			t.Fatal("JSON 字符串或已有敏感字段未脱敏")
		}
	}
	for _, keep := range []string{"business-value", "PUBLIC_CERTIFICATE"} {
		if !strings.Contains(output, keep) {
			t.Fatal("普通 key 或公开证书被误脱敏")
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatal("PEM 脱敏破坏 JSON 格式")
	}
}
