# NetProxy Agent Guide

本文件是仓库内自动化编码代理的根级约束，也是跨组件架构的唯一权威说明。开始修改前先阅读与任务相关的源码和本文件对应章节；Android 内部分层见 [src/android/ARCHITECTURE.md](src/android/ARCHITECTURE.md)，贡献流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

前半部分是编码约束，后半部分「架构参考」记录事实源、状态机与契约细节。

本文件只收录「违反后编译和测试都不报错、但运行时会静默出错」的约束。能被 `go vet`、`tsc`、Gradle lint 或现有测试拦住的规则不写在这里。

## 项目边界

- `src/module/`：Magisk、KernelSU 与 APatch 模块，包含生命周期脚本、`netproxyctl`、sing-box 配置、资源和打包内容。
- `src/native/netproxy/`：模块专用 Go 组件，负责节点转换、Provider、订阅、配置、eBPF 运行时、Service API 与唯一允许的后台 Worker。
- `src/webui/`：原生 TypeScript 终端式 WebUI，构建产物写入 `src/module/webroot/netproxy/`。
- `src/android/`：Android 管理器，使用 Compose、miuix-nav 和内置 Scripta 源码快照。
- `docs/`：VitePress 用户文档；`tests/`：Shell 契约与运行时回归测试。

`src/module/` 与设备上的 `/data/adb/modules/netproxy/` 1:1 对应，改脚本即改部署布局。

透明代理入站可选择 eBPF 或 Root TUN + auto_redirect，每次只运行一个受管入站；Catalog 是节点与订阅的持久事实源。

## 核心契约

- `netproxyctl` 是仓库唯一 Go 可执行文件，也是终端、Android 和 WebUI 的唯一模块管理入口；模块生命周期使用同一二进制的隐藏 `__internal` 入口。
- `__internal` 只允许 `boot` 与 `worker start|stop|run` 这类进程生命周期入口。Catalog、节点、订阅、配置、eBPF、Service API 和服务操作必须由公共命令直接调用 Go 领域处理器，不得重新建立内部命令转发层。
- 机器接口固定使用 `schema=1` JSON。stdout 只能包含结果 JSON，日志与诊断写 stderr；字段、错误码或状态语义变化必须同步检查 Shell、Go、Android、WebUI 和测试。
- Native 运行日志固定为 `[timestamp] [LEVEL] [component] [event] [result] [error_code] message`，成功或无错误码时写 `-`；消息必须在落盘前统一脱敏和限长。`logs show service` 的 `entries` 是 Android 展示事实源，不得回退到旧文本猜测。`logs show core` 保持 sing-box 文本，由客户端使用独立解析逻辑。
- Catalog 是持久节点事实源：每组使用 `data/catalog/<group-id>/meta.json` 与 `provider.json`。`staging/` 只存事务临时文件，不得作为持久状态读取。
- 节点选择只持久化 `ACTIVE_GROUP_ID` 与 `SELECTED_NODE_TAG`：空 tag 使用同组 Auto，非空 tag 手动选择。模式、节点引用和运行时标签由这两项派生，不读取旧选择字段或增加迁移逻辑。
- 用户选节点按生命周期锁、配置文件锁串行保存和应用；启动与重载只同步已保存选择，不再次调用用户保存入口。选择器 API 仅对临时通信或服务错误在总时限内重试，遵循请求取消；最终失败返回 `node.runtime_sync_failed` 并保留已保存选择，不重载兜底；Catalog 结构变化显式重载。
- 本地节点变更与删除订阅先等待生命周期锁、恢复未完成配置事务，再提交 Catalog；提交后即使取消也要有界完成本地选择整理，Worker 同步必须在取锁前建立独立收尾上下文，并在锁内读取最新选择。订阅同步状态落盘同样有界且不受请求取消影响，运行时操作仍使用原请求上下文。提交后的本地或运行时失败分别返回 `node.persisted_effect_failed` / `node.runtime_sync_failed` 或对应的 `subscription.*`，并携带 `persisted=true`。Worker 通过 `SyncCatalog` 回调复用同一流程；pending 重试必须重新应用已保存选择，不能只验证 Provider 后清除 pending。
- Provider 的运行时显示标签来自分组名称；名称冲突时才附加分组 ID。用户界面不得直接显示 UUID 代替可读名称。
- 自动选择必须落到 `Auto/<group>`，Provider/selector 的默认值绝不能静默回退到 `direct`。
- eBPF 与 TUN 都是 sing-box 的入站实现，不是独立代理核心。服务、模式和节点切换文案继续使用“服务”或“sing-box”，不要泛化为“eBPF 服务”或“TUN 服务”。
- `config/inbound/inbound.json` 是 backend、共用 app 策略与两套原生入站参数的唯一事实源，不在 module.conf 或客户端偏好中双写。外层固定为 `backend/app/ebpf/tun`，两个原生对象的 type 匹配分区、tag 均固定为 `netproxy-in`；主配置不得重复定义受管 eBPF/TUN 或占用该标签。
- 透明代理运行时只有 `runtime/inbound.json`，包含当前选择的一个入站；providers/outbounds 仍独立生成。切换不转换或清空另一套参数，失败不自动改用另一后端。
- 分应用策略持久化严格的 `<user-id>:<package>` 引用，Android 每个用户独立展示；Go 通过 Android package service 查询 UID，运行时生成 `include_uid` / `exclude_uid`。
- eBPF 数据路径由 `ebpf.local.enabled` 与 `ebpf.shared.enabled` 独立启用，选择 eBPF 时至少开启一条；本机 `data_plane` 只允许 `cgroup/tc`，共享网络只允许 `packet_rewrite/socket_assign`。禁用路径可保存全部偏好，但运行时只输出 `enabled: false`。
- 受管 TUN 固定 `auto_route=true` 与 `auto_redirect=true`，地址必须有效；不额外固定 MTU、stack、strict_route、marks 或 NFQUEUE 默认值。端口 bypass 使用主配置 route 的原生 pre-match 规则，不伪装成 TUN 字段或自动注入第四个 runtime。
- Service API 与 Clash API 的固定监听和密钥位于 `config/singbox/config.json` 的 `services` 与 `experimental.clash_api`。不要重新引入运行时随机 bootstrap，现有 WebUI 依赖固定入口。
- 服务状态只允许 `stopped/preparing/starting/ready/stopping/failed`。`ready_at` 只能在 sing-box API 与所选入站均就绪后写入。
- `service status` 的 `configured_backend` 是入站持久选择字符串；`active_backend` 仅在 ready、实际 PID 与启动记录匹配、API 毫秒级启动身份一致时非空，否则必须为 null。不从当前模板猜测旧核心后端，也不增加后端 PID/锁/状态文件。
- 出站模式的唯一持久事实源是主配置 `experimental.clash_api.default_mode`；可选列表复用内核 `clashmode.CalculateModeList` 并包含默认模式，使用原生名称，不保留模块字段或固定四模式映射。`service status.outbound_mode` 表示实际模式，`configured_outbound_mode` 表示默认模式，`available_outbound_modes` 表示配置中的模式列表。停止时显示默认模式；运行中 API 不可用时显示 `unknown`。Wi-Fi 策略只修改运行时，不覆盖默认模式。
- 模式保存与网络策略应用按生命周期锁、配置文件锁顺序串行执行；运行中仅使用 API 并回读确认，失败保留已保存默认模式并返回 `mode.runtime_sync_failed`，不得重载兜底。内核会恢复缓存模式，启动和重载必须在写入 ready 前校准当前网络所需模式；不能删除缓存或禁用其他缓存功能来规避模式恢复。

