# Wi-Fi 自动策略

Wi-Fi 自动策略根据当前网络临时选择基础出站模式或 Direct，不会覆盖你保存的基础 `OUTBOUND_MODE`。

## 配置项

```ini
WIFI_AUTO_SWITCH=0
WIFI_SSID_MODE="blacklist"
WIFI_SSID_LIST=""
PROXY_ON_CELLULAR=1
```

- `blacklist`：名单内 Wi-Fi 使用 Direct，其他 Wi-Fi 使用基础模式。
- `whitelist`：只有名单内 Wi-Fi 使用基础模式，其他 Wi-Fi 使用 Direct。
- `PROXY_ON_CELLULAR=1`：非 Wi-Fi 网络使用基础模式；设为 `0` 时使用 Direct。
- 多个 SSID 使用英文逗号分隔。

Android 管理器会以更直观的开关和名单编辑这些值。

## 如何判断当前网络

后台 Worker 监听 Android 路由、地址和接口事件，然后读取：

1. Wi-Fi 连接状态和 SSID。
2. 默认路由实际使用的接口。
3. 接口状态与热点状态。

因此，Wi-Fi 信号弱但仍显示 connected、实际流量已经走移动数据时，不会继续套用该 SSID 的规则。这不是某个品牌专用判断，而是所有双连接设备共用的真实出口判断。

热点状态用于区分本机上联网 Wi-Fi 与下游共享接口，避免把热点接口误认为当前上行 Wi-Fi。

## 触发与日志

网络事件到达后会先等待状态稳定，再进行一次策略评估，避免路由和 SSID 尚未同时就绪时误切换。读取失败时保持当前模式，不用未知状态覆盖已有策略。

查看评估记录：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 200'
```

如果策略不符合预期，请同时记录当前 SSID、默认路由接口和日志中的 network 事件。
