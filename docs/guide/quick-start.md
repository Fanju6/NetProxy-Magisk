---
description: 使用 Android 管理器导入节点、添加订阅、选择节点并启动 NetProxy。
---

# 快速开始

推荐先用 Android 管理器完成第一次配置。CLI 适合没有管理器的环境和高级排查。

## 1. 添加节点

打开“节点”页，点击右上角添加按钮：

- **单节点链接**：粘贴节点链接。
- **本地文件**：导入节点文本、Clash YAML 或 sing-box JSON；节点会追加到“本地配置”。

也可以直接前往“资源”页添加 URL。订阅名称留空时会尝试从响应头或 URL 自动获取。

## 2. 启动服务

在仪表盘启动服务。初始后端为 eBPF，切换方法见[入站配置](/config/inbound)。运行状态说明核心已就绪，不代表已选择代理节点。

## 3. 选择代理节点

打开「代理」页，在默认 `Proxy` 中选择已导入的节点。启动后可按组测速；停止时可查看配置成员，但不能切换或测速。默认未选择代理节点时会阻断需要代理的流量。

## 4. 调整代理范围

按需要打开对应页面：

| 入口 | 可配置内容 |
|---|---|
| 仪表盘 → 出站模式 | 规则、代理、直连或规则（允许广告）模式 |
| 设置 → 入站设置 | eBPF / TUN、DNS 接管、IPv6、接口、绕过与分应用代理，详见[入站配置](/config/inbound) |
| 设置 → 网络匹配 | Wi-Fi 名称匹配与非 Wi-Fi 网络策略，详见[Wi-Fi 自动策略](/guide/wifi-policy) |
| 设置 → 内核配置 | sing-box 主配置、DNS、路由与本地规则，详见[配置参考](/config/singbox) |

服务运行时，保存当前入站参数或有效分应用策略会应用配置，切换后端会短暂断开连接；编辑未选后端仅保存。服务停止时，保存设置不会自动启动服务。

## 5. 遇到问题

先打开“日志”页查看模块日志和核心日志。反馈问题时使用导出功能生成脱敏诊断包，不要直接发送 Catalog 或原始节点配置。

没有管理器时，可从终端检查：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
```

完整命令见 [CLI](/guide/cli)，常见故障见[常见问题与诊断](/guide/faq)。
