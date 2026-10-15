# NetProxy V9 架构设计规范（Draft v0.1）

> 状态：设计草案，尚未提交 GitHub，尚未完成 PoC 验证
> 日期：2026-10-15
> 仓库：https://github.com/Fanju6/NetProxy-Magisk
> 适用：V9 WebUI、Tauri 客户端、Go 后端、Android Root 模块、未来 Linux / Windows 服务
> 优先级：本文件先定义 V9 **目标架构**；V8 现有契约以仓库根目录 `AGENTS.md`、实际代码与测试为准，变更前需制定迁移方案。

## 0. 决策摘要

**产品定位**：轻量、现代、跨平台的 sing-box 代理管理与网络接管工具。Android、Windows、Linux 共享一套 WebUI 和主要 Go 业务，平台专有的进程、网络、安装能力由平台适配实现。

**三阶段**：
1. **WebUI 与 Android 功能闭环**：建立现代 UI，重构交互，按功能与安全语义对齐 V8 Android，不强行复制旧视觉和导航。同期完成 Tauri Android 技术 PoC。
2. **Tauri 三平台统一壳与 Go 平台适配**：共享前端/原生插件接口，分别落地 Android Root、Linux systemd、Windows Service 与相应网络接管。
3. **质量打磨与稳定发布**：安全、性能、UX、异常恢复、安装升级、兼容性和 CI 全面验收。安全/测试从第一阶段就必须覆盖，第三阶段只做加强。

**选型**：React 19 + TypeScript strict + Vite 8 稳定版本 + Tailwind CSS 4 + shadcn/ui（Vega / Zinc / Base UI / Lucide）+ TanStack Router/Query + React Hook Form/Zod + CodeMirror 6；Zustand、uPlot 等按实际需要引入；pnpm 管理依赖；Tauri 2 跨平台壳；Go 后端复用现有 `netproxyctl` 的领域代码；HTTP API + SSE 继续作为正式业务协议。

**不可退化**：Root 管理能力不得暴露给不可信页面；证书身份验证、配置 revision、持久化提交语义、服务启动身份、防旧事件串台、数据原子性、错误码、日志脱敏与既有用户数据不得因迁移而退化。

### 0.1 已确定与待验证

| 类别 | 决策 | 状态 |
| --- | --- | --- |
| Web 前端 | React/Vite/TS、shadcn/ui 官方 Vega + Base UI | 决定 |
| 三端视觉 | 同一设计系统、移动/桌面自适应布局 | 决定 |
| Go | 单一业务事实源，不使用 Rust 重写已有业务 | 决定 |
| Tauri | V9 统一应用壳，平台差异通过受限原生插件 | 决定 |
| Android API | 保留现有本机 HTTPS + Bearer + 精确证书验证 | 决定 |
| Android Tauri 通信 | Kotlin 插件复用受信任连接和 OkHttp，事件桥采用有界流 | 待 PoC |
| Android 就地升级 | 包名、签名、manifest 组件及数据兼容 | 待真机 PoC |
| Web 浏览器模式 | 与 Tauri 原生通道隔离；独立同源认证，不暴露主 Secret | 需要 ADR |
| Windows/Linux 本地 IPC | 先复用经过身份验证的本机 Go API；是否采用命名管道/Unix socket 由 PoC 评估 | 待 ADR |
| 发布 | 三阶段里程碑；具体版本号由完成度决定 | 建议 |

## 1. 产品原则与非目标

