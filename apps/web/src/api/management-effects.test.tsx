import { act, cleanup, render, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import { z } from 'zod'
import { ApiContext, PreferencesContext } from '@/app/context'
import { translator } from '@/app/i18n'
import { createFixtures } from '@/mocks/fixtures'
import { ApiClient } from './client'
import {
  ApiError,
  apiOperations,
  type ApiEvent,
  type ApiRequest,
  type ApiTransport,
} from './contracts'
import { ModuleEvents } from './events'
import { managementEpoch, managementKey, useManagementAction } from './management'

const caches: QueryClient[] = []
afterEach(() => {
  cleanup()
  caches.splice(0).forEach((cache) => cache.clear())
})
const frame = (name: ApiEvent['name'], data: unknown): ApiEvent => ({
  name,
  body: { schema: 1, ok: true, code: 'fixture', message: '', data },
})
function setup() {
  const fixture = createFixtures()
  let consume!: (event: ApiEvent) => void
  const close = vi.fn()
  const subscribe = vi.fn<ApiTransport['subscribe']>((_topics, listener) => {
    consume = listener
    return close
  })
  const request = vi.fn<ApiTransport['request']>().mockResolvedValue({
    status: 200,
    body: frame('status', {}).body,
  })
  const api = new ApiClient({ kind: 'native', capabilities: apiOperations, request, subscribe })
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
  caches.push(cache)
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <PreferencesContext
        value={{
          preferences: { theme: 'light', locale: 'en' },
          t: translator('en'),
          update: vi.fn(),
        }}
      >
        <ApiContext value={api}>
          <QueryClientProvider client={cache}>{children}</QueryClientProvider>
        </ApiContext>
      </PreferencesContext>
    )
  }
  render(<ModuleEvents />, { wrapper: Wrapper })
  const hook = renderHook(useManagementAction, { wrapper: Wrapper })
  act(() => {
    consume(frame('status', fixture.service))
    consume(frame('groups', fixture.policy))
    consume(
      frame(
        'subscriptions',
        fixture.snapshot.groups.map(({ group }) => group),
      ),
    )
    cache.setQueryData(['traffic-history'], [7, 8])
  })
  return { cache, hook, request, subscribe, close }
}
const validate: ApiRequest = {
  method: 'POST',
  path: '/config/validate',
  query: { target: 'module/auto_start' },
  body: { revision: 'fixture', content: '{"auto_start":true}' },
}
const probe: ApiRequest = {
  method: 'POST',
  path: '/diagnostics/ebpf',
  body: { mode: 'configured', raw: false },
}

it.each([validate, probe])(
  'keeps the SSE session and traffic after preflight %j',
  async (input) => {
    const h = setup()
    await act(async () => {
      await h.hook.result.current.run(input, z.unknown())
    })
    expect(managementEpoch(h.cache)).toBe(0)
    expect(h.cache.getQueryData(managementKey)).toEqual({ epoch: 0, pending: false })
    expect(h.subscribe).toHaveBeenCalledOnce()
    expect(h.close).not.toHaveBeenCalled()
    expect(h.cache.getQueryData(['traffic-history'])).toEqual([7, 8])
  },
)

it.each([validate, probe])(
  'keeps the SSE session after rejected or cancelled preflight %j',
  async (input) => {
    const h = setup()
    for (const failure of [
      new ApiError('config.invalid', 'Invalid', 422),
      new DOMException('Cancelled', 'AbortError'),
    ]) {
      h.request.mockRejectedValueOnce(failure)
      await act(async () => {
        await expect(h.hook.result.current.run(input, z.unknown())).rejects.toBeInstanceOf(ApiError)
      })
    }
    expect(managementEpoch(h.cache)).toBe(0)
    expect(h.subscribe).toHaveBeenCalledOnce()
    expect(h.cache.getQueryData(['traffic-history'])).toEqual([7, 8])
  },
)

it.each(['success', 'unknown', 'persisted'] as const)(
  'reconciles real writes after %s results',
  async (result) => {
    const h = setup()
    if (result === 'unknown') h.request.mockRejectedValueOnce(new Error('connection lost'))
    if (result === 'persisted')
      h.request.mockRejectedValueOnce(
        new ApiError('node.runtime_sync_failed', 'Not confirmed', 502, { persisted: true }, true),
      )
    await act(async () => {
      await h.hook.result.current
        .run({ ...validate, method: 'PUT', path: '/config' }, z.unknown())
        .catch(() => {})
    })
    expect(managementEpoch(h.cache)).toBe(1)
    await waitFor(() => expect(h.subscribe).toHaveBeenCalledTimes(2))
    expect(h.close).toHaveBeenCalledOnce()
    expect(h.cache.getQueryData(['traffic-history'])).toEqual([])
  },
)

it('retains write admission while preflight is pending and does not exempt invalid requests', async () => {
  const h = setup()
  let finish!: (value: Awaited<ReturnType<ApiTransport['request']>>) => void
  h.request.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  )
  let pending!: Promise<unknown>
  act(() => {
    pending = h.hook.result.current.run(validate, z.unknown())
  })
  await expect(h.hook.result.current.run(probe, z.unknown())).rejects.toMatchObject({
    code: 'api.busy',
  })
  await act(async () => {
    finish({ status: 200, body: frame('status', {}).body })
    await pending
  })
  await act(async () => {
    await h.hook.result.current
      .run({ ...validate, body: { arbitrary: true } }, z.unknown())
      .catch(() => {})
  })
  expect(managementEpoch(h.cache)).toBe(1)
})
