# Wi-Fi 自动策略

按实际联网的 Wi-Fi 名称，在主配置的默认出站模式与 `Direct` 之间切换。绕过时保留应用 DNS 请求的原目标，不交给模块的公共 DNS。

## 在管理器中设置

打开 **设置 → 网络匹配**：

1. 在 **Wi-Fi 自动切换** 中选择关闭、黑名单或白名单。
2. 设置 **非 Wi-Fi 网络走代理**；关闭后，移动数据和有线等网络使用 Direct。
3. 点击右上角 **+**，输入 Wi-Fi 名称，或勾选系统已保存的 Wi-Fi，点击添加。
4. 点击名单条目可编辑名称或删除。返回上一页或将管理器切到后台后，已确认的修改统一保存。

黑名单中的 Wi-Fi 使用 Direct；白名单中的 Wi-Fi 使用默认模式，其他 Wi-Fi 使用 Direct。两份名单独立保存，切换模式不会清空另一份。关闭自动切换后使用主配置的默认模式，并保留原名单模式。

名称完整匹配，区分大小写并保留空格；空黑名单不绕过 Wi-Fi，空白名单绕过所有 Wi-Fi。读取不到系统候选时，仍可手动添加。

## 配置项

Wi-Fi 策略保存在 `config/module.json` 的 `wifi` 对象。`module/wifi` 分区使用以下候选格式：

```json
{
  "wifi": {
    "enabled": false,
    "mode": "blacklist",
    "blacklist": [],
    "whitelist": [],
    "proxy_on_non_wifi": true
  }
}
```

- `wifi.enabled`：`false` 关闭，`true` 启用。
- `wifi.mode`：`"blacklist"`、`"whitelist"`。管理器将开关与名单模式合并显示为关闭、黑名单、白名单。
- `wifi.blacklist` 与 `wifi.whitelist` 使用 JSON 字符串数组，例如 `["家庭 Wi-Fi", "Office, Wi-Fi"]`；名称须为 1 至 32 个 UTF-8 字节，不含控制字符。
- `wifi.proxy_on_non_wifi=true` 使用默认模式，`false` 使用 Direct；自动切换关闭时不应用此项。
- 默认模式来自主配置 `experimental.clash_api.default_mode`，网络策略不会覆盖它。

保存使用读取 `module/wifi` 时的独立 revision，只替换 `wifi`，保留最新节点选择与开机自启。候选不能用 `{}` 删除分区，也不能包含 `selection` 或 `auto_start`。读取与应用示例见 [模块设置](/config/module#修改与检查)。

旧格式升级只能选择 **仅保留节点与订阅** 或 **全新安装**，不迁移旧配置。

## DNS 与运行时

主配置 `route.rules` 的第一条必须是下列规则，先于 sniff、hijack-dns 等规则：

```json
{
  "clash_mode": "Direct",
  "action": "route",
  "outbound": "direct"
}
```

默认配置已包含该规则。自定义配置也可以指向自己定义的 direct 类型出站；不满足条件时报告错误，不自动改写路由。

Direct 模式下，eBPF 已启用路径的 DNS 模式临时设为 `off`，TUN 临时设为 `disabled`；恢复其他模式时使用保存的入站参数。仅生成运行时副本，不修改入站配置偏好。

有效 DNS 参数变化时会原位重载核心，既有连接可能中断；参数不变时只按需通过 API 切换模式。相同策略的 Wi-Fi 之间切换不会重复重载。API 失败不触发重载兜底。

这会保留应用 DNS 的原目标，并不停止核心，也不改变应用自行配置的 DoH、DoT 或 Android 私人 DNS。

## 网络判断与排查

Worker 监听路由、接口、地址与 Wi-Fi 连接事件，按实际默认出口对应的 station 接口读取 SSID。Wi-Fi 仍显示连接、实际流量已走移动数据时，使用非 Wi-Fi 策略；热点 AP 不作为本机上联网 Wi-Fi。

网络未知时不视为移动网络，运行中保持当前策略。开机尚无默认路由时先使用默认模式，等待网络就绪。正常运行不定时轮询网络；监听失败时重新订阅。

查看策略日志和系统候选：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 200'
su -c '/data/adb/modules/netproxy/netproxyctl network wifi-list'
```

若策略未触发，核对名单名称、实际出口和日志中的 `network` 事件。设备的 nl80211 查询不可用时会明确报错，不从文本猜测 Wi-Fi 状态。
