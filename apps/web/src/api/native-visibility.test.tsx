import { act, cleanup, render, renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import { z } from 'zod'
import { ApiContext, PreferencesContext } from '@/app/context'
import { translator } from '@/app/i18n'
import { createFixtures } from '@/mocks/fixtures'
import { ApiClient } from './client'
import type { ApiEvent } from './contracts'
import { ModuleEvents } from './events'
import { managementKey, useManagementAction } from './management'
import { NativeTransport, type NativeIpc } from './native'

afterEach(() => {
  cleanup()
  window.dispatchEvent(new Event('pagehide'))
})

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

async function flush() {
  await act(async () => {
    for (let i = 0; i < 30; i++) await Promise.resolve()
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

function setup() {
  const fixture = createFixtures()
  const initial = deferred<unknown>()
  const management = deferred<unknown>()
  const pulls = new Map<unknown, ReturnType<typeof deferred<unknown>>>()
  const callbacks: ((event: unknown) => void)[] = []
  const unregister = vi.fn().mockResolvedValue(undefined)
  const listen = vi.fn<NativeIpc['listen']>((callback) => {
    callbacks.push(callback)
    return Promise.resolve({ unregister })
  })
  const body = (data: unknown) => ({ schema: 1, ok: true, code: 'fixture', message: '', data })
  const invoke = vi.fn(async (name: string, args: Record<string, unknown>): Promise<unknown> => {
    let value: unknown
    if (name.endsWith('|visibility')) value = await initial.promise
    else if (name.endsWith('|next_event')) {
      const pull = deferred<unknown>()
      pulls.set(args.subscriptionId, pull)
      value = await pull.promise
    } else if (name.endsWith('|management')) value = await management.promise
    else if (name.endsWith('|read')) {
      const data = {
        service: fixture.service,
        policy_groups: fixture.policy,
        mode: {
          mode: fixture.service.configured_outbound_mode,
          available: fixture.service.available_outbound_modes,
        },
      }
      value = { status: 200, body: body(data[args.resource as keyof typeof data]) }
    }
    return value
  })
  const transport = new NativeTransport({
    invoke: <T,>(name: string, args: Record<string, unknown>) => invoke(name, args) as Promise<T>,
    listen,
  })
  const api = new ApiClient(transport)
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity, staleTime: Infinity } },
  })
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
  const calls = (operation: string) =>
    invoke.mock.calls.filter(([name]) => name.endsWith(`|${operation}`))
  const visible = async (value: boolean) => {
    act(() => callbacks.at(-1)!({ visible: value }))
    await flush()
  }
  const emit = async (name: ApiEvent['name'], data: unknown) => {
    const id = calls('open_events').at(-1)![1].subscriptionId
    const pull = pulls.get(id)!
    pulls.delete(id)
    pull.resolve({ name, body: body(data) })
    await flush()
  }
  const complete = async () => {
    await emit('status', fixture.service)
    await emit('groups', fixture.policy)
    await emit(
      'subscriptions',
      fixture.snapshot.groups.map(({ group }) => group),
    )
    expect(cache.getQueryData(['events-connection'])).toBe('live')
  }
  return {
    initial,
    management,
    cache,
    Wrapper,
    calls,
    visible,
    emit,
    complete,
    listen,
    unregister,
    body,
  }
}

it.each(['events-first', 'management-first'] as const)(
  'shares native background cancellation and foreground SSE recovery with %s consumers',
  async (order) => {
    const h = setup()
    const mountEvents = () => render(<ModuleEvents />, { wrapper: h.Wrapper })
    const mountManagement = () => renderHook(useManagementAction, { wrapper: h.Wrapper })
    const events = order === 'events-first' ? mountEvents() : undefined
    const management = mountManagement()
    const eventView = events ?? mountEvents()
    await flush()
    expect(h.calls('open_events')).toHaveLength(0)
    h.initial.resolve({ visible: true })
    await flush()
    expect(h.listen).toHaveBeenCalledOnce()
    expect(h.calls('visibility')).toHaveLength(1)
    await h.complete()

    let request!: Promise<unknown>
    act(() => {
      request = management.result.current.run(
        {
          method: 'PUT',
          path: '/config',
          query: { target: 'module/auto_start' },
          body: { revision: 'a'.repeat(64), content: '{"auto_start":false}' },
        },
        z.unknown(),
      )
    })
    const rejected = expect(request).rejects.toMatchObject({ code: 'client.connection_failed' })
    expect(h.calls('management')).toHaveLength(1)
    await h.visible(false)
    await rejected
    expect(h.calls('cancel')).toEqual([
      ['plugin:module-api|cancel', { requestId: h.calls('management')[0]![1].requestId }],
    ])
    expect(h.calls('close_events')).toHaveLength(1)
    expect(h.cache.getQueryData(['events-connection'])).toBe('paused')
    expect(h.cache.getQueryData(managementKey)).toEqual({ epoch: 1, pending: false })
    h.management.resolve({ status: 200, body: h.body({ ignored: true }) })
    await flush()
    expect(h.calls('management')).toHaveLength(1)
    await h.visible(true)
    expect(h.calls('open_events')).toHaveLength(2)
    await h.complete()

    management.unmount()
    expect(h.unregister).not.toHaveBeenCalled()
    await h.visible(false)
    expect(h.calls('close_events')).toHaveLength(2)
    await h.visible(true)
    expect(h.calls('open_events')).toHaveLength(3)
    await h.complete()
    expect(h.listen).toHaveBeenCalledOnce()
    expect(h.calls('visibility')).toHaveLength(1)
    eventView.unmount()
    expect(h.unregister).toHaveBeenCalledOnce()
    h.cache.clear()
  },
)

it('keeps management cancellation active after ModuleEvents unmounts', async () => {
  const h = setup()
  const events = render(<ModuleEvents />, { wrapper: h.Wrapper })
  await flush()
  h.initial.resolve({ visible: true })
  await flush()
  const management = renderHook(useManagementAction, { wrapper: h.Wrapper })
  await flush()
  expect(h.listen).toHaveBeenCalledOnce()
  expect(h.calls('visibility')).toHaveLength(1)
  events.unmount()
  expect(h.unregister).not.toHaveBeenCalled()
  let request!: Promise<unknown>
  act(() => {
    request = management.result.current.run(
      { method: 'GET', path: '/config', query: { target: 'module' } },
      z.unknown(),
    )
  })
  const rejected = expect(request).rejects.toMatchObject({ code: 'client.connection_failed' })
  await h.visible(false)
  await rejected
  expect(h.calls('cancel')).toHaveLength(1)
  management.unmount()
  expect(h.unregister).toHaveBeenCalledOnce()
  h.cache.clear()
})
