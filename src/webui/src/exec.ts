import { exec } from 'kernelsu'

const CTL = '/data/adb/modules/netproxy/netproxyctl'
const DEFAULT_TIMEOUT_MS = 30_000
declare const ksu: any
export const inKsu = typeof ksu !== 'undefined' || !!(window as any).ksu || !!(window as any).KSU

export interface CtlResult<T = unknown> {
  schema: number
  ok: boolean
  code: string
  message: string
  data?: T
}

const shq = (v: string) => `'${v.replace(/'/g, `'"'"'`)}'`

async function run(cmd: string) {
  if (!inKsu) return { out: '', err: '[非 KernelSU 环境]\n请在 KernelSU WebUI 中打开此页面执行命令。', code: 0 }
  try { const r = await exec(cmd); return { out: r.stdout, err: r.stderr, code: r.errno } }
  catch (e: any) { return { out: '', err: e?.message || String(e), code: -1 } }
}

export async function ctl(args: string[]) { return run([CTL, ...args].map(shq).join(' ')) }

export async function ctlJson<T>(args: string[], timeoutMs = DEFAULT_TIMEOUT_MS): Promise<CtlResult<T>> {
  const timeoutSeconds = Math.max(1, Math.ceil(timeoutMs / 1000))
  const r = await run([CTL, '--json', '--timeout', `${timeoutSeconds}s`, ...args].map(shq).join(' '))
  const payload = r.out.trim()

  if (payload) {
    try {
      const result = JSON.parse(payload) as Partial<CtlResult<T>>
      if (result.schema === 1 && typeof result.ok === 'boolean' &&
        typeof result.code === 'string' && typeof result.message === 'string') {
        return result as CtlResult<T>
      }
    } catch {
      // 下面统一返回结构化的传输错误。
    }
  }

  if (r.code !== 0) {
    return {
      schema: 1,
      ok: false,
      code: 'transport.failed',
      message: r.err.trim() || `模块命令失败（退出码 ${r.code}）`
    }
  }
  return {
    schema: 1,
    ok: false,
    code: payload ? 'transport.invalid_json' : 'transport.empty',
    message: payload ? '模块返回的数据格式无效' : (r.err.trim() || '模块没有返回有效结果')
  }
}

export const shell = run

export async function completions() {
  const [cat, sub] = await Promise.all([ctlJson<any[]>(['catalog', 'list']), ctlJson<any[]>(['sub', 'list'])])
  return {
    groups: cat.ok && Array.isArray(cat.data) ? cat.data.map((g: any) => g.id).filter(Boolean) : [],
    subs: sub.ok && Array.isArray(sub.data) ? sub.data.map((g: any) => g.id).filter(Boolean) : []
  }
}
