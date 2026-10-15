# 第一批架构决策

完整目标架构：[NetProxy V9 架构设计规范 Draft v0.1](NETPROXY_V9_ARCHITECTURE.md)。由用户提供的 2026-10-15 原稿完整纳入，文字不改，仅规范化换行和行末空白；原稿状态栏保留其写作时间点，不代表当前已完成 PoC。已确认的范围调整记录如下，不回写或擅自提升原稿中的待验证决策。

## 采用方案

`View -> TanStack Query / feature mutation -> ApiClient -> ApiTransport`。

Transport 只提供类型化路径的 request 和可释放的事件订阅；它传递状态码及 Envelope，不接管业务。当前实际实现只有 Mock 和 unavailable。Browser/Native 是后续可实现的边界，不是空壳实现或已支持平台。

Query 管理权威远端快照；组件维护搜索、展开、弹层和视图模式。主题/语言由轻量 Context 管理。没有 Zustand、全局业务 Store、插件注册器、二次 Repository 或提前拆出的共享 packages。

路由用 TanStack Router 的 Hash history，以便后续静态资源宿主不依赖路径重写。四个一级目的地共用同一业务组件，桌面侧栏和手机底栏只改变导航呈现。除仪表盘外，业务路由懒加载。

## 技术与来源

| 技术                    | 固定版本           | 决策                                                                                                                           |
| ----------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| React / React DOM       | 19.3.1             | 官方稳定版本                                                                                                                   |
| TypeScript              | 6.0.3              | Strict、exactOptionalPropertyTypes、noUncheckedIndexedAccess；与 typescript-eslint 8.71.2 的 `<6.1` peer 范围一致，不强上 TS 7 |
| Vite / React plugin     | 8.3.4 / 6.1.2      | 独立 dist，CSS 与路由拆包                                                                                                      |
| Tailwind CSS            | 4.3.3              | 官方 Vite 插件，无第二套 CSS 框架                                                                                              |
| shadcn CLI / Base UI    | 4.21.5 / 1.9.0     | 官方生成 base-vega，Zinc、Lucide、Geist                                                                                        |
| TanStack Router / Query | 1.170.44 / 5.104.3 | 导航与服务端状态分工                                                                                                           |
| Zod                     | 4.6.7              | 外部数据验证与 DTO 类型推导，避免手写两份类型                                                                                  |
| pnpm                    | 12.11.2            | 精确依赖、独立锁、严格 peer；近期稳定包逐项固定 release-age 例外                                                               |
| Vitest / Playwright     | 5.0.5 / 1.64.1     | 契约/组件与真实浏览器测试                                                                                                      |

shadcn 初始化使用官方 CLI，实际配置位于 `components.json`。UI 文件保留官方实现，不复制 Compose 风格，不手写 Dialog/Select 的焦点与键盘系统。未使用的生成组件已删除。局部主题仅将默认圆角上限收敛至 8px，并加入安全区、100dvh、触摸目标、reduced-motion 和移动滚动约束。

许可证沿用仓库顶层 LICENSE；上游组件仍受各自许可证约束，完整依赖及声明由锁文件/安装包保留。React、Vite、Tailwind、TanStack、Base UI、shadcn、Zod 为 MIT，Lucide 为 ISC，Geist 字体为 SIL OFL 1.1。正式分发前应生成并核对完整传递依赖的 notices，不将本表当完整 SBOM。

