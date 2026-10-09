# module.conf

`module.conf` 位于：

```text
/data/adb/modules/netproxy/config/module.conf
```

它保存模块级启动、节点选择和 Wi-Fi 自动策略。推荐通过 Android 管理器修改；手动编辑后应使用 `netproxyctl` 检查并重启服务。默认出站模式位于 [sing-box 主配置](./singbox#出站模式)。

## 基础配置

### `AUTO_START`

开机是否自动启动服务：`1` 启用，`0` 禁用。默认值为 `0`。

### 节点选择

```ini
ACTIVE_GROUP_ID="default"
SELECTED_NODE_TAG=""
```

- `ACTIVE_GROUP_ID` 保存当前活动分组，例如 `default`。
- `SELECTED_NODE_TAG` 留空使用 `Auto/<group>` 自动测速；填写该分组的节点 tag 则手动选择，例如 `SELECTED_NODE_TAG="香港 01"`。
- 自动测速选出的当前节点由核心报告，不会写回 `SELECTED_NODE_TAG`。
- 订阅更新后手动节点消失时回退到同组 Auto，不会回退到 `direct`。

管理器和 CLI 在服务停止时也可保存选择，启动后自动应用。服务运行时通过 API 切换；若切换失败，已保存选择仍保留，命令会明确报告运行时未同步，不会自动重载核心。

## Wi-Fi 自动策略

```ini
WIFI_AUTO_SWITCH=0
WIFI_SSID_MODE="blacklist"
WIFI_SSID_BLACKLIST=[]
WIFI_SSID_WHITELIST=[]
PROXY_ON_NON_WIFI=1
```

- `WIFI_AUTO_SWITCH=0` 关闭策略，`1` 启用；关闭不清空名单或改变名单模式。
- `WIFI_SSID_MODE=blacklist` 绕过黑名单 Wi-Fi，`whitelist` 仅对白名单 Wi-Fi 使用基础模式。
- 黑白名单独立保存为 JSON 字符串数组，如 `["家庭 Wi-Fi", "Office"]`，名称精确匹配。
- `PROXY_ON_NON_WIFI=1` 表示非 Wi-Fi 网络使用基础模式；`0` 表示使用 Direct，策略关闭时忽略此项。

Wi-Fi 自动策略只改变运行时实际模式和 DNS 接管参数，不覆盖主配置的 `experimental.clash_api.default_mode` 或保存的入站偏好。Worker 按实际出口读取 SSID；绕过网络需要主配置首条为 `Direct` 直连规则。DNS 参数变化时会原位重载，其他情况只按需切换模式。

完整触发规则与排查方法见 [Wi-Fi 自动策略](/guide/wifi-policy)。

## 修改与检查

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```

节点和订阅不保存在 `module.conf`，而是在 `data/catalog/` 中维护。选择状态只保存分组 ID 和节点 tag，不要把节点文件路径或 UID 写入该文件。
