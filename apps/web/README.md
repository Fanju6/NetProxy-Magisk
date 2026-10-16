# NetProxy V9 WebUI

V9 独立前端工程。保留 V8 管理器、终端 WebUI、Go API 和模块发布流程；Android 原生壳的可信通信与服务控制范围见 [接入约定](docs/native-transport.md)。普通 Web 构建不连接真实模块。

## 开发

Node.js 24 LTS，pnpm 12.11.2。依赖使用精确版本和 `pnpm-lock.yaml`，不需要启动 Go 或连接手机。

```sh
cd apps/web
pnpm install --frozen-lockfile
pnpm dev
```

打开 <http://127.0.0.1:5180/>。开发服务器仅监听本机。页面始终标注“演示模式”，服务、节点和配置操作只修改当前 Mock 会话，刷新后重置；主题和语言偏好保留。

```sh
pnpm check          # 类型、Lint、格式、单元与组件测试
pnpm build          # 安全默认：无可信 Transport 时不连接、不操作模块
pnpm build:demo     # 显式包含 Mock 的可交互静态演示
pnpm build:native   # Tauri 受限 IPC 构建，输出 dist-native/
pnpm preview       # 预览最近一次构建，端口 5181
pnpm exec playwright install chromium
pnpm test:e2e      # 自动运行 Mock dev 与独立生产预览服务器
```

普通与演示构建输出本目录 `dist/`，原生构建输出 `dist-native/`，均不会覆盖 `src/module/webroot/`。不要直接将本工程替换进正式模块。

## 结构

| 位置                      | 职责                                                          |
| ------------------------- | ------------------------------------------------------------- |
| `src/main.tsx`            | 环境选择 Transport、组合入口；生产默认拒绝真实连接            |
| `src/app/`                | Hash 路由、响应式导航、依赖注入、主题和中英文本               |
| `src/api/`                | Go DTO 校验、结构化错误、Transport 边界、Query 与单条事件会话 |
| `src/features/`           | 仪表盘、节点/分组、订阅、设置；按领域组织                     |
| `src/components/ui/`      | 官方 shadcn CLI 生成的 Vega/Base UI 组件                      |
| `src/components/`         | 选择、响应式详情及加载/空/错误状态                            |
| `src/mocks/`              | 匿名源结构夹具与内存 Mock Transport                           |
| `e2e/`                    | 桌面、移动和生产安全默认检查                                  |
| `scripts/check-build.mjs` | 静态资源完整性与 Mock 构建隔离检查                            |
| `docs/`                   | V8 迁移基线、架构决策与本批交付报告                           |

## 设计与边界

- React + TanStack Router/Query；业务数据归 Query，短时交互归组件。未引入全局业务 Store。
- 官方 shadcn **Vega + Zinc + Base UI + Lucide + Geist**，不是仿制旧 Compose 样式。桌面侧栏、手机底栏共用相同四个目的地。
- 节点分组直接展开，支持网格/列表、搜索、Auto/手选及详情；空组无 Auto。停服后 Catalog 仍可浏览。
- 普通生产构建不包含 Mock，不内置 Secret、不请求 Root、不直连自签证书。显式原生构建通过受限 Tauri IPC 读取 info/service 和 SSE，写入禁用；真实 Android 通道仍需父任务验收，见 [原生 Transport 约定](docs/native-transport.md)。Browser Transport 尚未实现。
- 所有权威 API 路径均相对 `/api/v1`；后续 Transport 负责固定端点和可信连接，页面不得自行 `fetch`、IPC 或执行命令。
- 不修改模块持久化结构，不增加旧格式兼容或迁移逻辑。

详细依据见 [完整 V9 架构规范](docs/NETPROXY_V9_ARCHITECTURE.md)、[V8 功能与 API 基线](docs/v8-baseline.md)、[架构决策](docs/architecture.md) 和 [交付报告](docs/delivery.md)。
