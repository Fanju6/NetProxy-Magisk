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

- ViewModel 持有页面状态，并通过不可变 `StateFlow` 暴露。
- Repository 负责命令组合与模块响应映射，不保存 Compose 状态。
- `AppContainer` 只在应用入口创建长期依赖，不提供运行时服务定位。
- Navigation3 条目拥有自己的 ViewModelStore；列表、详情和编辑页面使用独立 ViewModel。
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
5. 当前产品以中文为主；新增可复用或计划本地化的文案优先放入字符串资源。
6. 不为尚未出现的需求预建抽象层，出现真实重复后再提取。
