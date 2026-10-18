---
description: 用 Catalog Provider 配置 sing-box 原生策略组，独立选择出口并请求测速。
---

# 原生策略组

Catalog 管节点来源，策略组管流量出口。策略组定义只保存在 `config/singbox/config.json` 的 `outbounds`；NetProxy 不按资源生成固定的 Auto/Select/Proxy 图，也不在资源增删时改写主配置的 outbounds、route 或 DNS。安装模板中的 `Proxy` 只是可编辑的原生 selector，不是模块保留角色。

## 资源与 Provider

在 V9“资源”页管理本地资源、节点与 URL 订阅。每个非空资源对应一个 Local Provider，标签固定为 `np/<catalog-id>`，可从资源的 `provider_tag` 查看或复制。例如默认本地资源是 `np/default`。

重命名资源不改变 Provider 标签。空资源不生成运行时 Provider，不能仅因它有 `provider_tag` 就假定核心中存在该 Provider。同名节点通过完整 Provider/member 标签区分，不用显示名称拼接运行时标签。不要在主配置中创建与 Catalog 同标签的 Provider。

本地资源支持创建、重命名和删除；默认资源 `default` 不能删除。删除资源、删除或重命名节点前，若主配置显式引用将消失的 Provider 或节点，必须先修改引用，否则返回 `catalog.provider_in_use`。模块不会自动修复用户路由或替换出口。

## 定义策略组

- `selector`：用户从成员中手动选择，各组互不覆盖。
- `urltest`：由 sing-box 自动选择，不支持手动切换成员。

例如，已有非空 `np/default` 时，可将下面两个对象追加到现有 `outbounds` 数组。它们不是完整分区，不能直接覆盖原数组：

```json
[
  {
    "type": "selector",
    "tag": "Work",
    "providers": ["np/default"],
    "interrupt_exist_connections": true
  },
  {
    "type": "urltest",
    "tag": "Fast",
    "providers": ["np/default"]
  }
]
```

使用实际资源的 Provider 标签替换示例值。`providers` 引用指定 Provider，`use_all_providers: true` 使用全部 Provider，`outbounds` 引用已有出站或其他策略组；`include/exclude` 可筛选成员。标签与引用须精确一致，避免重复标签和循环引用。

定义组不会自动改变流量走向。需要在现有 `route.rules` 中把目标流量送到该组，或按需修改 `route.final`；保留既有规则顺序及其他 DNS、HTTP Client 和出站引用。不要假定名称为 `Proxy` 的组一定存在。

V9“节点”页管理原生策略组，图形编辑复用 `singbox/outbounds` 的 revision 和配置事务：只修改目标出站，保留其他对象、未知字段及未编辑的数值原文；复杂字段可用 JSON 编辑。发生 `config.conflict` 时保留草稿，显式放弃后再读取新版本，不能借新 revision 自动重试旧草稿。

完整 JSON 编辑应先读取 `singbox/outbounds`，保留原数组再校验、保存。需要同时更改出站和路由引用时，使用完整 `singbox/config.json` 事务，避免分步保存产生悬空引用。详见 [HTTP API](./http-api.md#配置保存)。

## 选择与测速

服务就绪后，使用当前策略组快照中的原始标签操作。命令中的 `Work` 和成员标签仅为示例，必须替换为实际值：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl --json policy groups'
su -c '/data/adb/modules/netproxy/netproxyctl --json policy select "Work" "np/default/example-node"'
su -c '/data/adb/modules/netproxy/netproxyctl --json policy delay "Work"'
```

对应模块 HTTP API：

| 请求 | 正文 |
| --- | --- |
| `GET /api/v1/policy-groups` | 无 |
| `POST /api/v1/policy-groups/select` | `{"group_tag":"Work","member_tag":"np/default/example-node"}` |
| `POST /api/v1/policy-groups/delay` | `{"group_tag":"Work"}` |

V9 Native 使用固定 `policy_select/policy_delay` 命令，不能传任意 URL、方法或 Shell。每个 Selector 的选择由 sing-box `experimental.cache_file` 保存；保持 cache_file 启用才能跨重启保留选择。`module.json` 只有 `auto_start/wifi`，没有模块级节点选择或活动资源。

测速响应返回当前可用结果，不保证所有成员已经完成新测试；缺少有效结果不能显示为成功。选择或测速的 POST 不重试，断线或 `policy.runtime_sync_failed` 后重新读取并核对；`accepted=true` 表示核心已接受，但快照未确认。

## 停服与重启

停服时仍可管理 Catalog 和编辑原生策略定义；策略快照为 `live=false`、`core_started_at_millis=0`，只展示配置摘要与显式出站引用，不展开 Provider、不伪造选择或延迟，也不启动离线测速核心。选择与测速在未就绪时返回 `policy.core_not_ready`。

运行时快照带 `live=true` 和已验证的核心启动身份。核心重启或重载后，以新身份的完整快照为准，不能把旧实例的选择或测速结果带入新实例。实时 `groups` 事件与 `GET /policy-groups` 同结构，不再表示 Catalog 节点列表。
