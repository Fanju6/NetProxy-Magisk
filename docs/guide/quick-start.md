# 快速开始

这条路径用于完成最小闭环：检查服务、导入节点、启动核心、确认模式并打开面板。所有命令都需要 Root。

## 1. 检查服务状态

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
```

服务状态从 `stopped`、`preparing`、`starting`、`ready`、`stopping`、`failed` 中取值。只有 `ready` 才表示 sing-box 与 eBPF 入站都已经就绪。

## 2. 导入节点

导入单个链接：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'
```

导入节点文件或 Clash YAML：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/clash.yaml'
```

单链接和本地文件默认追加到 `default` 本地配置组。

## 3. 添加订阅

订阅名称可以省略，省略时会从响应头或 URL 自动获取：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl sub add 我的订阅 https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <分组 ID>'
```

服务停止时也可以更新订阅。更新失败会保留上一版有效 Provider，不会清空现有节点。

## 4. 查看与选择节点

```sh
su -c '/data/adb/modules/netproxy/netproxyctl catalog list'
su -c '/data/adb/modules/netproxy/netproxyctl node list'
```

自动测速选择：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node use auto default'
```

手动选择：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node use default/<节点标签>'
```

分组测速：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node delay auto default'
```

## 5. 启动服务

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service start'
```

启动完成后查看实际模式：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl mode'
```

可用出站模式：

- `rule`：按路由规则分流，默认模式。
- `global`：尽量全部交给代理出站。
- `direct`：全部直连。
- `AllowAds`：使用允许广告的路由策略。

例如切换到全局代理：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl mode global'
```

## 6. 打开控制面板

模块 WebUI 入口可从 Root 管理器的模块页面打开。服务启动后也可以直接使用：

- NetProxy WebUI：通过模块 WebUI 入口打开
- zashboard：`http://127.0.0.1:9999/ui/`
- sing-box Service API Dashboard：`http://127.0.0.1:9090/dashboard/`

两个 API 的默认密钥都是 `singbox`，默认仅监听本机。

## 7. 遇到问题先看日志

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```

服务日志记录模块、订阅和 Worker 事件；核心日志保留 sing-box 原始输出。需要反馈问题时优先导出脱敏诊断包：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs export /sdcard/Download/netproxy-diagnostics.tar.gz'
```
