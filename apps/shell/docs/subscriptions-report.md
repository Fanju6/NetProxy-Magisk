# M4.3.2：URL 订阅管理

日期：2026-10-17。分支：`feat/v9-webui-foundation`。
开发基线：`93a7aa3fd107e98529139eff1bc5a506d2eea4e2`。

## 实现与能力

沿用 React ApiClient → Tauri Rust → Kotlin → LocalHttpTransport → Go。
本地组只读，原有 GET、公开 DTO 投影、TLS 钉扎和 Bearer 隔离不变。

| 操作 | 原生命令与动作 | 固定 Go API |
| --- | --- | --- |
| 单个更新 | subscription_action/update | POST /subscriptions/{id}/update |
| 请求取消 | subscription_action/cancel | POST /subscriptions/{id}/cancel |
| 新增 | subscription_write/add | POST /subscriptions |
| 编辑 | subscription_write/edit | PUT /subscriptions/{id} |
| 删除 | subscription_write/remove | DELETE /subscriptions/{id} |
| 更新全部 | subscription_write/update_all | POST /subscriptions/update-all |

路径均带 `/api/v1`。Rust 与 Kotlin 独立拒绝未知字段、null、非法 ID、类型及超限正文。
删除的可选 replacement_group_id 由原生构造；不开放任意 query、Headers、路径或 HTTP 方法。
业务写入共享原有写槽；cancel 使用独立有界控制槽，避免被正在等待的更新阻塞。
服务、路由与订阅在进入 Transport 前同步检查准入，不仅禁用按钮。

资源页增加新增、编辑、二次确认删除、单个/全部更新与取消。编辑面板由底部展开，保留键盘和安全区。
活动组删除要求有效非空替代组，并在发送前重新读取 Catalog；最终一致性由 Go 锁内校验保证。
Mock 同步支持这些交互，普通浏览器生产构建仍拒绝连接，不用 Mock 掩盖 Native 失败。

## 私有字段

安全决策见 [ADR 002](adr-002-subscription-writes.md)。
不开放 private=true，GET/SSE 不返回已有 URL、Headers、HWID 或原始异常。
用户主动输入的新私有值短暂存在表单、一次 IPC 和原生正文中，受信页面遭 XSS 攻破时存在读取风险；不能声称从未进入 JS。
表单关闭、提交或卸载清理引用，不承诺字符串的物理清零。
敏感输入不进入 Query/Mutation 缓存、持久存储、路由、日志或错误提示。
模块 Bearer 与证书始终仅存原生私有存储。

编辑省略字段表示保留，明确启用替换后才发送；空字符串/空 Headers 对象表示清空，URL 不允许清空。
订阅 allow_insecure 只控制 Go 订阅下载，不影响管理接口证书校验。

## 长事务与恢复

- 先完成并在真机验证单个更新、取消，再扩展其余写入。
- Go 更新预算最多 600 秒，Kotlin 等待 615 秒，前端观察上限 620 秒；不使用路由操作的 150 秒窗口。
- 不重试写入。离页、后台或断线只取消客户端等待，不宣称 Go 已回滚。
- 恢复时读取 Catalog、详情、历史和进度；新 SSE 会话重新校准，不增加第二套重连器。
- 客户端 intent 只防止迟到回调串操作，不是服务端任务 ID。
- 进度只展示 download/convert/validate/apply，没有虚构百分比。文件消失不能证明成功或取消完成。
- 保留原始错误码、persisted 和运行时同步结果。新增/编辑失败可能已保存，不能回填旧值覆盖或自动重发。
- 无法归因的丢失响应继续显示未知结果；观察窗口结束后释放准入，不无限阻塞操作，也不宣布成功。
- 更新全部分别展示成功与失败分组，不将部分成功表述为全部成功。

Go 仅调整 update-all HTTP 错误 Envelope：保留领域层 Summary 的 updated/failed，附带原 first_error 及 persisted；原错误码与状态码不变。
增加两个 Go 回归测试，不修改下载引擎、持久化结构、安装规则或 V8 管理器。

