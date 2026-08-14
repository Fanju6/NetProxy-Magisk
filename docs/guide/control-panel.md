# 控制面板与 API

NetProxy 8.0 提供三个本机控制入口。模块 WebUI 是入口页，两个 sing-box 面板分别使用 Service API 和 Clash API。

## 模块 WebUI

从 KernelSU、Magisk 或 APatch 的模块页面进入 NetProxy WebUI。入口页提供：

- NetProxy WebUI：节点、订阅、服务和模块配置。
- zashboard：Clash API 代理组、连接和延迟控制。
- sing-box Dashboard：Service API 原生状态、连接、节点组和运行数据。

入口页本身不复制节点数据，也不维护另一套配置；所有持久化操作都经过 `netproxyctl`。

## zashboard / Clash API

默认配置：

- Controller：`127.0.0.1:9999`
- UI：`http://127.0.0.1:9999/ui/`
- Secret：`singbox`
- 外部 UI：模块 `webroot` 中内置的 zashboard

zashboard 适合查看代理组、当前选择、活动连接、流量和延迟，也可以切换 Rule、Global、Direct 等运行时模式。

## sing-box Service API Dashboard

默认配置：

- Service API：`127.0.0.1:9090`
- Dashboard：`http://127.0.0.1:9090/dashboard/`
- Secret：`singbox`
- Dashboard 资源：模块 `webroot` 中内置的 sing-box Dashboard

Service API 是 Android 管理器读取核心运行状态的事实来源，负责服务状态、节点组、URLTest、连接和流量等原生能力。

## 安全边界

两个 API 默认只监听 loopback。若需要从局域网访问，必须同时评估：

1. 监听地址是否需要扩大。
2. Secret 是否已经更换为独立随机值。
3. 是否启用 TLS、CORS 和 Private Network Access 限制。
4. 设备网络是否会把端口暴露给不可信用户。

不要只修改监听地址而继续使用默认密钥。

## 排障

面板打不开时，先确认服务状态：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```

如果 zashboard 能打开但节点组为空，检查 Catalog 是否存在可用节点以及当前活动分组；如果 Dashboard 无法打开，检查 Service API 是否在服务进入 `ready` 后监听。
