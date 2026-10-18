# 原生策略组隔离验证

`native-policy-core.mjs` 使用真实 `v1.15.0-alpha.11-reF1nd` 核心和合成数据。
每次在 Git 忽略的 `.agents/runs/` 下创建独立目录，每个场景使用独立子目录和随机
loopback Service API。脚本仅启动和终止自己的核心子进程，不访问设备、模块配置或现有服务。
缓存、配置与 Provider 均为匿名合成数据；测速仅访问本地 HTTP 204 目标。
所有命令结果、核心日志、快照、错误和 `evidence.json` 保留在本次目录中。
一个断言失败后仍执行其余场景，最终非零退出；每个场景结束前等待自己的核心退出。

使用已从校验和验证过的模块缓存构建的核心，直接在仓库根目录执行：

```powershell
node tests/native-policy-core.mjs `
  .agents/runs/2026-10-18-native-policy/sing-box-pinned.exe `
  .agents/runs/2026-10-18-native-policy
if ($LASTEXITCODE -ne 0) { throw '核心隔离验证失败' }
```

## 构建与执行

需要 Go 1.27、Node.js 22+。在仓库根目录使用 PowerShell：

```powershell
$root = (Get-Location).Path
$run = Join-Path $root '.agents/runs/native-policy-locked'
New-Item -ItemType Directory -Force $run | Out-Null
Push-Location src/native/netproxy
$locked = go mod download -json github.com/sagernet/sing-box | ConvertFrom-Json
Pop-Location
if ($locked.Path -ne 'github.com/reF1nd/sing-box' -or
    $locked.Version -ne 'v1.15.0-alpha.11-reF1nd' -or
    $locked.Sum -ne 'h1:4dVOGlE/zE66kKoCdls2UR7DoVGuqBbML8Gs8SXnyOg=') {
    throw '核心来源与锁定版本不一致'
}
$env:CGO_ENABLED = '0'
Push-Location $locked.Dir
go build -mod=readonly -buildvcs=false -tags with_clash_api `
  -ldflags '-X github.com/sagernet/sing-box/constant.Version=1.15.0-alpha.11-reF1nd' `
  -o (Join-Path $run 'sing-box.exe') ./cmd/sing-box
Pop-Location
if ($LASTEXITCODE -ne 0) { throw '核心构建失败' }
node tests/native-policy-core.mjs (Join-Path $run 'sing-box.exe') $run
if ($LASTEXITCODE -ne 0) { throw '核心隔离验证失败' }
```

版本字符串本身不证明来源，因此构建先核对锁定模块的路径、版本与校验和。
脚本再核对运行版本并记录二进制 SHA-256，不使用未锁定的本地源码或历史构建替代。

## 独立场景

| 场景 | 断言 |
|---|---|
| `direct-only` | 没有 Provider、selector 或 urltest 也能启动，`SubscribeGroups` 返回空列表，不将空列表当作未就绪 |
| `use-all-providers` | selector 的 `use_all_providers=true` 展开两个 Provider，不要求显式 `providers` |
| `disjoint-selectors` | 两个 selector 分别引用不相交的 Provider；两边选择独立，跨组成员和裸节点名均拒绝 |
| `urltest-multiple-providers` | 一个 urltest 显式引用多个 Provider，完整展开并实际测速每个成员，不接受手选 |
| `urltest-use-all-providers` | urltest 的 `use_all_providers=true` 同样展开并实际测速两个 Provider |
| `selector-*` / `urltest-*` | 分别验证空 Provider、缺失 Provider 引用、缺失静态成员引用和没有成员/Provider 的启动错误 |
| `multi-c-sorted-merge` | 倒序与正序 `-c` 参数得到相同结果，检查标量、数组和嵌套对象，并实际启动合并图 |
| `multi-c-duplicate-outbound-tag` | 重复出站标签在 `merge`、`check` 和 `run` 的合并解码阶段均失败 |
| `multi-c-duplicate-provider-tag` | 合并数组保留两条同名 Provider，运行时只使用排序后最后一条定义，旧成员不可选择 |
| `cache-and-provider-hot-update` | 任意组名、三文件合并、选择缓存、Provider 文件变化观测与组标签改名 |

总计 17 个独立场景。组快照顺序由核心决定，不假定与文件顺序相同。
跨 Provider 的相同节点名使用精确身份 `np/alpha/same` 与 `np/beta/same`，
不能按第一个 `/` 拆分 Provider 标签，也不能使用裸 `same`。

测速使用同一隔离核心的随机 loopback SOCKS 入站，将测试流量显式路由到本地 direct，
再访问宿主 HTTP 204 目标。Provider 中的节点是无凭据 SOCKS 节点，不伪造被解析器过滤的
`direct` Provider 节点。初始测速完成后再次触发 URLTest，等待所有成员产生新的历史时间，
并验证 HTTP 请求计数至少增加成员数；RPC 成功本身不代表异步测速完成。

