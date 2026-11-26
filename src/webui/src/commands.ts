interface CommandSpec {
  overview: string
  actions: readonly string[]
  help: string
}

export const COMMANDS = {
  api: {
    overview: 'api <命令>      模块 HTTP 管理接口',
    actions: ['init', 'start', 'status', 'access', 'traffic'],
    help: `api - 模块 HTTP 管理接口

  api init      初始化独立随机凭据，不覆盖已有配置
  api start     启动或恢复后台管理接口
  api status    查看实际接口就绪状态
  api access    读取地址与凭据，仅供 Root 客户端连接

  api traffic toggle | start | stop        切换或启停全局透明抓包
  api traffic status | clear                查看状态或清空所有抓包批次
  api traffic batches | list [批次ID]        查看批次或批次记录（默认当前或最近批次）
  api traffic page [批次ID [游标]]           分页读取已保存的记录
  api traffic delete-batch <批次ID>          删除已停止的批次
  api traffic detail <ID>                   查看出口、重写记录、请求和响应详情
  api traffic apps | set-apps <文件>        查看或保存 mode/all、blacklist、whitelist、disable_quic
  api traffic rules                         查看按 domains 分组的 JavaScript 重写规则
  api traffic set-rules <文件>               应用设备上的 JSON 规则文件
  api traffic mitm | set-mitm <文件>        查看或保存 HTTPS 解密配置
  api traffic pinned | clear-pinned        查看或清除证书固定跳过列表
  api traffic ca-init | ca                  初始化或查看公开调试 CA
  api traffic ca-remove                     移除模块安装的个人调试 CA（先关闭解密）
  api traffic ca-export <文件>              导出公开 CA，不覆盖已有文件

记录 TUN/eBPF 受管透明入站的 TCP 流量；不需要设置代理端口或凭据。
HTTPS 解密默认关闭，开启后仅解密所选主机。优先仅在测试客户端信任此 CA。

接口默认仅监听 127.0.0.1:9091，关闭 sing-box 不关闭管理接口。
凭据不可分享或粘贴到公共聊天中。
`,
  },
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
configured_backend 是保存的入站；active_backend 仅在 ready 且 PID/API 毫秒启动身份一致时非空，否则为 null。
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
    actions: ['list', 'show', 'get', 'export', 'add', 'import', 'edit', 'remove'],
    help: `node - 节点管理

  node list [分组]                 列出节点
  node show <分组>                 查看分组节点
  node get <分组>/<tag>            获取节点配置摘要
  node export <分组>/<tag>         导出节点链接
  node add <链接> [分组]           添加单节点链接
  node import <文件>               导入节点文件
  node edit <分组>/<tag> <来源>    编辑节点
  node remove <分组>/<tag>         删除节点
`,
  },
  policy: {
    overview: 'policy <命令>   原生策略组',
    actions: ['groups', 'select', 'delay'],
    help: `policy - sing-box 原生策略组

  policy groups                  查看策略组、成员与当前选择
  policy select <组标签> <成员标签>  选择 Selector 成员并保存默认节点
  policy delay <组标签>            请求测速；随后用 policy groups 查看结果

使用 policy groups 返回的原始标签，含空格时请加引号。停止时仅能查看配置。
`,
  },
  sub: {
    overview: 'sub <命令>      订阅管理',
    actions: ['list', 'show', 'add', 'edit', 'update', 'update-all', 'remove', 'history', 'cancel'],
    help: `sub - 订阅管理

  sub list                         列出订阅
  sub show <名称>                  查看订阅详情
  sub add [名称] <URL>             添加订阅
  sub edit <名称>                  编辑订阅
  sub update <名称>                更新单个订阅
  sub update-all                   更新所有订阅
  sub remove <名称>                删除订阅
  sub history <名称>               查看更新历史
  sub cancel <名称>                取消更新任务
`,
  },
  mode: {
    overview: 'mode <模式>     出站模式',
    actions: [],
    help: `mode - 出站模式

  mode            查看默认模式、实际模式与可选模式
  mode <模式名称> 保存主配置默认模式，运行时应用网络策略

可选模式来自主配置的路由与 DNS 规则，名称与内核一致。
默认配置：Rule（规则）、Proxy（代理）、Direct（直连）、RuleAllowAds（规则，允许广告）。
示例：mode Proxy、mode RuleAllowAds。
`,
  },
  network: {
    overview: 'network <命令>  网络策略评估',
    actions: ['evaluate', 'wifi-list'],
    help: `network - 网络策略

  network evaluate --type <类型> [--ssid <名称>]
                         评估当前网络并应用 Wi-Fi 策略
  network wifi-list       查看已保存的 Wi-Fi 名称
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

共用名单保存在 inbound.json 的 app 对象，运行时修改自动应用；只筛选本机应用。
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

只诊断 eBPF 能力，不代表实际 active_backend；TUN 通过配置检查、实际启动与日志确认。
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

  完整配置：module（module.json）、singbox/config.json、inbound
  模块分区：module/wifi、module/auto_start
  模块分区保留顶层字段，例如 {"auto_start":true}、{"wifi":{...}}；不能用 {} 删除。
  模块分区 revision 独立，保存时保留其他分区；auto_start 只影响下次开机。
  入站分区：inbound/backend、inbound/root_policy、inbound/ipv6、inbound/sniff_destination、inbound/app、inbound/ebpf、inbound/tun
  入站分区保留顶层字段，例如 {"backend":"tun"}；不能用 {} 删除。
  核心分区：singbox/dns、singbox/inbounds、singbox/route 等
  核心分区保留顶层字段，例如 {"dns":{...}}；{} 删除该分区。
  只读运行时：runtime/config.json
  apply/validate 在目标前加 --revision <读取值>，避免覆盖并发修改。
  切换强杀或清理未确认时中止并保留 journal，需要设备重启后再恢复。
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
const CONFIG_TARGETS = [
  'module', 'module/wifi', 'module/auto_start',
  'inbound', 'inbound/backend', 'inbound/root_policy', 'inbound/ipv6', 'inbound/sniff_destination', 'inbound/app', 'inbound/ebpf', 'inbound/tun', 'singbox/config.json',
  'singbox/log', 'singbox/dns', 'singbox/inbounds', 'singbox/outbounds', 'singbox/route',
  'singbox/experimental', 'singbox/http_clients', 'singbox/services',
]

export const VALUE_COMPLETIONS: Record<string, string[]> = {
  'api traffic': ['status', 'toggle', 'start', 'stop', 'batches', 'list', 'page', 'delete-batch', 'detail', 'clear', 'rules', 'set-rules', 'mitm', 'set-mitm', 'apps', 'set-apps', 'pinned', 'clear-pinned', 'ca', 'ca-init', 'ca-export', 'ca-remove'],
  'app mode': ['blacklist', 'whitelist'],
  'ebpf status': ['configured', 'all', 'local', 'shared', '--raw'],
  'logs show': ['service', 'core'],
  'logs clear': ['service', 'core'],
  'network evaluate': ['--type', '--ssid'],
  'network evaluate --type': ['wifi', 'not_wifi'],
  'config read': [...CONFIG_TARGETS, 'runtime/config.json'],
  'config apply': CONFIG_TARGETS,
  'config validate': CONFIG_TARGETS,
}
