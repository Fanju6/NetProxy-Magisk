# ebpf.conf

eBPF 透明代理主配置位于：

```text
/data/adb/modules/netproxy/config/ebpf/ebpf.conf
```

服务启动时，`runtime.sh` 读取该文件并生成 `config/singbox/runtime/ebpf.json`。运行时文件会在停止服务后清理，不应直接编辑。

## 基础设置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `EBPF_NETWORK` | 空 | 同时处理 TCP 与 UDP；也可设为 `tcp` 或 `udp` |
| `EBPF_UDP_TIMEOUT` | `5m` | UDP NAT 会话超时 |
| `EBPF_DNS_MODE` | `hijack` | `hijack` 接管 TCP / UDP 53，`off` 放行 |
| `EBPF_CGROUP_ENABLED` | `1` | 是否通过 cgroup 接管本机应用流量 |
| `EBPF_CGROUP_PATH` | 空 | 留空时由 sing-box 自动发现 cgroup v2 路径 |
| `EBPF_IPV6_MODE` | `auto` | IPv6 接管策略：`disabled` 关闭、`auto` 自动判断、`always` 始终接管、`shared` 仅接管共享网络 |

## 内核提前绕过

`EBPF_BYPASS_RULE_SETS="direct ChinaIP"` 会从可用规则集中提取纯 IP CIDR，并在 eBPF 层直接放行。被提前绕过的连接不会进入 sing-box 路由；如需严格 Global 模式，请将该值设为空。

## 分应用代理

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `APP_PROXY_ENABLE` | `1` | 是否按名单过滤应用 |
| `APP_PROXY_MODE` | `blacklist` | `blacklist` 绕过名单，`whitelist` 仅代理名单 |
| `APP_ANDROID_USERS` | 空 | 生效的 Android 用户 ID；留空表示全部用户 |
| `PROXY_APPS_LIST` | 空 | 白名单包名，空格分隔 |
| `BYPASS_APPS_LIST` | 空 | 黑名单包名，空格分隔 |

包名由 sing-box 在入站启动时通过 Android PackageManager 解析；安装、重装应用或改变用户范围后需重新加载服务。空白白名单不会退化为代理全部应用。

## 共享网络

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `EBPF_SHARED_NETWORK` | `0` | 是否启用热点或 USB 下游代理 |
| `EBPF_SHARED_INTERFACES` | `wlan2` | 下游接口名，多个值使用空格分隔 |
| `EBPF_SHARED_INCLUDE_SOURCE_CIDRS` | 空 | 只接管指定来源网段 |
| `EBPF_SHARED_EXCLUDE_SOURCE_CIDRS` | 空 | 绕过指定来源网段 |
| `EBPF_SHARED_TC_PRIORITY` | `1` | TC filter 优先级，Android 建议保持 `1` |

## Map 容量

以下值范围为 `1` 到 `1048576`，默认均为 `65536`：

- `EBPF_TCP_MAP_CAPACITY`
- `EBPF_UDP_MAP_CAPACITY`
- `EBPF_SOCKET_MAP_CAPACITY`
- `EBPF_SHARED_MAP_CAPACITY`

## 应用配置

修改 eBPF 配置后可执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service reload'
```

该命令会重启 sing-box，以卸载旧 eBPF 实例并应用新配置。
