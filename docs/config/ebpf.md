# ebpf.conf

eBPF 透明代理配置位于：

```text
/data/adb/modules/netproxy/config/ebpf/ebpf.conf
```

服务启动或 reload 时，Native 读取该文件并生成 `runtime/ebpf.json`。运行时文件是生成物，不应直接编辑。所有新 eBPF 字段都可以在 `ebpf.conf` 中手动设置，Android 管理器只展示常用选项。

## 数据路径

```ini
EBPF_MODE="local"
EBPF_NETWORK=""
EBPF_UDP_TIMEOUT="5m"
EBPF_TC_PRIORITY=1
EBPF_BYPASS_RULE_SET="direct,cn-ip"
```

`EBPF_MODE` 支持 `local`（当前默认出口接口）、`shared`（下游接口）和 `hybrid`（两者同时启用）。两条数据路径均由 sing-box 使用 TC 管理。`EBPF_NETWORK` 留空表示 TCP 和 UDP，也可以填写 `tcp`、`udp` 或 `tcp,udp`。`EBPF_TC_PRIORITY` 用于与同一接口上的其他 TC filter 协调顺序，通常保持默认值。

## local

```ini
EBPF_LOCAL_DNS_MODE="hijack"
EBPF_LOCAL_IPV6=1
EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS=1
EBPF_LOCAL_INCLUDE_UID=""
EBPF_LOCAL_INCLUDE_UID_RANGE=""
EBPF_LOCAL_EXCLUDE_UID=""
EBPF_LOCAL_EXCLUDE_UID_RANGE=""
EBPF_LOCAL_INCLUDE_ANDROID_USER=""
EBPF_LOCAL_INCLUDE_PACKAGE=""
EBPF_LOCAL_EXCLUDE_PACKAGE=""
EBPF_LOCAL_BYPASS_PORT=""
EBPF_LOCAL_BYPASS_PORT_RANGE=""
```

`EBPF_LOCAL_DNS_MODE` 支持 `hijack`、`respect_policy` 和 `off`。`hijack` 会优先接管所有可见的 53 端口流量；`respect_policy` 会先执行 UID、包名和目标地址策略。`EBPF_LOCAL_IPV6=0` 会让本机 IPv6 绕过该入站。local attachment 会跟随系统默认出口变化，切换期间由 sing-box 维护挂载状态。`EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS` 控制本机数据路径是否在进入 sing-box 前绕过私网与特殊用途地址。

`EBPF_LOCAL_BYPASS_PORT` 填写单个目标端口，`EBPF_LOCAL_BYPASS_PORT_RANGE` 使用 `start:end`；多个值都使用英文逗号分隔。FakeIP 和 DNS 接管优先于端口绕过。`EBPF_LOCAL_INCLUDE_PACKAGE` 和 `EBPF_LOCAL_EXCLUDE_PACKAGE` 是高级用户直接填写的包名列表。分应用页面使用下方的带用户范围格式，Native 会调用 Android `cmd package list packages --user <用户ID> -U` 解析 UID。

## 分应用代理

```ini
APP_PROXY_ENABLE=1
APP_PROXY_MODE="blacklist"
PROXY_APPS_LIST=""
BYPASS_APPS_LIST="0:com.example.app,10:com.example.app"
```

列表使用英文逗号分隔，每项必须是严格的 `<Android用户ID>:<包名>`。同一个包在不同用户下是两个独立条目。`blacklist` 中的 `BYPASS_APPS_LIST` 绕过代理；`whitelist` 中只有 `PROXY_APPS_LIST` 进入代理，Native 会自动加入 UID 0。

包名变更、应用重装或用户范围变化后，重启或 reload 服务会重新解析 UID。包名解析失败时配置事务失败，不会留下半应用配置。

## shared

```ini
EBPF_SHARED_DNS_MODE="hijack"
EBPF_SHARED_INTERFACES="wlan2"
EBPF_SHARED_IPV6=1
EBPF_SHARED_BYPASS_PRIVATE_ADDRESS=1
EBPF_SHARED_INCLUDE_SOURCE_CIDR=""
EBPF_SHARED_EXCLUDE_SOURCE_CIDR=""
EBPF_SHARED_INCLUDE_MAC_ADDRESS=""
EBPF_SHARED_EXCLUDE_MAC_ADDRESS=""
EBPF_SHARED_BYPASS_PORT=""
EBPF_SHARED_BYPASS_PORT_RANGE=""
```

`EBPF_SHARED_DNS_MODE` 与 local 独立，支持 `hijack`、`respect_policy` 和 `off`；`respect_policy` 会先执行来源 CIDR、MAC 与目标地址策略。仅启用 TCP 时也可以使用 DNS 劫持。`EBPF_SHARED_IPV6=0` 只关闭共享路径的 IPv6 接管，不会阻止系统自行转发 IPv6。`EBPF_SHARED_BYPASS_PRIVATE_ADDRESS` 独立控制共享网络数据路径的私网与特殊用途地址绕过。`shared` 和 `hybrid` 模式必须填写至少一个实际的下游接口。接口可以在服务启动后出现或消失，sing-box 会自动维护 TC 挂载；当该接口成为默认上游时会暂停 shared 接管。

`EBPF_SHARED_BYPASS_PORT` 与 `EBPF_SHARED_BYPASS_PORT_RANGE` 的格式和 local 相同，仅作用于共享网络数据路径。共享网络不会创建热点、DHCP、NAT、IPv6 RA 或 IP 转发，这些仍由 Android 或系统网络栈负责。

## 能力探测

模块不再携带 `bpftool`。能力探测直接使用 sing-box 内置命令，不挂载程序、不修改实际流量：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all --raw'
```

local 和 shared 都要求 TC classifier、透明 socket、socket lookup 与 `bpf_sk_assign` 等能力；local 还需要可用的 veth 与策略路由。厂商内核可能单独禁用或回移这些能力，兼容性以 sing-box 的运行时探测为准，不按内核版本猜测。cgroup socket hook 只用于可选的自身绕过与进程追踪优化，缺失时会回退用户态登记路径。

## 校验与应用

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```
