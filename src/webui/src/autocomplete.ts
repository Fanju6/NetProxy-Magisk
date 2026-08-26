export interface CompletionResult { completed: string; candidates: string[] }

import { parseCommandTokens, quoteCommandToken } from './command'
import { COMMANDS, HELP_TOPICS, ROOT_COMPLETIONS, VALUE_COMPLETIONS, type CommandName } from './commands'

const NODE_OPS = ['list', 'show', 'get', 'export', 'edit', 'remove']
const SUB_OPS = ['activate', 'update', 'show', 'edit', 'remove', 'history', 'cancel']

function lcp(items: string[]): string {
  let p = items[0] || ''
  for (let i = 1; i < items.length && p; i++) while (!items[i].startsWith(p) && p) p = p.slice(0, -1)
  return p
}

export function complete(input: string, knownGroups: string[] = [], knownSubs: string[] = []): CompletionResult {
  if (input.startsWith('!')) return { completed: input, candidates: [] }
  const parsed = parseCommandTokens(input)
  const toks = parsed.map(token => token.value)
  const trailing = input.endsWith(' ')
  const n = trailing ? toks.length : Math.max(0, toks.length - 1)
  const currentToken = trailing ? undefined : parsed[parsed.length - 1]
  const cur = currentToken?.value || ''
  let cands: string[] = []

  if (n === 0) {
    cands = ROOT_COMPLETIONS.filter(c => c.startsWith(cur))
  } else if (n === 1) {
    cands = (toks[0] === 'help' ? HELP_TOPICS : COMMANDS[toks[0] as CommandName]?.actions || []).filter(c => c.startsWith(cur))
  } else if (n >= 2) {
    const [cmd, sub] = toks
    if (cmd === 'node' && (sub === 'use' || sub === 'delay')) {
      if (n === 2) cands = cur !== 'auto' ? ['auto', ...knownGroups].filter(c => c.startsWith(cur)) : knownGroups.filter(c => c.startsWith(cur))
      else if (n === 3 && toks[2] === 'auto') cands = knownGroups.filter(c => c.startsWith(cur))
    } else if (cmd === 'sub' && SUB_OPS.includes(sub) && n === 2) {
      cands = knownSubs.filter(c => c.startsWith(cur))
    } else if (cmd === 'catalog' && sub === 'show' && n === 2) {
      cands = knownGroups.filter(c => c.startsWith(cur))
    } else if (cmd === 'node' && NODE_OPS.includes(sub) && n === 2) {
      cands = knownGroups.filter(c => c.startsWith(cur))
    } else {
      const valueKey = toks.slice(0, n).join(' ')
      cands = (VALUE_COMPLETIONS[valueKey] || []).filter(c => c.startsWith(cur))
    }
  }

  if (!cands.length) return { completed: input, candidates: [] }
  const prefix = lcp(cands)
  const base = currentToken ? input.slice(0, currentToken.start) : input
  const completed = cands.length === 1
    ? base + quoteCommandToken(cands[0]) + ' '
    : base + quoteCommandToken(prefix)
  return { completed, candidates: cands }
}
