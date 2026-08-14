# eBPF 透明代理与分应用代理

NetProxy 8.0 使用 sing-box 的 eBPF 入站接管流量。eBPF 是 sing-box 的入站模式，不是独立核心，也不是单独的服务。它通过 cgroup eBPF 处理本机连接，通过 TC eBPF 处理热点或 USB 下游流量。

本版本使用 sing-box eBPF 入站，不维护 iptables、nftables 或策略路由，也没有 TPROXY / REDIRECT 回退。

## 本机流量

默认配置：

```text
EBPF_NETWORK=""             # TCP + UDP
EBPF_DNS_MODE="hijack"      # 接管 TCP / UDP 53
EBPF_CGROUP_ENABLED=1
EBPF_CGROUP_IPV6_MODE="always"
EBPF_BYPASS_PRIVATE_ADDRESS=1
```

`EBPF_NETWORK` 可以设为 `tcp` 或 `udp`。`EBPF_DNS_MODE=hijack` 会优先接管 TCP / UDP 53，`off` 则不由 eBPF 入站处理 DNS。

`EBPF_CGROUP_IPV6_MODE` 只控制本机 cgroup IPv6 接管，可设为 `always`、`auto` 或 `off`。共享网络路径的 IPv6 行为由 sing-box eBPF 入站配置中的地址范围决定。

## 分应用代理

分应用配置位于 `config/ebpf/ebpf.conf`：

- `APP_PROXY_ENABLE=1`：启用名单过滤。
- `APP_PROXY_MODE=blacklist`：名单内应用绕过代理。
- `APP_PROXY_MODE=whitelist`：只有名单内应用进入代理。
- `APP_ANDROID_USERS`：限制生效的 Android 用户，留空表示全部用户。
- `PROXY_APPS_LIST`：代理包名列表。
- `BYPASS_APPS_LIST`：绕过包名列表。

多个值统一使用英文逗号分隔。包名和用户范围直接传给 sing-box，运行时由 Android PackageManager 解析，不需要手动查 UID，也不要填写 `user:package`。

应用安装、重装、UID 变化或用户范围变化后，重新加载服务即可让 sing-box 重新解析名单。

## 规则集提前绕过

```text
EBPF_BYPASS_RULE_SETS="direct,ChinaIP"
```

该配置只接受能够提取纯 IP CIDR 的规则集。命中的连接会在内核侧直接放行，不会进入 sing-box 路由，因此也不会经过 Global 模式或普通路由规则。需要严格 Global 测试时清空该项后重启服务。

## 热点与共享网络

```text
EBPF_SHARED_NETWORK=0
EBPF_SHARED_INTERFACES="wlan2"
EBPF_SHARED_INCLUDE_SOURCE_CIDRS=""
EBPF_SHARED_EXCLUDE_SOURCE_CIDRS=""
EBPF_SHARED_INCLUDE_MAC_ADDRESSES=""
EBPF_SHARED_EXCLUDE_MAC_ADDRESSES=""
```

启用后，sing-box 会向指定下游接口挂载 TC eBPF。热点接口暂时不存在不会阻止核心启动，热点开启后会自动尝试挂载。不同设备的热点和 USB 接口名称可能不同，必须填写实际接口。

共享网络的 TC 优先级由模块固定管理，普通用户不需要配置。

## 内核要求与诊断

本版本至少需要：

- BPF 与 cgroup v2
- cgroup socket address / sock create 等挂载能力
- Root 及所需 BPF 权限
- 共享网络场景所需的 TC eBPF 能力

模块内置 Android arm64 `bpftool`，可以查看内核能力、程序、Map 和 cgroup 状态：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
```

`ebpf status` 默认输出中文能力诊断；只有排查上游内核问题时才使用 `--raw` 查看 sing-box 原始输出。

停止服务时由 sing-box 清理 eBPF 程序、Map 和 TC 挂载。若进程无法在超时时间内退出，生命周期控制器才会执行强制回收。
