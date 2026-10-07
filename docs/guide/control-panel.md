# 控制面板与 API

模块 WebUI 用于模块操作，sing-box Dashboard 与 Clash API 用于核心运行时查看和控制。它们共用模块配置，不维护另一份节点数据。

## 模块 WebUI

从 KernelSU、Magisk 或 APatch 的模块详情页打开。当前 WebUI 是终端式界面：它提供命令补全、执行历史和易读结果渲染，所有操作仍统一调用 `netproxyctl`。

它适合无 Android 管理器的临时操作和排障，不维护另一份节点或订阅数据库。

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

两个 API 默认只监听 `127.0.0.1`。需要局域网访问时，必须同时评估监听范围、独立密钥、TLS、CORS 与 Private Network Access；不要只放宽监听地址并继续使用默认密钥。

面板不可用时先检查：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```
