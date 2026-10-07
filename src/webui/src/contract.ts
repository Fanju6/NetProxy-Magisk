export const CONTRACT_SCHEMA = 1

export type InboundBackend = 'ebpf' | 'tun'

export interface ServiceBackendStatus {
  configured_backend: string
  active_backend: InboundBackend | null
}

export interface PrepareResult {
  active_group_id: string
  active_group_tag: string
  selector_mode: string
  selected_node_ref: string
  outbound_mode?: string
  group_count: number
  node_count: number
  providers: string
  outbounds: string
  inbound: string
  backend: InboundBackend
}

export function decodeCtlResult<T>(r: ExecResult): CtlResult<T> {
  const payload = r.out.trim()

  if (payload) {
    try {
      const result = JSON.parse(payload) as Partial<CtlResult<T>>
      if (result.schema === CONTRACT_SCHEMA && typeof result.ok === 'boolean' &&
        typeof result.code === 'string' && typeof result.message === 'string') {
        if (!result.ok || r.code === 0) return result as CtlResult<T>
        return {
          ...result as CtlResult<T>,
          ok: false,
          code: 'transport.failed',
          message: r.err.trim() || `模块命令失败（退出码 ${r.code}）`
        }
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

export interface CtlResult<T = unknown> {
  schema: number
  ok: boolean
  code: string
  message: string
  data?: T
}

export interface ExecResult {
  out: string
  err: string
  code: number
}
