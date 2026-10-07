# CLI

公共命令入口固定为：

```text
/data/adb/modules/netproxy/netproxyctl
```

它不在系统 `PATH` 中，设备命令应使用完整路径和 Root：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl help'
```

## 输出契约

公共命令返回 `schema=1` JSON。stdout 只包含一份结果，运行日志和诊断写 stderr。脚本应同时检查进程退出码、`ok`、`code` 和 `schema`，不要从中文 `message` 猜测机器状态。

```json
{
  "schema": 1,
  "ok": true,
  "code": "service.status",
  "message": "服务状态",
  "data": {}
}
```

默认命令超时为 30 秒，`service start` 默认为 120 秒。可使用 `--timeout 5m` 显式覆盖。

## 命令组

```text
service  catalog  node  sub  mode
network  app      ebpf  config  logs
```

以设备上当前二进制的 `help` 输出为权威清单。

## 服务与模式

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl service start'
su -c '/data/adb/modules/netproxy/netproxyctl service stop'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
su -c '/data/adb/modules/netproxy/netproxyctl service reload'

su -c '/data/adb/modules/netproxy/netproxyctl mode'
su -c '/data/adb/modules/netproxy/netproxyctl mode Rule'
su -c '/data/adb/modules/netproxy/netproxyctl mode Global'
su -c '/data/adb/modules/netproxy/netproxyctl mode Direct'
su -c '/data/adb/modules/netproxy/netproxyctl mode AllowAds'
```

`mode` 返回主配置默认模式、可选模式及运行时实际模式。模式名称与内核一致，来自主配置规则，不限定为以上四种。

`service status.data.outbound_mode` 是核心当前实际模式；`configured_outbound_mode` 来自主配置的 `experimental.clash_api.default_mode`，`available_outbound_modes` 是可选模式列表。服务停止时显示默认模式，运行时 API 不可用则显示 `unknown`。Wi-Fi 策略只改变运行时结果，不覆盖默认模式。

`configured_backend` 是保存入站选择的字符串；`active_backend` 可为空，仅在 `ready`、实际 PID 与启动记录匹配、API 毫秒级启动身份一致时非空，否则为 `null`，不能用保存值猜测当前后端。

## 节点与订阅

```sh
su -c '/data/adb/modules/netproxy/netproxyctl catalog list'
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/Download/nodes.json'
su -c '/data/adb/modules/netproxy/netproxyctl node use auto default'
su -c '/data/adb/modules/netproxy/netproxyctl node use default/<节点标签>'
su -c '/data/adb/modules/netproxy/netproxyctl node delay auto default'
su -c '/data/adb/modules/netproxy/netproxyctl node export default/<节点标签>'

su -c '/data/adb/modules/netproxy/netproxyctl sub add https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub list'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub update-all'
su -c '/data/adb/modules/netproxy/netproxyctl sub history <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub cancel <分组 ID>'
```

节点引用固定为 `<分组ID>/<tag>`。自定义订阅 Header 使用 `--headers-file`，避免鉴权信息出现在 `/proc/<pid>/cmdline`。

## 分应用与网络策略

```sh
su -c '/data/adb/modules/netproxy/netproxyctl app list'
su -c '/data/adb/modules/netproxy/netproxyctl app mode whitelist'
su -c '/data/adb/modules/netproxy/netproxyctl app add 0:com.example.app'
su -c '/data/adb/modules/netproxy/netproxyctl app remove 0:com.example.app'
su -c '/data/adb/modules/netproxy/netproxyctl app enable'
su -c '/data/adb/modules/netproxy/netproxyctl app disable'

su -c '/data/adb/modules/netproxy/netproxyctl network evaluate --type wifi --ssid "Home WiFi"'
```

应用引用必须是 `<用户ID>:<包名>`。配置保存引用而不是 UID；生成运行时配置时，Go 组件按指定用户向 Android package service 查询 UID。

名单保存在 `inbound.json` 的共用 `app` 对象，命令参数不变；修改后重启服务应用，不按每次增删自动重启。只筛选本机应用，不识别热点客户端。

`network evaluate` 是高级排查入口。正常情况下后台 Worker 会根据 Android 网络事件自动评估，不需要定时手工调用。

## eBPF、配置与日志

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all --raw'

su -c '/data/adb/modules/netproxy/netproxyctl config list'
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound'
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound/backend'
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound/ebpf'
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound/tun'
su -c '/data/adb/modules/netproxy/netproxyctl config read singbox/dns'
su -c '/data/adb/modules/netproxy/netproxyctl config read singbox/config.json'
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl config validate singbox/dns /sdcard/candidate.json'

su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs export /sdcard/Download/netproxy-diagnostics.tar.gz'
```

`ebpf status` 默认返回整理后的 eBPF 能力诊断，`--raw` 返回 sing-box 原始输出；不代表实际 backend，没有 `tun status` 命令。诊断包不会导出 Catalog 节点内容。

`config list` 同时列出主配置、分区、本地规则和只读运行时。`singbox/dns` 的候选内容必须使用 `{"dns": {...}}`，`{}` 表示删除该字段；不能包含其他分区。完整替换使用 `singbox/config.json`。

受管入站使用 `inbound` 完整目标与 `inbound/backend`、`inbound/ebpf`、`inbound/tun` 分区；分区保留对应顶层字段，例如 `{"backend":"tun"}`，不能用 `{}` 删除。它们共用同一磁盘文件，没有 `config ebpf` 目标。实际入站只读目标为 `runtime/inbound.json`，另保留 `runtime/providers.json` 与 `runtime/outbounds.json`。

`config list` 的四个入站目标属于 `category: "inbound"`。运行时准备结果使用 `inbound` 路径字段与 `backend`，不再使用旧 `ebpf` 字段。切换强杀时中止并保留 journal，需要设备重启后再恢复，不做兜底清理。

`config read` 返回 `content` 和 `revision`。编辑期间需要防止覆盖并发修改时，在目标前传入读到的版本：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config apply --revision <读到的revision> singbox/dns /sdcard/candidate.json'
```

版本不一致返回 `config.conflict`，不保存；成功响应包含新的 `revision`。分区版本只跟踪该分区，整份配置版本跟踪整个文件。省略 `--revision` 表示主动覆盖所选目标，但分区写入仍不会覆盖其他字段。
