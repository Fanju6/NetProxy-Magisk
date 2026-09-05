# ebpf.conf

eBPF 透明代理配置位于：

```text
/data/adb/modules/netproxy/config/ebpf/ebpf.conf
```

服务启动或 reload 时，Native 读取该文件并生成 `runtime/ebpf.json`。运行时文件是生成物，不应直接编辑。所有 eBPF 字段都可以在 `ebpf.conf` 中手动设置，Android 管理器只展示常用选项。

## 基础设置

```ini
EBPF_NETWORK=""
EBPF_UDP_TIMEOUT="5m"
EBPF_TC_PRIORITY=1
EBPF_BYPASS_RULE_SET="geoip/cn"
```

`EBPF_NETWORK` 留空表示 TCP 和 UDP，也可以填写 `tcp`、`udp` 或 `tcp,udp`。`EBPF_TC_PRIORITY` 用于协调 TC 数据平面与同一接口上的其他 filter，通常保持默认值。

## 数据路径

本机与共享网络使用独立开关，不再使用统一的 mode 字段：

```ini
EBPF_LOCAL_ENABLED=1
EBPF_SHARED_ENABLED=0
```

两者至少启用一项。两项都开启时会同时接管本机应用和热点下游流量。

## 本机网络

```ini
EBPF_LOCAL_ENABLED=1
EBPF_LOCAL_DATA_PLANE="cgroup"
EBPF_LOCAL_CGROUP_PATH=""
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

`EBPF_LOCAL_DATA_PLANE` 支持：

- `cgroup`：默认值，在 socket 阶段接管本机 TCP/UDP，不依赖默认出口接口。需要 cgroup v2 和对应的 cgroup socket hook。
- `tc`：在当前默认出口接口上处理本机流量，适合无法使用 cgroup 数据平面的设备。

`EBPF_LOCAL_CGROUP_PATH` 只适用于 `cgroup`。留空时使用 sing-box 默认的 cgroup 层级；手动填写时必须是绝对路径。

`EBPF_LOCAL_DNS_MODE` 支持 `hijack`、`respect_policy` 和 `off`。`hijack` 会优先接管所有可见的 53 端口流量；`respect_policy` 会先执行 UID、包名和目标地址策略。`EBPF_LOCAL_IPV6=0` 会让本机 IPv6 绕过该入站。

`EBPF_LOCAL_BYPASS_PORT` 填写单个目标端口，`EBPF_LOCAL_BYPASS_PORT_RANGE` 使用 `start:end`；多个值都使用英文逗号分隔。`EBPF_LOCAL_INCLUDE_PACKAGE` 和 `EBPF_LOCAL_EXCLUDE_PACKAGE` 供高级用户直接填写包名。分应用页面使用下方的带用户范围格式，Native 会调用 Android package service 解析 UID。

## 分应用代理

```ini
APP_PROXY_ENABLE=1
APP_PROXY_MODE="blacklist"
PROXY_APPS_LIST=""
BYPASS_APPS_LIST="0:com.example.app,10:com.example.app"
```

列表使用英文逗号分隔，每项必须是严格的 `<Android用户ID>:<包名>`。同一个包在不同用户下是两个独立条目。

- `blacklist`：`BYPASS_APPS_LIST` 中的应用绕过代理。
- `whitelist`：只有 `PROXY_APPS_LIST` 中的应用进入代理，Native 会自动加入 UID 0。

包名变更、应用重装或用户范围变化后，重启核心会重新解析 UID。解析失败时配置事务失败，不会留下半应用配置。

## 共享网络

```ini
EBPF_SHARED_ENABLED=0
EBPF_SHARED_DATA_PLANE="packet_rewrite"
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

启用共享网络时，`EBPF_SHARED_INTERFACES` 必须填写至少一个真实下游接口。接口可以在服务启动后出现或消失，sing-box 会检查并恢复相应 attachment；当该接口成为默认上游时会暂停共享网络接管。

`EBPF_SHARED_DATA_PLANE` 支持：

- `packet_rewrite`：默认值，适用于 Android 热点和普通以太网接口。
- `socket_assign`：适用于 raw-IP、PPP 或隧道类接口。

`EBPF_SHARED_DNS_MODE`、IPv6、私网绕过和目标端口设置只作用于共享网络。来源 CIDR 与 MAC 地址可进一步限制哪些下游设备进入代理。共享网络不会创建热点、DHCP、NAT、IPv6 RA 或 IP 转发，这些仍由 Android 或系统网络栈负责。

## 能力探测

模块直接使用 sing-box 内置命令探测实际数据平面能力，不携带 `bpftool`：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all --raw'
```

`configured` 会按照当前启用路径及其 `data_plane` 检查。探测不会挂载程序或修改实际流量。厂商内核可能关闭或回移单项 BPF 能力，因此应以实际探测和启动结果为准，不能只根据内核版本判断。

## 校验与应用

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```
