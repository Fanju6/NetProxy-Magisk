# 7.2.0 与 8.0 对比

## 一页结论

| 领域 | 7.2.0 | 8.0 | 用户影响 |
|---|---|---|---|
| 透明代理 | TPROXY/iptables/IPSET Shell 数据面 | sing-box eBPF 入站 | 不再维护防火墙规则，但要求内核支持 eBPF |
| 业务实现 | 多个 Shell 脚本分担服务、节点、订阅和网络逻辑 | Go Native 统一业务，Shell 只做开机桥接 | 状态、错误和事务更集中 |
| 节点存储 | `CURRENT_CONFIG`、旧 outbounds 和文件扫描 | Catalog `meta.json` + `provider.json` | 节点和订阅可独立浏览、编辑和恢复 |
| 订阅 | Shell 下载、转换和调度 | Go 事务更新 + 唯一 Worker | 支持条件请求、更新状态和失败保留 |
| 控制入口 | 旧 Shell CLI 和文本输出 | `netproxyctl` + `schema=1` JSON | Android、WebUI 和终端使用同一契约 |
| 运行时切换 | Shell 改配置或重启相关路径 | Service API/Provider 热更新 | 同组切换通常不需要重启核心 |
| 管理界面 | 旧 WebUI 和内置 APK | Android 管理器、模块 WebUI、Service Dashboard、zashboard | 管理入口更多，但各自职责分离 |
| 规则资源 | 混合 source、tproxy 和内置资源 | local rules、remote SRS、Catalog、runtime 分离 | 用户配置和内置资源边界更清晰 |
| 发行包 | 单一旧模块布局 | 标准包与含管理器包 | 可以不安装内置 APK |

## 1. 架构变化

### 7.2 的控制和数据路径

```text
模块 service
  -> Shell service/runtime/switch/subscription/netmon
  -> sing-box TPROXY
  -> iptables/IPSET/路由表
  -> 节点文件和旧配置路径
```

多个 Shell 入口分别读取配置、生成 JSON、调用系统工具并判断进程状态。网络数据面和控制逻辑紧密耦合，节点、订阅和运行时配置之间通过文件路径关联。

### 8.0 的控制和数据路径

```text
Android / WebUI / 终端
          |
          v
     netproxyctl
          |
          v
     Go Native
   /     |       \
Catalog  Worker   module/service
   |       |        |
Provider  网络状态  sing-box
                     |
                     v
              eBPF 入站数据面
```

当前模块根目录的 Shell 只有 `src/module/service.sh`，它把模块 service 阶段交给 `netproxyctl __internal boot`。Shell 不再作为公共业务层，也不再解析 JSON 或拼接运行时配置。

这不是简单的“把 Shell 改成 Go”：同时变化了事实源、服务边界、节点模型、运行时投影和客户端契约。

## 2. 透明代理数据面

### 7.2

7.2 使用 TPROXY 入站和传统 Linux 网络控制路径，相关内容包括：

- `tproxy` inbound；
- `scripts/network/tproxy.sh`；
- `config/tproxy/` 区域文件和规则；
- iptables/IPSET、mark、路由表、防回环和清理逻辑；
- 模块内置 IPSET/内核模块资源。

### 8.0

8.0 由 sing-box 的 eBPF inbound 接管透明代理流量。配置由 Go 根据 `ebpf.conf` 生成，数据路径包含：

- 本机 cgroup TCP/UDP 接管；
- DNS 53 hijack；
- `include_package`、`exclude_package` 和 `include_android_user`；
- eBPF IPv6 模式和绕过规则集；
- shared-network、热点接口、源 CIDR 和 MAC 地址过滤；
- TCP/UDP redirect Map、bypass Map 和 fragment Map。

eBPF 是 sing-box 的入站模式，不是独立核心，也不应在服务状态、切换按钮或发布说明中称为“eBPF 服务”。

### 实际影响

- 不再由 NetProxy 维护整套 iptables/IPSET 透明代理规则；
- 删除传统 TPROXY/REDIRECT 回退后，内核能力成为硬要求；
- 不同 Android 厂商内核的兼容性需要设备验证，Host 构建不能替代这一验证。

## 3. 节点与 Catalog

### 7.2

7.2 通过 `CURRENT_CONFIG` 表达当前节点配置，并依赖旧 outbounds 目录、脚本扫描和文件路径来组织节点。节点选择、订阅分组和运行时出站之间不是独立的稳定数据模型。

