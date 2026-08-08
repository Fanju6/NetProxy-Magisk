import { exec } from 'kernelsu'

const CTL = '/data/adb/modules/netproxy/netproxyctl'
declare const ksu: any
export const inKsu = typeof ksu !== 'undefined' || !!(window as any).ksu || !!(window as any).KSU

const shq = (v: string) => `'${v.replace(/'/g, `'"'"'`)}'`

async function run(cmd: string) {
  if (!inKsu) return { out: '', err: '[非 KernelSU 环境]\n请在 KernelSU WebUI 中打开此页面执行命令。', code: 0 }
  try { const r = await exec(cmd); return { out: r.stdout, err: r.stderr, code: r.errno } }
  catch (e: any) { return { out: '', err: e?.message || String(e), code: -1 } }
}

export async function ctl(args: string[]) { return run([CTL, ...args].map(shq).join(' ')) }

export async function ctlJson<T>(args: string[]): Promise<T | null> {
  const r = await run([CTL, '--json', ...args].map(shq).join(' '))
  if (r.code !== 0) return null

  const payload = r.out.trim()
  if (!payload) return null

  try {
    const result = JSON.parse(payload)
    if (result?.schema !== 1 || result?.ok !== true) return null
    return result as T
  } catch {
    return null
  }
}

export const shell = run

export async function completions() {
  const [cat, sub] = await Promise.all([ctlJson<any>(['catalog', 'list']), ctlJson<any>(['sub', 'list'])])
  return {
    groups: Array.isArray(cat?.data) ? cat.data.map((g: any) => g.id).filter(Boolean) : [],
    subs: Array.isArray(sub?.data) ? sub.data.map((g: any) => g.id).filter(Boolean) : []
  }
}