## 命令入口与脚本布局

- `src/module/netproxyctl` 只负责定位 `bin/netproxyctl`；公共实现位于 `src/native/netproxy/cmd/netproxyctl`。Shell 不再保留公共命令 dispatcher。
- 命令组权威清单：`service catalog node sub mode network app ebpf config logs`。新增命令组必须同时更新 Go CLI、Android `NetProxyCtlClient`、WebUI `src/exec.ts` 和契约测试。
- `scripts/` 不承载运行时业务；配置、Catalog、状态和 Service API 业务统一由 Go 实现。
- 根目录 `service.sh` 负责模块开机桥接；运行时配置、节点切换、订阅事务和调度由 Go 负责。
- Go Worker 负责 Android 网络变化采集、Wi-Fi 状态读取和策略评估。
- `customize.sh` 在已开机安装时不得提前覆盖 live 模块目录；必须等待管理器写入 `update` 标记后再由脱离安装器 cgroup 的 Shell 完成目录切换。任何校验或切换失败都保留 `modules_update`，交回管理器下次开机处理。
- 设备上的调用形式是 `su -c /data/adb/modules/netproxy/netproxyctl [--json] <命令组> <命令>`；文档和排查步骤按此形式给出，不要写成裸 `netproxyctl`，它不在 PATH 里。

### 最终脚本边界

模块运行时只保留以下 Shell：

```text
src/module/service.sh
```

## Shell 约定