### 8.0

Catalog 是节点与订阅的持久事实源：

```text
data/catalog/
├── default/
│   ├── meta.json
│   └── provider.json
├── <group-id>/
│   ├── meta.json
│   ├── provider.json
│   └── history.jsonl
└── staging/
```

- `default` 固定表示本地配置组；单节点和本地文件节点追加到这里。
- URL 订阅使用稳定的分组 ID，名称保存于 `meta.json`。
- `provider.json` 是节点内容事实源，使用标准 sing-box Provider 文档。
- `meta.json` 保存分组类型、名称、节点数、revision、订阅状态和运行时同步状态。
- `staging` 只用于事务临时文件，不能被当作持久分组读取。
- 运行时标签优先使用可读分组名称，名称冲突时才追加分组 ID。

### 选择语义

```ini
ACTIVE_GROUP_ID="default"
SELECTOR_MODE=urltest
SELECTED_NODE_REF=""
OUTBOUND_MODE=rule
```

- `urltest` 对应 `Auto/<group>`，自动测速的实际节点由 Service API 返回。
- `manual` 保存 `<group-id>/<tag>`，不保存节点文件路径。
- 手动节点在订阅更新后消失时回退该组 Auto，不回退 direct。
- 8.0 不把某个 Auto 内部测速结果当成用户手动选择。

## 4. 订阅与更新事务

### 7.2

订阅下载、转换、调度、文件替换和日志由多个 Shell 脚本分担。网络失败、取消、元数据更新和运行时应用之间的状态边界不够集中。

### 8.0

Go 订阅服务按以下事务执行：

```text
分组锁
 -> 条件下载
 -> Header 解析
 -> 节点转换
 -> Provider 校验
 -> staging
 -> Provider/meta 原子提交
 -> 运行时同步
 -> 脱敏历史
```

支持的 HTTP/订阅信息包括：

- `Subscription-Userinfo` 流量和到期时间；
- `Profile-Title`、`Content-Disposition`；
- `profile-update-interval`；
- `ETag`、`Last-Modified` 和 304；
- User-Agent、HWID、自定义 Header、代理策略、超时和 TLS 校验。

失败语义分为两层：

1. 持久化失败：旧 Provider 必须保留，更新不能伪装成功。
2. 持久化成功但运行时同步失败：节点已经保存，记录 `runtime_sync_pending` 和错误，后续可以重试。

Catalog 通过跨进程锁、journal 和 staging 恢复避免 Provider/meta 半提交。Worker 负责自动更新，不依赖 `crond`。

## 5. 服务生命周期和性能语义

### 7.2

服务启动、停止、节点切换、网络监听和订阅调度由多个脚本和进程参与，运行状态主要通过进程和文件间接判断。

### 8.0

服务状态固定为：

```text
stopped -> preparing -> starting -> ready -> stopping -> stopped
                         \-> failed
```

只有 sing-box Service API 与 eBPF 入站都就绪后才写入 `ready_at`。`outbound_mode` 表示核心实际模式，`configured_outbound_mode` 表示用户保存的基础模式。

#### 可复现的用户操作场景对比

以下是在同一台 Windows Host 上，用 7.2.0 基线代码和当前 8.0 beta.7 代码生成相同规模的匿名节点夹具，分别执行用户实际会触发的操作。每次都启动独立命令进程并丢弃输出，只统计命令完成耗时，不包含网络请求。

| 场景 | 7.2.0 命令 | 8.0 beta.7 命令 | 用户感知 |
|---|---|---|---|
| 启动前准备 | `initialize_runtime_context` + `scan_runtime_nodes` + `write_runtime_outbounds` | `netproxyctl service start` 内部的 Go `module.Prepare` | 启动服务前生成运行时配置 |
| 节点页加载 | `scripts/cli node list` | `netproxyctl catalog list` | 打开节点页并读取分组摘要 |
| 仪表盘状态 | `scripts/cli service status` | `netproxyctl service status` | 首页轮询服务和当前配置状态 |
| 停止服务时切换节点 | `scripts/core/switch.sh config` | `netproxyctl node use` | 选择节点并持久化选择状态 |

平均耗时如下，单位为毫秒；1、100、1000 节点规模各运行 10 次，5000 节点规模除准备操作运行 1 次外，其余运行 5 次：

