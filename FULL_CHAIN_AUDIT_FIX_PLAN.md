# NetProxy 全链路审查与修复清单

本文件记录 2026-08-10 对模块、Go 原生组件、Android 管理器和 WebUI 的全链路审查结果。
修复按顺序进行；每一项必须完成代码修改、针对性测试和结果确认后才能勾选。
本轮不恢复旧节点目录、旧 CLI、TPROXY 或其他历史兼容逻辑。

## 当前边界

- Android 和 WebUI 只通过 `netproxyctl` 访问模块业务。
- `netproxy-native` 是内部实现；唯一长期 Go 进程是订阅/网络 Worker。
- sing-box 是代理核心，eBPF 只是透明代理入站。
- Catalog 是节点与订阅的持久事实源，`runtime/` 只保存可重建文件。
- Service API 和 Clash API 的客户端地址由静态配置提供。

## 修复清单

### 1. Worker 生命周期自动恢复

- [x] `service start`、`service restart` 和开机启动共用 Worker 确保逻辑。
- [x] Worker 已运行时不重复启动；Worker 异常退出后下次服务操作可以拉起。
- [x] `service stop` 不停止独立 Worker；卸载时仍然停止 Worker。
- [x] 验证 Worker 状态、PID 文件和服务状态的组合场景。

完成：`ManageService` 在 start/restart 后补偿启动 Worker，`Boot` 复用同一入口；
Worker 失败只记录警告，不影响已经 ready 的 sing-box。新增网络监听回调测试。

根因：Worker 目前只由 `module boot` 启动，普通服务启动不会修复已经退出的 Worker。

### 2. 启动前执行 sing-box 配置检查

- [x] 运行时配置生成后、sing-box 进程启动前执行 `sing-box check`。
- [x] 检查失败不启动进程、不留下运行时文件，并返回结构化错误。
- [x] 避免配置检查失败时提前持久化节点选择状态。
- [x] 保留 reload/check 的现有行为。

完成：`Prepare` 只生成候选运行时配置；`StartService` 和 `ReloadService` 在
sing-box check 成功后才同步 `module.conf` 的选择状态。启动失败路径会清理运行时文件。

根因：`StartService` 当前在 `Prepare` 后直接启动进程，只有独立 `check` 和 reload 路径执行配置校验。

### 3. 精确匹配 sing-box 进程

- [x] 只匹配规范化后的目标绝对路径。
- [x] 删除仅按可执行文件 basename 匹配的回退逻辑。
- [x] 保留 `/proc/<pid>/exe` 不可读时的安全降级，不误杀同名进程。
- [x] 补充同名不同路径进程测试。

完成：进程识别只接受规范化后的完整路径，同时继续兼容相同路径的 `(deleted)` 标记。

根因：basename 相同的其他 sing-box 进程可能被当作 NetProxy 核心。

### 4. 收紧 API 默认监听范围

- [x] Service API 和 Clash API 默认只监听 `127.0.0.1`。
- [x] 保持固定本地入口、固定 UI 路径和现有本机客户端行为。
- [x] 更新配置契约测试，确认管理器、WebUI 和 zashboard 本机访问正常。

完成：两个 API 的默认监听地址收紧为 loopback，并同步更新配置契约、README 和用户文档；
LAN 访问不再默认开放。

根因：当前 API 监听 `0.0.0.0` 且使用固定密钥，局域网内存在未授权控制风险。

### 5. 仪表盘使用 Go 的 ready_at

- [x] Android 不再用本地当前时间伪造启动完成时间。
- [x] 启动成功后以 `service.status` 返回的 `ready_at` 为唯一来源。
- [x] 运行时间在启动、停止、重启和跨页面恢复时保持一致。

完成：删除管理器本地 `readyAt` 覆盖和 PID 猜测，仪表盘只使用 Go 服务快照中的 `ready_at`。

根因：仪表盘 reducer 存在本地 `readyAt` 覆盖值，可能早于 Go 服务真正 ready 的时间。

### 6. Android 刷新请求不丢失

- [x] 节点页和订阅页在刷新进行中收到新刷新请求时记录待刷新标记。
- [x] 当前请求完成后自动执行一次最新刷新。
- [x] 保持操作期间的加载状态和错误提示稳定。

完成：节点和订阅 ViewModel 在刷新占用期间合并请求，当前请求结束后自动补发一次静默刷新。

根因：两个 ViewModel 在已有刷新任务时直接 return，操作完成后的刷新可能被静默丢弃。

### 7. 收敛升级脚本中的 Worker 调用

- [x] 删除升级脚本中的宽泛 `pkill -f`。
- [x] 升级和卸载通过 PID 感知的内部 Worker 停止入口操作。
- [x] 统一 Worker 默认路径，避免 customize、uninstall 和 Go 使用不同参数。
- [x] 核对升级时 webroot、APK 和模块资源的同步边界。

完成：升级和卸载统一使用 `subworker --module-dir`，由 Go 计算 Catalog、配置、日志和二进制路径；升级同步加入 WebUI，Catalog 数据继续保留，APK 仍由独立安装提示处理。

根因：`customize.sh` 仍直接拼接 `subworker` 参数并用进程名杀进程，和 Go 生命周期事实源重复。

### 8. WebUI 命令参数解析

- [x] 终端命令支持单引号、双引号和反斜杠转义。
- [x] 节点/订阅名称包含空格时保持为单个参数。
- [x] 保持按钮调用路径使用结构化参数，不改变 `netproxyctl` 契约。

完成：新增纯参数解析器，执行路径和自动补全共用同一套 token 规则，不执行 Shell 展开；构建产物已重新生成。

根因：WebUI 终端使用简单 `split(/\\s+/)`，不能表达带空格的参数。

### 9. 统一 CLI 固定路径与输出契约

- [x] 删除 `netproxyctl` 内部重复的路径字段和默认值。
- [x] 固定路径统一由 Go 路径层计算，内部调用不再重复传递。
- [x] 明确 `--json` 是固定 JSON 契约，不保留无效的伪开关。
- [x] Android、WebUI、Shell 契约测试全部通过。

完成：新增 Go `paths` 包，`netproxyctl` 仅传递模块根目录；原生模块命令按模块根目录计算 Catalog、配置、运行时和二进制路径，`--json` 只作为固定 JSON 契约标记，不再维护无效状态字段。

根因：CLI 仍保留迁移期间的显式路径参数和没有实际行为差异的 `--json` 状态。

## 延后项

以下项目不是当前运行时故障，本轮不为了减少行数进行高风险拆分：

- Go 中超过 500 行的大文件拆分。
- `ShellConfigFile` 等用于 UI 展示的纯内存解析器删除。
- WebUI `ctl` 与 `ctlJson` 两个入口合并。
- 引入 staticcheck、golangci-lint 或 pprof。

## 全量验收

- [x] `go build ./...`
- [x] `go test ./...`
- [x] `go vet ./...`
- [x] Shell Catalog、CLI、运行时和服务状态契约测试
- [x] WebUI 构建
- [x] Android 单元测试、lint 和 assemble
- [x] 真机验证服务、Worker、节点、订阅、模式、应用策略和 API 本机访问
- [x] `git diff --check`

验收记录：Go、Shell、WebUI、VitePress 和 Android 构建均通过；真机使用临时 Android 交叉编译 CLI 完成服务状态、Catalog、节点、模式读取，并验证应用配置、eBPF 诊断和 Clash API 本机访问。Worker 只做只读状态确认，未触发服务启停或订阅更新。
