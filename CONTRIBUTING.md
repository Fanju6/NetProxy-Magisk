# 参与贡献

感谢你为 NetProxy 提交改进。本仓库同时包含模块、原生组件、WebUI 和 Android 管理器，请把修改限制在对应目录，并说明是否改变跨组件契约。

## 目录边界

```text
src/module/          Magisk、KernelSU 与 APatch 模块文件
src/native/netproxy/ 节点、订阅与 Catalog 原生组件
src/webui/           模块 WebUI
src/android/         Android 管理器
```

Android 管理器通过 `netproxyctl` 的 `schema=1` JSON 契约访问模块。修改命令字段、错误码或状态语义时，必须同步检查原生组件、Shell、WebUI 和 Android 调用方。

## 代码约定

- 使用 UTF-8 和仓库规定的换行格式。
- 不提交订阅地址、节点凭据、签名文件、设备日志或本地开发配置。
- Shell 参数必须正确引用；JSON 输出只写 stdout，日志写 stderr。
- Android 页面只负责展示和事件分发，模块命令与 JSON 解析放在数据层。
- 修复缺陷时优先补充覆盖回归场景的测试。

## 本地检查

根据改动范围运行对应检查：

```bash
# 原生组件
cd src/native/netproxy && go test ./...

# WebUI
cd src/webui && npm ci && npm run build

# Android 管理器
cd src/android && ./gradlew testDebugUnitTest lintDebug assembleDebug
```

Android 管理器不会在仓库 CI 中自动构建。修改 Android 源码的贡献者需要在提交前完成本地检查；涉及 Root、模块命令、快捷设置磁贴或多用户应用时还需真机验证。

## Pull Request

- 一次 PR 只处理一个清晰主题。
- 描述修改原因、用户可见变化和验证方式。
- UI 修改请附截图或录屏。
- 不要把格式化、依赖更新和无关重构混入缺陷修复。
