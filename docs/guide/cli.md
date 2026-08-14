# CLI 使用

NetProxy 的公共命令入口只有 `netproxyctl`：

```text
/data/adb/modules/netproxy/netproxyctl
```

通过 Root 调用：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl help'
```

所有机器输出都是 `schema=1` JSON，stdout 只输出一份结果，日志和诊断写入 stderr。命令失败时请同时检查退出码、`code` 和 `message`，不要用文本内容猜测成功或失败。

## 命令总览

```text
netproxyctl [--json] [--timeout <秒|时长>] service status|start|stop|restart|reload|check|toggle
netproxyctl [--json] [--timeout <秒|时长>] catalog list|show <分组>
netproxyctl [--json] [--timeout <秒|时长>] node list|current|show|get|export|delay|add|import|edit|remove|use
netproxyctl [--json] [--timeout <秒|时长>] sub list|show|add|edit|update|update-all|activate|remove|history|cancel
netproxyctl [--json] [--timeout <秒|时长>] mode [rule|global|direct|AllowAds]
netproxyctl [--json] [--timeout <秒|时长>] network evaluate --type <wifi|not_wifi> [--ssid <名称>]
netproxyctl [--json] [--timeout <秒|时长>] app list|mode|users|add|remove|enable|disable
netproxyctl [--json] [--timeout <秒|时长>] ebpf status [configured|all|local|shared] [--raw]
netproxyctl [--json] [--timeout <秒|时长>] config list|read|check|validate|apply
netproxyctl [--json] [--timeout <秒|时长>] logs show|clear|export
```

默认命令超时为 30 秒，`service start` 为 120 秒。订阅增删改使用订阅自身的下载超时；需要时可以显式传入 `--timeout 5m`。

## service

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl service start'
su -c '/data/adb/modules/netproxy/netproxyctl service stop'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
su -c '/data/adb/modules/netproxy/netproxyctl service reload'
```

只有 `ready` 表示 sing-box Service API 与 eBPF 入站均已就绪。`service status` 中的 `outbound_mode` 是核心当前实际模式；基础配置值由 `configured_outbound_mode` 表示。

## node

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl node current'
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/nodes.yaml'
su -c '/data/adb/modules/netproxy/netproxyctl node use auto default'
su -c '/data/adb/modules/netproxy/netproxyctl node use default/<节点标签>'
su -c '/data/adb/modules/netproxy/netproxyctl node delay auto default'
su -c '/data/adb/modules/netproxy/netproxyctl node export default/<节点标签>'
su -c '/data/adb/modules/netproxy/netproxyctl node edit default/<节点标签> /sdcard/node.json'
su -c '/data/adb/modules/netproxy/netproxyctl node remove default/<节点标签>'
```

节点引用固定为 `<group-id>/<tag>`。本地链接和文件导入追加到 `default`；自动模式使用 `Auto/<group>`，不会保存某个 URLTest 结果为手动节点。

## sub

```sh
su -c '/data/adb/modules/netproxy/netproxyctl sub add 我的订阅 https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub add https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub list'
su -c '/data/adb/modules/netproxy/netproxyctl sub show <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub update-all'
su -c '/data/adb/modules/netproxy/netproxyctl sub history <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub cancel <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub remove <分组 ID>'
```

名称可以省略，NetProxy 会按 `Profile-Title`、响应文件名、URL 主机名的顺序自动取名。订阅更新不依赖 sing-box 正在运行；服务运行时会在提交后同步 Local Provider，失败则保留持久化节点并返回结构化运行时状态。

需要修改 URL、请求头、过滤条件或更新周期时使用 `sub edit`。自定义请求头应通过 `--headers-file` 传入 JSON 文件，不要把 token 直接写在命令行中。

## mode、app 与 network

```sh
su -c '/data/adb/modules/netproxy/netproxyctl mode'
su -c '/data/adb/modules/netproxy/netproxyctl mode rule'
su -c '/data/adb/modules/netproxy/netproxyctl mode global'
su -c '/data/adb/modules/netproxy/netproxyctl mode direct'
su -c '/data/adb/modules/netproxy/netproxyctl mode AllowAds'

su -c '/data/adb/modules/netproxy/netproxyctl app list'
su -c '/data/adb/modules/netproxy/netproxyctl app mode whitelist'
su -c '/data/adb/modules/netproxy/netproxyctl app users 0,999'
su -c '/data/adb/modules/netproxy/netproxyctl app add com.example.app'
su -c '/data/adb/modules/netproxy/netproxyctl app enable'

su -c '/data/adb/modules/netproxy/netproxyctl network evaluate --type wifi --ssid "Home WiFi"'
```

分应用配置保存包名和 Android 用户范围，不转换为 UID。Wi-Fi 自动切换由后台 Worker 读取当前实际网络并评估，基础 `OUTBOUND_MODE` 不会被覆盖。

## ebpf、config 与 logs

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured --raw'

su -c '/data/adb/modules/netproxy/netproxyctl config list'
su -c '/data/adb/modules/netproxy/netproxyctl config read 01_log.json'
su -c '/data/adb/modules/netproxy/netproxyctl config validate 03_dns.json /sdcard/candidate.json'

su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs export /sdcard/Download/netproxy-diagnostics.tar.gz'
```

`ebpf status` 默认返回面向用户的能力诊断，`--raw` 才返回 sing-box 原始探测输出。日志导出包会包含运行时配置摘要并脱敏订阅地址、凭据、UUID、Header 和 HWID。
