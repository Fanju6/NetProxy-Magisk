# 常见问题

## 服务启动失败

先查看两类日志：

```text
/data/adb/modules/netproxy/logs/service.log
/data/adb/modules/netproxy/logs/sing-box.log
```

再确认：

- `data/catalog/` 下至少有一个可用节点分组。
- 当前活动分组存在且包含节点。
- `config/ebpf/ebpf.conf` 的参数有效。
- sing-box 配置检查没有报错。

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl config validate'
```

## 订阅更新失败

确认订阅地址可以返回节点内容，并检查 `service.log` 中的订阅更新记录。核心停止时更新仍应可用；如果更新失败，旧 Provider 会继续保留，不会清空节点。

## 切换节点没有立即生效

同一分组内的选择优先通过 Service API 即时切换。跨分组或运行时 Provider 尚未加载时，模块会重新加载运行时配置。确认服务状态已经是 `ready`。

## 无法访问 zashboard

默认入口：

```text
http://127.0.0.1:9999/ui
```

确认服务已启动，并使用密钥 `singbox`。Clash API 主要供 zashboard 和第三方客户端使用，Android 管理器的核心状态通过 Service API 获取。

## 应用分身没有生效

分应用配置保存包名和 Android 用户范围，由 sing-box eBPF 入站在启动或 reload 时解析。不要填写 UID，也不要使用 `user:package` 格式。