| 节点总数 | 场景 | 7.2.0 | 8.0 beta.7 | 8.0 耗时变化 | 加速比 |
|---:|---|---:|---:|---:|---:|
| 1 | 启动前准备 | 286.30 | 28.50 | -90.05% | 10.05x |
| 1 | 节点页加载 | 323.29 | 32.78 | -89.86% | 9.86x |
| 1 | 仪表盘状态 | 468.95 | 33.48 | -92.86% | 14.01x |
| 1 | 停止服务时切换节点 | 503.85 | 92.57 | -81.63% | 5.44x |
| 100 | 启动前准备 | 240.66 | 28.08 | -88.33% | 8.57x |
| 100 | 节点页加载 | 954.70 | 34.31 | -96.41% | 27.83x |
| 100 | 仪表盘状态 | 465.27 | 34.81 | -92.52% | 13.37x |
| 100 | 停止服务时切换节点 | 527.25 | 93.97 | -82.18% | 5.61x |
| 1000 | 启动前准备 | 376.95 | 44.55 | -88.18% | 8.46x |
| 1000 | 节点页加载 | 1365.45 | 33.06 | -97.58% | 41.30x |
| 1000 | 仪表盘状态 | 464.96 | 37.59 | -91.92% | 12.37x |
| 1000 | 停止服务时切换节点 | 523.08 | 97.89 | -81.29% | 5.34x |
| 5000 | 启动前准备 | 1590.80 | 117.91 | -92.59% | 13.49x |
| 5000 | 节点页加载 | 3349.87 | 32.23 | -99.04% | 103.95x |
| 5000 | 仪表盘状态 | 514.24 | 49.08 | -90.46% | 10.48x |
| 5000 | 停止服务时切换节点 | 572.88 | 108.76 | -81.01% | 5.27x |

这组数据反映的是代码路径和命令入口的体感变化：

- 7.2 的节点页会遍历旧目录并逐个读取节点 JSON；8.0 的 `catalog list` 读取分组摘要，不解析非活动 Provider。
- 7.2 的状态和切换路径需要加载多个 Shell 文件，并依赖 `sed`、`awk`、`grep` 等外部命令；8.0 由 Go 直接生成结构化结果。
- 7.2 启动准备只扫描当前活动节点目录；8.0 会一次生成全部 Catalog 分组的 Provider、出站和 eBPF 运行时文件，因此两者处理范围并不完全相同，不能把准备耗时解释为同等工作量下的纯算法加速。
- 节点切换测试运行在服务停止状态，只测持久化选择状态；运行中的 Service API 切换、核心 ready 时间另行计量。

测试夹具、命令、样本数和原始结果已在发布前审计记录中留档；本文保留可复核的场景、样本数和完整结果。

#### 8.0 Catalog Host 基准

8.0 当前 Catalog 基准使用 40 个分组、每组 250 个节点：

| 路径 | 单次耗时 | 分配 |
|---|---:|---:|
| 只读摘要 | 约 17.3 ms | 约 153 KB |
| 解析所有节点 | 约 182.7 ms | 约 82 MB |

7.2 没有 Catalog 和对应的摘要扫描路径，因此这里不伪造 7.2 的对照数字。8.0 的数据说明摘要路径应避免解析非活动 Provider；它仍然是 Windows Host 业务层数据，不是 Android 真实启动、CPU、内存或网络吞吐结论。

#### 尚不能在 Windows 上可靠对比的指标

- 服务启动到 sing-box `ready` 的耗时；
- Worker 空闲 RSS、CPU 和耗电；
- 透明代理连接延迟、吞吐和 DNS 接管开销；
- 7.2 TPROXY 与 8.0 eBPF 在 Android 内核上的差异。

这些指标需要同一台 Android 设备、同一节点和同一网络条件，留到最后的可选真机阶段，不用 Host 数字替代。

## 6. 控制接口与客户端

### CLI

7.2 的公共入口是旧 `scripts/cli`；8.0 使用：

```sh
su -c /data/adb/modules/netproxy/netproxyctl service status
```

统一约束：

- stdout 只输出一份 `schema=1` JSON；
- stderr 输出诊断和日志；
- `ok`、`code`、`message` 和 `data` 可被 Android/WebUI 使用；
- `--timeout` 控制命令等待；
- Native 二进制不作为客户端平行入口。

### API

