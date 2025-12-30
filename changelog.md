

## 版本 4.0.0（2025-12-30）

---

### NetProxy 核心功能与架构（by @Fanju6）

* 入站端口逻辑调整

  * 不再从 `nbounds` 配置文件获取
  * 统一使用默认端口 **12345**
* NetProxy 专属 WebUI 管理器

  * 可直接打开模块 WebUI
  * 磁贴支持开启 / 关闭 / 重启，并显示运行状态
* 全新透明代理实现

  * 提升整体性能与稳定性
* 应用代理模式重构

  * 黑名单与白名单彻底拆分
  * 不再共用同一应用列表
* 代理能力增强

  * 支持移动数据、WiFi 热点、USB 网络共享
  * 支持 TCP / UDP / IPv6 代理开关
* 路由与规则能力增强

  * 新增路由规则设置功能
* 主题与外观配置

  * 新增主题与颜色设置
  * 支持 12 种主题色
* 配置与日志系统改进

  * 统一 `config/` 目录下配置文件格式为 `.conf`
  * 日志页新增 `tproxy.log`、`update.log` 显示
  * 日志页支持保存日志与配置文件
* Xray 运行方式调整

  * 使用 `root:net_admin` 用户启动
* 脚本体系重构

  ```
  scripts/
  ├── cli                      # 命令行工具（主入口）
  ├── core/                    # 核心服务脚本
  │   ├── start.sh
  │   ├── stop.sh
  │   └── switch-config.sh
  ├── network/                 # 网络 / 代理相关
  │   └── tproxy.sh
  │   
  ├── config/                  # 配置解析
  │   ├── url2json.sh
  │   └── subscription.sh
  └── utils/                   # 工具脚本
      ├── update-xray.sh
      └── clean_reject.sh
  ```
* 设备与启动相关改进

  * 重命名 `oneplus_a16` 修复脚本
  * 将修复脚本改为开关控制
  * 支持模块开机自启动
* 配置与节点 UI 优化

  * 节点信息左边距缩小
  * 延迟检测结果显示在节点信息内
  * 支持显示当前正在使用的节点
  * 优化配置页整体布局
* 协议解析修复

  * 修复 Trojan 节点解析问题

---

### WebUI（by @seyud）

* WebUI 样式体系重构

  * `mdui.css` 由本地文件改为通过 npm 引入
  * 移除本地 `assets/mdui.css`
  * 调整 `index.html` 中 CSS 加载顺序并添加说明
* 下拉菜单交互修复

  * 增加 `OpenDropdown` 状态管理
  * 确保同一时间仅打开一个下拉菜单
* UI 交互与可用性优化

  * 重构确认对话框结构，宽度限制为 400px
  * 扩大配置项“更多”按钮点击区域
* 应用列表显示优化

  * 包名支持自动换行，避免长包名溢出
  * 优化列表布局与图标间距
* 应用选择器修复与样式优化

  * 修复复选框点击触发两次状态切换的问题
  * 列表标题与描述支持换行
  * 复选框样式适配 Monet 主题颜色

---

### CLI 命令行工具（by @hexl）

* 新增 NetProxy CLI 管理脚本 `scripts/cli`
* 支持通过 `adb shell` 管理 NetProxy：

  * 服务控制：`status / start / stop / restart`
  * 配置管理：`list / switch / current / add / remove / show`
  * 订阅管理：`list / add / update / update-all / remove`
  * 代理设置：`mode / apps / add / remove / reload`

