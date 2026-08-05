# NetProxy 架构说明

本文记录 NetProxy 8.x 的跨组件架构、事实源和稳定契约。日常编码约束见 [AGENTS.md](AGENTS.md)，贡献流程见 [CONTRIBUTING.md](CONTRIBUTING.md)，Android 内部分层见 [src/android/ARCHITECTURE.md](src/android/ARCHITECTURE.md)。

## 系统边界

```text
Android Manager ─┐
WebUI ───────────┼─> netproxyctl ─> Shell 编排 ─> sing-box
终端用户 ───────┘        │              │
                         │              ├─> eBPF 入站运行时
                         │              └─> 订阅 worker / 网络监听
                         └─> netproxy-native
                              ├─> 节点与订阅转换
                              ├─> Provider 校验与原子写入
                              ├─> HTTP 元数据
                              └─> Service API
```

NetProxy 不维护独立常驻守护进程。Shell 负责 Android 模块生命周期、sing-box 进程、运行时配置和后台任务；Go 组件处理 Shell 不适合安全实现的类型化配置、网络和 Protobuf 能力。

## 事实源

| 数据 | 唯一事实源 | 说明 |
|---|---|---|
| 模块版本 | `src/module/module.prop` | `versionCode` 由打包工作流写入 |
| 模块设置 | `src/module/config/module.conf` | 保存活动分组、选择模式和出站模式 |
| eBPF 设置 | `src/module/config/ebpf/ebpf.conf` | 由运行时生成 sing-box eBPF inbound |
| 节点与订阅 | `src/module/config/catalog/<group-id>/` | `meta.json` + `provider.json` |
| sing-box 静态配置 | `src/module/config/singbox/confdir/` | 按编号组合加载 |
| sing-box 运行时配置 | `src/module/config/singbox/runtime/` | 启动或检查时生成，不由客户端编辑 |
| 服务状态 | `src/module/config/runtime/service.json` | `netproxyctl service status` 的状态源之一 |
| 实时核心状态 | Service API / Clash API | 连接、流量、测速和实际选择 |

Android 和 WebUI 不建立另一份节点数据库，也不直接修改这些文件。持久状态通过 `netproxyctl` 读取和变更，运行状态通过固定的 sing-box API 补充。

## Catalog 与 Provider

```text
config/catalog/
├── default/
│   ├── meta.json
│   └── provider.json
├── <group-id>/
│   ├── meta.json
│   ├── provider.json
│   └── history.jsonl
└── staging/
```

- `default` 是固定本地分组，接收单链接和本地文件导入。
- URL 订阅使用稳定的随机分组 ID；显示名称保存在 `meta.json`。
- `provider.json` 是标准 sing-box Provider 文档，也是节点内容事实源。
- 订阅节点只读；编辑前复制到本地分组。
- `history.jsonl` 只记录脱敏后的更新结果，默认保留最近 20 条。
- `staging/` 用于锁、下载、转换和校验的临时事务。进程崩溃后可清理，业务代码不得依赖其中内容恢复节点状态。

运行时把每个非空分组投影为 Local Provider，并生成：

- `Auto/<group>`：urltest，默认自动测速。
- `Select/<group>`：selector，手动选择。
- `Proxy`：顶层 selector，连接各分组的 Auto/Select 出站。

分组 ID 是内部稳定身份，运行时标签和界面优先使用分组名称；只有名称冲突时追加 ID 消歧。同组和跨组节点切换优先使用 Service API，新增或删除整个分组时才需要重新加载运行时配置。

## 选择状态

`module.conf` 使用以下字段：

```ini
ACTIVE_GROUP_ID="default"
SELECTOR_MODE=urltest
SELECTED_NODE_REF=""
OUTBOUND_MODE=rule
```

- 自动模式下 `SELECTED_NODE_REF` 必须为空，实际选中节点由 Service API 报告。
- 手动模式保存 `<group-id>/<tag>`，不保存文件路径。
- 手动节点在 Provider 更新后消失时回退该组 Auto。
- 任意故障都不得把代理选择器默认值静默改成 `direct`。
- 出站模式支持 `rule`、`global`、`direct` 和 `AllowAds`，客户端必须保持同一顺序和语义。

## 订阅事务

订阅更新独立于 sing-box 是否运行：

```text
获取分组锁
-> 创建 staging
-> 条件下载
-> 解析 HTTP Header
-> netproxy-native 转换
-> Provider 校验
-> 原子替换 Provider 与元数据
-> 通知运行中的 Local Provider
-> 写入脱敏历史
```

下载、转换和校验阶段可以取消，原子提交阶段不可取消。任何失败都保留上一版有效 Provider 和当前选择。核心 ready 时可按设置经本地代理下载；核心停止或代理下载失败时，`auto` 策略允许直连重试。

后台 `subworker.sh` 根据各订阅的 `next_update_at` 调度，不依赖 sing-box 和 `crond`。运行时进度放在 `/dev/netproxy/subscriptions/`，完成后不作为长期 UI 状态显示。

## 服务生命周期

服务状态机固定为：

```text
stopped -> preparing -> starting -> ready -> stopping -> stopped
                         \-> failed
```

