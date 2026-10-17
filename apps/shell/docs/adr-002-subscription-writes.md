# ADR 002: URL 订阅写入与长事务

日期：2026-10-17。状态：已实施；实际验证范围见 [M4.3.2 验收报告](subscriptions-report.md)。

## 业务事实与顺序

事实源为 Go httpapi/resources.go、module/app.go、subscription/update.go/edit.go。
先验证单组 update/cancel，再开放 add/edit/remove/update-all；不改 Go 更新引擎。

- add 先提交分组，edit 先提交设置，随后才下载/同步。失败可能 persisted=true，不能自动重试或回写旧草稿。
- update 成功结果含 group_id、revision、not_modified、persisted、runtime_synced/state/pending。
  304 不代表下载了新内容。下载或转换失败保留已有 Provider。
- 取消只写活动更新的取消标记，空闲时可以成功但没有取消任何任务。提交后的效果不能撤回；进度消失不代表成功或取消完成。
- 单次 HTTP 下载预算为 min(2*download_timeout+120s,600s)，全部更新总预算 600s。
  复用 Kotlin 既有订阅 615s 等待，不把路由操作的 150s 观察窗口当执行期限。
- 客户端后台/离页取消等待，不取消 Go 已接受的写入。恢复时读取 Catalog、详情、进度和历史；没有任务 ID 时只能展示当前事实，不能精确归因响应丢失的旧任务。
- 更新全部按 Catalog 顺序执行。现有 HTTP 错误路径丢弃 Summary.updated/failed，不能准确展示部分结果。
  允许仅在该 handler 的错误 Envelope 中附加已执行的 updated/failed，保留原 code/status/persisted 数据；补回归测试，不重构领域层。
- 删除必须确认、重新 GET Catalog 核对 URL 目标与替代组；活动组选择非空有效替代，Go 锁内最终校验。

## 私有字段：只写输入

不开放 private=true，也不新增私有 Metadata 读取。现有原生公开 DTO 投影保持有效。
用户主动输入的 URL、UA、HWID、自定义 Headers 允许短暂存在 React 表单内存、一次 IPC 参数和原生请求正文。
这意味着受信 WebView 内容/XSS 若被攻破，可能取得本次输入；不能声称凭据从未进入 JS。
应用仅加载打包页面，继续受 CSP、固定 IPC、无通用网络/Root/文件能力及既有 Android 信任根保护。
模块管理 Bearer 与证书始终只留在原生层，与用户订阅输入无关。

敏感草稿不放 Query/Mutation 缓存、LocalStorage、路由、日志、错误或诊断。发送不用保存 variables 的 TanStack Mutation；只缓存无敏感字段的操作元信息和投影后的结果。
提交、关闭或页面卸载清理表单引用；JS 字符串/IPC/运行时内存不能承诺可验证的物理清零。
错误仅保留白名单 code、状态、业务 ID、persisted/runtime 同步事实，不回显正文、URL 或原始异常。

编辑公开设置从公开详情取得；无法读取的字段默认“不修改”。明确勾选修改才发送：省略=保留，空字符串或空 Headers 对象=清空，null/错误类型/未知字段拒绝。
URL 不能清空；UA/HWID/include/exclude 可显式清空。高级项未修改时不注入默认值。
订阅 allow_insecure 是 Go 下载选项，不允许影响模块管理 TLS。

## 操作边界与恢复

Rust 使用操作枚举和强类型正文，Kotlin 再独立校验字段、类型、ID、大小并构造固定方法/路径/query。
普通订阅写入共享既有原生业务写槽，cancel 使用独立有界控制槽，否则无法取消正在等待的更新。
前端用现有 QueryClient 存最少的订阅操作元信息，和 service/routing 双向同步准入，不建立新的全局 Store 或重连器。
pending 与尚在服务预算内的 uncertain 阻止冲突写入；取消、读取、显式复核不被阻止。超过预算后不无限锁住用户，但仍保留未知结果和原错误，不自动重发。
成功响应之后重新读取权威资源；未知结果不通过进度或时间猜测为成功。删除后失效节点/选择/服务，SSE 初始帧和成员变化仍通过 GET 校准。

## 验证边界

采用匿名可控 HTTP 订阅源，覆盖有效内容、ETag/304、失败、慢下载和取消；先备份设备 Catalog/Provider/模块配置，验收后逐项核对恢复。
模拟测试不代替设备；没有可用设备、可控源或安全恢复条件时明确标记未验证。
只扩展订阅业务，不开放任意 HTTP、Root/Shell/文件、节点编辑、配置或应用策略。
