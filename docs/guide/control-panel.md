# Clash API 与 zashboard

NetProxy 默认启用 sing-box 的 Clash API，并内置 zashboard 作为前端面板。

## 默认配置

来自 `config/singbox/confdir/02_experimental.json` 的默认值：

- `external_controller`: `127.0.0.1:9999`
- `secret`: `singbox`
- `external_ui`: `/data/adb/modules/netproxy/bin/zashboard`

## 默认入口

- Controller：`http://127.0.0.1:9999`
- UI：`http://127.0.0.1:9999/ui`
- Secret：`singbox`

## 能做什么

通过 Clash API 或 zashboard，你可以：

- 查看当前代理组
- 观察当前选中的节点
- 测试延迟
- 查看活动连接
- 切换 `Rule / Global / Direct`
- 辅助判断当前运行时是否已经加载目标节点

## 与 Android 管理器、CLI 的关系

- Android 管理器：图形化日常入口
- CLI：脚本化和终端入口
- Clash API / zashboard：观察和运行时控制入口

三者共享同一套 sing-box 运行状态，不是互相独立的三套系统。

## 使用建议

- 局域网访问 zashboard 时，优先确认设备 IP
- 如果接口可访问但切换不生效，先检查当前节点是否已加载到运行实例
- 如果面板打不开，先检查 `service.log` 与 `sing-box.log`

## 安全提醒

默认控制器只监听 `127.0.0.1:9999`，仅供本机 zashboard 和管理器使用。
如果确实需要局域网访问，请显式调整控制地址，并同步设置独立密钥和可访问范围。
