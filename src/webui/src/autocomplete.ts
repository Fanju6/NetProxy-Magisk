export interface CompletionResult { completed: string; candidates: string[] }

const GROUPS = ['service', 'catalog', 'node', 'sub', 'mode', 'network', 'app', 'logs', 'config', 'ebpf', 'help', 'clear', 'exit']
const SUBS: Record<string, string[]> = {
  service: ['status', 'start', 'stop', 'restart', 'reload', 'check'],
  catalog: ['list', 'show'],
  node: ['list', 'snapshot', 'current', 'use', 'add', 'import', 'export', 'edit', 'remove', 'rm', 'delay'],
  sub: ['list', 'show', 'add', 'edit', 'update', 'update-all', 'activate', 'remove', 'rm', 'history', 'cancel'],
  mode: ['rule', 'global', 'direct', 'AllowAds'],
  network: ['evaluate'],
  app: ['list', 'mode', 'users', 'add', 'remove', 'rm', 'enable', 'disable'],
  logs: ['show', 'clear', 'export'],
  config: ['list', 'read', 'check', 'validate', 'apply'],
  ebpf: ['status'],
}
const VALS: Record<string, string[]> = {
  'app mode': ['blacklist', 'whitelist'],
  'ebpf status': ['configured', 'all', 'local', 'shared', '--raw'],
  'logs show': ['service', 'core'],
  'logs clear': ['service', 'core'],
  'network evaluate': ['--type', '--ssid'],
  'network evaluate --type': ['wifi', 'not_wifi'],
}
const NODE_OPS = ['list', 'export', 'edit', 'remove', 'rm', 'show']
const SUB_OPS = ['activate', 'update', 'show', 'edit', 'remove', 'rm', 'history', 'cancel']

function lcp(items: string[]): string {
  let p = items[0] || ''
  for (let i = 1; i < items.length && p; i++) while (!items[i].startsWith(p) && p) p = p.slice(0, -1)
  return p
}

export function complete(input: string, knownGroups: string[] = [], knownSubs: string[] = []): CompletionResult {
  if (input.startsWith('!')) return { completed: input, candidates: [] }
  const toks = input.split(/\s+/)
  const trailing = input.endsWith(' ')
  const n = trailing ? toks.length : toks.length - 1
  const cur = trailing ? '' : toks[toks.length - 1]
  const all = [...knownGroups, ...knownSubs]
  let cands: string[] = []

  if (n === 0 || (n === 1 && !trailing)) {
    cands = GROUPS.filter(c => c.startsWith(cur))
  } else if (n === 1 || (n === 2 && !trailing)) {
    cands = (SUBS[toks[0]] || []).filter(c => c.startsWith(cur))
  } else if (n >= 2) {
    const [cmd, sub] = toks
    if (cmd === 'help' && n === 2) {
      cands = GROUPS.filter(g => !['help', 'clear', 'exit'].includes(g) && g.startsWith(cur))
    } else if (cmd === 'node' && (sub === 'use' || sub === 'delay')) {
      if (n === 2) cands = cur !== 'auto' ? ['auto', ...all].filter(c => c.startsWith(cur)) : all.filter(c => c.startsWith(cur))
      else if (n === 3 && toks[2] === 'auto') cands = knownGroups.filter(c => c.startsWith(cur))
    } else if (cmd === 'sub' && SUB_OPS.includes(sub) && n === 2) {
      cands = knownSubs.filter(c => c.startsWith(cur))
    } else if (cmd === 'catalog' && sub === 'show' && n === 2) {
      cands = knownSubs.filter(c => c.startsWith(cur))
    } else if (cmd === 'node' && NODE_OPS.includes(sub) && n === 2) {
      cands = all.filter(c => c.startsWith(cur))
    } else {
      const valueKey = cur.startsWith('--') ? toks.slice(0, n - 1).join(' ') : toks.slice(0, n).join(' ')
      cands = (VALS[valueKey] || []).filter(c => c.startsWith(cur))
    }
  }

  if (!cands.length) return { completed: input, candidates: [] }
  const prefix = lcp(cands)
  const base = input.slice(0, input.length - cur.length)
  return { completed: cands.length === 1 ? cands[0] + ' ' : base + prefix, candidates: cands }
}
