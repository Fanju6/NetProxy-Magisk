# 控制面板与 API

模块 WebUI 用于模块操作，sing-box Dashboard 与 Clash API 用于核心运行时查看和控制。它们共用模块配置，不维护另一份节点数据。

## 模块 HTTP API

模块管理接口默认位于 `http://127.0.0.1:9091/api/v1`，可读取 Catalog、保存配置、更新订阅和操作服务。它独立于 sing-box，核心停止时仍可使用。

通过 Root 执行 `api access` 取得专用凭据；请求必须携带 Bearer header。当前终端 WebUI 和 Android 管理器仍使用 CLI。连接步骤与请求示例见 [HTTP API](/config/http-api)。

## 图形化 HTTP 管理面板

**入口：** `http://127.0.0.1:9091/ui/`，与模块 HTTP API 共享同源地址（9091），不需要额外的服务进程或第三方托管。

1. 更新到包含该面板的模块构建，并确认 `netproxyctl api status` 返回管理接口就绪。如需手动恢复，执行 `su -c '/data/adb/modules/netproxy/netproxyctl api start'`。
2. 通过 Root 终端运行 `su -c '/data/adb/modules/netproxy/netproxyctl api access'`，读取结果中 `data.secret` 的 **64 位十六进制** 密钥。
3. 在手机浏览器访问 `http://127.0.0.1:9091/ui/`，粘贴密钥连接。面板直接连接原生 API，支持图形化服务操作、节点选择/导入/编辑/删除、订阅设置/更新、JSON 配置编辑、日志、eBPF 检查、诊断包导出及实时事件。
4. 电脑使用 USB 管理时，在电脑执行 `adb forward tcp:9091 tcp:9091`，再在电脑浏览器打开同一地址。

密钥只在该页面的**内存**中使用，不写入 localStorage、sessionStorage、URL 或 Cookie；刷新、关闭或退出需要重新输入。前端不使用第三方 CDN。管理 API 的所有资源、操作和 SSE 事件仍要求 Bearer 鉴权，只有静态面板无需认证。静态资源依然校验 Host、Origin 并设定 CSP，不允许其他域跨域控制。

配置编辑从服务端取得原文与 revision，保存时回传相同 revision；遇到冲突不会自动覆盖。服务返回 `persisted=true` 表示文件已经落盘，即使运行时同步失败也不应重复提交。核心停止时仍可读取完整配置进行恢复。

## 模块终端 WebUI

从 KernelSU、Magisk 或 APatch 模块详情页打开的是原有终端式 WebUI：它提供命令补全、执行历史和易读结果渲染，所有操作调用 `netproxyctl`。两个界面共用业务和持久化数据，终端页面保留供高级操作和排障。

## Service API Dashboard

sing-box Dashboard 使用 Service API，展示原生服务状态、节点组、连接与可观测性数据：

- 地址：`http://127.0.0.1:9090/dashboard/`
- Secret：`singbox`

Android 管理器也通过 Service API 合并核心实时状态。

## Clash API

兼容的第三方 Clash 客户端可连接此 API，查看运行时状态和控制代理组：

- Controller：`http://127.0.0.1:9999`
- Secret：`singbox`

Clash API 与 Service API 的监听和密钥均保存在 `config/singbox/config.json`，分别位于 `experimental.clash_api` 和 `services`。详情见[配置参考](/config/singbox#api-与-dashboard)。

## 安全边界

核心的两个 API 默认只监听 `127.0.0.1`。需要局域网访问时，必须同时评估监听范围、独立密钥、TLS、CORS 与 Private Network Access；不要只放宽监听地址并继续使用默认密钥。模块 HTTP API 使用独立随机凭据，当前仅允许本机 IP，不开放局域网监听。

面板不可用时先检查：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```
