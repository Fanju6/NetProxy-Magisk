import { ApiError } from '@/api/contracts'

export function resourceWriteUnconfirmed(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.persisted) return true
  if (
    [
      'client.operation_denied',
      'client.transport_unavailable',
      'api.invalid_request',
      'api.body_too_large',
      'api.read_only',
      'api.busy',
      'api.request_exists',
    ].includes(error.code)
  )
    return false
  return !(
    error.responseReceived && [400, 401, 403, 404, 409, 413, 422, 429].includes(error.status)
  )
}
