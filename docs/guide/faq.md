# 常见问题

## 服务启动失败

先查看两类日志：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```

再确认：

- `data/catalog/` 下有至少一个包含节点的分组。
- `ACTIVE_GROUP_ID` 指向有效分组。
- `config/ebpf/ebpf.conf` 没有无效的 eBPF 参数。
- 内核具备 BPF、cgroup v2 和所需 Root 权限。

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
```

## 订阅更新失败

订阅更新不要求 sing-box 正在运行。确认 URL 能返回节点内容、TLS 校验没有被错误关闭，并检查 `service.log` 的结构化错误。

更新失败时旧 Provider 会继续保留；如果显示“已持久化但运行时未同步”，重启或 reload 服务会重试运行时应用，不需要重新添加订阅。

## 切换节点没有立即生效

同组切换优先通过 Service API 即时完成。跨分组切换、新增分组或 Provider 尚未加载时会受控 reload。确认服务状态已进入 `ready`，再查看 Service API Dashboard 的实际选择。

## Global 仍有国内直连

eBPF 的 `EBPF_BYPASS_RULE_SETS` 会在进入 sing-box 前提前放行 IP CIDR。进行严格 Global 测试前清空该配置并重启服务：

```text
EBPF_BYPASS_RULE_SETS=""
```

同时检查应用名单、私网绕过和路由规则。

## 无法访问面板

- zashboard：`http://127.0.0.1:9999/ui/`
- sing-box Dashboard：`http://127.0.0.1:9090/dashboard/`

两个地址都要求服务已启动，并使用密钥 `singbox`。API 默认只监听本机，不应直接从其他设备访问。

## 应用分身没有生效

分应用配置保存包名和 Android 用户范围，由 sing-box eBPF 入站在启动或 reload 时解析。不要填写 UID，也不要使用 `user:package` 格式；用户范围和包名列表使用英文逗号分隔。

## 如何提交诊断信息

不要直接上传包含节点凭据的原始目录。使用模块导出脱敏诊断包：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs export /sdcard/Download/netproxy-diagnostics.tar.gz'
```

诊断包包含管理器版本、模块版本、运行时配置摘要和脱敏日志，不包含 Catalog 节点凭据。
