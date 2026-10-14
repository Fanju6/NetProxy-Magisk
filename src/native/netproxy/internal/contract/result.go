package contract

import (
	"context"
	"errors"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

type Result struct {
	Schema  int    `json:"schema"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitzero"`
}

type Error struct {
	Status     int
	HTTPStatus int
	Code       string
	Message    string
	Data       any
}

func (err *Error) Error() string { return err.Message }

func Success(code, message string, data any) Result {
	return Result{Schema: 1, OK: true, Code: code, Message: message, Data: data}
}

// Failure 优先保留已持久化错误，不能让后续取消覆盖真实的写入结果。
func Failure(ctx context.Context, err error) (Result, int) {
	code, message, data, status := "command.failed", err.Error(), any(map[string]any{}), 1
	switch {
	default:
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code, message, status = "command.timeout", "命令执行超时", 124
		} else if errors.Is(err, context.Canceled) {
			code, message = "command.canceled", "操作已取消"
		}
	case errors.Is(err, module.ErrConfigConflict):
		code = "config.conflict"
	case errors.Is(err, module.ErrForcedTermination):
		code = "inbound.cleanup_unconfirmed"
	}
	if value, ok := errors.AsType[*Error](err); ok {
		code, message, data, status = value.Code, value.Message, value.Data, value.Status
	} else if value, ok := errors.AsType[*inbound.ValidationError](err); ok && len(value.Diagnostics) > 0 {
		code, data, status = value.Diagnostics[0].Code, map[string]any{"diagnostics": value.Diagnostics}, 1
	} else if value, ok := errors.AsType[*subscription.Error](err); ok {
		code, message, data, status = value.Code, value.Message, value.Data, 1
	} else if value, ok := errors.AsType[*service.Error](err); ok {
		code, message, data, status = value.Code, value.Message, value.Data, 1
	}
	if status <= 0 {
		status = 1
	}
	return Result{Schema: 1, OK: false, Code: code, Message: message, Data: data}, status
}