1. **常用操作零学习成本**：开关服务、导入订阅、节点切换、模式修改、测速、查看流量无需理解 sing-box 底层名词。
2. **简单模式与高级配置分离**：普通设置能解释目的和结果；高级入口提供原生 JSON、日志、诊断、平台网络参数。高级能力不能被删除。
3. **一个事实源**：业务配置、Catalog、运行时状态及日志由 Go 管理；前端缓存是视图状态，不是第二套持久化真相。
4. **同功能、不同布局**：手机底部导航和易点击控件；桌面 Sidebar、分栏、键盘操作与高密度列表。不要因为响应式而隐藏功能。
5. **先可维护，再抽象**：业务按功能域组织；没有实际第二个使用者时，不提前抽出全局库或通用插件框架。
6. **先稳定，再前沿**：优先维护中的稳定技术；试验性编译器/协议不得成为 V9 发布的硬依赖。
7. **向后兼容是明确设计任务**：升级、备份、回滚、数据保留、老模块/新壳版本协商都要定义；不以“新版本”作为格式破坏借口。

**V9.0 非目标**：多设备云控制台、远程公网面板、账号同步、iOS 客户端、自己重写代理核心、统一 Linux/Windows/Android 分应用代理实现、引入前后端两套业务数据库。未来需求由单独 ADR 决定。

## 2. 现有仓库基线（2026-10-15）

当前源码显示：
- `src/native/netproxy/internal/httpapi/` 提供 `/api/v1` JSON 管理 API 与 SSE，`src/native/netproxy/internal/apiconfig/` 配置 loopback HTTPS、每安装随机 Secret 与自签证书。
- `src/android/.../core/api/LocalHttpTransport.kt` 用 OkHttp 发送 Bearer，精确校验服务端证书；`ConnectionProvider` 接收受权限限制的私有凭据交付。
- `docs/config/http-api.md` 已定义 revision 冲突、`persisted=true`、断连后的状态复核、SSE 核心启动身份、4 MiB 请求上限等。
- `httpapi.New` 当前直接构造 Android 平台查询客户端，`/android/apps`、`/auth/provision` 与通用 API 尚未完全隔离。
- `module/lifecycle.go` 的 `Boot()` 等待 Android `sys.boot_completed`；Windows 进程管理实现仍有占位逻辑，**目前不等于完整跨平台可用**。
- `src/webui/` 是基于 Vite/TypeScript 的终端式模块 WebUI；V9 的产品 UI 将新建，不在旧终端 UI 上逐层叠加。
- `AGENTS.md` 是现有开发规则权威文件，V9 分支迁移时需要同步更新，不能直接用本草案覆盖 V8 规则。

