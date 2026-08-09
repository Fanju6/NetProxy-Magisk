package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

type result struct {
	Schema  int    `json:"schema"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type resultError struct {
	Code    string
	Message string
	Data    any
}

func (e *resultError) Error() string {
	return e.Message
}

type convertOptions struct {
	output          string
	metadataOutput  string
	diagnosticsFile string
	allowInsecure   bool
	include         string
	exclude         string
}

type serviceSnapshot struct {
	Memory           uint64 `json:"memory"`
	Goroutines       int32  `json:"goroutines"`
	ConnectionsIn    int32  `json:"connections_in"`
	ConnectionsOut   int32  `json:"connections_out"`
	TrafficAvailable bool   `json:"traffic_available"`
	Uplink           int64  `json:"uplink"`
	Downlink         int64  `json:"downlink"`
	UplinkTotal      int64  `json:"uplink_total"`
	DownlinkTotal    int64  `json:"downlink_total"`
	Selected         string `json:"selected"`
}

func readHeadersFile(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var headers map[string]string
	if err := json.Unmarshal(content, &headers); err != nil {
		return nil, fmt.Errorf("自定义请求头 JSON 无效: %w", err)
	}
	if headers == nil {
		return map[string]string{}, nil
	}
	return headers, nil
}

func writeOptionalJSON(path string, value any) error {
	if path == "" {
		return nil
	}
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return provider.WriteAtomic(path, content, 0o600)
}

func writeJSON(writer io.Writer, value any) {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func dependencyVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dependency := range info.Deps {
		if dependency.Path != path {
			continue
		}
		if dependency.Replace != nil && dependency.Replace.Version != "" {
			return dependency.Replace.Version
		}
		return dependency.Version
	}
	return "unknown"
}

func showUsage() {
	executable := filepath.Base(os.Args[0])
	fmt.Printf(`%s - NetProxy 原生组件

用法：
  %s convert link --input <链接> --output <provider.json>
  %s convert file --input <文件> --output <provider.json>
  %s convert subscription --url <地址> --output <provider.json>
  %s provider append --target <provider.json> --input <链接或文件>
  %s provider remove --target <provider.json> --tag <标签>
  %s catalog group-import --root <catalog> --group <分组 ID> --input <文件>
  %s catalog group-init --root <catalog> --group <分组 ID> --type <local|subscription>
  %s catalog group-ensure --root <catalog> --group <分组 ID> --type local
  %s catalog group-name --root <catalog> --group <分组 ID> --name <名称>
  %s catalog new-id --root <catalog> --kind <subscription|file> [--input <文件>]
  %s catalog node-append --group-dir <分组目录> --group <分组 ID> --input <链接或文件>
  %s catalog node-remove --group-dir <分组目录> --group <分组 ID> --tag <标签>
  %s catalog node-edit --group-dir <分组目录> --group <分组 ID> --tag <标签> --input <链接或文件>
  %s catalog resolve|group-has-nodes|group-first-tag|group-contains-tag ...
  %s catalog group-type|first-nonempty|node-get|node-export ...
  %s catalog group-private|group-delete|history ...
  %s provider inspect --input <provider.json> --format json
  %s provider export --input <provider.json> --tag <标签>
  %s provider validate --input <provider.json>
  %s catalog <groups|snapshot|group|show|runtime|schedule> --root <catalog>
  %s subscription update|edit --root <catalog> --group <group-id>
  %s service <ready|started-at|snapshot|groups|mode|select|urltest|close-all>
  netproxy-native module <prepare|sync|select|mode|app|node|sub|config|logs|state|service> ...
  control <status|nodes|snapshot|selection|groups|mode|runtime-mode|set-mode|delay|close-all> ...
  netproxy-native ebpf runtime --config <ebpf.conf> --output <ebpf.json>
  netproxy-native ebpf status --config <ebpf.conf> --sing-box <sing-box> [--mode <configured|all|local|shared>] [--raw]
  %s subworker <start|stop|restart|wake|status|once|run> --root <catalog> --module-conf <module.conf>
  netproxy-native version

转换选项：
  --include <正则>              仅保留匹配的节点
  --exclude <正则>              排除匹配的节点
  --allow-insecure              显式跳过 TLS 证书校验
  --diagnostics-output <文件>   写入结构化解析诊断
  --metadata-output <文件>      写入订阅 HTTP 元数据

订阅选项：
  --user-agent <值>             自定义 User-Agent
  --hwid <值>                   自定义 X-HWID
  --headers-file <文件>         从 JSON 对象读取自定义请求头
  --etag <值>                   发送 If-None-Match
  --last-modified <值>          发送 If-Modified-Since
  --proxy <URL>                 通过 HTTP 代理下载
  --timeout <时长>              下载超时，默认 60s
`, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable, executable)
}
