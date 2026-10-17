# 第一批架构决策

完整目标架构：[NetProxy V9 架构设计规范 Draft v0.1](NETPROXY_V9_ARCHITECTURE.md)。由用户提供的 2026-10-15 原稿完整纳入，文字不改，仅规范化换行和行末空白；原稿状态栏保留其写作时间点，不代表当前已完成 PoC。已确认的范围调整记录如下，不回写或擅自提升原稿中的待验证决策。

## 采用方案

`View -> TanStack Query / feature mutation -> ApiClient -> ApiTransport`。

Transport 只提供类型化路径的 request 和可释放的事件订阅；它传递状态码及 Envelope，不接管业务。第一批实现 Mock 与 unavailable；第二里程碑新增受限的 Android Native 只读 Transport，见 [原生壳说明](../../shell/README.md)。Browser 仍是未实现的边界，Native PoC 不代表全部业务迁移。

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

## 第三里程碑：可信配对与生命周期

实现与证据见 [安全 ADR](../../shell/docs/adr-001-trusted-pairing.md) 及 [验收报告](../../shell/docs/trusted-pairing-report.md)。新增的匿名初始请求只触发固定官方目标的 Root 私有交付，不从初始响应获取或信任证书；业务请求仍须精确钉扎。没有新增 Go 数据结构、JS Secret、业务写入 IPC 或第二套重连。

Kotlin 继续直接编译 V8 安全源码，补齐宿主实例重绑和代际任务释放；JS pagehide 取消自身读取与订阅。原生 CI 独立验证 Debug 包和测试探针，不读取正式签名 Secrets、不改变 V8 发布。真机功能性重建通过与长期零泄漏是不同结论，上游 Activity 引用及多 ROM 验收限制保留在报告中。本轮不开始完整业务迁移。

## M4.1：服务控制与操作能力

- `ApiTransport.capabilities` 取代粗粒度 `readOnly`。当前 Native 只有 `info.read/service.read/service.start/service.stop`；节点和订阅仍来自 SSE，未开放其写入。Mock 保留原有演示操作，普通生产仍拒绝连接。
- 真实写入只有 `service_action(start|stop)`。React、ApiClient、NativeTransport、Rust 枚举与 Kotlin 固定路径分别检查；Capability 不提供通用请求、Root、Shell 或文件访问。Kotlin 直接复用 V8 的钉扎、Bearer、Provider 与非重放写入。
- 服务操作的短生命周期元数据保存在现有 QueryClient，不新增全局 Store。同步登记意图防止跨组件重复提交，离页不取消已接受操作；原生后台/销毁取消的是本次等待，不代表 Go 回滚。
- 不发布 POST 返回状态的乐观快照。操作结束或等待中断后先更换事件代际、GET `/service` 复核，再建立新 SSE；旧回调即时失效。现有 Go 没有事件序号，不能用客户端到达时间给旧流和新 GET 排序。
- `401/403/409/429` 等明确拒绝与断线、取消、`502/504` 等不确定结果分开。`persisted=true` 始终保留；不回滚数据、不重发写入、不自动 restart/reload。未取得业务响应时，即使临时看到 failed，也不能认定操作已经结束；目标状态可确认完成，其他终态须等到有界观察窗口后再复核。
- 观察窗口采用 150 秒，覆盖现有 120 秒 Go 操作预算与 135 秒原生调用等待并留出余量。它只限制客户端等待保护，不是服务端回滚或任意内部步骤一定结束的承诺；窗口到期不触发 POST，也不靠定时轮询查询。

完整验证与限制见 [M4.1 验收报告](../../shell/docs/service-control-report.md)。原始目标规范不回写，本轮不开始其他业务迁移。

## M4.2：节点、模式与测速

