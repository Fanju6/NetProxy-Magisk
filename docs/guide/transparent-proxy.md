# eBPF 透明代理与分应用代理

NetProxy 使用 sing-box 的实验性 eBPF 入站接管流量。它通过 cgroup eBPF 处理本机连接，通过 TC eBPF 处理热点或 USB 下游流量，不创建 TUN 设备，也不维护 iptables、nftables 或策略路由规则。

## 本机流量

默认同时接管 TCP 与 UDP，并在 eBPF 入站优先劫持 TCP / UDP 53：

```text
EBPF_NETWORK=""
EBPF_DNS_MODE="hijack"
EBPF_CGROUP_ENABLED=1
EBPF_CGROUP_IPV6_MODE="always"
EBPF_BYPASS_PRIVATE_ADDRESS=1
```

`EBPF_NETWORK` 可设为 `tcp` 或 `udp`，留空表示两者都处理。DNS 劫持依赖 UDP；将网络限制为 `tcp` 时不应同时用于需要 UDP DNS 的共享网络。

`EBPF_CGROUP_IPV6_MODE` 只控制本机 cgroup 的 IPv6 接管，可设为 `always`、`auto` 或 `off`。共享网络的 IPv6 是否启用由 `redirect_address` 的 IPv6 前缀决定，不能使用旧的“仅共享网络”模式表达。

`EBPF_BYPASS_PRIVATE_ADDRESS=1` 时，私网和特殊用途地址会在 eBPF 层绕过，不进入 sing-box 路由；需要严格 Global 行为时应设为 `0`。

## 分应用代理

分应用配置位于 `config/ebpf/ebpf.conf`：

- `APP_PROXY_ENABLE=1`：按应用名单过滤
- `APP_PROXY_MODE="blacklist"`：名单内应用绕过
- `APP_PROXY_MODE="whitelist"`：仅名单内应用进入代理
- `APP_ANDROID_USERS`：限制策略生效的 Android 用户，留空表示全部用户
- `PROXY_APPS_LIST`：白名单应用
- `BYPASS_APPS_LIST`：黑名单应用

模块把包名和 Android 用户范围直接写入 eBPF 入站，sing-box 在启动时通过 Android PackageManager 解析对应 UID。应用安装、重装、UID 变化或新增 Android 用户后，需要重新加载服务。

## 规则集提前绕过

默认值：

```text
EBPF_BYPASS_RULE_SETS="direct ChinaIP"
```

命中的 IP CIDR 会在内核侧直接放行，不再进入 sing-box。这样可以降低常见直连流量开销，但也会绕过 Global 模式。需要严格全局代理时应清空该项并重启服务。

## 热点与共享网络

```text
EBPF_SHARED_NETWORK=0
EBPF_SHARED_INTERFACES="wlan2"
EBPF_SHARED_INCLUDE_SOURCE_CIDRS=""
EBPF_SHARED_EXCLUDE_SOURCE_CIDRS=""
EBPF_SHARED_INCLUDE_MAC_ADDRESSES=""
EBPF_SHARED_EXCLUDE_MAC_ADDRESSES=""
```

启用后，sing-box 会向指定下游接口挂载 TC eBPF。接口暂时不存在不会阻止核心启动，热点开启后会自动尝试挂载。不同 ROM 的热点或 USB 接口名可能不同，必须填写实际接收下游流量的接口。

## Map 容量

TCP、UDP、套接字绕过 Map，以及共享网络的代理、绕过、分片 Map 默认容量均为 `65536`；只有在日志明确提示容量不足时才需要调整。

## 内核要求

本版本没有 TPROXY / REDIRECT 回退。内核至少需要：

- BPF 与 cgroup v2
- cgroup socket address / sock create 等挂载能力
- Root 及所需 BPF 权限
- 共享网络场景所需的 TC eBPF 能力

停止服务时会先发送 SIGTERM，让 sing-box 清理 eBPF 程序、Map 与 TC 挂载。若进程无法在超时时间内退出才会使用 SIGKILL。