缓存场景使用独立 `cache_file`，重启后恢复精确组/成员标签，不写模块选择状态。
连续 Provider 文件变化会记录内核是否向 Service API 发布成员更新；若已发布，验证新增成员、
有效选择保留、删除已选成员后的回退，并核对 PID 和毫秒级启动身份，证明没有依靠重启或 reload。
若锁定内核未在当前文件系统发布该事件，探针会将结果写入 evidence，而不是伪造成功；模块随后
通过提交后的 Service API 快照核对，并在未收敛时执行一次受事务保护的 reload。
改名组标签后不继承旧标签缓存。

## 实测语义

- 多 `-c` 按传入路径字符串排序，不按参数顺序。已有标量由排序最前的文件保留，
  包括显式 `false`；数组按排序后的顺序追加；对象递归合并，后续对象只补齐缺失标量键。
  这不是“最后文件覆盖”，也不按数组条目的 `tag` 合并。
- 重复 outbound 标签是 `duplicate outbound/endpoint tag`，合并解码即失败。
  重复 Provider 标签却允许进入合并结果，Provider manager 最后定义生效；两者不可类推。
- 空 Provider 返回 `no servers found`；缺失 Provider、缺失静态成员、没有任何成员均明确失败，
  不回退 direct。但这些场景的 `check` 均返回 0，只有 `run` 的启动阶段发现错误。
  因此 `check` 成功不能替代启动验证。
- 原生 RPC 的业务错误可放在响应头的 `grpc-status`，不一定有 trailer 数据帧；
  脚本区分真实非零 gRPC 状态与连接失败/超时，不能让任何拒绝都算业务校验通过。

实测证据只保留在 `.agents/runs/2026-10-18-native-policy/probe-*/`。
根级 `evidence.json` 含版本、二进制 SHA-256、场景结果与失败堆栈；每个场景另存
`evidence.json`、`command-*-check.json` / `command-*-merge.json` 和 `core-<pid>.log`。
这些是宿主隔离验证，不代表 Android 权限或透明入站已验收。

## Android 适配评估

可使用仓库打包的 Android 二进制做隔离验收：

```powershell
node tests/native-policy-core.mjs src/module/bin/sing-box `
  .agents/runs/native-policy-android --adb <serial>
```

它只复制二进制到唯一的 `/data/adb/netproxy-native-policy-<nonce>/`，全部配置、Provider、缓存、日志和 PID 文件都留在该目录。它只创建本次 API 的 `adb forward` 与 URLTest 的 `adb reverse`，停止时只终止 PID 文件记录的核心并清理自己的目录和映射；不读写模块目录、不查询或停止 live sing-box，也不使用 `pidof`、`pkill` 或共享临时目录清理。

设备探针记录 Android ABI 的二进制 SHA-256、版本、Service API、Provider 解析、原生选择、URLTest、缓存和多配置合并结果。当前文件系统未将 Local Provider 变化发布到 Service API 时，evidence 会明确写入 `provider_watcher=not_observed_on_android`；宿主环境同样可能记录 `not_observed_on_host`，两者都不是静默成功。NetProxy 会用 Service API 回读成员；未收敛时执行一次受事务保护的 reload，避免已提交 Catalog 长期处于陈旧运行状态。

## Go 与 HTTP 契约

```powershell
Push-Location src/native/netproxy
go test ./internal/service ./internal/module ./internal/httpapi -run '^TestPolicy' -count=1
go test ./internal/httpapi -run '^TestResources' -count=1
go vet ./internal/service ./internal/module ./internal/httpapi
Pop-Location
git diff --check
```

固定接口为 `GET /api/v1/policy-groups`、`POST /api/v1/policy-groups/select`
与 `POST /api/v1/policy-groups/delay`。选择正文为 `{group_tag,member_tag}`，测速正文为
`{group_tag}`，均不接收 URL、路径或任意 HTTP 参数。

成功响应的 `data` 统一为 `{groups,live,core_started_at_millis,default_outbound}`。
运行态 `groups` 完全来自核心，前后核对 PID 与毫秒级启动身份。停止态只读取主配置的
selector/urltest、显式静态成员和 `route.final`，不猜测 Provider 展开结果、已选成员或延迟。
停止态 select/delay 返回 `policy.core_not_ready`，不启动离线核心或 Worker。

HTTP 错误状态为：非法正文 400，未就绪 409，组或成员不存在 404，不可手选或配置摘要
无效 422，核心通信或回读未确认 502。核心已接受操作但回读失败时附带 `accepted=true`，
不自动重发，不以模块持久化或 reload 兜底。`GET /selection`、`PUT /selection` 和
`POST /nodes/delay` 已移除；Catalog 与策略组互不混合。
