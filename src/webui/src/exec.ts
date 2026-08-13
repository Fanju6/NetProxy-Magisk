import { exec } from 'kernelsu'
import { CONTRACT_SCHEMA, type CtlResult, type ExecResult } from './contract'
import { mockCtl } from './mock'

const CTL = '/data/adb/modules/netproxy/netproxyctl'
const DEFAULT_TIMEOUT_MS = 30_000
declare const ksu: any
const browserWindow = typeof window === 'undefined' ? undefined : (window as any)
export const inKsu = typeof ksu !== 'undefined' || !!browserWindow?.ksu || !!browserWindow?.KSU
const useMock = import.meta.env.DEV && !inKsu

const shq = (v: string) => `'${v.replace(/'/g, `'"'"'`)}'`

async function run(cmd: string): Promise<ExecResult> {
  if (!inKsu) return { out: '', err: '[非 KernelSU 环境]\n请在 KernelSU WebUI 中打开此页面执行命令。', code: 0 }
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
  const payload = r.out.trim()

  if (payload) {
    try {
      const result = JSON.parse(payload) as Partial<CtlResult<T>>
      if (result.schema === CONTRACT_SCHEMA && typeof result.ok === 'boolean' &&
        typeof result.code === 'string' && typeof result.message === 'string') {
        return result as CtlResult<T>
      }
    } catch {
      // 下面统一返回结构化的传输错误。
    }
  }

  if (r.code !== 0) {
    return {
      schema: CONTRACT_SCHEMA,
      ok: false,
      code: 'transport.failed',
      message: r.err.trim() || `模块命令失败（退出码 ${r.code}）`
    }
  }
  return {
    schema: CONTRACT_SCHEMA,
    ok: false,
    code: payload ? 'transport.invalid_json' : 'transport.empty',
    message: payload ? '模块返回的数据格式无效' : (r.err.trim() || '模块没有返回有效结果')
  }
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
