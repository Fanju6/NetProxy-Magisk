# Android 管理器

Android 管理器是 NetProxy 的图形化主入口，目前通过 Google Play 分发：

[`NetProxy - Google Play`](https://play.google.com/store/apps/details?id=com.fanjv.netproxy)

当前不提供公开源码仓库。

它不是旧模块 WebUI 的替代皮肤，而是一套独立维护的原生 Android 应用。

## 管理器负责什么

管理器当前覆盖的核心能力包括：

- 仪表盘与服务状态
- 当前节点与出站模式
- 节点导入、订阅管理、测速、导出链接
- 分应用代理与黑白名单
- 自动启动、动态测速等常用开关
- sing-box / eBPF / JSON 配置编辑
- 日志查看、导出与基础排障

## 推荐使用流程

### 1. 仪表盘

先确认：

- 服务是否运行
- 当前节点是否正确
- 当前出站模式是否符合预期

### 2. 节点与订阅

管理器适合做这些日常动作：

- 从剪贴板导入节点
- 添加订阅并更新
- 查看节点列表与当前节点
- 做延迟测试
- 导出节点分享链接

### 3. 模式切换

常用模式：

- `Rule`
- `Global`
- `Direct`

如果你启用了“动态选择节点”，运行时会优先使用测速组，而不是固定手动节点。

### 4. 分应用代理

管理器可以直接维护：

- 黑名单模式
- 白名单模式
- 应用列表
- 是否启用分应用代理

这部分最终会写入模块的 `ebpf.conf`，并由服务重启重新生成 eBPF 入站。

### 5. 配置与日志

当你需要进一步排障时，管理器适合查看：

- 服务日志
- sing-box 日志
- 当前 JSON 配置
- eBPF / sing-box 常用参数

## 与 CLI、Clash API 的关系

- **Android 管理器**：最适合日常使用
- **CLI**：最适合终端、脚本和批量操作
- **Clash API + zashboard**：最适合看代理组、连接和延迟

三者不是互斥关系，而是共享同一套模块状态。

## 版本与构建说明

当前管理器源码中：

- `applicationId`：`com.fanjv.netproxy`
- `versionName`：`8.0.0-beta.1`
- `minSdk`：`31`

普通用户直接通过 Google Play 安装即可。
