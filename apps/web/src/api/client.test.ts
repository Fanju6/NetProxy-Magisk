import { describe, expect, it, vi } from 'vitest'
import { z } from 'zod'
import { ApiClient } from './client'
import { ApiError, apiOperations, type ApiTransport } from './contracts'

function transport(status: number, body: unknown) {
  return {
    kind: 'mock',
    capabilities: apiOperations,
    request: vi.fn().mockResolvedValue({ status, body }),
    subscribe: () => () => {},
  } satisfies ApiTransport
}
describe('schema=1 boundary', () => {
  it('requires schema, HTTP success and ok together', async () => {
    for (const [status, body] of [
      [200, { schema: 2, ok: true, code: 'api.info', message: '', data: {} }],
      [500, { schema: 1, ok: true, code: 'api.info', message: '', data: {} }],
      [200, { schema: 1, ok: false, code: 'api.busy', message: '', data: {} }],
      [200, { schema: 1, ok: true, code: 'api.info', message: '', data: { value: 4 } }],
    ] as const) {
      await expect(
        new ApiClient(transport(status, body)).request(
          { method: 'GET', path: '/info' },
          z.object({ value: z.string() }),
        ),
      ).rejects.toBeInstanceOf(ApiError)
    }
  })
  it('preserves revision conflict and persisted partial success without replay', async () => {
    for (const [status, code, data] of [
      [409, 'config.conflict', {}],
      [502, 'node.runtime_sync_failed', { persisted: true, runtime_synced: false }],
    ] as const) {
      const mock = transport(status, { schema: 1, ok: false, code, message: 'fixture', data })
      const error = await new ApiClient(mock)
        .request({ method: 'PUT', path: '/selection', body: {} }, z.unknown())
        .catch((value: unknown) => value)
      expect(error).toMatchObject({ code, status, persisted: status === 502 })
      expect(mock.request).toHaveBeenCalledTimes(1)
    }
  })
  it('checks UTF-8 body size before sending a write', async () => {
    const mock = transport(200, {})
    await expect(
      new ApiClient(mock).request(
        { method: 'PUT', path: '/config', body: { content: '中'.repeat(1_400_000) } },
        z.unknown(),
      ),
    ).rejects.toMatchObject({ code: 'api.body_too_large', status: 413 })
    expect(mock.request).not.toHaveBeenCalled()
  })
  it('does not retry uncertain write failures', async () => {
    const mock = transport(200, {})
    mock.request.mockRejectedValue(new TypeError('connection interrupted'))
    await expect(
      new ApiClient(mock).request({ method: 'POST', path: '/service/start' }, z.unknown()),
    ).rejects.toBeInstanceOf(TypeError)
    expect(mock.request).toHaveBeenCalledTimes(1)
  })
})