| 接口 | 地址 | 主要消费者 |
|---|---|---|
| NetProxy CLI | 模块路径 | Android、WebUI、终端 |
| Service API | `127.0.0.1:9090` | sing-box Dashboard、运行时控制 |
| Clash API | `127.0.0.1:9999` | zashboard、第三方 Clash 客户端 |
| mixed 入站 | `127.0.0.1:7080` | 订阅下载的本机代理路径 |

Android 和 WebUI 不再读取 `module.conf`、Catalog、PID 或旧日志文本来猜测业务状态。

## 7. Android 管理器和 WebUI

### Android

当前源码进入主仓库后，管理器采用：

```text
Compose -> ViewModel -> Repository -> NetProxyCtlClient -> netproxyctl
```

主要能力：

- 仪表盘服务、流量、CPU/内存和实际节点状态；
- 节点分组、Auto/手动选择、测速、编辑、导出和删除；
- URL 订阅管理、流量和更新历史；单节点和本地文件导入会追加到本地配置；
- eBPF 代理设置、分应用策略和多用户/应用分身；
- sing-box JSON 编辑器、内置 schema、补全和校验；
- 日志、诊断包、Quick Settings Tile 和文件导入。

7.2 到 8.0 的 UI 间距、导航动画、加载态和 Snackbar 修复属于稳定性/体验改进，不在对比文档中逐条列出。

### WebUI

当前 WebUI 是原生 TypeScript 终端式入口，所有 Root 命令经 `src/webui/src/exec.ts` 调用 `netproxyctl`。模块 WebUI 还提供两个独立面板入口：

- sing-box Service API Dashboard；
- Clash API zashboard。

三者职责不同，不把 zashboard 当作 Android 管理器内部数据源，也不让 WebUI 直接拼接模块路径。

## 8. 配置和资源布局

### 7.2

配置中混合了模块设置、sing-box source、旧 TPROXY 数据、节点 outbounds 和运行时文件。

### 8.0

```text
config/                 用户可编辑配置
├── module.conf
├── ebpf/ebpf.conf
└── singbox/confdir/    静态 sing-box 配置片段

data/catalog/           节点和订阅事实源
runtime/                启动时生成的 providers/outbounds/ebpf
webroot/                模块 WebUI、Dashboard、zashboard
```

规则进一步区分：

- `rules/local/`：随配置使用的本地 source 规则；
- `rules/remote/`：由远程 Provider 管理的内置 SRS 规则资源；
- Catalog：用户节点和订阅 Provider；
- runtime：不可由用户编辑的运行时投影。

用户不应手动编辑 runtime 文件，也不应把旧 outbounds 或 TPROXY 规则复制到 8.0。

## 9. 安装、发行包和兼容性

8.0 安装流程支持：

- 保留现有数据；
- 全新安装；
- 安装超时默认保留现有数据；
- 含管理器包中检查 APK 是否存在、是否已安装以及版本；
- 已开机安装时后台应用模块更新，避免强制重启。

发行包分为：

| 包 | 内容 |
|---|---|
| 标准包 | 模块、sing-box、Native、CLI、WebUI、规则和面板，不含 APK |
| 含管理器包 | 标准包加可选安装的内置管理器 APK |

7.2 用户需要注意：8.0 使用新的 Catalog 和运行时布局，旧 `CURRENT_CONFIG`、旧 outbounds 和 TPROXY 路径不是当前事实源。升级前应备份并按 8.0 方式重新导入节点/订阅；不要把旧目录直接覆盖到新 Catalog。

## 10. 安全与可靠性

8.0 相比 7.2 的主要变化：

- 管理入站、Service API 和 Clash API 默认只监听 loopback；
- 订阅 URL、Header、HWID、节点凭据和日志统一按敏感信息处理；
- 配置使用候选文件、完整检查、原子替换和 reload；
- Catalog 更新使用跨进程锁和事务恢复；
- 结构化错误保留 machine code，客户端不再把空输出当成功；
- 运行状态、持久化状态和运行时同步状态分开表达。

这些机制的目的是减少静默失败和半提交，不等于已经完成所有 Android 内核兼容性验证。

## 11. 相关文档

- [安装与升级](/guide/installation)
- [节点与订阅](/guide/nodes-subscriptions)
- [透明代理与分应用代理](/guide/transparent-proxy)
- [CLI 使用](/guide/cli)
- [配置参考](/config/module)
- [V7 历史升级指南](/guide/upgrade-v7)