## 自动化验证

| 命令 | 实际结果 |
| --- | --- |
| apps/web: pnpm check | 类型、Lint、格式检查通过；16 文件、347 项 Vitest 通过 |
| apps/web: pnpm test:e2e | 18 项通过，含桌面、移动端、生产与 Native 浏览器隔离 |
| apps/web: build/build:demo/build:native | Playwright 构建验证通过 |
| cargo fmt --all -- --check | 通过 |
| cargo test --locked -p tauri-plugin-module-api --all-targets | 9 项通过 |
| cargo clippy --locked -p tauri-plugin-module-api --all-targets -- -D warnings | 通过 |
| Gradle :tauri-plugin-module-api:testDebugUnitTest :tauri-plugin-module-api:lintDebug | 129 项 Kotlin 测试及 Lint 通过 |
| node --test apps/shell/tests/subscription-source.test.mjs | 2 项通过 |
| sh tests/verify.sh quick webui | Go 测试/vet、Shell/Catalog/打包契约和旧终端构建通过；保留脚本原有 Windows 条件跳过 |
| V8 Gradle :app:testDebugUnitTest :app:lintDebug :platform:test | 通过 |
| Tauri Android arm64 Debug 构建及固定签名安装 | APK 完成构建并安装；本地 CLI 外层 PowerShell 在完成后未退出，单独终止其空闲包装进程 |

测试覆盖固定 IPC 权限、遗漏/清空、已提交失败、304、取消/取消过晚、并发准入、部分成功、SSE 与 GET 竞争、卸载和旧回调、写入不重放及缓存不含私有输入。
远端 Web 与 Android Shell 工作流结果以本提交对应的 GitHub Actions 为准。

## 真机证据

专用 Mi 10，Android 16/API 36。同包名、固定签名、versionCode 1102 测试 APK。
采用仓库内匿名 loopback 测试源，经 ADB reverse 连接，不使用用户私有订阅作破坏性实验。
设备上的 Root 私有备份保留原 Catalog、Provider、配置及旧二进制；测试后逐文件哈希核对 Catalog、Provider 和三份配置已恢复，服务恢复原 ready 状态。

已实际验证：

- 单组 304、内容变化、下载失败保持 Provider 不变。
- 活跃下载取消、取消过晚后再次更新，以及后台取消等待后 Go 仍完成且请求未重放。
- 新增、仅改名称保留私有字段、明确清空私有字段。
- 编辑下载失败时 persisted=true 且 Provider 不变；新增失败留下已提交的空组。
- 普通写入被共享槽拒绝时，独立取消仍可执行。
- 更新全部成功/失败 ID 分别返回，保留原错误事实。
- 活动组删除并选择原非空组作为替代。
- 直接非法 IPC、额外字段、null、任意路径与私有查询被拒绝。
- 最终 WebView 浅/深色和真实软键盘：保存、关闭可触达，无横向溢出。
- 最终 APK 两次 Activity.recreate 后 Catalog/detail/nodes/mode/service 读取正常，销毁前 SSE 已连接。

脱敏断言、截图和完整设备备份仅留本地/测试设备，不提交私有资料。
部分 UI 自动化的 CDP 命中坐标受 Android IME 影响，改用实际 ADB 触摸并验证弹层关闭；不将该脚本初次失败冒充通过。

## 限制与后续

Go 无事务任务 ID：响应丢失时不能精确归因历史记录，已明确展示未知结果。
编辑接口没有条件 revision，不能提供服务端不存在的跨客户端编辑冲突保护。
更新全部仅有每组成功/失败 ID 与首个错误详情，不能推导其余分组的详细失败原因。
后台长写入在先行单更新版本实测；最终 APK 的 Activity 重建单独实测，最终版本“长写入同时重建”的组合由自动化覆盖，未作额外真机证明。
本次未覆盖全部厂商系统或 Root 框架，不声称 Windows/Linux 可用。
下一阶段须先独立审查本轮权限、私有输入与事务确认；本轮不迁移配置或应用策略，不合并 main，不发布。
