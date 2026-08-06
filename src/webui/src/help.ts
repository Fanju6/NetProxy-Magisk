const BANNER = `NetProxy Terminal
`

const MAIN = `
命令:
  service <cmd>    服务控制 (start/stop/status/restart/reload)
  node <cmd>       节点管理 (list/use/current/delay/add/remove)
  mode <mode>      路由模式 (rule/global/direct)
  sub <cmd>        订阅管理 (list/add/update/update-all/activate/remove)
  app <cmd>        分应用代理 (list/mode/add/remove/enable/disable)
  logs <cmd>       日志 (show/clear)
  config <cmd>     配置 (list/read/check)
  ebpf <cmd>       eBPF 诊断
  ! <命令>         执行任意 Shell 命令 (Root 权限)
  clear            清屏

快捷键:
  Tab              自动补全
  ↑/↓              历史命令
  Enter            执行

帮助:
  help <主题>      查看详细帮助 (service/node/mode/sub/app/logs/config/ebpf/shell)
  help all         查看完整文档

示例:
  service start && node use auto && mode rule
  sub update-all
  node list
  ! ls /data/adb/modules/netproxy/config/
  ! ps -A | grep sing-box
`

const TOPICS: Record<string, string> = {
  service: `
service - 服务控制

  service status      查看运行状态 (运行时间、流量、当前节点)
  service start       启动代理服务
  service stop        停止代理服务
  service restart     重启服务 (修改配置后使用)
  service reload      热重载配置 (不重启进程)

状态: stopped / preparing / starting / ready / stopping / failed
`,
  node: `
node - 节点管理

  node list [分组]           列出节点 (可按分组过滤)
  node current               显示当前节点
  node use auto [分组]       自动选择最快节点 (推荐)
  node use <分组>/<tag>      手动选择节点 (例: node use default/tokyo)
  node delay [auto 分组]     测量节点延迟
  node add <链接>            通过分享链接添加节点 (ss:// vmess:// trojan:// 等)
  node remove <分组>/<tag>   删除节点
`,
  sub: `
sub - 订阅管理

  sub list                  列出订阅
  sub add <名称> <URL>      添加订阅
  sub update <名称>         更新单个订阅
  sub update-all            更新所有订阅
  sub activate <名称>       激活订阅分组
  sub remove <名称>         删除订阅
`,
  mode: `
mode - 路由模式

  mode rule       规则模式 (国内直连，国外代理) [默认]
  mode global     全局代理
  mode direct     全局直连
`,
  app: `
app - 分应用代理

  app list                查看配置
  app mode blacklist      黑名单: 列表内应用走代理
  app mode whitelist      白名单: 仅列表内应用不走代理
  app add <包名>          添加应用 (例: app add com.android.chrome)
  app remove <包名>       移除应用
  app enable              启用分应用代理
  app disable             禁用分应用代理
`,
  logs: `
logs - 日志

  logs show service       服务日志 (启动/停止/错误)
  logs show core          sing-box 内核日志 (连接详情)
  logs show sub           订阅更新日志
  logs clear <类型>       清空日志
`,
  config: `
config - 配置 (高级)

  config list             列出所有配置文件
  config read <目标>      读取配置内容
                          目标: module / ebpf / singbox/confdir/<文件>
  config check            校验配置
`,
  ebpf: `
ebpf - eBPF 诊断

  ebpf status             查看当前 eBPF 模式
  ebpf status all         检测所有支持模式
`,
  shell: `
shell - 执行任意 Shell 命令 (! 前缀)

  前缀加 ! 可直接执行任意 Android Shell 命令，拥有 Root 权限。
  支持管道、重定向、逻辑运算符等标准 Shell 语法。

  ! <命令>               执行命令

示例:
  ! ls /data/adb/modules/netproxy/config/
  ! ps -A | grep sing-box
  ! cat /data/adb/modules/netproxy/config/tproxy.conf
  ! netstat -tlnp
`
}

export function getHelp(topic?: string): string {
  if (!topic) return BANNER + MAIN
  const t = topic.toLowerCase()
  if (t === 'all') return BANNER + MAIN + '\n' + Object.values(TOPICS).join('\n')
  return TOPICS[t] || `未知主题: ${topic}\n可用: ${Object.keys(TOPICS).join(', ')}\n输入 help 查看用法。`
}