启动流程：

1. 校验二进制、静态配置、Catalog 和活动选择。
2. 生成 providers、outbounds 与 eBPF runtime 配置。
3. 运行 sing-box 配置检查。
4. 启动 sing-box 并等待 Service API 与 eBPF 入站就绪。
5. 写入 `ready_at`，客户端从此时开始显示完整运行时间。

eBPF 只负责透明代理入站。停止服务由 sing-box 关闭并清理其 eBPF 程序、Map 和 TC 挂载；项目不再维护 TPROXY/IPSET 兼容路径。

分应用配置保存包名和可选 Android 用户范围，`ebpf.sh` 直接生成 `include_package`、`exclude_package` 与 `include_android_user`，包名到 UID 的解析由 sing-box 在入站启动时完成。应用安装、重装、UID 变化或用户范围变化后，通过配置 reload 重新解析，不维护模块侧 UID 缓存。

本机 cgroup 与热点 shared-network 是可独立启用的数据路径。关闭本机 cgroup 时，运行时配置省略 cgroup 路径、原生 IPv6 策略、应用/UID 策略和本机 Map 字段；本机与共享网络同时关闭属于无效配置。

## sing-box 配置组合

`service.sh` 通过 `-C config/singbox/confdir` 加载静态配置，并追加运行时文件：

- `providers.json`：Catalog Local Provider 投影。
- `outbounds.json`：Auto/Select/Proxy 出站图。
- `ebpf.json`：由 `ebpf.conf` 生成的透明代理入站。

`confdir` 中的编号文件按职责拆分：日志、实验特性/Clash API、DNS、用户入站、路由、HTTP Client 和 Service API。运行时文件由脚本生成，用户配置编辑器只能修改受管理的静态文档。

当前控制入口是稳定产品契约：

| 接口 | 本机客户端地址 | 用途 |
|---|---|---|
| Service API | `127.0.0.1:9090` | 核心状态、流量、节点组、选择和测速 |
| Clash API | `127.0.0.1:9999` | zashboard 与第三方 Clash 客户端 |
| 模块 WebUI | 模块管理器 WebView | 持久管理与状态展示 |

静态配置当前监听 `0.0.0.0`，本机客户端统一通过 `127.0.0.1` 访问；配置文件使用固定密钥 `singbox`，用于 WebUI 自动进入面板。改变监听、路径或鉴权方式属于安全及跨 Android、WebUI、面板和文档的契约变更，不能只改一端。

## 管理接口

`netproxyctl --json` 返回统一结构：

```json
{
  "schema": 1,
  "ok": true,
  "code": "service.status",
  "message": "服务状态",
  "data": {}
}
```

约束如下：

- stdout 只输出一份完整 JSON；stderr 承载日志。
- `schema` 不匹配时客户端必须拒绝解析，不能猜测字段。
- `code` 是稳定机器语义，`message` 是用户可读中文说明。
- 敏感读取命令只能由 Root 客户端调用，普通列表只返回安全摘要。
- 写操作使用稳定退出码，并保证 JSON 中 `ok` 与进程退出状态一致。

`netproxy-native` 的 JSON 只供 Shell 内部使用。Android/WebUI 不绕过 `netproxyctl` 直接调用它，以免形成两套公共契约。

## 客户端边界

### Android

Android 按 `core/`、`feature/`、`navigation/` 组织。每个功能域拥有自己的 Repository、ViewModel 和 UI state；应用级长生命周期依赖由 `AppContainer` 组合。Root 命令只从 `NetProxyCtlClient` 发出，页面不拼接 Shell。

### WebUI

WebUI 使用 `ModuleClient` 封装 CLI JSON，使用 `ksu.ts` 封装 KernelSU WebView 能力。开发环境可以提供 mock，但 mock 不得改变生产契约或掩盖非零退出状态。

两端底部一级入口保持“仪表盘 / 节点 / 订阅 / 设置”。节点和订阅在服务停止时仍可浏览；延迟、流量和当前选择等运行状态在服务 ready 后合并。

## 构建与发布

- CI 从 `module.prop` 读取版本，以 Git 提交数写入 `versionCode`。
- 构建动作先测试并交叉编译 `netproxy-native`，再构建 WebUI，最后打包模块。
- 完整包包含 `NetProxy.apk`；Lite 包只排除该 APK，代理能力保持一致。
- Android 管理器不由普通模块 CI 构建或发布，Google Play 是推荐更新渠道；内置 APK 为无 Play 环境保留。
- `update-resources.yml` 统一维护内核、规则、Web 资源、Go/npm/Gradle/Android 依赖；高风险或大版本更新进入报告，不自动静默升级。

## 安全边界

- Catalog 元数据、订阅 Header 和 Provider 权限必须限制为 Root 可读。
- 订阅 URL、节点凭据、HWID、Header 和设备日志不得写入仓库、普通日志或未脱敏诊断包。
- LAN 控制、CORS、Private Network Access 和远程鉴权变更属于安全设计，必须单独评审。
- 配置保存遵循“候选文件 -> 完整检查 -> 原子替换 -> reload”；检查失败恢复磁盘和编辑器状态，不留下半应用配置。
