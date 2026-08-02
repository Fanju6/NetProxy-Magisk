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
| `EBPF_CGROUP_PATH` | 空 | 留空时由 sing-box 自动发现 cgroup v2 路径 |
| `EBPF_IPV6` | `1` | 是否启用 IPv6 透明代理 |

## 内核提前绕过

`EBPF_BYPASS_RULE_SETS="direct ChinaIP"` 会从可用规则集中提取纯 IP CIDR，并在 eBPF 层直接放行。被提前绕过的连接不会进入 sing-box 路由；如需严格 Global 模式，请将该值设为空。

## 分应用代理

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `APP_PROXY_ENABLE` | `1` | 是否按名单过滤应用 |
| `APP_PROXY_MODE` | `blacklist` | `blacklist` 绕过名单，`whitelist` 仅代理名单 |
| `PROXY_APPS_LIST` | 空 | 白名单包名，空格分隔 |
| `BYPASS_APPS_LIST` | 空 | 黑名单包名，空格分隔 |

包名支持 `user:package` 格式。空白白名单不会退化为代理全部应用。

## 共享网络

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `EBPF_SHARED_NETWORK` | `0` | 是否启用热点或 USB 下游代理 |
| `EBPF_SHARED_INTERFACES` | `wlan2` | 下游接口名，多个值使用空格分隔 |

## Map 容量

以下值范围为 `1` 到 `1048576`，默认均为 `65536`：

- `EBPF_TCP_MAP_CAPACITY`
- `EBPF_UDP_MAP_CAPACITY`
- `EBPF_SOCKET_MAP_CAPACITY`
- `EBPF_SHARED_MAP_CAPACITY`

## 应用配置

修改 eBPF 配置后可执行：

```sh
su -c '/data/adb/modules/netproxy/scripts/cli ebpf reload'
```

该命令会重启 sing-box，以卸载旧 eBPF 实例并应用新配置。
