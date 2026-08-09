# NetProxy Agent Guide

本文件是仓库内自动化编码代理的根级约束，也是跨组件架构的唯一权威说明。开始修改前先阅读与任务相关的源码和本文件对应章节；Android 内部分层见 [src/android/ARCHITECTURE.md](src/android/ARCHITECTURE.md)，贡献流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

前半部分是编码约束，后半部分「架构参考」记录事实源、状态机与契约细节。

本文件只收录「违反后编译和测试都不报错、但运行时会静默出错」的约束。能被 `go vet`、`tsc`、Gradle lint 或现有测试拦住的规则不写在这里。

## 项目边界

- `src/module/`：Magisk、KernelSU 与 APatch 模块，包含生命周期脚本、`netproxyctl`、sing-box 配置、资源和打包内容。
- `src/native/netproxy/`：模块专用 Go 组件，负责节点转换、Provider、订阅、配置、eBPF 运行时、Service API 与唯一允许的订阅 Worker。
- `src/webui/`：React + TypeScript 终端式 WebUI，构建产物写入 `src/module/webroot/netproxy/`。
- `src/android/`：Android 管理器，使用 Compose、miuix、Navigation3 和内置 Scripta 源码快照。
- `docs/`：VitePress 用户文档；`tests/`：Shell 契约与运行时回归测试。

`src/module/` 与设备上的 `/data/adb/modules/netproxy/` 1:1 对应，改脚本即改部署布局。

不要恢复 8.0 已废弃的旧节点目录、`CURRENT_CONFIG`、`_meta.json`、TPROXY/iptables/IPSET 数据面或旧 CLI——8.0 起透明代理入站只有 eBPF 一条数据面。除非任务明确要求，不为旧版配置增加迁移和兼容分支。

## 核心契约

- `netproxyctl` 是终端、Android 和 WebUI 的唯一模块管理入口；`netproxy-native` 是内部实现，不是平行的公开 CLI。
- 机器接口固定使用 `schema=1` JSON。stdout 只能包含结果 JSON，日志与诊断写 stderr；字段、错误码或状态语义变化必须同步检查 Shell、Go、Android、WebUI 和测试。
- Catalog 是持久节点事实源：每组使用 `data/catalog/<group-id>/meta.json` 与 `provider.json`。`staging/` 只存事务临时文件，不得作为持久状态读取。
- `ACTIVE_GROUP_ID` 保存分组 ID；`SELECTOR_MODE` 只允许 `urltest` 或 `manual`；`SELECTED_NODE_REF` 只在手动模式保存 `<group-id>/<tag>`。
- Provider 的运行时显示标签来自分组名称；名称冲突时才附加分组 ID。用户界面不得直接显示 UUID 代替可读名称。
- 自动选择必须落到 `Auto/<group>`，Provider/selector 的默认值绝不能静默回退到 `direct`。
- eBPF 是 sing-box 的入站实现，不是独立代理核心。服务、模式和节点切换文案继续使用“服务”或“sing-box”，不要泛化为“eBPF 服务”。
- 分应用策略持久化包名与 Android 用户范围，运行时直接生成 `include_package` / `exclude_package` / `include_android_user`；不得恢复模块侧包名转 UID 或 `user:package` 格式。
- `EBPF_CGROUP_ENABLED=0` 时只允许 shared-network 数据路径，运行时不得输出 cgroup 路径、IPv6 模式、应用/UID 策略或本机 Map 配置。
- Service API 与 Clash API 的固定监听和密钥位于 `02_experimental.json`、`08_services.json`。不要重新引入运行时随机 bootstrap，现有 WebUI 依赖固定入口。
- 服务状态只允许 `stopped/preparing/starting/ready/stopping/failed`。`ready_at` 只能在 sing-box API 与 eBPF 入站均就绪后写入。

## 命令入口与脚本布局

- `src/module/netproxyctl` 只负责定位 `bin/netproxyctl`；公共实现位于 `src/native/netproxy/cmd/netproxyctl`。Shell 不再保留公共命令 dispatcher。
- 命令组权威清单：`service catalog node sub mode network app ebpf config logs`。新增命令组必须同时更新 Go CLI、Android `NetProxyCtlClient`、WebUI `src/exec.ts` 和契约测试。
- `scripts/utils/` 不承载运行时业务；配置、Catalog、状态和 Service API 业务统一由 Go 实现。
- `scripts/core/` 只保留 `service.sh`；运行时配置、节点切换、订阅事务和调度由 `netproxy-native` 负责。
- Go Worker 负责 Android 网络变化采集、Wi-Fi 状态读取和策略评估。
- 设备上的调用形式是 `su -c /data/adb/modules/netproxy/netproxyctl [--json] <命令组> <命令>`；文档和排查步骤按此形式给出，不要写成裸 `netproxyctl`，它不在 PATH 里。

