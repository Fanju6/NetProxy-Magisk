# 入站配置

受管透明代理配置的唯一事实源是：

```text
/data/adb/modules/netproxy/config/inbound/inbound.json
```

外层只有 `backend`、`app`、`ebpf`、`tun`。`backend` 初始为 `ebpf`，可选 `tun`；两套原生参数分别保存，切换不会转换、清空或同步另一套参数。一次只运行一个受管入站，不自动回退另一后端。

`ebpf` 和 `tun` 对象都包含原生 `type` 与固定 `tag: "netproxy-in"`，类型必须匹配分区。参数分别见 [eBPF](/config/ebpf) 和 [TUN](/config/tun)。本文件带 NetProxy 包装，不是 sing-box 根配置，不能直接作为 `sing-box -c` 输入。

## 共用应用策略

```json
{
  "app": {
    "enabled": true,
    "mode": "blacklist",
    "proxy_apps": [],
    "bypass_apps": ["0:com.example.app", "10:com.example.app"]
  }
}
```

这是完整文件中 `app` 的示例，不是独立配置文件或 `inbound/app` 目标。名单是 JSON 字符串数组，每项必须为 `<用户ID>:<包名>`。黑名单中的 `bypass_apps` 绕过代理；白名单只代理 `proxy_apps`，并自动包含 UID 0。修改应用名单后重启服务应用。

模块通过 Android package service 按用户查询 UID，不持久化 UID 缓存。关闭共用策略时保留原生 UID 筛选；开启时合并到当前后端的本机 UID 筛选，反向 UID 筛选或原生 package/user 筛选存在歧义时拒绝配置。仅启用 eBPF 共享网络时不查询应用 UID；本机应用名单不筛选热点客户端。

## 配置目标

| 目标 | 内容 | 权限 |
|---|---|---|
| `inbound` | 完整四字段包装 | 可编辑 |
| `inbound/backend` | `{"backend":"tun"}` 或 `{"backend":"ebpf"}` | 可编辑 |
| `inbound/ebpf` | `{"ebpf":{...}}` | 可编辑 |
| `inbound/tun` | `{"tun":{...}}` | 可编辑 |
| `runtime/inbound.json` | 只有当前选中受管入站的 sing-box 文档 | 只读 |

入站分区必需，不能用 `{}` 删除。分区保存只替换自己拥有的字段，保留其他分区和数组顺序。整份保存校验两套参数；启动只校验当前选中分区，不探测未选后端。整份 JSON 损坏会失败，不会回退默认值。

`config list` 将以上四个逻辑目标归为 `category: "inbound"`；只读生成物仍属 `runtime`。运行时准备结果 `Prepare` 的路径字段是 `providers`、`outbounds`、`inbound`，另有选中 `backend`，不再返回旧 `ebpf` 路径字段。

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound'
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound/backend'
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound/tun'
su -c '/data/adb/modules/netproxy/netproxyctl config validate --revision <读到的revision> inbound/backend /sdcard/candidate.json'
su -c '/data/adb/modules/netproxy/netproxyctl config apply --revision <读到的revision> inbound/backend /sdcard/candidate.json'
```

切换候选只需 `{"backend":"tun"}`，参数由保存的 `tun` 对象提供。同分区旧版本返回 `config.conflict`，需要重新读取；修改其他分区不会直接造成冲突。没有 `config ebpf` 配置目标或独立入站命令组。

## 保存与运行状态

服务停止时保存不会启动核心或 Worker。服务运行时，修改选中后端会应用配置；修改未选后端只保存，不 reload。切换 backend 会短暂断开连接：先检查候选，再停止旧核心、启动新核心并确认；失败尝试恢复旧配置与运行实例，不允许双接管。

若旧核心被强杀或接管清理无法确认，切换中止并保留 journal，需要设备重启后再恢复。不会在可能残留接管时启动新后端，也不会执行兜底的宽泛 iptables、TC 或 cgroup 清理。

`service status` 中 `configured_backend` 是保存选择的字符串；`active_backend` 可为空，仅在状态为 `ready`、实际 PID 与启动记录匹配、Service API 毫秒级启动身份一致时非空。其他情况均为 `null`，不能从当前模板猜测。服务仍使用原有六态，只有 Service API 和选中入站就绪后才写入 `ready_at`。

`runtime/inbound.json` 可重建，不应编辑或作为安装保留数据。`runtime/providers.json` 与 `runtime/outbounds.json` 继续独立生成。主配置允许 mixed、HTTP 等其他入站，但不得额外定义受管 eBPF/TUN 或占用 `netproxy-in`。

安装器不检测或转换旧版本。保留全数据要求当前 `inbound.json`、主配置与 Catalog 完整；缺少入站文件会明确失败，请主动选择“仅保留节点与订阅”或“全新安装”，不会静默补默认。
