# 常见问题与诊断

## 服务启动失败

依次执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```

确认已有可用节点、活动分组有效，并查看错误来自静态配置、Provider、eBPF 能力还是核心启动。

## 订阅更新失败会清空节点吗

不会。下载、转换或校验失败会保留上一版有效 Provider。若提示“已持久化但运行时未同步”，表示节点已经保存，但运行核心尚未确认新内容；可以重试更新或重启核心。

## 节点测速为什么全部超时

先确认节点本身可用。服务停止时会启动临时核心测速，首次准备可能比运行中稍慢；若所有节点同时超时，请检查核心日志、DNS、网络权限和 Provider 配置，而不是只提高超时时间。

## Global 模式为什么仍有直连

`EBPF_BYPASS_RULE_SET`、私网绕过和应用名单可以在流量进入普通路由前放行。严格测试 Global 时清空提前绕过规则并重启核心，同时确认没有应用或共享网络筛选。

## DNS 泄漏是什么

检测站通常把“DNS 请求没有经过当前代理出口”称为 DNS 泄漏。默认配置的最终 DNS 使用 `dns-proxy`，不使用 FakeIP。需要让所有兜底解析也走代理时，可以调整 `03_dns.json`，代价是代理不可用时解析更依赖节点，并可能增加延迟。

## 应用分身没有生效

同一包名在不同 Android 用户下必须分别选择。保存格式为 `<用户ID>:<包名>`；修改后重启核心，让模块重新查询 UID。系统组件代发流量不一定属于目标应用 UID。

## Wi-Fi 策略没有触发

策略根据 SSID 与实际默认出口评估。Wi-Fi 仍显示连接但流量已切到移动数据时，会按非 Wi-Fi 网络处理。查看 service 日志中的 `network` 和 Wi-Fi 策略事件，并确认后台 Worker 正在运行。

## 如何安全反馈问题

在 Android 管理器日志页导出诊断包，或运行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs export /sdcard/Download/netproxy-diagnostics.tar.gz'
```

诊断包会脱敏日志和运行时摘要，不导出 Catalog 节点数据。请附上设备型号、Android 版本、内核版本、Root 方案、模块版本和复现步骤，不要发送原始订阅 URL 或节点凭据。
