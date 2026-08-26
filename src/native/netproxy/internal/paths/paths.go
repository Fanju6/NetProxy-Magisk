// Package paths 提供 NetProxy 生产目录的统一布局。
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultDevRoot = "/dev/netproxy"

// Layout 描述一个 NetProxy 模块实例的固定目录布局。
type Layout struct {
	moduleRoot string
	devRoot    string
}

// New 根据模块根目录创建路径布局。
func New(root string) Layout {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return Layout{moduleRoot: filepath.Clean(root), devRoot: defaultDevRoot}
}

// Default 返回当前模块的默认路径布局。
func Default() Layout { return New(Root()) }

// Root 返回模块根目录。
func Root() string {
	if root := os.Getenv("NETPROXY_MODULE_DIR"); root != "" {
		return filepath.Clean(root)
	}
	executable, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(filepath.Dir(executable))
}

// Root 返回模块根目录。
func (l Layout) Root() string { return l.moduleRoot }

// Config 返回用户配置目录。
func (l Layout) Config() string { return filepath.Join(l.moduleRoot, "config") }

// Data 返回持久化数据目录。
func (l Layout) Data() string { return filepath.Join(l.moduleRoot, "data") }

// Catalog 返回 Catalog 持久化目录。
func (l Layout) Catalog() string { return filepath.Join(l.Data(), "catalog") }

// ModuleConfig 返回模块配置文件路径。
func (l Layout) ModuleConfig() string { return filepath.Join(l.Config(), "module.conf") }

// EBPFConfig 返回 eBPF 配置文件路径。
func (l Layout) EBPFConfig() string { return filepath.Join(l.Config(), "ebpf", "ebpf.conf") }

// SingBoxDir 返回 sing-box 静态配置根目录。
func (l Layout) SingBoxDir() string { return filepath.Join(l.Config(), "singbox") }

// Runtime 返回运行时生成目录。
func (l Layout) Runtime() string { return filepath.Join(l.moduleRoot, "runtime") }

// Logs 返回日志目录。
func (l Layout) Logs() string { return filepath.Join(l.moduleRoot, "logs") }

// ModuleProp 返回模块元信息文件路径。
func (l Layout) ModuleProp() string { return filepath.Join(l.moduleRoot, "module.prop") }

// Bin 返回原生二进制目录。
func (l Layout) Bin() string { return filepath.Join(l.moduleRoot, "bin") }

// SingBox 返回 sing-box 二进制路径。
func (l Layout) SingBox() string { return filepath.Join(l.Bin(), "sing-box") }

// Executable 返回模块唯一 Go 可执行文件路径。
func (l Layout) Executable() string { return filepath.Join(l.Bin(), "netproxyctl") }

// ServiceLog 返回统一服务日志路径。
func (l Layout) ServiceLog() string { return filepath.Join(l.Logs(), "service.log") }

// DevRoot 返回运行时状态目录。
func (l Layout) DevRoot() string { return l.devRoot }

// ServiceState 返回服务状态文件路径。
func (l Layout) ServiceState() string { return filepath.Join(l.DevRoot(), "service.json") }

// WorkerPID 返回后台 Worker PID 文件路径。
func (l Layout) WorkerPID() string { return filepath.Join(l.DevRoot(), "worker.pid") }

// ProgressDir 返回订阅进度目录。
func (l Layout) ProgressDir() string { return filepath.Join(l.DevRoot(), "subscriptions") }

// DelayDir 返回离线节点测速的临时会话目录。
func (l Layout) DelayDir() string { return filepath.Join(l.DevRoot(), "delay") }

// WiFiState 返回 Wi-Fi 自动策略状态文件路径。
func (l Layout) WiFiState() string { return filepath.Join(l.DevRoot(), "wifi_state") }

// SingBoxConfDir 返回给定 sing-box 配置根目录下的配置片段目录。
func SingBoxConfDir(singBoxDir string) string { return filepath.Join(singBoxDir, "confdir") }

// SingBoxRulesDir 返回给定 sing-box 配置根目录下的规则资源目录。
func SingBoxRulesDir(singBoxDir string) string { return filepath.Join(singBoxDir, "rules") }

// SingBoxLocalRulesDir 返回给定 sing-box 配置根目录下的本地规则目录。
func SingBoxLocalRulesDir(singBoxDir string) string {
	return filepath.Join(SingBoxRulesDir(singBoxDir), "local")
}

// SingBoxRemoteRulesDir 返回给定 sing-box 配置根目录下的远程规则目录。
func SingBoxRemoteRulesDir(singBoxDir string) string {
	return filepath.Join(SingBoxRulesDir(singBoxDir), "remote")
}
