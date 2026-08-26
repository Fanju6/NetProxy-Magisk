import { COMMAND_NAMES, COMMANDS, HELP_TOPICS, type CommandName } from './commands'

const BANNER = `NetProxy Terminal
`

const MAIN = `
命令:
${COMMAND_NAMES.map(name => `  ${COMMANDS[name].overview}`).join('\n')}

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

const SHELL_HELP = `shell - WebUI 本地扩展

  ! <命令>                      执行 Root Shell 命令

此功能不属于 netproxyctl 公共命令，只在 KernelSU WebUI 中可用。
`

function topicHelp(topic: string): string | undefined {
  if (topic === 'shell') return SHELL_HELP
  return COMMANDS[topic as CommandName]?.help
}

export function getHelp(topic?: string): string {
  if (!topic) return BANNER + MAIN
  const normalized = topic.toLowerCase()
  if (normalized === 'all') return BANNER + MAIN + '\n' + [...COMMAND_NAMES.map(name => COMMANDS[name].help), SHELL_HELP].join('\n')
  return topicHelp(normalized) || `未知主题: ${topic}\n可用主题: ${HELP_TOPICS.join(', ')}\n输入 help 查看用法。`
}
