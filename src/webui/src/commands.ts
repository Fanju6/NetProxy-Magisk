interface CommandSpec {
  overview: string
  actions: readonly string[]
  help: string
}

export const COMMANDS = {
  service: {
    overview: 'service <命令>  服务控制',
    actions: ['status', 'start', 'stop', 'restart', 'reload', 'check', 'toggle'],
    help: `service - 服务控制

  service status      查看服务状态、运行时间和流量
  service start       启动 sing-box 服务
  service stop        停止 sing-box 服务
  service restart     重启 sing-box 核心
  service reload      重载配置
  service check       检查服务配置
  service toggle      切换服务状态

状态: stopped / preparing / starting / ready / stopping / failed
`,
  },
  catalog: {
    overview: 'catalog <命令>  分组查看',
    actions: ['list', 'show'],
    help: `catalog - 节点分组

  catalog list             列出所有节点分组
  catalog show <分组>      查看分组详情
`,
  },
  node: {
    overview: 'node <命令>     节点管理',
    actions: ['list', 'snapshot', 'current', 'show', 'get', 'export', 'delay', 'add', 'import', 'edit', 'remove', 'use'],
    help: `node - 节点管理

  node list [分组]                 列出节点
  node snapshot [分组]             查看运行时节点快照
  node current                     查看当前选择
  node show <分组>                 查看分组节点
  node get <分组>/<tag>            获取节点配置摘要
  node export <分组>/<tag>         导出节点链接
  node delay [目标] [分组]         测量节点延迟
  node delay auto <分组>           测量分组内所有节点
  node add <链接> [分组]           添加单节点链接
  node import <文件>               导入节点文件
  node edit <分组>/<tag> <来源>    编辑节点
  node remove <分组>/<tag>         删除节点
  node use auto [分组]             自动选择最快节点
  node use <分组>/<tag>            手动选择节点
`,
  },
  sub: {
    overview: 'sub <命令>      订阅管理',
    actions: ['list', 'show', 'add', 'edit', 'update', 'update-all', 'activate', 'remove', 'history', 'cancel'],
    help: `sub - 订阅管理

  sub list                         列出订阅
  sub show <名称>                  查看订阅详情
  sub add [名称] <URL>             添加订阅
  sub edit <名称>                  编辑订阅
  sub update <名称>                更新单个订阅
  sub update-all                   更新所有订阅
  sub activate <名称>              激活订阅分组
  sub remove <名称>                删除订阅
  sub history <名称>               查看更新历史
  sub cancel <名称>                取消更新任务
`,
  },
  mode: {
    overview: 'mode <模式>     出站模式',
    actions: ['rule', 'global', 'direct', 'AllowAds'],
    help: `mode - 出站模式

  mode rule       规则分流
  mode global     全局代理
  mode direct     全局直连
  mode AllowAds   允许广告规则
`,
  },
  network: {
    overview: 'network <命令>  网络策略评估',
    actions: ['evaluate'],
    help: `network - 网络策略

  network evaluate --type <类型> [--ssid <名称>]
                         评估当前网络并应用 Wi-Fi 策略
  类型: wifi / not_wifi
`,
  },
  app: {
    overview: 'app <命令>      分应用代理',
    actions: ['list', 'mode', 'add', 'remove', 'enable', 'disable'],
    help: `app - 分应用代理

  app list                    查看应用代理配置
  app mode blacklist          使用黑名单模式
  app mode whitelist          使用白名单模式
  app add <用户ID>:<包名>      添加指定用户的应用
  app remove <用户ID>:<包名>   移除指定用户的应用
  app enable                  启用分应用代理
  app disable                 禁用分应用代理
`,
  },
  ebpf: {
    overview: 'ebpf <命令>     eBPF 能力诊断',
    actions: ['status'],
    help: `ebpf - eBPF 能力诊断

  ebpf status                         查看已配置能力
  ebpf status configured              查看当前配置
  ebpf status all                     检查全部支持能力
  ebpf status local                   检查本机数据路径
  ebpf status shared                  检查共享网络数据路径
  ebpf status all --raw               输出原始诊断信息
`,
  },
  config: {
    overview: 'config <命令>   配置管理',
    actions: ['list', 'read', 'check', 'validate', 'apply'],
    help: `config - 配置管理

  config list                         列出配置文件
  config read <目标>                  读取配置
  config check                        检查全部配置
  config validate <目标> <内容文件>   校验配置
  config apply <目标> <内容文件>      应用配置
`,
  },
  logs: {
    overview: 'logs <命令>     日志管理',
    actions: ['show', 'clear', 'export'],
    help: `logs - 日志管理

  logs show service             查看服务日志
  logs show core                查看 sing-box 日志
  logs clear <类型>             清空日志
  logs export                   导出运行时配置和脱敏日志
`,
  },
} as const satisfies Record<string, CommandSpec>

export type CommandName = keyof typeof COMMANDS
export const COMMAND_NAMES = Object.keys(COMMANDS) as CommandName[]
export const ROOT_COMPLETIONS = [...COMMAND_NAMES, 'help', 'clear', 'exit']
export const HELP_TOPICS = [...COMMAND_NAMES, 'shell']

export const VALUE_COMPLETIONS: Record<string, string[]> = {
  'app mode': ['blacklist', 'whitelist'],
  'ebpf status': ['configured', 'all', 'local', 'shared', '--raw'],
  'logs show': ['service', 'core'],
  'logs clear': ['service', 'core'],
  'network evaluate': ['--type', '--ssid'],
  'network evaluate --type': ['wifi', 'not_wifi'],
}
