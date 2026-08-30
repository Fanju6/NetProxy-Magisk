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
EBPF_BYPASS_RULE_SET="direct,cn-ip"
```

`EBPF_MODE` 支持 `local`（本机 cgroup）、`shared`（下游接口 TC）和 `hybrid`（两者同时启用）。`EBPF_NETWORK` 留空表示 TCP 和 UDP，也可以填写 `tcp`、`udp` 或 `tcp,udp`。

## local

```ini
EBPF_LOCAL_DNS_MODE="hijack"
EBPF_LOCAL_CGROUP_PATH=""
EBPF_LOCAL_IPV6=1
EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS=1
EBPF_LOCAL_INCLUDE_UID=""
EBPF_LOCAL_INCLUDE_UID_RANGE=""
EBPF_LOCAL_EXCLUDE_UID=""
EBPF_LOCAL_EXCLUDE_UID_RANGE=""
EBPF_LOCAL_INCLUDE_ANDROID_USER=""
EBPF_LOCAL_INCLUDE_PACKAGE=""
EBPF_LOCAL_EXCLUDE_PACKAGE=""
```

`EBPF_LOCAL_DNS_MODE` 支持 `hijack`、`respect_policy` 和 `off`。`hijack` 会优先接管所有可见的 53 端口流量；`respect_policy` 只接管 UID、包名筛选范围内的 DNS，但不会让已选中的 DNS 再按目标私网或规则集绕过。`EBPF_LOCAL_IPV6=0` 会让原生 IPv6 绕过该入站，运行期间不会再根据默认路由自动切换。`EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS` 控制本机数据路径是否在进入 sing-box 前绕过私网与特殊用途地址。`EBPF_LOCAL_INCLUDE_PACKAGE` 和 `EBPF_LOCAL_EXCLUDE_PACKAGE` 是高级用户直接填写的包名列表。分应用页面使用下方的带用户范围格式，Native 会调用 Android `cmd package list packages --user <用户ID> -U` 解析 UID。

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
EBPF_SHARED_TC_PRIORITY=1
```

`EBPF_SHARED_DNS_MODE` 与 local 独立，支持 `hijack`、`respect_policy` 和 `off`；`respect_policy` 按来源 CIDR/MAC 筛选 DNS，已选中的 DNS 不再按目标私网或规则集绕过。仅启用 TCP 时也可以使用 DNS 劫持。`EBPF_SHARED_IPV6=0` 只关闭共享路径的 IPv6 接管，不会阻止系统自行转发 IPv6。`EBPF_SHARED_BYPASS_PRIVATE_ADDRESS` 独立控制共享网络数据路径的私网与特殊用途地址绕过。`shared` 和 `hybrid` 模式必须填写至少一个实际的下游接口。接口可以在服务启动后出现或消失，sing-box 会自动维护 TC 挂载。

共享网络固定使用 TC 目标令牌重写数据面，不再提供数据面、数据包 mark、策略路由表或状态容量选项。来源 CIDR、MAC 地址和 TC 优先级仍可用于筛选客户端或协调已有 TC 规则。

## 能力探测

模块不再携带 `bpftool`。能力探测直接使用 sing-box 内置命令，不挂载程序、不修改实际流量：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all --raw'
```

local UDP 需要 Linux 5.2 及相应 cgroup hook；Android 主要验证目标为 GKI 5.10 及以上。Linux 6.6.0 至 6.6.46 在使用 UID、包名、CIDR 筛选时存在 LPM trie 崩溃风险，应升级到 6.6.47+ 或使用包含修复的厂商内核。

## 校验与应用

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```