### 最终脚本边界

模块运行时只保留以下 Shell：

```text
src/module/scripts/core/service.sh
```

以下旧业务脚本已从运行时删除，不得重新添加：`subscription.sh`、`subworker.sh`、`ebpf.sh`、`switch.sh`、`runtime.sh`、`utils/api.sh`、`utils/catalog.sh`、`utils/metadata.sh`、`network/tproxy.sh`、`utils/ipset.sh` 和 `core/subsched.sh`。`customize.sh` 中仍可保留一次性的旧 Worker/TPROXY 清理分支，但这些分支只用于安装时清理残留，不属于当前运行时兼容层。

## Shell 约定

- 运行时脚本面向 Android `/system/bin/sh`，只写 POSIX/mksh 可执行语法，不使用 Bash 数组、`[[ ]]`、进程替换或 Bash 专属选项。
- 参数和路径始终双引号包裹；跨进程传递复杂数据时使用文件或 JSON，不使用 `eval` 拼装命令。
- 公共业务能力统一放在 Go；Shell 只保留 `scripts/core/service.sh` 生命周期编排，不要在 `netproxyctl`、service 和 worker 中复制配置、Catalog 或 API 解析器。
- 配置写入使用候选文件、校验和原子替换。订阅更新失败必须保留上一版有效 Provider。
- 新增可执行文件时同步检查 `customize.sh` 权限列表和模块打包结果。

## Go 组件

- `src/native/netproxy` 是 Catalog、Provider、订阅事务、配置、eBPF 运行时和 Service API 的业务事实源；Shell 只负责模块生命周期与 Android 平台编排。
- 允许且仅允许一个 Go Worker。它承载订阅调度和可选的 Android 网络监听，不能演变为通用控制守护进程、REST 服务或第二个代理核心。
- 使用 reF1nd sing-box 的类型定义解析、生成和校验 Provider，不通过字符串替换拼接协议配置。
- reF1nd 依赖版本必须与打包的 sing-box 内核兼容；升级时同时验证转换 fixtures、Provider 和 Service API。
- Provider 修改必须保持完整校验、稳定 tag、`0600` 权限和原子替换。错误必须返回结构化 diagnostics，不允许空输出加成功退出码。
- 新增协议或修复解析缺陷时补充不含真实凭据的 fixture/golden 测试。

## Android 管理器

- 数据流保持 `Compose -> ViewModel -> Repository -> NetProxyCtlClient -> netproxyctl`。页面不直接读取 `/data/adb`、Catalog 文件、PID 或 Shell 文本推断业务状态。
- ViewModel 按功能域持有不可变 `StateFlow`；Repository 负责命令组合和响应映射。不要重新堆回全能 Repository、全能 ViewModel 或静态 Service Locator。
- 构造依赖由 `AppContainer` 和 `NetProxyViewModelFactory` 提供，不引入 Hilt/Koin，除非先完成明确的全项目架构决策。
- 遵循现有 miuix 视觉和交互：二级页使用 `AdaptiveTopAppBar`，分组标题使用 miuix `SmallTitle`，列表保持 Lazy item 粒度，卡片优先复用 `groupedCardItems`。有 miuix 对应组件时不另造 Material 风格替代品。
- Navigation3 是导航状态唯一所有者。主分页动画必须从真实当前页开始，禁止通过临时目标页制造过渡。
- `third_party/scripta` 是带来源记录的固定源码快照。修改其代码时保留来源、许可证和 NetProxy 扩展说明，不把它悄悄替换成浮动远程依赖。
- `src/module/NetProxy.apk` 是独立维护的完整包发行资产。本地 Android 构建和普通 CI 不得自动覆盖它；Lite 包继续排除该 APK。

## WebUI

- 当前 WebUI 是终端式界面（约 580 行）：所有 Root 命令统一经 `src/webui/src/exec.ts` 调用 `netproxyctl` 并渲染输出，其他模块不得自行拼接 Root 命令。
- 持久节点和订阅在核心停止时也必须可读，数据来自 `netproxyctl`；运行时延迟、流量和选择状态再与 sing-box API 合并。
- 不要把错误、加载状态或内部 UUID 直接暴露为界面主信息。
- `npm run dev` 使用 mock 数据，可在普通浏览器开发，不需要设备。
- 修改 WebUI 后必须构建并检查 `src/module/webroot/netproxy/` 产物路径，但不要手工编辑该生成目录。

