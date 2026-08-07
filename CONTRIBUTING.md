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

## Android 管理器

Android 管理器不会在普通模块 CI 中自动构建，本地检查是唯一防线。修改 Android 源码的贡献者需要在提交前完成本地检查；涉及 Root、模块命令、快捷设置磁贴、多用户与应用分身、Navigation 动画或 eBPF 时还需真机验证。

## Pull Request

- 一次 PR 只处理一个清晰主题。
- 描述修改原因、用户可见变化和验证方式。
- UI 修改请附截图或录屏。
- 不要把格式化、依赖更新和无关重构混入缺陷修复。
