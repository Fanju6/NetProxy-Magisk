# V9 第一阶段首批交付报告

日期：2026-10-16。范围：WebUI 工程基础与可交互 Mock，不是完整 V9 或正式模块更新。

## 结果

- 已在 `feat/v9-webui-foundation` 建立独立 `apps/web/`，没有合并 main 或发布。
- 实施验收时的源码基线：`3fd2a1cb9710b59f2723d3c0bb2a01c7090e0998`（v8.6.1）；本报告记录提交前的验证快照，后续提交哈希以 Git 历史为准。
- 完整阅读用户提供的 V9 架构规范，核对 AGENTS、Android 分层与实际页面、Go HTTP/SSE、HTTP 文档、旧终端构建及 CI。迁移依据记录在 [V8 基线](v8-baseline.md)，差异及下一步记录在 [架构决策](architecture.md)。
- V8 Android、Go、模块配置/安装器、终端 WebUI、原有 CI/Release 工作流均无源码差异。原有 `.codex-remote-attachments/` 未跟踪附件保留不动。

## 已实现

| 功能       | 当前行为                                                                                                |
| ---------- | ------------------------------------------------------------------------------------------------------- |
| 导航与布局 | 桌面侧栏、手机底栏、Hash 路由、标题、跳到正文、路由懒加载                                               |
| 设计系统   | 官方 Vega/Zinc/Base UI 组件、Lucide、Geist、明暗/系统主题、统一 Loading/Empty/Error                     |
| 仪表盘     | Mock 服务启停、停服确认、出站模式、流量与 30 点会话历史、内存、连接、当前节点、模块信息                 |
| 节点与分组 | 直接展开/折叠、搜索、网格/列表、确认选择、Auto 实际节点、空组无 Auto、响应式详情                        |
| 订阅       | 本地组和 URL 组列表、节点数、计划摘要与详情；未实现编辑时没有虚假操作按钮                               |
| 设置       | 中英语言、主题、带 revision 的 Mock 开机自启、后续设置结构与明确未迁移标记                              |
| Transport  | request + 事件订阅边界；Mock 与 unavailable 两个实际实现，没有真实 HTTPS/Root 调用                      |
| 契约       | schema=1、HTTP 状态 + ok、严格核心字段、4 MiB UTF-8 正文、revision 冲突、persisted 结果、写入不自动重发 |
| 事件       | 单条前台会话、完整快照、核心启动身份过滤、旧流回调隔离、断线清空流量、取消旧 GET 后发布事件快照         |
| 安全默认   | 演示永久标识；普通生产包不包含 Mock、无可信 Transport 则拒绝连接；不内置或持久化真实凭据                |

UI 的服务状态、选节点、模式与配置改变仅发生在内存 Mock，刷新即重置。模拟数据不证明实际网络已接管。正式模块布局和持久文件没有变化。

## 文件清单

仅修改根级 `.gitignore`（忽略新工程产物）与 `AGENTS.md`（界定 V9 独立边界）；新增独立 `.github/workflows/verify-v9-web.yml`。其余新增文件均在 `apps/web/`：

| 分组      | 文件                                                                                                                                                                              |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 工程      | `package.json`、`pnpm-lock.yaml`、`pnpm-workspace.yaml`、`.npmrc`、`.env.demo`、`components.json`                                                                                 |
| 编译      | `index.html`、`vite.config.ts`、`tsconfig.json`、`tsconfig.app.json`、`tsconfig.node.json`                                                                                        |
| 质量      | `eslint.config.js`、`.prettierrc.json`、`.prettierignore`、`vitest.config.ts`、`playwright.config.ts`、`scripts/check-build.mjs`                                                  |
| 应用      | `src/main.tsx`、`src/styles.css`、`src/app/{context,providers,router,shell,i18n}` 的 ts/tsx 文件                                                                                  |
| API       | `src/api/{contracts,client,queries,events}` 的 ts/tsx 文件                                                                                                                        |
| 页面      | `src/features/{dashboard,nodes,subscriptions,settings}.tsx`                                                                                                                       |
| 共享      | `src/components/{choice,details,states}.tsx`、`src/lib/{format,utils}.ts`                                                                                                         |
| 官方 UI   | `src/components/ui/{badge,button,collapsible,dialog,drawer,input,select,sheet,skeleton,switch,toggle-group,toggle,tooltip}.tsx`                                                   |
| Mock      | `src/mocks/{fixtures,transport}.ts`                                                                                                                                               |
| 单元/组件 | `src/api/{client.test.ts,events.test.ts,events-lifecycle.test.tsx}`、`src/app/i18n.test.ts`、`src/components/states.test.tsx`、`src/mocks/transport.test.ts`、`src/test/setup.ts` |
| 浏览器    | `e2e/{app,production}.spec.ts`                                                                                                                                                    |
| 素材      | `public/netproxy.svg`，复用仓库已有 NetProxy 标志                                                                                                                                 |
| 文档      | `README.md`、`docs/{v8-baseline,architecture,delivery}.md`                                                                                                                        |

