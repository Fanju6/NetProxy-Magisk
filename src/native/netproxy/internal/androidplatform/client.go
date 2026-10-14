package androidplatform

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os/exec"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
)

type Request struct {
	Operation  string                `json:"operation"`
	IconCache  string                `json:"icon_cache,omitempty"`
	Connection *apiconfig.Connection `json:"connection,omitempty"`
}

type Client struct{ DEX string }

func (client Client) Query(ctx context.Context, request Request) (jsontext.Value, error) {
	content, err := json.Marshal(request, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/system/bin/app_process", "-Djava.class.path="+client.DEX, "/system/bin", "com.fanjv.netproxy.platform.Main")
	command.Stdin = bytes.NewReader(content)
	command.WaitDelay = time.Second
	output := limitedBuffer{limit: 8 << 20}
	command.Stdout = &output
	runErr := command.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if runErr != nil && output.Len() == 0 {
		return nil, &contract.Error{Code: "android.unavailable", Message: "Android 平台查询失败"}
	}
	var result struct {
		Schema  int            `json:"schema"`
		OK      bool           `json:"ok"`
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Data    jsontext.Value `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &result, json.RejectUnknownMembers(true)); err != nil || result.Schema != 1 || result.Code == "" {
		return nil, &contract.Error{Code: "android.invalid_output", Message: "Android 平台没有返回有效数据"}
	}
	if !result.OK || runErr != nil {
		// 辅助程序不能把输入凭据或系统异常文本回传为界面错误。
		return nil, &contract.Error{Code: "android.query_failed", Message: "无法读取 Android 平台信息"}
	}
	return result.Data, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	if len(value) > buffer.limit-buffer.Len() {
		return 0, errors.New("平台响应超出大小限制")
	}
	return buffer.Buffer.Write(value)
}