- 运行时脚本面向 Android `/system/bin/sh`，只写 POSIX/mksh 可执行语法，不使用 Bash 数组、`[[ ]]`、进程替换或 Bash 专属选项。
- 参数和路径始终双引号包裹；跨进程传递复杂数据时使用文件或 JSON，不使用 `eval` 拼装命令。
- 公共业务能力统一放在 Go；运行时 Shell 只保留 `service.sh` 开机桥接，不要在 Shell 中复制配置、Catalog、API 或进程管理逻辑。
- 配置写入使用候选文件、校验和原子替换。订阅更新失败必须保留上一版有效 Provider。
- 新增可执行文件时同步检查 `customize.sh` 权限列表和模块打包结果。

## Go 组件

- `src/native/netproxy` 是 Catalog、Provider、订阅事务、配置、eBPF 运行时、Service API 与 sing-box 生命周期的业务事实源；Shell 只负责模块 service 阶段进入 Go 的平台桥接。
- 模块、配置、Catalog、运行时、日志、二进制与 `/dev/netproxy` 状态路径统一由 `internal/paths.Layout` 推导。生产代码不得自行拼接这些布局；测试和用户指定的导入、导出、临时路径仍可显式注入。
- 允许且仅允许一个 Go Worker。它承载订阅调度、可选的 Android 网络监听和设备统计，不能演变为通用控制守护进程、REST 服务或第二个代理核心。
- 设备统计只从 Go 公共命令与真实核心启动采集 `module_active`、`service_start_result`，不增加 Android/WebUI SDK。Worker 以 Root 查询用户 0 的 `ANDROID_ID`，用固定 NetProxy 命名空间的 SHA-256 派生设备身份，每个 Worker 成功读取后只缓存在内存；读取失败不得回退随机身份。CLI 不查询设备标识，`config/telemetry/state.json` 只保存每日去重和有界队列，不进入编辑器、日志或诊断包；原始标识不得落盘或上传，Token 只由构建注入。损坏状态不得静默覆盖，系统查询和网络请求不得持有状态锁，上传不得阻塞业务命令。停服入口不得为统计重新启动 Worker，以免停服操作产生后台启动副作用。
- 使用 reF1nd sing-box 的类型定义解析、生成和校验 Provider，不通过字符串替换拼接协议配置。
- reF1nd 依赖版本必须与打包的 sing-box 内核兼容；升级时同时验证转换 fixtures、Provider 和 Service API。
- Native JSON 编解码统一使用 Go 标准库 `encoding/json/v2` 与 `encoding/json/jsontext`，依赖严格字段匹配、重复键拒绝和 UTF-8 校验；持久文件与 `schema=1` 输出必须显式传入 `json.Deterministic(true)`，不要回退到 v1 或设置 `GOEXPERIMENT=nojsonv2`。
- `cmd/netproxyctl/default.pgo` 只使用真实 Android 上的只读工作负载生成；正式构建保持 `-pgo=auto`，更新 profile 前必须确认不含订阅、节点或设备数据，并对比非 PGO 产物。
- Provider 修改必须保持完整校验、稳定 tag、`0600` 权限和原子替换。错误必须返回结构化 diagnostics，不允许空输出加成功退出码。
- Catalog 与配置回滚使用同目录临时文件替换目标，全部恢复成功并记录恢复完成后才删除备份；恢复失败不得清理 journal，否则再次中断可能丢失已恢复的数据。
- Catalog 事务完成后的清理是可重试收尾，不改变已提交或已恢复的结果；必须先删除全部备份再删除完成标记。仍有备份但 journal 缺失时不得猜测事务结果或丢弃备份。
- 配置应用按「生命周期锁 → 固定顺序的配置文件锁」执行。内部选择同步显式复用已持有的配置写入器，不能重复获取文件锁；所有配置写入共享同一路径锁，分应用增删必须在锁内读取最新名单。
- Catalog 等待锁使用调用方 context，分组锁先于根锁。锁文件不保存业务或 owner 状态，互斥由操作系统文件锁保证。
- sing-box 静态事实源只有 `config/singbox/config.json`。分区编辑由 Go 在配置事务锁内替换指定顶层字段，保留其他字段和数组顺序；客户端使用读取时的 `revision`，同分区冲突返回 `config.conflict`。不能在 Android 中把整份旧快照合并写回。
- 入站复用 `config read/apply/validate` 的 `inbound`、`inbound/backend`、`inbound/ebpf`、`inbound/tun` 目标，不增加公共 inbound 命令组或旧 config ebpf 别名。入站分区必需，不能用 `{}` 删除；全部目标与 app 增删共用 `inbound.json.lock`，仅在锁内合并最新文件。
- `ebpf status` 是保存的 eBPF 模板所选能力的预检，TUN 模式也可执行，不代表当前挂载状态。普通 `data.content` 始终是可读诊断，原始 JSON 仅在显式 `--raw` 时作为正文返回；预检通过不能表述为实际接管成功。
- 启动只校验当前原生分区；保存分区校验该分区，完整保存校验两套格式，整份 JSON 损坏必须失败。未选分区变化不 reload，停止时保存不启动核心或 Worker。切换后端先停旧实例再启动新实例；强杀或清理未确认时中止并保留 journal，必须设备重启后再恢复，不增加兜底清理。
- 新增协议或修复解析缺陷时补充不含真实凭据的 fixture/golden 测试。

