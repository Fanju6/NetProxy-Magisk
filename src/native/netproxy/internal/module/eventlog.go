package module

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func logEvent(options Options, level, component, event, result, format string, args ...any) {
	logEventWithCode(options, level, component, event, result, "", format, args...)
}

func logEventWithCode(options Options, level, component, event, result, errorCode, format string, args ...any) {
	if strings.TrimSpace(options.LogDir) == "" {
		return
	}
	err := logfile.AppendEntry(filepath.Join(options.LogDir, "service.log"), logfile.Entry{
		Level: level, Component: component, Event: event, Result: result,
		ErrorCode: errorCode, Message: fmt.Sprintf(format, args...),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Native 日志写入失败: %v\n", err)
	}
}

func logOperation(options Options, component, event, message string, persisted bool, err error) {
	if err == nil {
		logEvent(options, "INFO", component, event, "success", "%s", message)
		return
	}
	errorCode, reason := operationFailure(err)
	if persisted {
		logEventWithCode(options, "WARN", component, event, "persisted", errorCode, "%s，但后续操作失败：%s", message, reason)
		return
	}
	logEventWithCode(options, "ERROR", component, event, "failed", errorCode, "%s失败：%s", message, reason)
}

func operationFailure(err error) (string, string) {
	var subscriptionError *subscription.Error
	if errors.As(err, &subscriptionError) {
		return subscriptionError.Code, structuredFailureReason(subscriptionError.Message, subscriptionError.Data)
	}
	var serviceError *service.Error
	if errors.As(err, &serviceError) {
		return serviceError.Code, structuredFailureReason(serviceError.Message, serviceError.Data)
	}
	return "operation.failed", err.Error()
}

func structuredFailureReason(message string, data any) string {
	cause := ""
	if fields, ok := data.(map[string]any); ok {
		cause, _ = fields["cause"].(string)
	}
	message = strings.TrimSpace(message)
	cause = strings.TrimSpace(cause)
	if cause == "" || cause == message {
		return message
	}
	return message + "：" + cause
}
