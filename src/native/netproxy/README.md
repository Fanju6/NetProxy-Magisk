# netproxy-native

`netproxy-native` 是 NetProxy 模块内部使用的原生组件，不作为独立通用工具发布。

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
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build ./cmd/netproxy-native
```

依赖的 reF1nd sing-box 版本必须与模块内核同步更新。
