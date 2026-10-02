import { exec } from 'kernelsu'
import { decodeCtlResult, type CtlResult, type ExecResult } from './contract'
import { mockCtl } from './mock'

const CTL = '/data/adb/modules/netproxy/netproxyctl'
const DEFAULT_TIMEOUT_MS = 30_000
declare const ksu: any
const browserWindow = typeof window === 'undefined' ? undefined : (window as any)
export const inKsu = typeof ksu !== 'undefined' || !!browserWindow?.ksu || !!browserWindow?.KSU
const useMock = import.meta.env.DEV && !inKsu

const shq = (v: string) => `'${v.replace(/'/g, `'"'"'`)}'`

async function run(cmd: string): Promise<ExecResult> {
  if (!inKsu) return { out: '', err: '[非 KernelSU 环境]\n请在 KernelSU WebUI 中打开此页面执行命令。', code: 1 }
  try { const r = await exec(cmd); return { out: r.stdout, err: r.stderr, code: r.errno } }
  catch (e: any) { return { out: '', err: e?.message || String(e), code: -1 } }
}

async function runCtl(args: string[]): Promise<ExecResult> {
  if (useMock) return mockCtl(args)
  return run([CTL, ...args].map(shq).join(' '))
}

export async function ctl(args: string[]) { return runCtl(args) }

export async function ctlJson<T>(args: string[], timeoutMs = DEFAULT_TIMEOUT_MS): Promise<CtlResult<T>> {
  const timeoutSeconds = Math.max(1, Math.ceil(timeoutMs / 1000))
  const r = await runCtl(['--json', '--timeout', `${timeoutSeconds}s`, ...args])
  return decodeCtlResult<T>(r)
}

export const shell = run

interface CatalogCompletion {
  id: string
  type: string
}

function isCatalogCompletion(value: unknown): value is CatalogCompletion {
  if (typeof value !== 'object' || value === null) return false
  const item = value as Record<string, unknown>
  return typeof item.id === 'string' && item.id.length > 0 && typeof item.type === 'string'
}

export async function completions() {
  const result = await ctlJson<unknown[]>(['catalog', 'list'])
  const groups = result.ok && Array.isArray(result.data) ? result.data.filter(isCatalogCompletion) : []
  return {
    groups: groups.map(group => group.id),
    subs: groups.filter(group => group.type === 'subscription').map(group => group.id)
  }
}
