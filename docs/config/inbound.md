# 入站配置

受管透明代理配置的唯一事实源是：

```text
/data/adb/modules/netproxy/config/inbound/inbound.json
```

外层包含 `backend`、`root_policy`、`app`、`ebpf`、`tun`。`backend` 初始为 `ebpf`，可选 `tun`；两套原生参数分别保存，切换不会转换、清空或同步另一套参数。一次只运行一个受管入站，不自动回退另一后端。

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

`app` 支持独立分区编辑，候选内容使用 `{"app": {...}}` 包装。名单是 JSON 字符串数组，每项必须为 `<用户ID>:<包名>`。黑名单中的 `bypass_apps` 绕过代理；白名单只代理 `proxy_apps`，不自动加入 UID 0。服务运行时，有效策略变化自动应用；服务停止时只保存。

模块通过 Android package service 按用户查询 UID，不持久化 UID 缓存。开启共用策略时，白名单合并到原生 `include_uid`，黑名单合并到 `exclude_uid`，排除规则优先。白名单为空或应用全部不存在，且没有原生 include UID/range 时，本机 UID 流量全部绕过；独立 Root 策略仍可接管 UID 0。关闭共用策略时原生筛选保持不变；原生 package/user 筛选不能与共用策略混用。仅启用 eBPF 共享网络时不查询应用 UID；本机应用名单不筛选热点客户端。

DNS 遵循所选入站的原生设置：eBPF `respect_policy` 遵循 UID 策略，`hijack` 优先拦截 DNS，`off` 不拦截 53 端口。系统代发 DNS 按实际执行进程或 socket 的 UID 判断，不按发起查询的应用名称判断。

## Root 进程

管理器 → 设置 → 入站设置 → 通用设置 → Root 进程。

| 选择 | `root_policy` | 行为 |
|---|---|---|
| 默认 | `default` | 不干预 UID 0，沿用应用与原生筛选 |
| 接管 | `include` | 在 UID 筛选中接管 Root 进程，覆盖 UID 0 的排除设置 |
| 绕过 | `exclude` | 在 UID 筛选中排除 Root 进程，与应用代理模式无关 |

默认是 `default`，不会自动加入 UID 0。选择后即时保存；运行中仅有效策略变化会重载，停止时只保存。该字段在 eBPF/TUN 间共用，仅作用于本机流量；仅启用 eBPF 共享网络时保留选择但不应用。

此处 Root 指 UID 0 的进程，不是 Android 用户 0，也不是所有系统应用。Termux 普通命令按 Termux 应用筛选，`su` 后的 Root 命令才适用此项。接管后仍按路由规则决定出站，接口、DNS、地址与其他绕过条件不变。策略只投影到运行时，不改写保存的原生 UID 名单和范围。

手动配置了 `include_android_user` 时，接管 Root 要求该名单包含 Android 用户 `0`，否则返回明确冲突，不自动扩大用户范围。

## 配置目标

| 目标 | 内容 | 权限 |
|---|---|---|
| `inbound` | 完整五字段包装 | 可编辑 |
| `inbound/backend` | `{"backend":"tun"}` 或 `{"backend":"ebpf"}` | 可编辑 |
| `inbound/root_policy` | `{"root_policy":"default"}`、`include` 或 `exclude` | 可编辑 |
| `inbound/app` | `{"app":{...}}` | 可编辑 |
| `inbound/ebpf` | `{"ebpf":{...}}` | 可编辑 |
| `inbound/tun` | `{"tun":{...}}` | 可编辑 |
| `runtime/inbound.json` | 只有当前选中受管入站的 sing-box 文档 | 只读 |

入站分区必需，不能用 `{}` 删除。分区保存只替换自己拥有的字段，保留其他分区和数组顺序。整份保存校验两套参数；启动只校验当前选中分区，不探测未选后端。整份 JSON 损坏会失败，不会回退默认值。

`config list` 将以上六个可编辑目标归为 `category: "inbound"`；只读生成物仍属 `runtime`。运行时准备结果 `Prepare` 的路径字段是 `providers`、`outbounds`、`inbound`，另有选中 `backend`。

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

安装器不检测或转换旧版本。保留全数据要求当前 `config/module.json`、`inbound.json`、主配置与 Catalog 完整；缺少当前模块 JSON 或入站文件会明确失败。旧格式用户只能选择“仅保留节点与订阅”或“全新安装”，不会静默补默认。
