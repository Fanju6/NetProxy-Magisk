# 安装与升级

## 安装前准备

- Android 12 或更高版本的 `arm64-v8a` 设备
- Magisk、KernelSU 或 APatch，以及可用的 Root 权限
- 支持 BPF、TC classifier、透明 socket 与 socket lookup 的内核
- local 模式还需要 veth 和策略路由能力

NetProxy 不提供 TPROXY 或 REDIRECT 回退。内核能力不足时，透明代理入站无法启动。

## 选择安装包

Release 提供两个代理能力相同的模块包：

| 安装包 | 内容 |
|---|---|
| `NetProxy_<版本>_<构建号>.zip` | 标准模块，不含 Android 管理器 APK |
| `NetProxy_<版本>_<构建号>_with-manager.zip` | 标准模块，并附带可选安装的管理器 APK |

推荐从 [Google Play](https://play.google.com/store/apps/details?id=com.fanjv.netproxy) 安装和更新管理器。无法使用 Google Play 时再选择含管理器包。

## 刷入模块

1. 从 [GitHub Releases](https://github.com/Fanju6/NetProxy-Magisk/releases) 下载模块包。
2. 在 Root 管理器中选择并安装。
3. 升级时选择安装方式：
   - **保留现有数据**：要求已有 `config/singbox/config.json`，保留节点、订阅、模块设置、主配置及 `direct.json`、`proxy.json`、`block.json` 本地规则。本次升级会重置 `ebpf.conf`，请记录原设置后重新配置。
   - **全新安装**：忽略现有数据，使用安装包默认内容。
4. 未在倒计时内选择时，默认保留现有数据。
5. 含管理器包会继续询问是否安装 APK；标准包只显示获取管理器的提示。

::: warning 从配置片段布局升级
安装器不迁移旧配置。请先导出节点、记录订阅地址与个人设置，安装时选择“全新安装”；这会重置现有节点、订阅和配置。缺少 `config/singbox/config.json` 时，选择或超时默认“保留现有数据”都会中止安装，不会自动改为全新安装。
:::

在已开机环境中安装成功后，模块会后台应用更新，不要求立即重启。安装结束提示的短暂等待期间不要重启，以免 Root 管理器仍按待更新目录完成切换。Recovery 安装或 Root 管理器明确要求重启时，以其提示为准。

## 第一次启动

模块默认不会自动启动服务。安装后请：

1. 安装并打开 Android 管理器。
2. 导入本地节点或添加订阅。
3. 在节点页选择分组与自动/手动节点。
4. 返回仪表盘启动服务。
5. 确认状态为“运行中”后，再按需要开启自动启动。

下一步见[快速开始](/guide/quick-start)。

## 升级注意事项

- 不要从旧安装包手工复制二进制、运行时文件或临时状态目录。
- 运行时配置会根据持久设置重新生成，不需要备份。
- 内置远程规则随模块更新；用户本地规则和 Catalog 数据属于持久数据。
- 大版本历史背景见[架构迁移记录](/guide/compare-v7-v8)与[历史升级指南](/guide/upgrade-v7)，不要把历史路径当作当前操作说明。