## 安全与生成物

- 不提交订阅地址、节点凭据、UUID、密钥、HWID、自定义 Header、签名材料、设备日志或 `local.properties`。
- 日志、历史和诊断包必须复用统一脱敏逻辑；修复问题时使用匿名 fixture，不把用户提供的真实链接写入测试。
- 不手工修改 `src/module/bin/` 下的 `netproxy-native`、`sing-box`、`bpftool`，也不手工修改 WebUI 构建目录或工作流生成的版本号。更新二进制和资源时使用对应构建/更新流程并核对来源。

## 验证

每次改动至少运行 `git diff --check`，并按影响范围执行：

```sh
# Go 原生组件
(cd src/native/netproxy && go test ./... && go vet ./...)

# Shell/Catalog 契约（先准备 netproxy-native 测试二进制）
mkdir -p .tmp
(cd src/native/netproxy && go build -o ../../../.tmp/netproxy-native ./cmd/netproxy-native && go build -o ../../../.tmp/netproxyctl ./cmd/netproxyctl)
sh tests/catalog_subscription_test.sh ./.tmp/netproxy-native
sh tests/netproxyctl_contract_test.sh ./.tmp/netproxyctl ./.tmp/netproxy-native
sh tests/runtime_catalog_test.sh ./.tmp/netproxy-native
sh tests/config_utils_test.sh ./.tmp/netproxy-native
sh tests/service_state_test.sh ./.tmp/netproxy-native
sh tests/module_scripts_test.sh

# WebUI
(cd src/webui && npm ci && npm run build)

# Android
(cd src/android && ./gradlew testDebugUnitTest lintDebug assembleDebug)

# 文档
(cd docs && npm ci && npm run build)
```

Android Root、开机启动、模块命令、快捷设置磁贴、eBPF、热点、多用户与应用分身、跨分组切换和 Navigation 动画必须在真机验证。UI 改动检查窄屏、深色模式、加载/空/失败状态，并提供截图或录屏。

## Git 提交约束

- 一次提交只处理一个清晰主题。不要把格式化、依赖升级和无关重构混进缺陷修复——混合提交会让回滚被迫连带撤销无关改动。
- 提交信息使用 Conventional Commits：`<type>(<scope>): <中文主题>`。type 只用仓库在用的六个：`feat`、`fix`、`refactor`、`docs`、`ci`、`chore`。不要引入 `style`、`perf`、`test` 等本仓未使用的类型。
- scope 用英文小写，取组件或功能域，例如 `module`、`native`、`webui`、`android`、`sub`、`catalog`、`ebpf`、`agents`。跨多个组件时省略 scope，不要写成 `a,b` 列表。
- 主题用中文，描述行为变化而非文件变化，不加句号，整行不超过 72 字符。例：`feat(sub): 订阅名称留空时自动获取`、`fix(android): 修复自动更新周期选择器卡在 24 小时`。
- 涉及多个文件或层级、包含多个行为变化，或单一主题但改动量、运行时影响或回滚风险较大的提交，必须写 body；即使主题本身单一，也不能只留下标题。body 至少说明主要行为变化、影响范围和验证结果，必要时补充触发条件、实现取舍、迁移约束或已知影响。
- body 只写 diff 里看不出来的信息，不要逐文件复述改了什么；真正微小且标题已经完整表达意图的提交才可以省略 body。
- 改动 `schema=1` JSON 字段、错误码或状态语义时，body 必须列出需要同步的调用方（Shell / Go / Android / WebUI / tests）。
- 除非用户在当前请求中明确授权，不执行 `git add`、`git commit`、`git commit --amend`、`git rebase`、`git push`、发布或创建 PR。授权只在提出它的那一轮请求内有效，不延续到后续轮次。
- 保留用户已有的未提交改动。不要用 `git reset --hard`、破坏性 checkout 或批量清理来整理工作区。
- 历史中存在 `ci fix`、`格式化·` 这类不合规主题。它们是离群值，不作为格式先例；不要模仿，也不要为统一格式改写历史。
- 新增长期有效的架构、契约、平台或发布约束时，同步更新本文件。

## 代码注释约束

