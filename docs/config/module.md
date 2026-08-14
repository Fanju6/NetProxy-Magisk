# module.conf

`module.conf` 位于：

```text
/data/adb/modules/netproxy/config/module.conf
```

它保存模块级启动、节点选择、出站模式和 Wi-Fi 自动策略。推荐通过 Android 管理器修改；手动编辑后应使用 `netproxyctl` 检查并重启服务。

## 基础配置

### `AUTO_START`

开机是否自动启动服务：`1` 启用，`0` 禁用。默认值为 `0`。

### `OUTBOUND_MODE`

支持四种模式：

| 值 | 行为 |
|----|------|
| `rule` | 按 sing-box 路由规则分流，默认值 |
| `global` | 尽量全部交给代理出站 |
| `direct` | 全部直连 |
| `AllowAds` | 使用允许广告的路由策略 |

### 节点选择

```ini
SELECTOR_MODE=urltest
ACTIVE_GROUP_ID="default"
SELECTED_NODE_REF=""
```

- `SELECTOR_MODE=urltest` 使用 `Auto/<group>` 自动测速，`SELECTED_NODE_REF` 必须为空。
- `SELECTOR_MODE=manual` 使用 `<group-id>/<tag>` 手动选择。
- `ACTIVE_GROUP_ID` 保存当前活动分组，例如 `default`。
- 订阅更新后手动节点消失时回退到同组 Auto，不会回退到 `direct`。

## Wi-Fi 自动策略

```ini
WIFI_AUTO_SWITCH=0
WIFI_SSID_MODE="blacklist"
WIFI_SSID_LIST=""
PROXY_ON_CELLULAR=1
```

- `WIFI_AUTO_SWITCH=1` 启用后台 Worker 的 SSID 策略评估。
- `WIFI_SSID_MODE=blacklist` 时名单内 Wi-Fi 使用 Direct。
- `WIFI_SSID_MODE=whitelist` 时只有名单内 Wi-Fi 使用基础模式。
- `WIFI_SSID_LIST` 使用英文逗号分隔。
- `PROXY_ON_CELLULAR=1` 表示非 Wi-Fi 网络使用基础模式；`0` 表示使用 Direct。

Wi-Fi 自动策略只改变运行时实际模式，不覆盖 `OUTBOUND_MODE` 保存的基础模式。Worker 会检查实际默认路由，避免 Wi-Fi 仍显示 connected 但流量已经走移动数据时误判。

## 修改与检查

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```

节点和订阅不保存在 `module.conf`，而是在 `data/catalog/` 中维护。选择状态只使用分组 ID、节点 tag 和当前选择模式，不要把节点文件路径或 UID 写入 8.0 配置。
