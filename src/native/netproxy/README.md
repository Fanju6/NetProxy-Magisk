# netproxyctl

`netproxyctl` 是 NetProxy 的唯一 Go 可执行文件，同时承载公共 schema=1 CLI 和模块内部生命周期入口。

跨组件职责、Catalog/Provider 状态和公开接口边界见仓库根目录的 [AGENTS.md](../../../AGENTS.md)。

初始代码从 `Fanju6/Proxylink@4812c95` 的 NetProxy 专属分支迁入，并包含迁移时尚未提交的 Service API 快照能力；后续只在本仓库维护。公共 Proxylink 继续保留通用转换工具定位。

它负责以下需要类型化配置、HTTP 或 Protobuf 的能力：

- 将节点链接、文件和订阅转换为 sing-box Provider；
- 校验、检查和原子修改 Provider；
- 下载订阅并解析 HTTP 元数据；
- 调用 reF1nd sing-box Service API。

模块的公开管理入口仍是 `netproxyctl`。Shell 脚本只通过结构化 JSON 调用本程序，不应解析其内部文件或依赖未声明的输出文本。

## 验证

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -pgo=auto ./cmd/netproxyctl
```

正式构建会自动读取 `cmd/netproxyctl/default.pgo`。该 profile 由真实 Android
设备上的只读工作负载生成，覆盖服务状态、节点快照和 Catalog 运行时投影；更新
profile 时使用 `internal/pgoworkload`，不得把订阅、节点或设备数据写入仓库。

```sh
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test -c -trimpath \
  -o netproxy-pgo-bench ./internal/pgoworkload
adb push netproxy-pgo-bench /data/local/tmp/
adb shell su -c 'NETPROXY_PGO_MODULE_DIR=/data/adb/modules/netproxy \
  /data/local/tmp/netproxy-pgo-bench -test.run=^$ \
  -test.bench=^BenchmarkAndroidPGO$ -test.benchtime=10s \
  -test.cpuprofile=/data/local/tmp/default.pgo'
adb pull /data/local/tmp/default.pgo cmd/netproxyctl/default.pgo
```

只有真实热点发生明显变化时才重新采样；更新后必须重新对比 `-pgo=off` 和
`-pgo=auto` 的 Android arm64 产物。

依赖的 reF1nd sing-box 版本必须与模块内核同步更新。