## Android 管理器

- 数据流保持 `Compose -> ViewModel -> Repository -> NetProxyCtlClient -> netproxyctl`。页面不直接读取 `/data/adb`、Catalog 文件、PID 或 Shell 文本推断业务状态。
- ViewModel 按功能域持有不可变 `StateFlow`；Repository 负责命令组合和响应映射。不要重新堆回全能 Repository、全能 ViewModel 或静态 Service Locator。
- 构造依赖由 `AppContainer` 和 `NetProxyViewModelFactory` 提供，不引入 Hilt/Koin，除非先完成明确的全项目架构决策。
- 遵循现有 miuix 视觉和交互：二级页使用 `AdaptiveTopAppBar`，分组标题使用 miuix `SmallTitle`，列表保持 Lazy item 粒度，卡片优先复用 `groupedCardItems`。有 miuix 对应组件时不另造 Material 风格替代品。
- Miuix Nav 是页面导航状态唯一所有者。主分页动画必须从真实当前页开始，禁止通过临时目标页制造过渡。
- 主分页底部导航由 `MainBottomBar` 单一实现统一承载；主题偏好不改变其结构或布局形态。
- 管理器由 Android 原生资源自动匹配中文、英语和俄语，英文是默认资源；界面文案放入字符串资源，不自行保存或强制覆盖系统语言。补全与校验逻辑不得依据翻译后的文本判断类型或错误分类。
- `third_party/scripta` 是带来源记录的固定源码快照。修改其代码时保留来源、许可证和 NetProxy 扩展说明，不把它悄悄替换成浮动远程依赖。
- 模块包必须包含 `NetProxy.apk`，由独立 Android 任务通过共享 Action 从当前源码构建，并使用 GitHub Secrets 中的固定密钥签名；不得提交签名材料或手工维护该生成物。安装器不检查已安装版本，两处安装选择共用音量加循环、音量减确认；10 秒无操作默认执行 `pm install -r`，操作后 20 秒未确认取消安装；APK 安装失败不卸载应用或清除数据，也不阻塞模块安装。

## WebUI

- 当前 WebUI 是原生 TypeScript 终端式界面：所有 Root 命令统一经 `src/webui/src/exec.ts` 调用 `netproxyctl` 并渲染输出，其他模块不得自行拼接 Root 命令。
- 持久节点和订阅在核心停止时也必须可读，数据来自 `netproxyctl`；运行时延迟、流量和选择状态再与 sing-box API 合并。
- 不要把错误、加载状态或内部 UUID 直接暴露为界面主信息。
- `npm run dev` 使用 mock 数据，可在普通浏览器开发，不需要设备。
- 修改 WebUI 后必须构建并检查 `src/module/webroot/netproxy/` 产物路径，但不要手工编辑该生成目录。

## 安全与生成物

- 不提交订阅地址、节点凭据、UUID、密钥、HWID、自定义 Header、签名材料、设备日志或 `local.properties`。
- 日志、历史和诊断包必须复用统一脱敏逻辑；修复问题时使用匿名 fixture，不把用户提供的真实链接写入测试。
- 不手工修改 `src/module/bin/` 下的 `netproxyctl`、`sing-box`，也不手工修改 WebUI 构建目录或工作流生成的版本号。更新二进制和资源时使用对应构建/更新流程并核对来源。

## 验证

每次改动至少运行 `git diff --check`，并按影响范围执行：

本地可通过 `sh tests/verify.sh quick|webui|android|docs|full` 编排下列既有检查；它不自动暂存、提交或发布。WebUI 构建检查页面引用的本地资源是否完整，生成产物由 CI 打包，不纳入 Git。

