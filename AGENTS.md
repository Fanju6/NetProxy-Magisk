# NetProxy Agent Guide

本文件是仓库内自动化编码代理的根级约束。开始修改前先阅读与任务相关的源码和文档；跨组件架构、状态所有权与运行时流程见 [ARCHITECTURE.md](ARCHITECTURE.md)，Android 细节见 [src/android/ARCHITECTURE.md](src/android/ARCHITECTURE.md)。

## 项目边界

- `src/module/`：Magisk、KernelSU 与 APatch 模块，包含生命周期脚本、`netproxyctl`、sing-box 配置、资源和打包内容。
- `src/native/netproxy/`：模块专用 Go 组件，负责节点转换、Provider、订阅 HTTP 元数据和 Service API 客户端。
- `src/webui/`：React + TypeScript 终端式 WebUI，构建产物写入 `src/module/webroot/netproxy/`。
- `src/android/`：Android 管理器，使用 Compose、miuix、Navigation3 和内置 Scripta 源码快照。
- `docs/`：VitePress 用户文档；`tests/`：Shell 契约与运行时回归测试。

不要恢复 8.0 已废弃的旧节点目录、`CURRENT_CONFIG`、`_meta.json`、TPROXY/IPSET 数据面或旧 CLI。除非任务明确要求，不为旧版配置增加迁移和兼容分支。

## 核心契约

- `netproxyctl` 是终端、Android 和 WebUI 的唯一模块管理入口；`netproxy-native` 是内部实现，不是平行的公开 CLI。
- 机器接口固定使用 `schema=1` JSON。stdout 只能包含结果 JSON，日志与诊断写 stderr；字段、错误码或状态语义变化必须同步检查 Shell、Go、Android、WebUI 和测试。
- Catalog 是持久节点事实源：每组使用 `config/catalog/<group-id>/meta.json` 与 `provider.json`。`staging/` 只存事务临时文件，不得作为持久状态读取。
- `ACTIVE_GROUP_ID` 保存分组 ID；`SELECTOR_MODE` 只允许 `urltest` 或 `manual`；`SELECTED_NODE_REF` 只在手动模式保存 `<group-id>/<tag>`。
- Provider 的运行时显示标签来自分组名称；名称冲突时才附加分组 ID。用户界面不得直接显示 UUID 代替可读名称。
- 自动选择必须落到 `Auto/<group>`，Provider/selector 的默认值绝不能静默回退到 `direct`。
- eBPF 是 sing-box 的入站实现，不是独立代理核心。服务、模式和节点切换文案继续使用“服务”或“sing-box”，不要泛化为“eBPF 服务”。
- 分应用策略持久化包名与 Android 用户范围，运行时直接生成 `include_package` / `exclude_package` / `include_android_user`；不得恢复模块侧包名转 UID 或 `user:package` 格式。
- `EBPF_CGROUP_ENABLED=0` 时只允许 shared-network 数据路径，运行时不得输出 cgroup 路径、IPv6 模式、应用/UID 策略或本机 Map 配置。
- Service API 与 Clash API 的固定监听和密钥位于 `02_experimental.json`、`08_services.json`。不要重新引入运行时随机 bootstrap，现有 WebUI 依赖固定入口。
- 服务状态只允许 `stopped/preparing/starting/ready/stopping/failed`。`ready_at` 只能在 sing-box API 与 eBPF 入站均就绪后写入。

## Shell 约定

- 运行时脚本面向 Android `/system/bin/sh`，只写 POSIX/mksh 可执行语法，不使用 Bash 数组、`[[ ]]`、进程替换或 Bash 专属选项。
- 参数和路径始终双引号包裹；跨进程传递复杂数据时使用文件或 JSON，不使用 `eval` 拼装命令。
- 公共能力优先放在 `scripts/utils/`，生命周期编排放在 `scripts/core/`；不要在 `netproxyctl`、service 和 worker 中复制配置、Catalog 或 API 解析器。
- Shell 注释、日志和帮助默认使用中文，分段沿用 `#######################################` 风格；协议名、字段名和命令名保持原文。
- 配置写入使用候选文件、校验和原子替换。订阅更新失败必须保留上一版有效 Provider。
- 新增可执行文件时同步检查 `customize.sh` 权限列表和模块打包结果。

## Go 组件

- `src/native/netproxy` 只实现需要类型化 sing-box 配置、HTTP 或 Protobuf 的能力；Shell 负责模块生命周期和平台编排。
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

- 当前 WebUI 是终端式界面：所有 Root 命令统一经 `src/webui/src/exec.ts` 调用 `netproxyctl`，其他模块不得自行拼接 Root 命令。
- 持久节点和订阅在核心停止时也必须可读，数据来自 `netproxyctl`；运行时延迟、流量和选择状态再与 sing-box API 合并。
- 不要把错误、加载状态或内部 UUID 直接暴露为界面主信息。
- 修改 WebUI 后必须构建并检查 `src/module/webroot/netproxy/` 产物路径，但不要手工编辑该生成目录。

## 安全与生成物

- 不提交订阅地址、节点凭据、UUID、密钥、HWID、自定义 Header、签名材料、设备日志或 `local.properties`。
- 日志、历史和诊断包必须复用统一脱敏逻辑；修复问题时使用匿名 fixture，不把用户提供的真实链接写入测试。
- 不手工修改 `src/module/bin/netproxy-native`、WebUI 构建目录或工作流生成的版本号。更新二进制和资源时使用对应构建/更新流程并核对来源。
- 保留用户已有的未提交改动。不要用 `git reset --hard`、破坏性 checkout 或批量清理来整理工作区。

## 验证

每次改动至少运行 `git diff --check`，并按影响范围执行：

```sh
# Go 原生组件
(cd src/native/netproxy && go test ./... && go vet ./...)

# Shell/Catalog 契约（先准备 netproxy-native 测试二进制）
mkdir -p .tmp
(cd src/native/netproxy && go build -o ../../../.tmp/netproxy-native ./cmd/netproxy-native)
sh tests/catalog_subscription_test.sh ./.tmp/netproxy-native
sh tests/netproxyctl_contract_test.sh ./.tmp/netproxy-native
sh tests/runtime_catalog_test.sh ./.tmp/netproxy-native
sh tests/service_state_test.sh

# WebUI
(cd src/webui && npm ci && npm run build)

# Android
(cd src/android && ./gradlew testDebugUnitTest lintDebug assembleDebug)

# 文档
(cd docs && npm ci && npm run build)
```

Android Root、开机启动、eBPF、热点、应用分身、跨分组切换和 Navigation 动画必须在真机验证。UI 改动检查窄屏、深色模式、加载/空/失败状态，并提供截图或录屏。

## Git 工作流

- 一次修改只处理一个清晰主题；避免把格式化、依赖更新和无关重构混入缺陷修复。
- 提交信息使用项目现有 Conventional Commits 风格，主题用简洁中文。
- 除非当前请求明确授权，不执行 commit、amend、rebase、push、发布或创建 PR。
- 新增长期有效的架构、契约、平台或发布约束时，同步更新本文件和对应架构文档。
