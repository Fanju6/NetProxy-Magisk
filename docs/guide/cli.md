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
su -c '/data/adb/modules/netproxy/netproxyctl mode rule'
su -c '/data/adb/modules/netproxy/netproxyctl mode global'
su -c '/data/adb/modules/netproxy/netproxyctl mode direct'
su -c '/data/adb/modules/netproxy/netproxyctl mode AllowAds'
```

`service status.data.outbound_mode` 是核心当前实际模式；`configured_outbound_mode` 是保存的基础模式。Wi-Fi 策略只改变运行时结果，不覆盖基础模式。

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

`network evaluate` 是高级排查入口。正常情况下后台 Worker 会根据 Android 网络事件自动评估，不需要定时手工调用。

## eBPF、配置与日志

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all --raw'

su -c '/data/adb/modules/netproxy/netproxyctl config list'
su -c '/data/adb/modules/netproxy/netproxyctl config read singbox/dns'
su -c '/data/adb/modules/netproxy/netproxyctl config read singbox/config.json'
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl config validate singbox/dns /sdcard/candidate.json'

su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs export /sdcard/Download/netproxy-diagnostics.tar.gz'
```

`ebpf status` 默认返回整理后的能力诊断，`--raw` 返回 sing-box 原始输出。诊断包不会导出 Catalog 节点内容。

`config list` 同时列出主配置、分区、本地规则和只读运行时。`singbox/dns` 的候选内容必须使用 `{"dns": {...}}`，`{}` 表示删除该字段；不能包含其他分区。完整替换使用 `singbox/config.json`。

`config read` 返回 `content` 和 `revision`。编辑期间需要防止覆盖并发修改时，在目标前传入读到的版本：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config apply --revision <读到的revision> singbox/dns /sdcard/candidate.json'
```

版本不一致返回 `config.conflict`，不保存；成功响应包含新的 `revision`。分区版本只跟踪该分区，整份配置版本跟踪整个文件。省略 `--revision` 表示主动覆盖所选目标，但分区写入仍不会覆盖其他字段。
