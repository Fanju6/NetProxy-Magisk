# ebpf.conf

eBPF 透明代理主配置位于：

```text
/data/adb/modules/netproxy/config/ebpf/ebpf.conf
```

服务启动或 reload 时，NetProxy 读取该文件并生成 `runtime/ebpf.json`。运行时文件是生成物，不应直接编辑。

## 基础设置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `EBPF_NETWORK` | 空 | 同时处理 TCP 与 UDP；也可设为 `tcp` 或 `udp` |
| `EBPF_UDP_TIMEOUT` | `5m` | UDP NAT 会话超时，通常不需要调整 |
| `EBPF_DNS_MODE` | `hijack` | 接管 TCP / UDP 53；`off` 表示放行 |
| `EBPF_CGROUP_ENABLED` | `1` | 是否接管本机应用流量 |
| `EBPF_CGROUP_PATH` | 空 | 留空时由 sing-box 自动发现 cgroup v2 |
| `EBPF_CGROUP_IPV6_MODE` | `always` | 本机 IPv6 接管策略：`always`、`auto`、`off` |
| `EBPF_BYPASS_PRIVATE_ADDRESS` | `1` | 是否在 eBPF 层绕过私网和特殊用途地址 |

`EBPF_CGROUP_ENABLED=0` 时只保留共享网络数据路径，运行时不会输出 cgroup、IPv6、应用名单或本机 Map 配置。

## 规则集提前绕过

```ini
EBPF_BYPASS_RULE_SETS="direct,ChinaIP"
```

多个规则集使用英文逗号分隔。只有能提取纯 IP CIDR 的规则集会生效；命中的连接在内核侧直接放行，不会进入 sing-box 路由。需要严格 Global 行为时清空该值并重启服务。

## 分应用代理

```ini
APP_PROXY_ENABLE=1
APP_PROXY_MODE="blacklist"
APP_ANDROID_USERS=""
PROXY_APPS_LIST=""
BYPASS_APPS_LIST=""
```

- `blacklist`：名单内应用绕过代理。
- `whitelist`：只有代理名单内应用进入代理。
- `APP_ANDROID_USERS` 留空表示全部 Android 用户。
- `PROXY_APPS_LIST` 与 `BYPASS_APPS_LIST` 都使用英文逗号分隔。

配置保存包名和用户范围，运行时直接写入 sing-box eBPF 入站的 `include_package`、`exclude_package` 和 `include_android_user`。不要填写 UID 或 `user:package`。

## 共享网络

```ini
EBPF_SHARED_NETWORK=0
EBPF_SHARED_INTERFACES="wlan2"
EBPF_SHARED_INCLUDE_SOURCE_CIDRS=""
EBPF_SHARED_EXCLUDE_SOURCE_CIDRS=""
EBPF_SHARED_INCLUDE_MAC_ADDRESSES=""
EBPF_SHARED_EXCLUDE_MAC_ADDRESSES=""
```

所有列表使用英文逗号分隔。共享网络的 TC 优先级由模块固定管理，不提供普通用户编辑入口。热点或 USB 接口暂时不存在时不会阻止核心启动，接口出现后 sing-box 会尝试挂载。

## Map 与高级参数

以下 Map 容量默认均为 `65536`，范围为 `1` 到 `1048576`：

- `EBPF_TCP_MAP_CAPACITY`
- `EBPF_UDP_MAP_CAPACITY`
- `EBPF_SOCKET_MAP_CAPACITY`
- `EBPF_SHARED_PROXY_MAP_CAPACITY`
- `EBPF_SHARED_BYPASS_MAP_CAPACITY`
- `EBPF_SHARED_FRAGMENT_MAP_CAPACITY`

这些参数由模块提供稳定默认值，Android 管理器不展示编辑入口。只有日志明确提示 Map 容量不足或特定内核需要调整时，才建议手动修改。

## 应用配置与诊断

修改后执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```

诊断 eBPF 能力、程序、Map 和 cgroup：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
```

默认输出中文诊断；排查上游内核时追加 `--raw` 查看 sing-box 原始结果。
