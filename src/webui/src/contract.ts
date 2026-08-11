export const CONTRACT_SCHEMA = 1

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
