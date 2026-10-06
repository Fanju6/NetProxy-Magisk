# 参与贡献

感谢你为 NetProxy 提交改进。本仓库同时包含模块、原生组件、WebUI 和 Android 管理器，请把修改限制在对应目录，并说明是否改变跨组件契约。

开始修改前请先阅读 [AGENTS.md](AGENTS.md)：目录职责、跨组件契约、各组件编码约定、提交与注释要求，以及按改动范围执行的验证命令都在那里，本文件不重复。Android 内部分层见 [src/android/ARCHITECTURE.md](src/android/ARCHITECTURE.md)；源码结构的概览见 [README](README.md) 的「源码结构」。

Android 管理器通过 `netproxyctl` 的 `schema=1` JSON 契约访问模块。修改命令字段、错误码或状态语义时，必须同步检查原生组件、Shell、WebUI 和 Android 调用方。

## 提交前

- 按 AGENTS.md 验证章节列出的检查执行，并在 PR 里说明实际跑了哪些。
- 使用 UTF-8 和仓库规定的换行格式。
- 不提交订阅地址、节点凭据、签名文件、设备日志或本地开发配置。这类内容一旦进入 Git 历史就很难彻底移除。
- 修复缺陷时优先补充覆盖回归场景的测试。
- 提交信息格式见 AGENTS.md 的「Git 提交约束」。

## 本地验证

使用 `tests/verify.sh` 选择与改动相符的范围；它只编排仓库已有检查，不会自动暂存、提交或发布任何文件：

```sh
sh tests/verify.sh quick    # Native、模块 Shell 契约与工作流脚本
sh tests/verify.sh webui    # WebUI 类型检查、单测与构建
sh tests/verify.sh android  # Android 单测、Lint 与 Debug 构建
sh tests/verify.sh docs     # 文档内容检查、单测与构建
sh tests/verify.sh full     # 发布前全量验证
```

WebUI 修改需要提交构建后的模块页面时，使用 `sh tests/verify.sh webui --check-generated`。该选项只确认工作区生成结果与 Git 索引一致；请自行检查和暂存生成文件。

GitHub Pull Request 会运行同一套无密钥验证。来自 Fork 的代码不会读取签名、发布或 Telegram Secrets，也不会生成 Nightly 包；合入 `main` 后才由现有 CI 构建签名管理器和模块包。

## 报告问题

请通过 GitHub 的“缺陷报告”表单提交可复现问题，并附完整版本、设备环境和最小复现步骤。Issue 公开可见，禁止提交订阅地址、节点凭据、请求头、Token、签名文件或未脱敏诊断数据；优先使用管理器导出的脱敏诊断包。

## Android 管理器

CI 的管理器任务与标准模块构建并行启动，按变更范围在同一次 Gradle 调用中执行 Android 单元测试、Lint 和 Release 构建，并启用构建缓存。管理器 APK 使用 GitHub Secrets 提供的固定密钥签名，发布阶段仅将其追加到标准包，不重复编译或压缩；该 APK 和签名材料不提交到仓库。Android 源码改动会同时触发模块重打包。CI 管理器版本带提交短哈希，不显示安装来源或签名警告；使用同一固定密钥的后续构建可以覆盖升级。构建需要 `ANDROID_KEYSTORE_BASE64`、`ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_ALIAS` 和 `ANDROID_KEY_PASSWORD`，纯验证任务不需要签名材料。修改 Android 源码仍需在提交前完成本地构建；涉及 Root、模块命令、快捷设置磁贴、多用户与应用分身、Navigation 动画或 eBPF 时还需真机验证。

## Pull Request

- 一次 PR 只处理一个清晰主题。
- 描述修改原因、用户可见变化和验证方式。
- UI 修改请附截图或录屏。
- 不要把格式化、依赖更新和无关重构混入缺陷修复。
