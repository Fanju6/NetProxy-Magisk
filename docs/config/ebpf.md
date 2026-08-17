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
EBPF_DNS_MODE="hijack"
EBPF_BYPASS_PRIVATE_ADDRESS=1
EBPF_BYPASS_RULE_SET="direct,ChinaIP"
```

`EBPF_MODE` 支持 `local`（本机 cgroup）、`shared`（下游接口 TC）和 `hybrid`（两者同时启用）。`EBPF_NETWORK` 留空表示 TCP 和 UDP，也可以填写 `tcp`、`udp` 或 `tcp,udp`。`EBPF_DNS_MODE` 支持 `hijack`、`respect_bypass` 和 `off`。共享或 hybrid 模式启用 DNS 劫持时必须包含 UDP。

## local

```ini
EBPF_LOCAL_CGROUP_PATH=""
EBPF_LOCAL_IPV6_MODE="auto"
EBPF_LOCAL_INCLUDE_UID=""
EBPF_LOCAL_INCLUDE_UID_RANGE=""
EBPF_LOCAL_EXCLUDE_UID=""
EBPF_LOCAL_EXCLUDE_UID_RANGE=""
EBPF_LOCAL_INCLUDE_ANDROID_USER=""
EBPF_LOCAL_INCLUDE_PACKAGE=""
EBPF_LOCAL_EXCLUDE_PACKAGE=""
EBPF_LOCAL_STATE_CAPACITY=0
```

`EBPF_LOCAL_INCLUDE_PACKAGE` 和 `EBPF_LOCAL_EXCLUDE_PACKAGE` 是高级用户直接填写的包名列表。分应用页面使用下方的带用户范围格式，Native 会调用 Android `cmd package list packages --user <用户ID> -U` 解析 UID。

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
EBPF_SHARED_INTERFACES="wlan2"
EBPF_SHARED_IPV6_MODE="always"
EBPF_SHARED_INCLUDE_SOURCE_CIDR=""
EBPF_SHARED_EXCLUDE_SOURCE_CIDR=""
EBPF_SHARED_INCLUDE_MAC_ADDRESS=""
EBPF_SHARED_EXCLUDE_MAC_ADDRESS=""
EBPF_SHARED_STATE_CAPACITY=0
EBPF_SHARED_TC_PRIORITY=1
```

`shared` 和 `hybrid` 模式必须填写至少一个实际的下游接口。接口可以在服务启动后出现或消失，sing-box 会自动维护 TC 挂载。来源 CIDR、MAC 地址、状态容量和 TC 优先级属于高级配置，Android 管理器不要求展示全部字段。

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
