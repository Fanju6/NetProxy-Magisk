# 架构说明

本文说明 NetProxy Android Manager 的代码边界、数据流和扩展原则。它不是模块实现文档；模块侧命令与 JSON 字段以兼容版本的 `netproxyctl` 契约为准。

跨组件事实源、Catalog、Provider、sing-box 运行时与发布边界见仓库根目录的 [AGENTS.md](../../AGENTS.md)，自动化编码约束同样在该文件。

## 设计目标

- Android 端只负责交互、Android 平台能力和状态呈现。
- 节点、订阅、服务与配置的事实源位于 NetProxy 模块。
- 页面不直接读写 `/data/adb`，也不通过 PID 猜测服务状态。
- Root 命令集中封装，所有参数经过 shell 转义。
- 功能域可以独立测试和演进，避免共享的全能 Repository 或 ViewModel。

## 分层

```text
Compose Screen
    ↓ event / StateFlow
ViewModel
    ↓ typed operation
Repository
    ↓ schema=1 JSON
NetProxyCtlClient
    ↓ escaped root command
netproxyctl
```

### `core`

`core` 只放跨功能域基础设施：

- `command`：`netproxyctl` 传输、严格 JSON 解码和短生命周期输入文件。
- `di`：应用组合根与 ViewModel 构造注入。
- `module`：模块安装环境和服务公共模型。
- `shell`：libsu 初始化与 root 可用性检查。
- `ui`：不包含业务状态的共享 Compose 组件和主题。

`core` 不依赖具体 feature。新增模块能力时，应先判断它属于某个业务域还是确实会被多个业务域复用。

### `feature`

每个功能域按需要包含：

```text
feature/<name>/
├── data/          # Repository 与平台数据源
├── model/         # 领域模型
└── presentation/  # Screen、ViewModel 与纯展示状态
```

不是每个目录都必须存在。只被一个页面使用的小型展示组件可以与 Screen 同文件；拥有独立状态、生命周期或复用价值时再拆分。

## 状态所有权

- `feature/inbound` 管理入站后端和原生 eBPF/TUN 表单，复用 `ConfigRepository` 的候选文件与分区 revision。唯一事实源为 `config/inbound/inbound.json`，表单更新只替换所拥有的原生字段并保留其余字段；冲突必须重新加载。
- `SettingsViewModel` 管理模块开关与独立「网络匹配」页的 Wi-Fi 策略；入站页只依赖 `InboundViewModel`，分应用入口位于入站页。分应用通过 `ConfigRepository` 保存带 revision 的 `inbound/app` 分区，Go 负责应用与回滚；`AppsViewModel` 合并全部模式、开关与名单编辑，仅在离页或进入后台提交，不设闲置计时器。应用条目不保存勾选状态，统一读取当前名单；搜索匹配结果与勾选独立，只有关键词、过滤条件和应用清单变化时重新计算。默认列表与搜索列表复用选中优先和反序的显示排序，勾选只更新名单与排序投影；搜索重排保持视口位置，不跟随已选条目上移。搜索层由 Scaffold 的 popupHost 承载，仅搜索框上移，普通列表不做位移动画，模糊仍只采集普通内容；两份列表的滚动状态均在切换内容之外持有，展开状态由搜索框位置动画完成后确认，不由输入焦点改写。
- 入站原生参数和网络匹配同样合并已确认的页面草稿，离页或进入后台时提交；未确认弹窗不进入草稿。普通返回先登记提交快照，立即导航，不展示待保存提示。`AppContainer` 持有应用级 `ConfigurationWrites`，只管理任务生命周期、按配置文件等待写入和失败通知；不管理草稿类型或配置事务。任务不随页面 ViewModel 销毁取消，`ConfigRepository` 只等待目标文件的在途写入，写后确认在解除等待后读取配置。草稿比较、revision 和事务结果仍归各功能域，没有磁盘草稿或后台保存服务。写入期间的新编辑和撤销继续保留，旧结果只推进确认基线与 revision；提交触发后的新编辑须等下次触发，不自动追随排队或在途保存。页面存续时失败保留草稿，重新加载须显式放弃，不借用新 revision 重试。后端切换、重启和进入入站子页仍先等待参数提交；已确认切换与重启由应用级任务完成收尾，未确认切换不在离页后执行。开机自启、节点、模式和主题保持即时操作，显式保存的编辑器不受影响。
- `feature/routing` 直接复用 `ConfigRepository` 编辑 proxy/direct/block 本地规则集。表单只编辑单条件规则，保留其他原生规则及顺序；多条件规则继续使用 JSON 编辑器。保存失败保留草稿，只有确认原 revision 未变时才允许修正后重试，否则要求显式重新加载。
- 后端切换在服务运行时需要确认；只有配置与实际服务后端均确认后才显示成功。`active_backend` 保留内部校验，不作为常驻设置项展示，为 null 时不从配置推测。首次配置与选项读取完成后一次展示表单，刷新保留旧表单和正常颜色；保存取消在途刷新，避免旧结果覆盖新 revision，写入仍串行并检查冲突。完整入站 JSON 编辑位于入站页，包装 Schema 引用内置 sing-box 原生定义。
- 入站列表输入每行一个值，候选勾选直接操作列表，不按逗号拆分原生标签或接口名。保存失败保留候选与原 revision 供用户核对；后续刷新失败时实际状态未知，不沿用旧 ready 快照或自动重试。
- 入站保存不弹出整页加载框或改变表单颜色；耗时操作仅在对应项显示局部进度。编辑弹窗固定筛选方式、输入区和按钮，只有候选列表独立滚动，长输入在输入框内滚动。
- 运行时受管入站唯一为 `runtime/inbound.json`，原有 providers/outbounds 仍单独保留并只读展示。

