import { expect, it } from 'vitest'
import { ApiError } from '@/api/contracts'
import { resourceWriteUnconfirmed } from './resource-write'

it.each([400, 401, 403, 404, 409, 413, 422, 429])(
  'allows a corrected submission after an explicit %i rejection',
  (status) => {
    expect(resourceWriteUnconfirmed(new ApiError('fixture.rejected', '', status, null, true))).toBe(
      false,
    )
  },
)

it.each([408, 500, 502, 504])(
  'does not replay a write after an ambiguous %i response',
  (status) => {
    expect(resourceWriteUnconfirmed(new ApiError('fixture.failed', '', status, null, true))).toBe(
      true,
    )
  },
)

it('blocks replay after a lost, invalid or persisted response', () => {
  for (const error of [
    new TypeError('fixture lost response'),
    new ApiError('client.connection_failed', '', 0),
    new ApiError('client.invalid_response', '', 200),
    new ApiError('node.runtime_sync_failed', '', 409, { persisted: true }, true),
  ])
    expect(resourceWriteUnconfirmed(error)).toBe(true)
})

it.each(['api.busy', 'client.operation_denied', 'api.body_too_large'])(
  'allows correction after local admission rejection %s',
  (code) => {
    expect(resourceWriteUnconfirmed(new ApiError(code, '', 409))).toBe(false)
  },
)
