# 策略管理

NetProxy Android App 的“策略”入口管理路由策略定义，而不是节点分组。内置 AI、YouTube、TikTok、Netflix、GitHub、Google 和 Google FCM 七个策略；用户也可以在 App 中新增、编辑和删除自定义策略。节点和订阅仍在“节点”页面管理。

## API

- `GET /api/v1/policies` 返回 `schema=1` 策略列表及可用 Catalog 分组和节点。
- `POST /api/v1/policies` 创建自定义策略，至少提供一个 `package_names`、`domain_suffixes` 或 `ports` 匹配条件。未指定 `mode` 时默认跟随 `proxy`，不要求本地节点组非空；保存后可另选地区或节点。显式指定 `auto` 或 `manual` 时仍校验对应分组和节点。
- `PUT /api/v1/policies/{id}` 保存选择，并可修改自定义策略的 `name`、`package_names`、`domain_suffixes` 和 `ports`。内置策略只允许修改出口选择，也可以传 `disabled=true/false` 停用或恢复。
- `DELETE /api/v1/policies/{id}` 删除自定义策略；内置策略不能物理删除，因为缺失时会由默认集合补回。不需要的内置策略应在 App 中停用，停用后不会生成运行时路由规则。
- `PUT /api/v1/policies/{id}` 的 `mode` 支持 `auto`、`manual`、`direct` 和 `proxy`；`auto` 可通过 `region` 绑定一个地区并在该地区内自动选择节点，`manual` 选择具体节点，后两种分别选择直连或普通 Proxy。
- `POST /api/v1/policies/{id}/url-test` 使用现有节点测速服务测试当前策略选择。

策略选择保存在模块数据目录的 `data/policies.json`（重启后保留；安装升级保留逻辑由独立改动处理），使用独立的 schema=1 文件，不会改写 Provider 或 runtime 文件。服务运行时保存策略会请求 Native 重载；若重载失败，接口明确返回“已持久化、未同步”，Android 页面显示错误并重新读取已保存的策略，避免重复新增。服务停止时只保存选择，启动服务后由 Native 生成并应用运行时策略。订阅更新删除了手动选中的节点时，Native 会将选择回退为该节点来源内的 Auto。

新增或编辑策略时，弹窗等待保存结果：失败会保留输入内容并在弹窗内显示原因；成功才关闭弹窗并滚动到保存后的卡片。保存响应确认成功后，即使列表重读失败也保留已确认保存的卡片，并提示刷新错误。静默刷新不会清除保存失败或运行时同步失败的提示。

策略页面会展示服务运行状态，以及最近一次测速返回的节点状态：有延迟结果为可用、完成测速但没有延迟为失败、没有测速时间为未测速。自动测速结果由 sing-box 缓存，因此测速后显示的是核心返回的最新状态，不保证每个节点都由本次请求更新。

策略会生成独立的 runtime route overlay，并随服务启动、检查和重载加载。

AI 策略同时匹配 AI 域名、OpenAI/ChatGPT 相关域名后缀和已知 Android AI 应用包名，
其中包括 `com.openai.chatgpt`。因此 ChatGPT 应用不依赖域名识别也会进入 AI 策略的出口；
其他应用是否命中还取决于它是否使用已识别的包名，无法识别的应用仍需要通过域名规则匹配。
如果 ChatGPT 仍显示错误地区，优先检查 AI 策略当前选择的节点出口 IP 的地理归属；策略命中并不保证节点服务商把 IP 标记为目标地区。

自动地区模式选择的是地区，不是单个节点。Android 页面先选择地区，Native 会根据节点名称生成地区 URLTest
分组，并在运行时节点名称（`分组/节点`）上筛选，因此会在所选地区内自动选择可用节点。

Google FCM 使用独立的 `fcm` 本地规则集，覆盖 Google 推送域名，并对 TCP 5228、5229、5230 提供端口回退匹配。它不会扩大到整个 `google.com` 或 `googleapis.com`。FCM 默认跟随 Proxy，但可以在策略页单独切换到直连、某个分组节点、Auto；端口回退会跟随同一个 FCM 出口选择。
