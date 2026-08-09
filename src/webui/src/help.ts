const BANNER = `NetProxy Terminal
`

const MAIN = `
命令:
  service <命令>  服务控制
  catalog <命令>  分组查看
  node <命令>     节点管理
  sub <命令>      订阅管理
  mode <模式>     出站模式
  network <命令>  网络策略评估
  app <命令>      分应用代理
  ebpf <命令>     eBPF 能力诊断
  config <命令>   配置管理
  logs <命令>     日志管理

全局选项:
  --json                 输出 schema=1 JSON
  --timeout <秒|时长>    覆盖默认命令超时

本地命令:
  help [主题]       查看帮助
  clear             清空终端
  exit              显示退出当前终端的提示
  ! <命令>          执行 Root Shell 命令

示例:
  service start
  node use auto default
  mode rule
  sub update-all
  node delay auto default

输入 help all 查看完整命令说明。
`

const TOPICS: Record<string, string> = {
  service: `
service - 服务控制

  service status      查看服务状态、运行时间和流量
  service start       启动 sing-box 服务
  service stop        停止 sing-box 服务
  service restart     重启服务
  service reload      重载配置
  service check       检查服务配置

状态: stopped / preparing / starting / ready / stopping / failed
`,
  catalog: `
catalog - 节点分组

  catalog list             列出所有节点分组
  catalog show <分组>      查看分组详情
`,
  node: `
node - 节点管理

  node list [分组]                 列出节点
  node snapshot [分组]             查看运行时节点快照
  node current                     查看当前选择
  node show <分组>                 查看分组节点
  node get <分组>/<tag>            获取节点配置摘要
  node export <分组>/<tag>         导出节点链接
  node delay [目标] [分组]         测量节点延迟
  node delay auto <分组>           测量分组内所有节点
  node add <链接> [分组]           添加单节点链接
  node import <文件> [名称]        导入节点文件
  node edit <分组>/<tag> <来源>    编辑节点
  node remove <分组>/<tag>         删除节点
  node use auto [分组]             自动选择最快节点
  node use <分组>/<tag>            手动选择节点
`,
  sub: `
sub - 订阅管理

  sub list                         列出订阅
  sub show <名称>                  查看订阅详情
  sub add <名称> <URL>             添加订阅
  sub edit <名称>                  编辑订阅
  sub update <名称>                更新单个订阅
  sub update-all                   更新所有订阅
  sub activate <名称>              激活订阅分组
  sub remove <名称>                删除订阅
  sub history <名称>               查看更新历史
  sub cancel <名称>                取消更新任务
`,
  mode: `
mode - 出站模式

  mode rule       规则分流
  mode global     全局代理
  mode direct     全局直连
  mode AllowAds   允许广告规则
`,
  network: `
network - 网络策略

  network evaluate --type <类型> [--ssid <名称>]
                         评估当前网络并应用 Wi-Fi 策略
  类型: wifi / not_wifi
`,
  app: `
app - 分应用代理

  app list                    查看应用代理配置
  app mode blacklist          使用黑名单模式
  app mode whitelist           使用白名单模式
  app users                   查看 Android 用户范围
  app add <包名>              添加应用
  app remove <包名>           移除应用
  app enable                  启用分应用代理
  app disable                 禁用分应用代理
`,
  ebpf: `
ebpf - eBPF 能力诊断

  ebpf status                         查看已配置能力
  ebpf status configured              查看当前配置
  ebpf status all                     检查全部支持能力
  ebpf status local                   检查本机数据路径
  ebpf status shared                  检查共享网络数据路径
  ebpf status all --raw               输出原始诊断信息
`,
  config: `
config - 配置管理

  config list                  列出配置文件
  config read <目标>           读取配置
  config check                 检查配置
  config validate              校验配置
  config apply                 应用配置
`,
  logs: `
logs - 日志管理

  logs show service             查看服务日志
  logs show core                查看 sing-box 日志
  logs clear <类型>            清空日志
  logs export                  导出运行时配置和脱敏日志
`,
  shell: `
shell - WebUI 本地扩展

  ! <命令>                      执行 Root Shell 命令

此功能不属于 netproxyctl 公共命令，只在 KernelSU WebUI 中可用。
`
}

export function getHelp(topic?: string): string {
  if (!topic) return BANNER + MAIN
  const normalized = topic.toLowerCase()
  if (normalized === 'all') return BANNER + MAIN + '\n' + Object.values(TOPICS).join('\n')
  return TOPICS[normalized] || `未知主题: ${topic}\n可用主题: ${Object.keys(TOPICS).join(', ')}\n输入 help 查看用法。`
}
