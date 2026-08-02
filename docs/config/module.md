# module.conf

`module.conf` 是 NetProxy 的模块级默认配置文件，位于：

```text
/data/adb/modules/netproxy/config/module.conf
```

它主要决定服务启动后的默认行为，而不是透明代理底层细节。

## 当前常用项

### `AUTO_START`

- 默认值：`0`
- 作用：是否在系统启动后自动拉起 NetProxy 服务

### `OUTBOUND_MODE`

- 默认值：`rule`
- 可选值：`rule` / `global` / `direct`
- 作用：控制默认出站模式

对应含义：

- `rule`：按规则分流
- `global`：除直连保留项外尽量走代理
- `direct`：全局直连

### `SELECTOR_MODE`

- 默认值：`urltest`
- 可选值：`manual` / `urltest`
- 作用：控制运行时代理组生成方式

含义：

- `manual`：以手动选择为主
- `urltest`：生成动态测速组，优先选择更低延迟节点

### `GMS_FIX`

- 默认值：`0`
- 可选值：`0` / `1`
- 作用：启用设备兼容性修复逻辑，主要用于部分 Google Play / GMS 相关联网问题

### `CURRENT_CONFIG`

- 默认值：

```text
CURRENT_CONFIG=""
```

- 作用：记录当前选中的 sing-box 节点配置文件

通常不建议手动填写到模块外部路径，推荐让 Android 管理器或 CLI 维护这个值。

## 推荐修改方式

优先顺序：

1. Android 管理器
2. CLI
3. 手动编辑配置文件

如果你手动修改了 `module.conf`，建议随后重启服务，确保运行时状态和持久化配置一致。

## WiFi 自动切换

WiFi 自动切换也由 `module.conf` 管理：

- `WIFI_AUTO_SWITCH=0`：默认关闭
- `WIFI_SSID_MODE="blacklist"`：名单内使用 Direct
- `WIFI_SSID_LIST=""`：SSID 名单，英文逗号分隔
- `PROXY_ON_CELLULAR=1`：非 WiFi 网络使用基础模式
- `WIFI_INTERFACE="wlan0"`：用于读取当前 SSID 的接口
