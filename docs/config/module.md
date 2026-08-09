# module.conf

`module.conf` 位于：

```text
/data/adb/modules/netproxy/config/module.conf
```

它保存模块级启动、选择和出站模式设置。

## 常用配置

### `AUTO_START`

开机是否自动启动服务，`1` 启用，`0` 禁用。

### `OUTBOUND_MODE`

出站模式可设为：

- `rule`：按规则分流。
- `global`：统一使用代理出站。
- `direct`：统一直连。
- `AllowAds`：按允许广告模式运行。

### `SELECTOR_MODE`

- `urltest`：自动测速，运行时选择 `Auto/<group>`。
- `manual`：手动选择，`SELECTED_NODE_REF` 保存 `<group-id>/<tag>`。

### `ACTIVE_GROUP_ID`

当前活动 Catalog 分组的 ID，例如 `default` 或订阅分组 ID。

### `SELECTED_NODE_REF`

仅在 `SELECTOR_MODE=manual` 时使用，格式为：

```ini
SELECTED_NODE_REF="<group-id>/<tag>"
```

自动模式必须保持为空。节点在订阅更新后消失时，模块会回退到该分组的自动测速，不会静默切到 `direct`。

## 修改方式

推荐使用 Android 管理器或 `netproxyctl config` 修改。手动编辑后执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config validate'
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```

节点和订阅不保存在 `module.conf`，而是在 `data/catalog/` 中维护。