- 原生资源新增 `nodes/selection/mode`；写入仅新增 `select_node/change_mode/test_delay`，Rust 参数类型和 Kotlin 固定路径独立约束。节点编辑、删除、订阅、配置和任意 HTTP 仍未开放，安全传输直接复用 V8 源码。
- 持久选择使用 `group_id` 与原始 `node_tag`，Auto 使用空 tag。GET selection 的手动 `selected` 是持久引用，PUT 返回的同名字段却是运行时标签，不能将 PUT 响应发布为完整快照。运行时关联使用 `group.runtime_tag`，名称和节点 tag 中的斜杠不拆解。
- 节点与模式共用一个短生命周期写入槽，但分别复核 selection/nodes 与 mode/service；不将服务启停的状态机扩展成通用事务框架。所有写入显式禁止重试，断线、取消和无效响应先保留待确认；已保存的运行时失败保留原始错误及 persisted，不回滚 UI。
- PUT 响应不等于权威状态确认：只有后续读取匹配持久目标才结束待确认，明确业务拒绝立即返回原错误。150 秒观察窗口耗尽后进入 `unconfirmed`，保留原始错误及 persisted、显示无法确认并解除排他保护；计时器只结束观察，不查询、不重发写入。后续匹配快照仍可核对目标，但没有操作 ID，不能证明究竟是哪位客户端写入了当前值。
- 服务与路由的 pending/uncertain 双向互斥：Hook 在调用 Transport 前同步读取并登记现有 QueryClient 槽位，同一事件循环也只准入首个操作。到期但未确认的路由结果不永久阻塞后续操作；Kotlin 的共享写入槽仍作为独立防线。
- 应用模式可能重建核心，复核先 GET service 确认身份，再读相应资源。SSE Hub 的新订阅可能重放同核心旧缓存：routing 操作后的连接先消耗完整初始快照，再做权威读取；期间不发布旧 status/groups/mode，复核完成才标记实时。不新增重连引擎，不改变没有 routing 操作时 M4.1 的服务启停流程。
- Go 未提供跨资源 revision 或事件序号，上述边界不构成多客户端线性一致性承诺。后续合法外部修改仍以 Go 的新快照展示，不能为保护自己的选择而永久拒绝不同的服务端状态。
- 测速是独立的可取消页面任务，不放入持久写入确认槽。响应必须匹配请求的运行时 target、分组及节点集合；url_test_time 为 Unix 秒。有效延迟与时间保留，成功仅说明取得最近可用结果，未返回有效结果不等于测完全部节点。离页/后台取消等待，不会撤回 Go 已接受的有界测试，也不补发。
- 核心身份变化清除运行时选择和测速缓存；初始 groups 先于 status 时暂存，不能让无核心身份的初始帧覆盖已知实例。普通 Web 生产默认仍拒绝连接，Mock 与 Native 构建继续隔离。

实现、功能矩阵和实际验证见 [M4.2 验收报告](../../shell/docs/core-controls-report.md)。本阶段不接入其他业务。

## M4.3.1：Catalog 与订阅只读

- “资源”页复用当前导航，标题仅位于紧凑顶栏，不重复网页式大标题。GET `/catalog` 与 `/subscriptions` 使用相同摘要缓存，真实 Go 类型为 `local/subscription`，不保留错误的 Mock `url` 类型别名。
- `subscription_read` 只开放固定 GET 详情、历史、进度，Rust 与 Kotlin 独立校验分组 ID。私有参数、任意 URL/方法和全部订阅写入均未授权；原生投影在交付 JS 前剔除未知字段、敏感设置、官网 URL 与原始异常，GET 和 SSE 共用这一边界。
- 继续使用现有 SSE/QueryClient：每次会话更换取消资源旧读取；订阅首帧只触发权威读取，不覆盖新缓存。后续摘要按 revision/updated_at 防回退，分组增删重新读取，详情随摘要失效；断线保留带非实时标记的快照。
- 详情仅在前台连接实时且可见时读取。只有活动阶段每 5 秒读取进度，单次观察最多 60 秒，失败、离页和后台停止；手动刷新或新前台会话可以重新观察。进度消失另读历史与详情，不自行断言成功，也不虚构任务 ID、百分比或跨事务排序。
- Go 缺少全局 Catalog revision、操作 ID 和进度单调序号；同 revision、同秒的外部并发修改不能形成线性一致性证明。当前实现不为消除该限制更改 Go 或引入第二套重连与轮询框架。

实际测试、真机样本限制及下一步设计建议见 [M4.3.1 验收报告](../../shell/docs/resources-report.md)。