- 注释语言按语种统一：Shell、Kotlin、Go 与 WebUI 的 TypeScript/TSX 一律中文。协议名、字段名、命令名、类型名保持原文，不翻译。Go 现有注释中英混用，新增和触及的注释写中文，不做全量翻译式改写。
- 导出的 Go 标识符若需 godoc 注释，按 Go 惯例以标识符名开头，其余说明用中文。
- Shell 文件头沿用现有格式，四个字段齐全：文件、功能、用法、依赖。
- Shell 函数头沿用 `# 参数:` / `# 返回:`，并注明退出码含义。新增函数补齐两项；改签名时同步更新——签名与注释不一致比没有注释更容易误导。
- 日志、帮助文本默认中文，分段沿用 `#######################################` 风格。
- 只写代码本身表达不出来的信息：踩坑根因、时序或顺序约束、为什么不能改成看起来更自然的写法、跨进程或跨组件的隐含约定。
- 不写这些：复述下一行代码的标签、外部文档链接与版本沿革、被注释掉的旧实现（删掉，历史在 Git 里）。
- 收录门槛：只有当「删掉这条注释后，下一个人会写出编译通过但运行时出错的代码」时才值得写。
- 删除或改写注释前先判断它是不是某条不变式的唯一记录点。若某约束只写在注释里而没进本文件或测试，先把它落到测试或文档，再删注释。
- 不规定注释比例。函数没有非显然约束时不写注释是正确的。

## 防回退条款

以下写法看起来不规范，但都是上一版已被证伪写法的替代品。不要以「重构」或「统一风格」为由改回去。

- 分应用策略直接生成 `include_package` / `exclude_package` / `include_android_user`——包名转 UID 和 `user:package` 格式在应用分身与多用户场景下静默漏配。
- Service API 与 Clash API 使用 `02_experimental.json`、`08_services.json` 中的固定监听与密钥——改回运行时随机 bootstrap 会让 WebUI 连不上核心且无任何报错。
- Android 依赖由 `AppContainer` 与 `NetProxyViewModelFactory` 手工构造——引入 Hilt/Koin 需先有全项目架构决策。
- Provider 与 selector 的默认值必须落到 `Auto/<group>`——回退到 `direct` 会让用户以为已代理而实际直连。
- `src/module/NetProxy.apk` 由独立流程维护——本地 Android 构建覆盖它会把调试包发进正式模块。
- 订阅自定义请求头走 `--headers-file` 而非命令行参数——命令行对全系统可见（`/proc/<pid>/cmdline`），会泄露鉴权 token。
- 订阅请求的默认 User-Agent 是 `sing-box`——多数机场按 UA 白名单返回 `Subscription-Userinfo`，改成自定义 UA 会拿到 200 但没有流量信息。
- 新增此类条款时写故障现象，不写设计理由：现象能阻止下一次回退，理由不能。

## 版本与发布

- `src/module/module.prop` 中只有 `version=` 由人手动维护；`versionCode` 由 CI 按提交数写入，手改会在下次构建被覆盖。
- CI 打包的是 `src/module/` 的内容而非目录本身，模块 zip 根目录直接是 `module.prop`。新增顶层文件时确认它应当出现在模块根目录。

---

# 架构参考

以下记录 NetProxy 8.x 的事实源、状态机与契约细节，供按需查阅。上文的约束条款是这些契约的执行要求。

## 系统边界

```text
Android Manager ─┐
WebUI ───────────┼─> netproxyctl ─> Go 业务层 ─> Shell 编排 ─> sing-box
终端用户 ───────┘        │              │              │
                         │              │              ├─> eBPF 入站运行时
                         │              │              └─> 网络事件采集
                         │              ├─> 节点、订阅、Provider、配置
                         │              ├─> eBPF runtime 与 Service API
                         │              └─> 订阅 Worker
                         └─> schema=1 JSON 契约
```

NetProxy 不维护通用独立控制守护进程。唯一长期 Go 进程是模块启动的 Worker，负责订阅调度、Android 网络事件和策略评估；Shell 负责 Android 模块生命周期与 sing-box 进程，Go 负责类型化配置、网络事务、Provider 与业务状态。

## 事实源

| 数据 | 唯一事实源 | 说明 |
|---|---|---|
| 模块版本 | `src/module/module.prop` | `versionCode` 由打包工作流写入 |
| 模块设置 | `src/module/config/module.conf` | 保存活动分组、选择模式和出站模式 |
| eBPF 设置 | `src/module/config/ebpf/ebpf.conf` | 由运行时生成 sing-box eBPF inbound |
| 节点与订阅 | `src/module/data/catalog/<group-id>/` | `meta.json` + `provider.json` |
| sing-box 静态配置 | `src/module/config/singbox/confdir/` | 按编号组合加载 |
| sing-box 运行时配置 | `src/module/runtime/` | 启动或检查时生成，不由客户端编辑 |
| 服务状态 | `src/module/runtime/service.json` | `netproxyctl service status` 的状态源之一 |
| 实时核心状态 | Service API / Clash API | 连接、流量、测速和实际选择 |

