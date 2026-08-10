// Package paths 统一计算 NetProxy 的固定目录布局。
package paths

import (
	"os"
	"path/filepath"
)

// Root 返回模块根目录，测试和宿主工具可以通过 NETPROXY_MODULE_DIR 覆盖。
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

// Catalog 返回 Catalog 持久化目录。
func Catalog(root string) string { return filepath.Join(root, "data", "catalog") }

// ModuleConfig 返回模块配置文件路径。
func ModuleConfig(root string) string { return filepath.Join(root, "config", "module.conf") }

// EBPFConfig 返回 eBPF 配置文件路径。
func EBPFConfig(root string) string { return filepath.Join(root, "config", "ebpf", "ebpf.conf") }

// SingBox 返回 sing-box 二进制路径。
func SingBox(root string) string { return filepath.Join(root, "bin", "sing-box") }

// SingBoxConfig 返回 sing-box 静态配置目录。
func SingBoxConfig(root string) string { return filepath.Join(root, "config", "singbox") }

// Runtime 返回运行时目录。
func Runtime(root string) string { return filepath.Join(root, "runtime") }

// Logs 返回日志目录。
func Logs(root string) string { return filepath.Join(root, "logs") }
