package module

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"syscall"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
)

const lifecycleEvidenceLimit = 16 << 10

type lifecycleFailure struct {
	code, message string
	cause         error
}

func (e *lifecycleFailure) Error() string { return e.message }
func (e *lifecycleFailure) Unwrap() error { return e.cause }

func readyFailure(err error) *lifecycleFailure {
	failure := &lifecycleFailure{"service.api_unavailable", "控制接口未返回可用状态", err}
	if response, ok := errors.AsType[*serviceapi.HTTPError](err); ok {
		failure.message = fmt.Sprintf("控制接口返回 HTTP %d", response.Status)
		if response.Status == http.StatusUnauthorized || response.Status == http.StatusForbidden {
			failure.code = "service.api_auth"
			failure.message += "（认证失败）"
		}
		return failure
	}
	switch {
	case errors.Is(err, context.Canceled):
		failure.code, failure.message = "service.cancelled", "已取消就绪检查"
	case errors.Is(err, context.DeadlineExceeded):
		failure.code, failure.message = "service.api_timeout", "控制接口请求超时"
	case errors.Is(err, syscall.ECONNREFUSED):
		failure.code, failure.message = "service.api_refused", "控制接口拒绝连接"
	case errors.Is(err, serviceapi.ErrIncompatibleVersion):
		failure.code, failure.message = "service.api_version", err.Error()
	case errors.Is(err, serviceapi.ErrNoActiveInstance):
		failure.code, failure.message = "service.no_instance", "控制接口尚未报告活动内核"
	case errors.Is(err, errUnchangedCore):
		failure.code, failure.message = "service.instance_unchanged", "控制接口仍报告重载前的内核实例"
	}
	return failure
}

var errUnchangedCore = errors.New("unchanged core instance")

var timestampedCoreFatal = regexp.MustCompile(`^[+-]\d{4} \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} FATAL (.+)$`)

// The checkpoint is after log rotation, before the child or reload can write.
// Verify the previous bytes too: clearing the log truncates the same inode.
type coreLogCheckpoint struct {
	info   os.FileInfo
	anchor []byte
}

func checkpointCoreLog(path string) coreLogCheckpoint {
	file, err := os.Open(path)
	if err != nil {
		return coreLogCheckpoint{}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return coreLogCheckpoint{}
	}
	anchor := make([]byte, min(info.Size(), 64))
	if _, err := file.ReadAt(anchor, info.Size()-int64(len(anchor))); err != nil {
		return coreLogCheckpoint{}
	}
	return coreLogCheckpoint{info, anchor}
}

func (checkpoint coreLogCheckpoint) failure(path string, cause error) error {
	if checkpoint.info == nil || errors.Is(cause, context.Canceled) {
		return cause
	}
	file, err := os.Open(path)
	if err != nil {
		return cause
	}
	defer file.Close()
	info, err := file.Stat()
	size := checkpoint.info.Size()
	if err != nil || !os.SameFile(checkpoint.info, info) || info.Size() <= size {
		return cause
	}
	anchor := make([]byte, len(checkpoint.anchor))
	if _, err := file.ReadAt(anchor, size-int64(len(anchor))); err != nil || !bytes.Equal(anchor, checkpoint.anchor) {
		return cause
	}
	offset := max(size, info.Size()-lifecycleEvidenceLimit)
	buf, err := io.ReadAll(io.NewSectionReader(file, offset, info.Size()-offset))
	if err != nil {
		return cause
	}
	if offset > size || (len(anchor) != 0 && anchor[len(anchor)-1] != '\n') {
		_, buf, _ = bytes.Cut(buf, []byte{'\n'})
	}
	if reason := coreFailureLine(buf); reason != "" {
		return &lifecycleFailure{"service.core_fatal", reason + "；" + cause.Error(), cause}
	}
	return cause
}

func coreFailureLine(buf []byte) string {
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		return ""
	}
	buf = buf[:end+1]
	for line := range strings.SplitSeq(logfile.RedactText(string(buf)), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "FATAL"); ok && strings.HasPrefix(after, "[") {
			if _, reason, ok := strings.Cut(after, "]"); ok {
				return logfile.RedactMessage(reason)
			}
		}
		if strings.HasPrefix(line, "FATAL ") || strings.HasPrefix(line, "panic: ") || strings.HasPrefix(line, "fatal error: ") {
			return logfile.RedactMessage(line)
		}
		if match := timestampedCoreFatal.FindStringSubmatch(line); match != nil {
			return logfile.RedactMessage(match[1])
		}
	}
	return ""
}

type configCheckOutput struct{ buffer bytes.Buffer }

func (b *configCheckOutput) Write(p []byte) (int, error) {
	_, _ = b.buffer.Write(p[:min(len(p), lifecycleEvidenceLimit-b.buffer.Len())])
	return len(p), nil
}
