# 6.x 到 7.x 升级

7.x 是一次完整的大版本迁移。  
如果你从 6.0.1 或更早版本升级，请先理解下面这些变化。

## 核心变化

### 1. Xray -> sing-box

- 运行核心已从 **Xray** 切换为 **sing-box**
- 节点、路由、运行时配置与控制接口都围绕 sing-box 重建

### 2. 内置 WebUI 已移除

7.x 不再提供旧的模块 WebUI。  
现在的正式入口改为：

1. Android 管理器
2. CLI
3. Clash API + zashboard

### 3. 配置目录迁移

旧目录：

```text
/data/adb/modules/netproxy/config/xray/
```

新目录：

```text
/data/adb/modules/netproxy/config/singbox/
```

### 4. 节点与订阅模型调整

- 节点统一转换为 sing-box 配置
- 当前节点由 `CURRENT_CONFIG` 指向具体文件
- 运行时会根据当前目录和 `SELECTOR_MODE` 生成选择器 / 测速组

## 升级后建议做什么

1. 检查 `module.conf` 是否仍然指向有效节点文件
2. 检查 `data/catalog/` 下是否有可用节点
3. 重新确认默认模式为 `rule`
4. 通过 Android 管理器或 CLI 验证节点切换与订阅更新
5. 检查 zashboard 是否可正常访问

## 新的默认控制入口

- Controller：`http://<设备IP>:9999`
- UI：`http://<设备IP>:9999/ui`
- Secret：`singbox`

## 常见迁移误区

### 还在找旧 WebUI

7.x 已不再维护旧 WebUI，请改用 Android 管理器或 zashboard。

### 仍然按 `config/xray` 思路排查

7.x 的配置主目录已经变为 `config/singbox`，请不要再按旧路径查找运行文件。

### 以为 CLI 还是旧结构

7.x CLI 已按 sing-box 架构重写，命令分组和节点切换逻辑都与旧版不同。