官方设计依据：[shadcn CLI](https://ui.shadcn.com/docs/cli)、[Vite 安装](https://ui.shadcn.com/docs/installation/vite)、[Vega 风格](https://ui.shadcn.com/docs/changelog/2025-12-shadcn-create)、[Base UI](https://base-ui.com/react/overview/quick-start)、[Vite 8](https://vite.dev/blog/announcing-vite8)。

## 与总体草案的差异

1. **本批先做 Web 基础，不做 Tauri PoC。** 草案要求 PoC 前置；本次用户明确要求后续任务再做，因此仅保留安全传输边界。没有将 ADR-004 的真实传输验证标为通过。
2. **不新增 `/capabilities`、`/bootstrap` 或新 api_schema。** 当前 Go 只有 `/info` 中的 capabilities 和 schema=1。未来能力契约需要后端单独评审，不能在 Mock 发明接口。
3. **Mock 直接实现 Transport，而不是额外 MSW 服务层。** 本批不连 HTTP，无需增加 Service Worker/拦截层；正式 Browser Transport 的集成测试再决定是否使用 MSW。
4. **不提前引入表单框架、CodeMirror、图表库或虚拟化。** 本批只含基础交互；30 点速率图由有界数据渲染。复杂编辑、大目录与持续流量性能尚未验收。
5. **生产默认无 Transport。** 不先降低 TLS/CORS 或塞主 Secret 来完成演示；`build:demo` 才显式包含 Mock，默认构建进行静态检查和浏览器拒绝连接测试。
6. **不接入正式模块流水线。** 独立 CI 不发布、不读取 Secrets、不替代 V8 检查。Node 24 是 CI 基准，本机使用 Node 26；跨平台产品功能不因此宣称支持。

## 下一任务：Tauri Android PoC

先做一条纵向只读链路，暂不迁移全部页面或修改 Go 持久化。

1. 在独立壳中验证原 applicationId、固定签名、版本递增、Provider authorities 和覆盖升级；使用测试数据证明偏好/凭据保留。失败则停止替换计划。
2. Kotlin 继续承担固定签名的私有 Provider 交付与证书钉扎 OkHttp；凭据留在原生私有存储。Rust/JS 只接收必要公开 DTO，禁止任意 URL、Shell 或返回主 Secret 的桥接。
3. 实现 `ApiTransport` 的 Native 适配：固定 `/api/v1` 白名单、严格方法、4 MiB 正文限制、有界响应，普通读取可取消、写入不重放。Tauri Capability 只授权本插件所需操作，不放开全量 HTTP 或 fs。
4. 先贯通 `/info`、`/service` 与一条 SSE；验证后台取消、前台新快照、旧实例帧丢弃、证书变化、401、慢客户端和 Worker 重启。至少完成 Android 实体机证据，不能只跑桌面 WebView。
5. 测试安全区、软键盘、系统返回键、焦点/触摸；另外逐项核验 Tile、文件导入导出/分享。Browser Transport 的同源会话与独立授权另立安全设计，不复用原生主 Secret。
6. 通过 PoC 后再决定 Browser/Tauri 公共能力发现与原生重连策略的集成；保留 V8 回退产物，待全功能矩阵迁移通过才删除原生管理器。

本批既未实现这些平台功能，也未将 PoC 的待验证项转换为正式支持承诺。

## 首批审查补充：事件连接

本轮用户确认在 Web 基础工程内补齐以下行为；不新增后端接口、不重发写入、不启动 Tauri PoC。

- **连接状态独立于服务状态。** `connecting/live/reconnecting/offline/paused` 是客户端连接元数据，不写入 Go 服务的六态。断线保留最后快照，但全局提示和仪表盘明确标记“非实时”，速率与图表不沿用旧值。
- **完整恢复门槛。** 每次订阅重新等待 status、groups、subscriptions 三份完整快照，30 秒未齐则释放并视作连接失败；流量帧不能宣告连接恢复。ready(A) 直接变为 ready(B) 同样清除流量和历史，并拒绝 A 的后续流量。
- **恢复有界。** 通信失败最多自动重连三次，分别等待 1、2、4 秒；短暂收齐快照不重置预算，稳定 30 秒后才允许新一轮恢复。预算耗尽保持离线，保留主动重连；后台释放订阅和计时器，恢复前台或主动重连启动新预算。不使用轮询兜底。
- **不可恢复错误。** 无效帧不自动重连；Transport 报告的 4xx（408/429 除外）直接离线，避免持续请求无效凭据或契约。真实 TLS、鉴权和原生错误分类仍由后续安全 Transport 验证。
- **同步回调契约。** subscribe 允许返回前发布事件、报告错误或触发生命周期释放；先注销 generation 再关闭，返回后只接管仍有效的关闭函数。Transport 若抛出启动异常，必须自行回收未交付的资源，调用方无法释放没有取得的句柄。

上述重连只建立事件订阅，不调用 mutation 或补发 POST/PUT/DELETE。安全传输、持久化格式和原始架构规范不变。
