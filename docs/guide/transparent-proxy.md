# eBPF 透明代理与分应用代理

NetProxy 使用 sing-box 的 eBPF 入站接管本机和共享网络流量。eBPF 是 sing-box 入站模式，不是独立代理核心，也不是单独的服务。本版本不提供旧的 TPROXY、REDIRECT 或模块内 `bpftool` 回退。

## 配置模式

```ini
EBPF_MODE="local"       # local、shared 或 hybrid
EBPF_NETWORK=""         # 空值表示 TCP + UDP
EBPF_DNS_MODE="hijack"  # hijack、respect_bypass 或 off
EBPF_LOCAL_IPV6_MODE="auto"
EBPF_SHARED_INTERFACES="wlan2"
```

`local` 通过 cgroup socket hook 接管本机连接，`shared` 通过 TC 接管热点或 LAN 下游接口，`hybrid` 同时启用两条路径。`shared` 和 `hybrid` 必须使用实际存在的下游接口。共享接口出现、消失或 TC filter 被移除时，sing-box 会自动检查并重新挂载。

## 分应用代理

分应用配置位于 `config/ebpf/ebpf.conf`：

```ini
APP_PROXY_ENABLE=1
APP_PROXY_MODE="blacklist"
PROXY_APPS_LIST="0:com.example.app,10:com.example.app"
BYPASS_APPS_LIST=""
```

每个应用项都必须使用 `<用户ID>:<包名>`。因此用户 0 和用户 10 的同一个包会在 Android 页面显示为两个独立条目，也可以分别切换。Native 通过 Android package service 查询指定用户的 UID，并把结果写入 sing-box 的 `include_uid` 或 `exclude_uid`；白名单始终包含 UID 0。

包名策略只保证目标 UID 直接创建的 socket。系统 DNS resolver、DownloadManager、isolated process 和 SDK sandbox 代发的流量可能属于其他 UID。

## 内核能力探测

厂商内核可能单独禁用或回移 BPF 能力，不能只依赖内核版本。使用 sing-box 内置的纯 Go 探测：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all --raw'
```

探测只创建并关闭临时对象，不挂载程序，不修改 cgroup、qdisc、路由、sysctl 或实际流量。local UDP 需要 Linux 5.2 的 cgroup UDP hook；Android 主要验证目标为 GKI 5.10 及以上。Linux 6.6.0 至 6.6.46 使用 UID、包名或 CIDR 筛选存在 LPM trie 风险，应升级到 6.6.47+。

停止服务时由 sing-box 清理 eBPF 程序、Map 和 TC 挂载。
