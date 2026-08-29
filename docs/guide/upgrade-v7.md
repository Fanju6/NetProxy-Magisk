# 6.x → 7.x 历史升级指南

::: warning 历史资料
本页只记录从 6.x 升级到 7.x 时的迁移背景，不适用于当前版本。当前安装说明见[安装与升级](/guide/installation)。
:::

7.x 将代理核心从 Xray 切换为 sing-box，并重组了当时的节点、路由与控制入口。

## 当时的主要变化

### 核心与配置目录

旧目录：

```text
/data/adb/modules/netproxy/config/xray/
```

7.x 目录：

```text
/data/adb/modules/netproxy/config/singbox/
```

节点和路由需要使用 sing-box 可识别的配置，不能继续把 Xray 配置复制到新目录。

### 节点选择

7.x 使用 `CURRENT_CONFIG` 指向当时的节点文件，并按 `SELECTOR_MODE` 生成 selector 或 URLTest。升级后应检查它是否仍指向存在且有效的配置文件。

Catalog、`provider.json` 和当前 `netproxyctl` 契约属于后续架构，不是 7.x 的组成部分。

### 管理入口

7.x 不再维护 6.x 的旧模块 WebUI，主要使用：

1. Android 管理器
2. 当时版本的 CLI
3. Clash API 与 zashboard

默认 Clash Controller 为 `127.0.0.1:9999`，密钥为 `singbox`。

## 当时的升级检查

1. 确认 `config/singbox/` 已替代旧 Xray 配置。
2. 确认 `CURRENT_CONFIG` 指向可用节点文件。
3. 检查默认出站模式与 selector 设置。
4. 使用对应 7.x 管理器或 CLI 验证节点选择。
5. 服务运行后确认 zashboard 能连接 Clash API。

如果设备已使用当前版本，请不要按本页恢复旧路径或旧命令。
