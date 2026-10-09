# module.json

`module.json` 是模块设置的唯一持久文件，位于：

```text
/data/adb/modules/netproxy/config/module.json
```

它保存模块级启动、节点选择和 Wi-Fi 自动策略。推荐通过 Android 管理器或配置命令修改。默认出站模式位于 [sing-box 主配置](./singbox#出站模式)。

默认内容：

```json
{
  "auto_start": false,
  "selection": {
    "group_id": "default",
    "node_tag": ""
  },
  "wifi": {
    "enabled": false,
    "mode": "blacklist",
    "blacklist": [],
    "whitelist": [],
    "proxy_on_non_wifi": true
  }
}
```

## 基础配置

### `auto_start`

开机是否自动启动服务：`true` 启用，`false` 禁用。默认值为 `false`。只影响下次开机，单独保存不会启动、停止或重载当前服务。

### 节点选择

- `selection.group_id` 保存当前活动分组，例如 `default`。
- `selection.node_tag` 留空使用 `Auto/<group>` 自动测速；填写该分组的节点 tag 则手动选择，例如 `"香港 01"`。
- 自动测速选出的当前节点由核心报告，不会写回 `selection.node_tag`。
- 订阅更新后手动节点消失时回退到同组 Auto，不会回退到 `direct`。

管理器和 CLI 在服务停止时也可保存选择，启动后自动应用。服务运行时通过 API 切换；若切换失败，已保存选择仍保留，命令会明确报告运行时未同步，不会自动重载核心。

## Wi-Fi 自动策略

- `wifi.enabled=false` 关闭策略，`true` 启用；关闭不清空名单或改变名单模式。
- `wifi.mode="blacklist"` 绕过黑名单 Wi-Fi，`"whitelist"` 仅对白名单 Wi-Fi 使用基础模式。
- `wifi.blacklist` 与 `wifi.whitelist` 独立保存为 JSON 字符串数组，如 `["家庭 Wi-Fi", "Office"]`，名称精确匹配并保留空格、大小写和逗号。
- `wifi.proxy_on_non_wifi=true` 表示非 Wi-Fi 网络使用基础模式；`false` 表示使用 Direct，策略关闭时忽略此项。

Wi-Fi 自动策略只改变运行时实际模式和 DNS 接管参数，不覆盖主配置的 `experimental.clash_api.default_mode` 或保存的入站偏好。Worker 按实际出口读取 SSID；绕过网络需要主配置首条为 `Direct` 直连规则。DNS 参数变化时会原位重载，其他情况只按需切换模式。

完整触发规则与排查方法见 [Wi-Fi 自动策略](/guide/wifi-policy)。

## 修改与检查

`config read/apply/validate module` 读取或替换整份 JSON。只修改 Wi-Fi 或开机自启时，使用 `module/wifi` 或 `module/auto_start`，候选内容分别为 `{"wifi": {...}}` 与 `{"auto_start": true}`。分区必需，不能用 `{}` 删除或携带其他顶层字段；没有 `module/selection` 目标，选节点使用 `node use`。

`config read` 返回 `content` 和 `revision`。两个模块分区各自使用独立 revision；保存时在同一个 `module.json.lock` 内合并最新文件，保留 `selection` 和其他字段，不会用旧快照覆盖并发节点选择。同分区冲突返回 `config.conflict`，需要重新读取，不能借用新 revision 重试旧草稿。

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config read module'
su -c '/data/adb/modules/netproxy/netproxyctl config read module/wifi'
su -c '/data/adb/modules/netproxy/netproxyctl config read module/auto_start'
su -c '/data/adb/modules/netproxy/netproxyctl config apply --revision <读到的revision> module/auto_start /sdcard/candidate.json'
su -c '/data/adb/modules/netproxy/netproxyctl config check'
```

节点和订阅不保存在 `module.json`，而是在 `data/catalog/` 中维护。选择状态只保存分组 ID 和节点 tag，不要把节点文件路径或 UID 写入该文件。

旧格式升级只能选择 **仅保留节点与订阅** 或 **全新安装**，不迁移、不读取旧格式。保留全数据要求当前 `module.json`、入站、主配置与 Catalog 完整；缺失或损坏时明确失败，不回退默认配置。