```sh
# Go 原生组件
(cd src/native/netproxy && go test ./... && go vet ./...)

# Shell/Catalog 契约（先准备 netproxyctl 测试二进制）
mkdir -p .tmp
(cd src/native/netproxy && go build -o ../../../.tmp/netproxyctl ./cmd/netproxyctl)
sh tests/runtime_catalog_test.sh ./.tmp/netproxyctl
sh tests/module_scripts_test.sh
sh tests/customize_hot_update_test.sh

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

- 注释语言按语种统一：Shell、Kotlin、Go 与 WebUI 的 TypeScript 一律中文。协议名、字段名、命令名、类型名保持原文，不翻译。Go 现有注释中英混用，新增和触及的注释写中文，不做全量翻译式改写。
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

- 分应用策略按 `<user-id>:<package>` 保存并在 Go 中按用户查询 UID——把多个 Android 用户合并成包名或直接把包名交给 sing-box 会在应用分身场景下静默漏配。
- Service API 与 Clash API 使用主配置中 `services`、`experimental.clash_api` 的固定监听与密钥——改回运行时随机 bootstrap 会让 WebUI 连不上核心且无任何报错。
- Android 依赖由 `AppContainer` 与 `NetProxyViewModelFactory` 手工构造——引入 Hilt/Koin 需先有全项目架构决策。
- Provider 与 selector 的默认值必须落到 `Auto/<group>`——回退到 `direct` 会让用户以为已代理而实际直连。
- CI 与 Release 管理器 APK 使用同一固定签名；轮换密钥或改回临时签名会导致后续 APK 无法覆盖已安装版本。应用不按安装来源或签名证书限制使用，也不显示来源警告。
- 订阅自定义请求头走 `--headers-file` 而非命令行参数——命令行对全系统可见（`/proc/<pid>/cmdline`），会泄露鉴权 token。
- 订阅请求的默认 User-Agent 是 `sing-box`——多数机场按 UA 白名单返回 `Subscription-Userinfo`，改成自定义 UA 会拿到 200 但没有流量信息。
- 新增此类条款时写故障现象，不写设计理由：现象能阻止下一次回退，理由不能。

## 版本与发布

- `src/module/module.prop` 中只有 `version=` 由人手动维护；`versionCode` 由 CI 按提交数写入，手改会在下次构建被覆盖。
- CI 打包的是 `src/module/` 的内容而非目录本身，模块 zip 根目录直接是 `module.prop`。新增顶层文件时确认它应当出现在模块根目录。

---

# 架构参考

以下记录当前 NetProxy 的事实源、状态机与契约细节，供按需查阅。上文的约束条款是这些契约的执行要求。

## 系统边界

```text
Android Manager ─┐
WebUI ───────────┼─> netproxyctl ─> Go 业务层 ─> sing-box
终端用户 ───────┘        │              │          │
                         │              │          ├─> eBPF 或 TUN 入站运行时
                         │              │          └─> 网络事件采集
                         │              ├─> 节点、订阅、Provider、配置
                         │              ├─> 受管入站 runtime 与 Service API
                         │              └─> 后台 Worker
                         └─> schema=1 JSON 契约
