import { z } from 'zod'
import { ApiError, envelopeSchema, type ApiRequest, type ApiTransport } from './contracts'

export class ApiClient {
  constructor(readonly transport: ApiTransport) {}

  async request<S extends z.ZodType>(
    request: ApiRequest,
    schema: S,
    signal?: AbortSignal,
  ): Promise<z.infer<S>> {
    if (this.transport.readOnly && request.method !== 'GET')
      throw new ApiError('client.read_only', 'This connection is read-only', 403)
    if (
      request.body !== undefined &&
      new TextEncoder().encode(JSON.stringify(request.body)).length > 4 * 1024 * 1024
    ) {
      throw new ApiError('api.body_too_large', 'Request body exceeds 4 MiB', 413)
    }
    const response = await this.transport.request(request, signal)
    const result = envelopeSchema.safeParse(response.body)
    if (!result.success)
      throw new ApiError('client.invalid_response', 'Invalid schema=1 response', response.status)
    const { ok, code, message, data } = result.data
    if (!ok || response.status < 200 || response.status >= 300)
      throw new ApiError(code, message, response.status, data)
    const parsed = schema.safeParse(data)
    if (!parsed.success)
      throw new ApiError('client.invalid_response', 'Invalid response data', response.status)
    return parsed.data
  }
}

export const unavailableTransport: ApiTransport = {
  kind: 'unavailable',
  readOnly: true,
  request: () =>
    Promise.reject(
      new ApiError('client.transport_unavailable', 'Trusted transport is not configured', 0),
    ),
  subscribe: (_topics, _consume, onError) => {
    queueMicrotask(() =>
      onError(
        new ApiError('client.transport_unavailable', 'Trusted transport is not configured', 0),
      ),
    )
    return () => {}
  },
}