主要源码入口：
- [HTTP API 文档](https://github.com/Fanju6/NetProxy-Magisk/blob/main/docs/config/http-api.md)
- [Go API Server](https://github.com/Fanju6/NetProxy-Magisk/blob/main/src/native/netproxy/internal/httpapi/server.go)
- [Android HTTP Transport](https://github.com/Fanju6/NetProxy-Magisk/blob/main/src/android/app/src/main/java/com/fanjv/netproxy/core/api/LocalHttpTransport.kt)
- [Android 架构说明](https://github.com/Fanju6/NetProxy-Magisk/blob/main/src/android/ARCHITECTURE.md)
- [根级开发规范](https://github.com/Fanju6/NetProxy-Magisk/blob/main/AGENTS.md)

## 3. 逻辑架构

```text
                  ┌──────────────────────────────────┐
                  │ React WebUI（统一页面/领域 Hook） │
                  │  shadcn/ui  + TanStack Query/SSE │
                  └─────────────────┬────────────────┘
                                    │ ApiTransport / PlatformBridge
                 ┌──────────────────┼─────────────────────┐
                 │                  │                     │
        Android Tauri       Linux Tauri           Windows Tauri
        Kotlin 插件         Rust 平台集成           Rust 平台集成
                 └──────────────────┼─────────────────────┘
                                    │ 受认证本地通道
                        ┌───────────▼─────────────┐
                        │ Go HTTP API / 事件服务 │
                        │ 配置、Catalog、订阅、日志│
                        └───────────┬─────────────┘
                                    │
                   Go 领域层 / Service API Client
                                    │
             ┌──────────────────────┼───────────────────────┐
             │                      │                       │
       Android Root        Linux platform            Windows platform
       boot/eBPF/TUN       systemd/TUN/route         SCM/TUN/route
             └──────────────────────┼───────────────────────┘
                                    │
                               sing-box
```

### 3.1 不允许出现的反向依赖
- React 页面不得直接调用 Shell、Root、文件系统或任意 Tauri invoke；只能通过被明确授权的 Transport / PlatformBridge。
- Tauri 插件不得再实现 Catalog、订阅、节点转换、配置事务及核心业务状态机。
- Go 领域包不得依赖 React/Tauri 的页面或窗口状态。
- 平台专属网络与进程管理不得污染通用业务模型。
- Android 业务页不得直接读取 `/data/adb` 或绕过 API 修改持久文件。

### 3.2 两类桥接 API，职责分明

```ts
interface ApiTransport {
  request<T>(method: HttpMethod, path: ApiPath, body?: unknown, options?: RequestOptions): Promise<T>;
  events(topics: readonly EventTopic[], onEvent: (event: ApiEvent) => void): () => void;
  download(path: ApiPath, options?: DownloadOptions): Promise<DownloadedFile>;
}
interface PlatformBridge {
  selectFile(options: FilePickerOptions): Promise<SelectedFile | null>;
  shareFile(file: DownloadedFile): Promise<void>;
  openExternal(url: string): Promise<void>;
  getAppearance(): Promise<Appearance>;
}
```

> 这是概念契约，具体字段、版本和事件取消语义需要 PoC 冻结。不要把 `path` 变成任意地址转发器。所有 Transport 只接入受信任的 Go 后端。

**BrowserTransport**：供开发 Mock 和未来明确启用的浏览器模式。浏览器真实管理模式需使用同源 UI、独立会话与 CSRF 防护，不能把 Go 主 Secret 注入 JS。

**TauriTransport**：Rust/Kotlin 层执行证书校验与授权请求，受能力范围约束，只能访问本机已验证的 API。大量数据和 SSE 不应无界桥接。

### 3.3 进程生命周期
- **Android**：现有 Root 模块的单一 Go Worker/HTTP 服务独立于 Tauri UI 生命周期；退出壳不停止代理。Android 证书钉扎及私有 Provider 交付保持不变。
- **Linux**：Go 后端作为 systemd 服务运行，Tauri 是控制端。先实现非 GUI 服务运行与故障恢复，再集成安装器。
- **Windows**：Go 后端作为 Windows Service 运行，Tauri 是控制端。实现真实 SCM 生命周期、健康检测、关停与网络回滚，不能把 `Kill()` 或 PID 猜测作为稳定设计。
- **所有平台**：sing-box、Go 管理服务、UI 三种生命周期独立；UI 关闭不能导致网络失控。服务停止时 Catalog 和配置管理 API 仍可使用。

## 4. UI / UX 设计规范

### 4.1 设计令牌
- 官方 shadcn/ui **Vega + Zinc + Base UI + Lucide** 为设计基线；Tailwind 4 / CSS Variables 管理亮暗主题，避免混搭 MUI/Ant Design/另一套设计系统。
- 优先直接使用官方 Button、Input、Switch、Dialog、Drawer、Sheet、Popover、Tabs、Select、Tooltip、Sidebar 等组件；修改官方组件需注明原因。
- 整个 App 使用统一间距、层级、阴影、圆角、字号与动效时长；不要每个功能页自行定义设计变量。
- 品牌色保持克制；状态颜色只用于真实状态、失败、警示，不把所有统计卡片涂彩色。
- 国际化从第一天支持中英；字符串不得散落在页面代码；其他语言按后续维护能力扩展。

### 4.2 移动和桌面约定
- 手机：固定紧凑标题栏、底部导航、单列优先、Bottom Sheet、输入时键盘避让、Android 安全区；关键操作无需多层跳转。
- 桌面：Sidebar + 分栏、右侧详情面板、快捷键、键鼠操作、支持更高密度；保持与手机**同等业务功能**。
- 节点和策略组：**就地展开**并提供列表/网格两种视图，不强制进入新页面才能选择。
- 页面需有加载、空内容、操作中、成功、部分成功、失败、离线、模块不存在、凭据失效等独立状态。
- 危险操作需有范围明确的二次确认；普通高频操作避免连续确认弹窗。
- 尊重 `prefers-reduced-motion`；提供可见键盘焦点、屏幕阅读器语义和色彩对比。

### 4.3 数据和状态管理
- TanStack Query 管理远端状态、读取缓存、失效与复核；SSE 更新或失效对应 Query；不让每个组件单独建立订阅。
- Zustand 仅用于必要的跨页本地 UI 偏好；输入框草稿和弹窗状态先用 React 局部状态。
- 服务启停、配置保存、节点选择不因接口发起就显示确定成功；必须基于 Go 确认结果。
- 不能丢弃用户未提交草稿；`config.conflict` 应保留草稿并要求显式处理；`persisted=true` 显示“已保存但运行时应用失败”。
- 日志和应用列表使用虚拟滚动；复杂编辑器按需加载；高频流量状态不能拖动整个 App 重渲染。
- Mock Transport 使用与真实 API 一致的类型与错误码，显著标识 Mock，生产包不能默认进入 Mock。

## 5. API 契约与协议演进

### 5.1 绝对保留
- 既有 `/api/v1` 与 `schema=1` 成功/失败模型、稳定错误码。
- `revision`、分区写入、配置事务、锁顺序、原子提交、恢复日志等 Go 现有安全语义。
- 写操作被接受后客户端断连不意味着取消或失败；不得自行无条件重试 POST/PUT/DELETE。
- SSE 快照 + 心跳 + 断线重新获取；验证 `instance_id` / `core_started_at_millis` 避免新旧核心串台。
- 上传正文 4 MiB 现有限制；诊断包由后端脱敏；文件路径不直接来自不可信网页。

### 5.2 V9 优先新增（设计目标，不是已有 API）

| 建议端点 / 功能 | 用途 | 优先级 |
| --- | --- | --- |
| `GET /api/v1/capabilities` | 平台、架构、支持/可用功能、不可用原因 | P0 |
| `GET /api/v1/bootstrap` | 轻量应用首屏所需信息与版本协商 | P1 |
| OpenAPI 描述与 TypeScript 类型生成 | 防止 Go / 前端契约漂移 | P0 |
| 统一认证上下文与作用域 | 原生壳与未来浏览器会话使用同一权限判断入口 | P0 |
| 幂等任务模型与 `task_id` | 长时间订阅更新、批量测速、诊断等 | P2（复用现有接口直到必要） |
| 增量日志 / 分页 / 按需资源 | 长日志和大型 Catalog 性能 | P1/P2 |

`capabilities` 必须区分 `supported`、`available` 和 `reason`，UI 禁止仅通过平台字符串推断网络功能。平台相关 API 可使用明确的命名空间；不适用的功能要返回可解释的错误，而不是伪造空成功。

### 5.3 SSE 的建议
- 第一阶段仍使用 SSE，不为了“先进”改用 WebSocket。
- 一个前端会话内共享一个事件连接，合并关注主题；离页或后台取消不再使用的高频订阅。
- 后端根据实际订阅者数量治理资源，保持有界队列、心跳、后压/慢客户端断连。
- 如引入事件序号，需定义断线后全快照重建规则；不要误称可恢复完整事件历史。

### 5.4 未来浏览器访问（独立 ADR）
- **不是第一阶段实现前提**，前端可先使用 Mock 和受信任 Tauri 通道。
- 真实浏览器访问采用 Go 同源托管静态 WebUI、受保护的配对/登录会话、服务端权限与 CSRF 防护；生产 HTTPS 与证书信任需正确部署。
- 现有 Android `api.json` 的单一 Bearer 仅限原生受信任连接，不应给普通网页。
- 遵循 Host/Origin 检查、防 DNS Rebinding、会话撤销、限速和敏感信息屏蔽；远程访问默认关闭。

## 6. 平台抽象与差异

按能力拆出 **ServiceController、NetworkController、ApplicationProvider、DeviceCapabilities、HostPaths** 等小接口，而不是巨大的全能 `PlatformAdapter`。先保留现有 Go 包和测试；当第二平台确实需要时逐步抽取。

| 能力 | Android | Linux | Windows |
| --- | --- | --- | --- |
| 后端驻留 | Root Worker | systemd | SCM Service |
| 透明代理 | eBPF / TUN | TUN + auto_route / auto_redirect（按实际环境） | TUN + 路由/DNS 管理 |
| 应用策略 | UID / 包名 / 多用户 | 单独评估 UID/cgroup 等 | 单独评估进程/过滤能力 |
| Wi-Fi 策略 | 当前 Android 实现 | 平台能力评估后加入 | 平台能力评估后加入 |
| 原生 UI 集成 | Kotlin 插件 | Tauri/Rust | Tauri/Rust |

平台持久化目录、配置文件权限和运行时目录应由对应平台布局接口提供；不可把 `/data/adb`、`/dev/netproxy` 硬编码到其他平台。跨平台统一的是产品概念，不是未经验证的底层网络机制。

## 7. 安全与信任边界（P0）

1. WebUI 不保存主 Bearer Secret，也不得在 URL、日志、埋点、浏览器持久化存储或错误报告中显示凭据。
2. Android 保留既有私有签名校验的 Provider 交付与 OkHttp TLS pinning；禁止 `onReceivedSslError(...).proceed()` 和 `trustAllCerts`。
3. Tauri 只加载打包可信静态资源；Capability/Permission 最小授权，禁用任何可使 UI 执行任意系统命令的通用桥。
4. Go 原生 API 仅监听受限本地地址，具备认证、Host/Origin 检查及请求大小上限；与未来浏览器会话分离。
5. Rust/Kotlin 插件只提供白名单操作及约束类型；对路由、文件路径、URL、下载大小进行验证。
6. 禁止无鉴权局域网面板；若未来开放远程控制，另立安全威胁模型与审查里程碑。
7. Android 保持 APK 原签名与权限兼容评估；Windows/Linux 需明确后端特权范围，UI 不应长期运行管理员/root 身份。
8. 服务异常关闭必须验证 DNS、路由、代理开关及防火墙清理结果，不能仅依据进程退出判断恢复成功。
9. 默认 CSP、无远端动态 JS、外链走系统浏览器，拒绝把订阅元数据渲染为未过滤的 HTML。
10. 新增依赖记录许可证与来源，CI 检查锁文件、依赖漏洞和产物完整性。

## 8. 目标目录与演进方式

```text
apps/
  web/                       # 新 React UI（唯一业务 UI）
    src/app/                 # Router, Providers, 应用外壳
    src/features/            # dashboard, nodes, subscriptions, inbound,
                             # routing, logs, settings ...
    src/components/          # 官方 shadcn/ui + 少量领域组件
    src/api/                 # 领域 Hook / ApiTransport / DTO
    src/platform/            # 最薄的原生能力适配器
    src/mocks/               # Mock 数据与测试服务
    src/styles/              # design tokens
  shell/                     # Tauri 2（Android / Linux / Windows）
    src-tauri/               # Rust 与 Capabilities
    ...                      # Android Kotlin 原生插件
src/native/netproxy/         # 沿用现有 Go 模块，渐进拆平台接口
src/module/                  # 沿用 Android Root 模块
src/android/                 # 迁移期保留 V8 Compose 对照与测试
src/webui/                   # 迁移期保留 V8 终端 WebUI
contracts/                   # 当稳定 OpenAPI 契约形成后添加
tests/                       # 集成、契约、升级、安装验证
```

**不要**在第一项 PR 中大迁移所有目录；先建 `apps/web` 和 Tauri PoC，保留 V8 的构建/发布路径与测试。共享 `packages/` 仅在确认第二个真实复用方后引入，避免抽象膨胀。

## 9. 版本兼容与升级

- 第一阶段先录制 V8 功能矩阵和关键 API fixtures；接口变化先补契约，再更新前端。
- 模块数据是长期资产：升级不能清空 Catalog、订阅 URL/Header、配置、用户策略、密钥；修复策略保持损坏数据可恢复。
- 采用明确的 `api_schema` 与产品构建版本区分，启动时检测壳/API 兼容性；不支持时展示可解释的升级提示。
- Tauri Android 真机 PoC 核验：**原 applicationId、固定签名、版本号、安装覆盖升级、Provider authorities、QS Tile、文件分享、数据保留与启动**。无法满足就不能声称无损就地替换。
- V8 -> V9 的降级/回滚能力单独定义；新持久格式改变前需做可恢复备份和升级测试。
- Linux/Windows 安装器要有安装、升级、卸载三种路径，且卸载时复核 DNS、路由和代理设置。

## 10. CI、测试与可观测性

**PR 必须通过**：
- Web：tsc strict、Lint、Vitest、组件交互和关键页面截图；Mock 与真实契约一致。
- Go：单元/竞态测试、API handler 测试、revision/持久化场景与 SSE 重连测试。
- Android：Kotlin/Rust 插件单元测试、Android 构建、至少一套真机烟测（签名、配对、SSE、QS Tile、键盘、前后台）。
- Linux/Windows：交叉编译仅作编译检查；系统服务、TUN、DNS/路由恢复必须使用对应操作系统测试。
- 发布：依赖锁定、许可证、静态资源 hash、包签名、升级与回滚测试。

**重点性能观测**：首次渲染、页面切换、节点批量渲染、流量图更新、应用图标冷缓存、WebView 前后台恢复、Go RSS/CPU/Goroutines/FD；先测 V8 基线，再设 V9 可执行预算，不凭空承诺数值。

## 11. 交付阶段与退出条件

### Phase 1 — 全功能 WebUI + Android
- P0 技术 PoC：Tauri Android 能启动；保留官方 APK 身份并能连接模块；证书钉扎/Provider 交付可运行；SSE 可以稳定关闭、重连、恢复。
- 设计系统：Vega/Zinc/Base UI 统一主题、中英文本、导航和 responsive shell。
- 功能迁移：仪表盘、服务、节点/分组、订阅、测速、入站、分应用多用户、Wi-Fi、规则、配置/Schema、日志/诊断、设置/关于。
- 核心行为：服务停止时浏览节点与修改配置；离线提示；revision 冲突；保存部分成功；文件导入导出；Mock。
- **退出门槛**：V8 已有关键能力全覆盖，且 V9 Android 能通过完整回归；不要求复制旧 Compose 外观。

### Phase 2 — Tauri Android / Linux / Windows
- Android 正式替换旧 UI 壳，确认覆盖升级；Root 后台不随壳关闭。
- Linux Go 守护程序、systemd、TUN、网络和 DNS 恢复、浏览器/Tauri 客户端，完成自动化安装与卸载。
- Windows Go Windows Service、TUN、网络和 DNS 恢复、Tauri 壳与打包；**不得将占位实现当作已支持**。
- 平台能力发现和功能提示，跨平台配置迁移与统一诊断。
- **退出门槛**：三端均可独立安装并实际代理流量；UI/服务开关与故障恢复均经过原生系统测试。

### Phase 3 — 稳定发布打磨
- 交互细节：软键盘、导航、无障碍、列表滚动、深浅色、桌面窗口/托盘。
- 可靠性：断连、内核崩溃、网络切换、系统休眠、后台恢复、升级、卸载与回滚。
- 性能、依赖治理、产品文档、诊断、安全检查、CI 与 Release 流水线。
- **退出门槛**：有证据证明可恢复、可更新、可维护，达到正式发布标准。

## 12. 首批决策记录（ADR）

| 编号 | 主题 | 决策/需要验证 |
| --- | --- | --- |
| ADR-001 | 前端 | React + Vite + shadcn/ui Vega/Base UI，移动/桌面共用 |
| ADR-002 | 客户端壳 | Tauri 2 统一壳；P0 Android PoC 优先 |
| ADR-003 | 后端 | Go 为事实源；Rust/Kotlin 仅系统能力与可信 IPC |
| ADR-004 | Android HTTPS | 保留 Provider + pinned OkHttp；禁止不安全证书例外 |
| ADR-005 | Browser access | 后续设计同源会话；不暴露原生主 Secret |
| ADR-006 | 实时机制 | SSE 为默认；有明确双向需求时再评估 WebSocket |
| ADR-007 | 平台能力 | `/capabilities` 作为唯一能力发现来源 |
| ADR-008 | 存储迁移 | V8 持久化安全语义优先于 UI 重构速度 |

## 13. 第一批开发 PR（建议顺序）

1. **PR-01 架构基线与测试夹具**：功能矩阵、API 样例、历史行为金丝雀测试；无运行时行为变化。
2. **PR-02 前端工程初始化**：`apps/web`、稳定依赖锁、严格 TS、ESLint/格式化、Vitest/MSW/Playwright、Vega/Zinc/Base UI、Mock。
3. **PR-03 Tauri Android 最小可行验证**：静态页面、同包名/签名验证、调用只读 `/info`、凭据交付、证书钉扎与 SSE；保留原 Android 管理器可回退。
4. **PR-04 typed API Transport**：浏览器 Mock / Android Tauri 实现、错误码与写事务状态对齐，SSE 会话共享与重连。
5. **PR-05 页面外壳与仪表盘**：移动/桌面导航、主题、服务状态/启停/流量、错误状态、无障碍。
6. **PR-06 节点、分组和订阅**：单节点、订阅 CRUD、导入、更新、测速、选择器、详情。
7. **PR-07 入站、分应用、Wi-Fi**：能力与表单、原生应用图标、安全配置保存。
8. **PR-08 规则、配置编辑、诊断**：CodeMirror、分区 revision、冲突与持久化结果、日志导出。
9. **PR-09 完整 Android 回归**：旧功能逐项验收，CI 全绿，决定 Compose 删除时机。

> PR 粒度以风险和真实文件依赖调整；PoC 不应为了单次演示绕过安全。拒绝“先将 Secret 放 localStorage，正式版再改”的路线。

## 14. Agent/贡献者规则

- 开发前阅读 `AGENTS.md`、相关实际代码与测试；本草案不是对现有代码行为的凭空替代。
- 每项变更必须写明：解决什么问题、哪些模块受影响、是否改变契约、是否改变持久化、如何测试/回退。
- 不进行全仓库无关重命名、自动格式化或依赖批量升级。
- 重大架构变化先以 ADR 写明候选方案、风险与被拒绝方案，再编码。
- 复杂功能应先创建 Go/前端对应单元测试或契约 fixture，避免只靠 UI 模拟验证。
- 禁止为快速修复而放开 TLS、Origin、Root、Host 或授权范围。
- 不得声明某个平台“已支持”，除非该平台的安装、网络接管、恢复和卸载均真实通过测试。

## 15. 外部设计依据

- Tauri 2：移动 Kotlin 原生插件 https://v2.tauri.app/develop/plugins/develop-mobile/
- Tauri 2：Capability 和最小授权 https://tauri.app/security/capabilities/
- shadcn/ui：2026-07 Base UI 默认 https://ui.shadcn.com/docs/changelog/2026-07-base-ui-default
- shadcn/ui：官方 Vega 风格 https://ui.shadcn.com/docs/changelog/2025-12-shadcn-create
- Vite 8：稳定发布与 Rolldown https://vite.dev/blog/announcing-vite8.html

---

**审阅提示**：本规范可以作为 V9 的首版方向性文档；在进入编码前，必须以 PoC 输出确认 Android Tauri、安全传输与原地升级可行性，并将相应 ADR 从“待验证”更新为“已决定”。