```

NetProxy 不维护通用独立控制守护进程。唯一长期 Go 进程是模块启动的 Worker，负责订阅调度、Android 网络事件、策略评估和设备统计；`service.sh` 仅在模块 service 阶段通过 `su -c` 进入 Go，Go 负责 sing-box 生命周期、类型化配置、网络事务、Provider 与业务状态。

## 事实源

| 数据 | 唯一事实源 | 说明 |
|---|---|---|
| 模块版本 | `src/module/module.prop` | `versionCode` 由打包工作流写入 |
| 模块设置 | `src/module/config/module.conf` | 保存活动分组、节点选择和 Wi-Fi 策略 |
| 默认出站模式 | `config/singbox/config.json` 的 `experimental.clash_api.default_mode` | 可选模式来自 route/DNS 规则与默认模式；API 报告运行时实际模式 |
| 设备统计队列 | `config/telemetry/state.json` | 每日去重和离线队列；设备身份由 Worker 从系统派生，不作为用户配置展示 |
| 入站与应用策略 | `src/module/config/inbound/inbound.json` | backend、app 与 eBPF/TUN 原生对象；只生成当前所选入站 |
| 节点与订阅 | `src/module/data/catalog/<group-id>/` | `meta.json` + `provider.json` |
| sing-box 静态配置 | `src/module/config/singbox/config.json` | 单一主配置，支持整份或按顶层字段编辑 |
| sing-box 运行时配置 | `src/module/runtime/` | inbound.json、providers.json、outbounds.json；可重建，不由客户端编辑或安装保留 |
| 服务状态 | `/dev/netproxy/service.json` | 本次启动周期的状态快照；缺失时按 stopped 处理 |
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

分组 ID 是内部稳定身份，运行时标签和界面优先使用分组名称；只有名称冲突时追加 ID 消歧。同组和跨组节点切换优先使用 Service API，新增或删除整个分组时才需要重新加载运行时配置。已有 Local Provider 的内容更新依赖 sing-box 文件监听，并通过 Service API 出站快照确认；不得把 `runtime_sync_pending` 直接当作整核 reload 条件。

订阅名称留空时按 `Profile-Title`、`Content-Disposition` 文件名、URL 主机名、默认名「订阅」的顺序自动取名，在首次取得响应头后回填。

## 选择状态

`module.conf` 使用以下字段：

```ini
ACTIVE_GROUP_ID="default"
SELECTED_NODE_TAG=""
```

- `SELECTED_NODE_TAG` 为空使用同组 Auto，实际选中节点由 Service API 报告，不写回手动选择。
- 手动模式仅保存当前分组节点的 tag，不重复保存分组 ID 或文件路径。
- 手动节点在 Provider 更新后消失时回退该组 Auto。
- 公开命令仍使用 `node use auto [group]` 或 `node use <group-id>/<tag>`；JSON 的 `selector_mode` 与 `selected_node_ref` 是派生结果，不是另一个持久事实源。订阅更新在配置文件锁内读取最新选择后计算变更，不能覆盖并发用户选择。
- 出站模式使用主配置与内核生成的原生列表；客户端翻译已知模式的显示文案，自定义名称原样显示。模式切换保存默认值，当前 Wi-Fi 绕过策略仍可使实际模式为 `Direct`。

## 订阅事务

订阅更新独立于 sing-box 是否运行：

```text
获取分组锁
-> 创建 staging
-> 条件下载
-> 解析 HTTP Header
-> netproxyctl 内部转换
-> Provider 校验
-> 原子替换 Provider 与元数据
-> 通知运行中的 Local Provider
-> 写入脱敏历史
```

下载、转换和校验阶段可以取消，原子提交阶段不可取消。任何失败都保留上一版有效 Provider 和当前选择。核心 ready 时可按设置经本地代理下载；核心停止或代理下载失败时，`auto` 策略允许直连重试。

HTTP 验证器只跟随已接受的 Provider 更新：失败保留原 ETag/Last-Modified，成功 200 替换或清空，304 只更新实际返回的值。跨域后的整个重定向链不得重新携带初始敏感 Header，HTTPS 降级与超过跳数限制均拒绝，不触发直连重试。

Worker 根据各订阅的 `next_update_at` 调度，不依赖 sing-box 和 `crond`；同时通过 netlink 监听 Android 路由、地址和接口变化，再读取 Wi-Fi 与实际出口状态进行策略评估，不得改回文件或定时轮询。运行时进度放在 `/dev/netproxy/subscriptions/`，完成后不作为长期 UI 状态显示。

失败退避按分组独立计算，正常调度和重试取最早截止时间；每轮读取最新启用状态，关闭自动更新或删除分组后丢弃重试。退避仅保存在 Worker 内存，配置唤醒重新计算调度但不提前重试失败分组。

订阅响应头的解析要点：`Subscription-Userinfo` 提供流量与到期，空值（如 `expire=`）表示不适用而非畸形；`Profile-Title` 可能带 `base64:` 前缀或 RFC 2047 编码；`Content-Disposition` 的 `filename` 可能是 RFC 5987 形式，也可能直接携带原始 UTF-8 字节。

## 服务生命周期

服务状态机固定为：

```text
stopped -> preparing -> starting -> ready -> stopping -> stopped
                         \-> failed