- ViewModel 持有页面状态，并通过不可变 `StateFlow` 暴露。
- Repository 负责命令组合与模块响应映射，不保存 Compose 状态。
- `AppContainer` 只在应用入口创建长期依赖，不提供运行时服务定位。
- Miuix Nav 条目拥有自己的 ViewModelStore；列表、详情和编辑页面使用独立 ViewModel。
- `MainActivity` 组合主分页，`MainBottomBar` 是唯一底部导航实现；主题状态不参与导航结构选择。
- `ModuleAccessViewModel` 在应用入口检查 Root 和模块控制接口，启动与恢复前台共用同一状态，检查在途时不清空上次结果。未确认可用时主导航只保留仪表盘和设置，主题与关于可独立使用；模块路由由 `ModulePage` 统一阻止无效读取，并保存编辑器的可恢复状态。分页保存目的地身份，不保存会随权限变化失效的索引。
- 首次读取失败使用 `ContentStatus` 在 Scaffold 内容区居中展示，不作为空列表或默认配置；后续读取失败保留已加载内容和草稿。日志按服务/核心分别持有加载与错误状态，不让另一页成功读取清除本页失败。
- 仪表盘快照合并放在纯 Kotlin reducer 中，避免异步响应在 UI 层互相覆盖。

## CLI 契约

Android 端只接受一份完整 JSON：

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

- stdout 只能包含 JSON，模块日志写入 stderr。
- `schema` 不匹配时拒绝解析，避免静默误读新旧接口。
- 命令失败统一转换为 `NetProxyCtlException`，保留稳定错误码。
- 文件导入与配置保存使用应用缓存中的短生命周期文件，调用结束后清理。
- 短读使用共享 Shell 的异步结果等待，长操作和写入使用独立 Shell。页面取消不会杀共享 Shell，也不会中断已开始的 Native 事务；输入文件在实际命令结束后删除。生命周期写入获得完整的 120 秒预算，订阅下载仍由 Native 的订阅超时控制。
- 诊断包保存后删除临时副本；分享副本保留一天，在下次导出时清理过期文件，避免外部应用读取前被删除。
- UI 不解析 `module.conf`、Catalog 文件、日志文本或进程列表来推断业务结果。

## 安全边界

- 应用数据禁止系统云备份。
- FileProvider 只共享 `cache/reports/` 下的诊断包。
- 用户输入不得直接拼接到 shell 命令。
- 日志与诊断包的敏感信息脱敏由模块统一完成，Android 端不重复实现另一套规则。
- 发布签名、订阅地址、节点凭据和设备日志不得进入仓库。

## 测试策略

- `NetProxyCtlCodecTest` 固定 CLI JSON 与错误语义。
- reducer、解析器、配置转换和 schema 补全使用 JVM 单元测试。
- Root 授权、模块命令、文件分享、快捷设置磁贴和多用户应用枚举使用真机验证。
- 每次提交至少运行：

```bash
./gradlew testDebugUnitTest lintDebug assembleDebug
```

发布前额外运行 `assembleRelease`，确保 R8 和资源压缩可用。

## 扩展原则

1. 新命令先在模块定义稳定 JSON 契约，再增加 Android Repository 方法。
2. ViewModel 通过构造参数接收依赖，不读取 Application 单例。
3. 业务判断优先写成纯函数并覆盖测试。
4. 页面文件按职责拆分，不以行数作为唯一标准。
5. 界面文案使用完整的英文默认资源及中文、俄语资源，由 Android 自动匹配系统语言；未支持的语言使用英文，不增加应用内语言状态。
6. 不为尚未出现的需求预建抽象层，出现真实重复后再提取。
