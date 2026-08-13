package module

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestLogOperationWritesStructuredSanitizedFailure(t *testing.T) {
	options := Options{LogDir: t.TempDir()}
	logOperation(options, "subscription", "subscription.update", "订阅更新", false, nil)
	logOperation(options, "subscription", "subscription.edit", "订阅编辑", true, &subscription.Error{
		Code:    "subscription.convert_failed",
		Message: "订阅下载、转换或校验失败",
		Data: map[string]any{
			"cause": "请求 https://user:password@example.test/?token=secret 失败，Authorization: Bearer bearer-secret，HWID=hwid-secret，节点 vmess://node-secret，connection refused",
		},
	})
	logOperation(options, "node", "node.import", "节点导入", false, errors.New("vmess://secret"))

	content, err := os.ReadFile(filepath.Join(options.LogDir, "service.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, expected := range []string{
		"[INFO] [subscription] [subscription.update] [success] [-] 订阅更新",
		"[WARN] [subscription] [subscription.edit] [persisted] [subscription.convert_failed] 订阅编辑，但后续操作失败：订阅下载、转换或校验失败",
		"connection refused",
		"[ERROR] [node] [node.import] [failed] [operation.failed] 节点导入失败：[节点链接已隐藏]",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("统一动作日志缺少 %q:\n%s", expected, text)
		}
	}
	for _, secret := range []string{"secret", "example.test", "password", "bearer-secret", "hwid-secret", "vmess://"} {
		if strings.Contains(text, secret) {
			t.Fatalf("原始动作日志泄露失败原因 %q: %s", secret, text)
		}
	}
}