Android 和 WebUI 不建立另一份节点数据库，也不直接修改这些文件。持久状态通过 `netproxyctl` 读取和变更，运行状态通过固定的 sing-box API 补充。

## Catalog 与 Provider

```text
data/catalog/
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
- 本地与订阅节点都可直接编辑、导出和删除；订阅再次更新时会按远端内容重新生成该组 Provider。
- `history.jsonl` 只记录脱敏后的更新结果，默认保留最近 20 条。
- `staging/` 用于锁、下载、转换和校验的临时事务。进程崩溃后可清理，业务代码不得依赖其中内容恢复节点状态。

运行时把每个非空分组投影为 Local Provider，并生成：

- `Auto/<group>`：urltest，默认自动测速。
- `Select/<group>`：selector，手动选择。
- `Proxy`：顶层 selector，连接各分组的 Auto/Select 出站。

分组 ID 是内部稳定身份，运行时标签和界面优先使用分组名称；只有名称冲突时追加 ID 消歧。同组和跨组节点切换优先使用 Service API，新增或删除整个分组时才需要重新加载运行时配置。

订阅名称留空时按 `Profile-Title`、`Content-Disposition` 文件名、URL 主机名、默认名「订阅」的顺序自动取名，在首次取得响应头后回填。

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

Worker 根据各订阅的 `next_update_at` 调度，不依赖 sing-box 和 `crond`；同时轮询 Android 路由表并读取 Wi-Fi 状态，网络策略是否生效由每次评估读取的配置决定。它由 Go 实现并替换 `subworker.sh`；运行时进度放在 `/dev/netproxy/subscriptions/`，完成后不作为长期 UI 状态显示。

订阅响应头的解析要点：`Subscription-Userinfo` 提供流量与到期，空值（如 `expire=`）表示不适用而非畸形；`Profile-Title` 可能带 `base64:` 前缀或 RFC 2047 编码；`Content-Disposition` 的 `filename` 可能是 RFC 5987 形式，也可能直接携带原始 UTF-8 字节。

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

eBPF 只负责透明代理入站。停止服务由 sing-box 关闭并清理其 eBPF 程序、Map 和 TC 挂载。

分应用配置保存包名和可选 Android 用户范围，`netproxy-native ebpf runtime` 直接生成 `include_package`、`exclude_package` 与 `include_android_user`，包名到 UID 的解析由 sing-box 在入站启动时完成。应用安装、重装、UID 变化或用户范围变化后，通过配置 reload 重新解析，不维护模块侧 UID 缓存。

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

**Android**：按 `core/`、`feature/`、`navigation/` 组织。每个功能域拥有自己的 Repository、ViewModel 和 UI state；应用级长生命周期依赖由 `AppContainer` 组合。Root 命令只从 `NetProxyCtlClient` 发出，页面不拼接 Shell。底部一级入口为「仪表盘 / 节点 / 订阅 / 设置」。

**WebUI**：终端式界面，Root 命令经 `src/exec.ts` 统一发出。开发环境可以提供 mock，但 mock 不得改变生产契约或掩盖非零退出状态。

两端的节点和订阅在服务停止时仍可浏览；延迟、流量和当前选择等运行状态在服务 ready 后合并。

## 构建与发布

- 构建动作先测试并交叉编译 `netproxy-native`，再构建 WebUI，最后打包模块。
- 完整包包含 `NetProxy.apk`；Lite 包只排除该 APK，代理能力保持一致。
- Android 管理器不由普通模块 CI 构建或发布，Google Play 是推荐更新渠道；内置 APK 为无 Play 环境保留。
- `update-resources.yml` 统一维护内核、规则、Web 资源、Go/npm/Gradle/Android 依赖；高风险或大版本更新进入报告，不自动静默升级。

## 安全边界

- Catalog 元数据、订阅 Header 和 Provider 权限必须限制为 Root 可读。
- LAN 控制、CORS、Private Network Access 和远程鉴权变更属于安全设计，必须单独评审。
- 配置保存遵循「候选文件 -> 完整检查 -> 原子替换 -> reload」；检查失败恢复磁盘和编辑器状态，不留下半应用配置。