```

启动流程：

1. 校验二进制、静态配置、Catalog 和活动选择。
2. 生成 providers、outbounds 与唯一 inbound runtime 配置。
3. 运行 sing-box 配置检查。
4. 启动 sing-box 并等待 Service API 与当前所选入站就绪，记录实际 backend 与实例身份。
5. 校准实际出站模式，消除缓存覆盖并应用当前网络策略，再写入 `ready_at`。

eBPF 与 TUN 只负责透明代理入站。停止服务由 sing-box 关闭并清理当前入站的程序、Map、TC 挂载或 TUN 与路由规则；PID 消失不代表接管资源必然清理。切换不新增 switching 状态，也不创建第二个核心或 Worker。

节点测速不要求正式服务处于 `ready`。服务停止时，Native 只允许启动不含透明代理入站、eBPF/TUN 和 Clash API 的短生命周期 sing-box 会话，使用目标 Provider 快照与随机 loopback Service API 完成测速；会话不得修改正式服务状态、选择状态或 Worker，结束和取消时必须清理进程与临时文件。

分应用配置保存在入站文件的 `app` 对象，`proxy_apps` 与 `bypass_apps` 是严格 `<user-id>:<package>` 字符串数组。Go 通过 Android package service 按用户查询 UID 后合并到所选入站本机的 `include_uid` 或 `exclude_uid`，不写回原生模板。应用安装、重装、UID 变化或用户范围变化后，通过重启或配置 reload 重新解析，不维护模块侧 UID 缓存；白名单自动包含 UID 0。app 命令只保存，沿用由用户重启应用的行为。

app 关闭时保留原生 UID 筛选；开启时合并去重同向 UID/range，反向 UID/range 或非空原生 package/user 筛选必须明确拒绝歧义。仅启用 eBPF 共享网络时不查询应用 UID，不把本机应用名单当成热点客户端过滤器。

eBPF 本机与热点下游接管分别由 `local.enabled`、`shared.enabled` 控制。本机默认使用 cgroup socket hook 与 `respect_policy` DNS，也可选择跟随默认接口的 TC；共享网络默认关闭，保存以太网 `packet_rewrite`、`hijack` DNS 与 `wlan2` 接口偏好，raw-IP、PPP 或隧道接口可选择 `socket_assign`。启用共享网络时必须配置至少一个下游接口。

TUN 使用原生 `include_interface/exclude_interface`、CIDR、规则集与 MAC 筛选，不建立 local/shared/hybrid 映射。默认接口 `netproxy`、地址 `172.19.0.1/30` 与 `fdfe:dcba:9876::1/126`、DNS `hijack`；不输出 stack 或额外默认 MTU/strict_route/marks。目标 sing-box check 不等于设备能力证明，TUN、NFQUEUE、IPv6 TPROXY 与清理仍需真机确认。eBPF 能力探测继续使用 `ebpf status`，不新增 `tun status`。

## sing-box 配置组合

Go 生命周期控制器通过 `-c config/singbox/config.json` 加载静态配置，并追加运行时文件：

- `providers.json`：Catalog Local Provider 投影。
- `outbounds.json`：Auto/Select/Proxy 出站图。
- `inbound.json`：由 `config/inbound/inbound.json` 当前后端生成的唯一受管透明代理入站。

主配置包含日志、实验特性/Clash API、DNS、用户入站、路由、HTTP Client 和 Service API。Android 的分区是 `config list` 返回的逻辑文档，不对应额外磁盘文件；完整编辑入口保留所有受核心支持的字段。运行时文件由 Go 生成并只读展示。

`config read` 返回 `content` 和 `revision`；`config apply/validate --revision <值> <目标> <候选文件>` 检测并发修改。`singbox/dns` 等分区使用带顶层键的 JSON，空对象删除该字段；`singbox/config.json` 替换整份主配置。保存后的 revision 对应本次实际写入内容，不通过无锁重新读取生成。

入站目标 `inbound` 替换完整四字段包装；`inbound/backend`、`inbound/ebpf`、`inbound/tun` 分别使用对应顶层键。分区 revision 只跟踪该分区，完整 revision 跟踪整个文件；入站分区不支持空对象删除，所有写入在同一个配置文件锁内合并最新其他字段。

`config list` 的四个入站逻辑目标归类为 `category=inbound`；Prepare JSON 使用 `providers/outbounds/inbound` 路径字段与 `backend`，不保留旧 `ebpf` 路径字段。schema=1 字段与类别变化需同步 Shell、Go、Android、WebUI 与 tests。

安装只处理当前数据布局，不读取、转换或清理旧版配置。保留现有数据包含整个用户配置目录（包括核心持久状态）、Catalog 与日志，但 `config/singbox/rules/remote` 始终使用本次安装包的内置规则；仅保留节点与订阅包含 Catalog 与日志；全新安装使用包内默认内容。保留模式要求对应数据完整，不能因缺失而静默回退默认配置。热切换前重新复制最新数据，不复制 Catalog staging 或可重建的运行时文件。

保留全数据缺少当前 `config/inbound/inbound.json` 时明确失败，提示选择仅保留节点或全新安装，不检查版本或转换旧 eBPF 文件。快照锁固定顺序为生命周期、inbound、module、sing-box 主配置，再按既有统计和 Catalog 锁执行；目录切换保留 `config/inbound/inbound.json.lock`、module、主配置与 Catalog 锁 inode。

安装快照与目录切换使用 Go 的生命周期、配置文件和 Catalog OS 锁，分组锁先于根锁；任一锁忙立即中止。Android mksh 调用外部 `flock` 时必须显式传递锁描述符（如 `9>&9`），否则会因 `Bad file descriptor` 回退。目录切换必须保留锁文件 inode，否则等待中的 Go 命令会与新命令使用两套锁。前台准备不停止服务，后台停服或切换失败尝试恢复提交前的 Worker 与服务；原先停止的服务不得自动开启。

当前控制入口是稳定产品契约：

| 接口 | 本机客户端地址 | 用途 |
|---|---|---|
| Service API | `127.0.0.1:9090` | 核心状态、流量、节点组、选择和测速 |
| Clash API | `127.0.0.1:9999` | 第三方 Clash 客户端 |
| 模块 WebUI | 模块管理器 WebView | 持久管理与状态展示 |

静态配置默认只监听 `127.0.0.1`，本机客户端统一通过 loopback 访问；配置文件使用固定密钥 `singbox`，用于 WebUI 自动进入面板。需要 LAN 控制时必须显式修改监听范围并同步评估鉴权和网络安全影响，不能只改一端。

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

`netproxyctl __internal` 只供模块生命周期内部使用。Android/WebUI 只能调用公开命令组，以免形成两套公共契约。

## 客户端边界

**Android**：按 `core/`、`feature/`、`navigation/` 组织。每个功能域拥有自己的 Repository、ViewModel 和 UI state；应用级长生命周期依赖由 `AppContainer` 组合。Root 命令只从 `NetProxyCtlClient` 发出，页面不拼接 Shell。底部一级入口为「仪表盘 / 节点 / 订阅 / 设置」。

**WebUI**：终端式界面，Root 命令经 `src/exec.ts` 统一发出。开发环境可以提供 mock，但 mock 不得改变生产契约或掩盖非零退出状态。

两端的节点和订阅在服务停止时仍可浏览；延迟、流量和当前选择等运行状态在服务 ready 后合并。

## 构建与发布

- CI 与 Release 共用 `build-module.yml` 的并行任务图；模块任务完成 Go/Shell 验证、`netproxyctl`、WebUI 与内容压缩，Android 任务构建并用固定密钥签名当前源码的 APK。共享工作流等待两者成功后汇合为唯一模块包，发布任务只下载最终产物，不重新编译或打包。Android 源码变化必须触发模块重打包；任一构建或所需验证失败均不得发布。
- `verify.yml` 面向 Pull Request 与手动源码验证，只读取公开源码并按变更范围执行 Native、WebUI、Android 或文档检查；它不得读取签名、发布、Telegram 或统计 Secrets，不得打包、上传或发布产物。只有 `main` 上的 `ci.yml` 和 tag 发布工作流可以构建签名管理器与模块包。
- CI 变更范围从同分支上次成功验证的提交计算，不能只比较本次 push：前一轮被取消或失败的改动仍须验证；基线不可用时执行全部检查。
- 版本计数与更新日志所需的 checkout 保留完整提交历史；可使用 `blob:none` 或稀疏检出减少历史文件下载。KernelSU 源码镜像仍须获取完整对象，不能套用部分克隆。
- 模块只发布 `NetProxy_<版本>_<构建号>.zip`，必须包含本次构建的管理器 APK。APK 只在首次解压检查时必需，用户安装或跳过后均清理，热切换校验不得要求它仍然存在。
- 模块使用 ZIP 容器和 XZ 9 压缩；汇合任务直接向模块内容归档以 Store 追加 APK，不复制第二份发行包。CI 上传归档时不再进行外层压缩。
- Android 受影响时，CI 在管理器构建任务中使用同一次 Gradle 调用执行单元测试、Lint 与 Release 构建；仅需模块打包时仍构建当前管理器，不额外执行 Android 验证。资源维护复用纯验证模式，不构建 APK，也不需要签名密钥。固定签名由 `ANDROID_KEYSTORE_BASE64`、`ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_ALIAS`、`ANDROID_KEY_PASSWORD` 四个 GitHub Secrets 注入；缺失时构建失败，不生成替代密钥。CI 版本名保留提交短哈希，仅用于区分构建。
- Release 只能从与 `src/module/module.prop` 的 `version=` 和 `docs/changelog.md` 对应章节完全一致的 `v<版本号>` tag 启动；预检失败时不得开始签名构建或创建 Release。
- `update-resources.yml` 统一维护内核、规则、Web 资源、Go/npm/Gradle/Android 依赖；规则与 Web 资源属于内容更新，sing-box 属于核心更新，工具链大版本进入报告。GitHub Actions 更新只报告、不由定时任务静默改写。
- Composite Action 的仓库 Variables 由调用工作流通过 inputs 显式传入；在 Action 清单中直接读取 `vars` 会导致 Runner 加载失败，尚未开始构建就退出。

## 安全边界

- Catalog 元数据、订阅 Header 和 Provider 权限必须限制为 Root 可读。
- LAN 控制、CORS、Private Network Access 和远程鉴权变更属于安全设计，必须单独评审。
- 配置保存遵循「候选文件 -> 完整检查 -> 原子替换 -> reload」；检查失败恢复磁盘和编辑器状态，不留下半应用配置。