代码体量的主要来源是新工程、官方组件与锁文件，不是对 V8 业务的复制：应用/API/Mock/样式约 2.65k 行，官方生成 UI 约 1.02k 行，测试约 0.57k 行，锁文件约 5.17k 行；不含 node_modules、dist、截图。没有删除 V8 生产代码。

## 实际验证

本机：Windows，Node v26.10.0。Scoop 的全局 pnpm 可执行文件异常，因此以下 pnpm 命令实际统一通过 `npx --yes --package=pnpm@12.11.2 pnpm ...` 调用同一固定版本；未修改系统 pnpm 或仓库现有 npm 工程。

| 命令                                                                | 结果                                                                                                 |
| ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `pnpm install --frozen-lockfile`                                    | 通过；558 个锁条目通过 pnpm 供应链策略，未重新解析锁                                                 |
| `pnpm check`                                                        | 类型、ESLint 零告警、Prettier、6 文件 21 项 Vitest 全通过                                            |
| `pnpm peers check`                                                  | 无 peer 依赖冲突                                                                                     |
| `pnpm audit --prod --audit-level=high`                              | 未发现已知生产依赖漏洞；这是扫描结果，不是无漏洞保证                                                 |
| `pnpm build:demo`                                                   | 通过；显式演示包包含 Mock，资源完整                                                                  |
| `pnpm build`                                                        | 通过；生产默认无 Mock，资源完整（Playwright 生产服务器也独立执行此命令）                             |
| `pnpm test:e2e`                                                     | Chromium：桌面 1440x1000、Pixel 7、生产预览；9 项全通过，含 320px/高度缩小、键盘焦点、reduced motion |
| `sh tests/verify.sh quick`                                          | V8 Go test/vet、默认配置、CLI/Catalog、安装/热更新、打包、发布和工作流脚本契约通过                   |
| `sh tests/verify.sh webui`                                          | V8 终端 13 项测试、TypeScript、构建与资源检查通过                                                    |
| `src/android/gradlew.bat testDebugUnitTest lintDebug assembleDebug` | V8 Android 单测、Lint 与 Debug 构建通过；74 个任务，无发行 APK 替换                                  |
| `git diff --check`                                                  | 通过；新文件另行以 no-index whitespace 检查                                                          |

浏览器覆盖：启停确认与取消、启停后 Catalog 可读、选节点、空组、搜索、分组收起、列表切换、模式修改、开机自启、主题/语言跨路由与刷新、详情 Drawer/Sheet 关闭及焦点恢复、生产拒绝连接与无额外网络访问。

单元/组件覆盖：Envelope 与数据错误、HTTP/ok 不一致、4 MiB UTF-8 限制、写入不重放、revision 冲突、persisted 文案、Mock 查询参数/错误码、可取消读取与接受后写入、事件快照与释放、核心身份、旧连接帧、后台释放、慢 GET 不覆盖新事件、中英文键完整性。

截图已人工查看桌面浅色和手机深色布局，并保存在忽略的 `apps/web/test-results/`：`desktop/mobile-dashboard-light.png`、`desktop/mobile-dashboard-dark.png`、`desktop/mobile-nodes.png`、窄屏截图。它们是本次证据，不提交为产品素材。

## 限制与风险

1. **未连接真实 Go API/手机。** Native/Browser Transport、Provider、证书 pin、Android 覆盖升级、真实 SSE 和系统返回均待下一任务；不得以 Mock 通过宣布迁移完成。
2. **没有真实软键盘与刘海设备验收。** 当前仅 viewport-fit、100dvh、安全区 CSS、触摸尺寸与模拟视口高度变化；未运行 iOS/WebKit、实体 Android 或屏幕阅读器。
3. **CI 未在 GitHub 运行。** 工作流独立、不读取 Secrets、不打包发布；本地完成其核心命令，不把未推送的工作流称作线上成功。CI Node 24 与本机 Node 26 的组合仍需远端复核。
4. **Windows Shell 限制。** quick 明确提示 NTFS 权限位与缺少 flock 的互斥断言须在 Linux 执行，不把其余 Shell 通过当这两项已验证。Android 构建另有既存 Gradle 10 弃用提示，本批不升级工具链。
5. **首批功能有限。** 订阅写入、入站/应用/网络/规则/配置/日志/诊断、测速、文件能力未迁移；Mock 不模拟全部原生故障、并发文件锁和回滚。
6. **实时与性能仍需实测。** 固定三个主题、显式断线重连、30 点图、普通节点渲染用于基础验收；未做大目录虚拟化、持续性能预算或真实客户端功耗测试。
7. **完整分发审计未完成。** 未生成传递依赖 SBOM/notices，未做正式包签名或跨平台产品发布。

## 下一步与停止边界

下一项只建议做 [Tauri Android 只读纵向 PoC](architecture.md#下一任务tauri-android-poc)：保持 APK 身份，原生安全交付与钉扎，贯通 `/info`、`/service` 和 SSE，实测前后台及覆盖升级后再安排功能迁移。

本任务到此停止，不自行开发 Tauri、删除 V8、合并 main、修改版本或发布。
