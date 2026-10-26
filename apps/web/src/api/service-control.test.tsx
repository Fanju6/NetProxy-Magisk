import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { onlineManager, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiContext, PreferencesContext } from '@/app/context'
import { translator } from '@/app/i18n'
import { AppProviders } from '@/app/providers'
import { DashboardPage } from '@/features/dashboard'
import { NodesPage } from '@/features/nodes'
import { SettingsPage } from '@/features/settings'
import { createFixtures } from '@/mocks/fixtures'
import { ApiClient } from './client'
import {
  ApiError,
  type ApiEvent,
  type ApiRequest,
  type ApiTransport,
  type ServiceStatus,
} from './contracts'
import { ModuleEvents } from './events'
import { moduleQueries, useServiceAction } from './queries'
import type { ServiceAction } from './service-control'

type Response = Awaited<ReturnType<ApiTransport['request']>>

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

const status = (state: ServiceStatus['state']): ServiceStatus => ({
  ...createFixtures().service,
  state,
  ...(state === 'ready' ? {} : { active_backend: null, pid: null }),
})
const envelope = (data: unknown) => ({ schema: 1, ok: true, code: 'fixture', message: '', data })
const response = (data: unknown): Response => ({ status: 200, body: envelope(data) })
const rejection = (httpStatus: number, data: unknown = null): Response => ({
  status: httpStatus,
  body: { schema: 1, ok: false, code: 'service.rejected', message: 'Fixture rejection', data },
})
const frame = (name: ApiEvent['name'], data: unknown): ApiEvent => ({ name, body: envelope(data) })
const target = (action: ServiceAction) => (action === 'start' ? 'ready' : 'stopped')
const opposite = (action: ServiceAction) => (action === 'start' ? 'stopped' : 'ready')
const clients: QueryClient[] = []

afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
  vi.useRealTimers()
})

function setup(initial: ServiceStatus['state'] = 'stopped') {
  const calls: {
    request: ApiRequest
    signal: AbortSignal | undefined
    aborted: ReturnType<typeof vi.fn>
    result: ReturnType<typeof deferred<Response>>
  }[] = []
  const sessions: {
    consume: (event: ApiEvent) => void
    fail: (error: Error) => void
    close: ReturnType<typeof vi.fn>
  }[] = []
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: ['info.read', 'service.read', 'service.start', 'service.stop'],
    request: vi.fn((request, signal) => {
      const result = deferred<Response>()
      const aborted = vi.fn()
      signal?.addEventListener('abort', aborted, { once: true })
      calls.push({ request, signal, aborted, result })
      return result.promise
    }),
    subscribe: (_topics, consume, fail) => {
      const close = vi.fn()
      sessions.push({ consume, fail, close })
      return close
    },
  }
  const cache = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
      mutations: { retry: 3, retryDelay: 0, gcTime: Infinity },
    },
  })
  clients.push(cache)
  cache.setQueryData(['service'], status(initial))
  const api = new ApiClient(transport)
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
  const writes = () => calls.filter(({ request }) => request.method === 'POST')
  const reads = () => calls.filter(({ request }) => request.method === 'GET')
  const read = async (index: number, value: ServiceStatus) => {
    await waitFor(() => expect(reads()).toHaveLength(index + 1))
    expect(reads()[index]!.request).toEqual({ method: 'GET', path: '/service' })
    await act(async () => reads()[index]!.result.resolve(response(value)))
  }
  const settled = () => waitFor(() => expect(cache.isMutating()).toBe(0))
  const complete = (value: ServiceStatus, session = sessions.at(-1)!) => {
    const { snapshot, policy } = createFixtures()
    act(() => {
      session.consume(frame('status', value))
      session.consume(frame('groups', policy))
      session.consume(
        frame(
          'subscriptions',
          snapshot.groups.map(({ group }) => group),
        ),
      )
    })
  }
  return {
    api,
    cache,
    sessions,
    calls,
    writes,
    reads,
    read,
    settled,
    complete,
    mount: () => renderHook(useServiceAction, { wrapper: Wrapper }),
    events: () => render(<ModuleEvents />, { wrapper: Wrapper }),
  }
}

describe('useServiceAction lifecycle', () => {
  it.each(['start', 'stop'] as const)(
    'dispatches %s and loopback service/info GETs while offline without queuing a write for reconnect',
    async (action) => {
      const wasOnline = onlineManager.isOnline()
      const h = setup(opposite(action))
      onlineManager.setOnline(false)
      const hook = h.mount()
      try {
        act(() => {
          hook.result.current.mutate(action)
          hook.result.current.mutate(action)
        })
        await waitFor(() => expect(h.writes()).toHaveLength(1))
        expect(onlineManager.isOnline()).toBe(false)
        expect(h.writes()[0]!.request).toEqual({ method: 'POST', path: `/service/${action}` })
        expect(
          h.cache
            .getMutationCache()
            .getAll()
            .every(({ state }) => !state.isPaused),
        ).toBe(true)
        await act(async () =>
          h.writes()[0]!.result.resolve(response({ action, status: status(target(action)) })),
        )
        await waitFor(() => expect(h.reads()).toHaveLength(1))
        expect(onlineManager.isOnline()).toBe(false)
        expect(h.cache.getQueryState(['service'])?.fetchStatus).toBe('fetching')
        await h.read(0, status(target(action)))
        await h.settled()
        await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
        expect(hook.result.current.isPending).toBe(false)
        const info = h.cache.fetchQuery(moduleQueries(h.api).info)
        void info.catch(() => {})
        await waitFor(() => expect(h.reads()).toHaveLength(2))
        expect(h.reads()[1]!.request).toEqual({ method: 'GET', path: '/info' })
        expect(onlineManager.isOnline()).toBe(false)
        const details = {
          api_version: 1,
          version: 'fixture',
          version_code: '1',
          instance_id: 'fixture',
          pid: 1,
          platform: 'android',
          capabilities: ['events'],
          memory: {
            heap_alloc_bytes: 1,
            heap_inuse_bytes: 1,
            heap_released_bytes: 0,
            stack_inuse_bytes: 1,
          },
        }
        await act(async () => h.reads()[1]!.result.resolve(response(details)))
        await expect(info).resolves.toEqual(details)
        expect(
          h.cache
            .getMutationCache()
            .getAll()
            .map(({ state }) => state.status),
        ).toEqual(['success'])
        await act(async () => onlineManager.setOnline(true))
        expect(h.writes()).toHaveLength(1)
        expect(h.reads()).toHaveLength(2)
        expect(h.cache.isMutating()).toBe(0)
      } finally {
        hook.unmount()
        h.cache.clear()
        onlineManager.setOnline(wasOnline)
      }
    },
  )

  it('coalesces synchronous duplicate and conflicting calls across hook instances', async () => {
    const h = setup()
    const first = h.mount()
    const second = h.mount()
    act(() => {
      first.result.current.mutate('start')
      first.result.current.mutate('start')
      first.result.current.mutate('stop')
      second.result.current.mutate('start')
      second.result.current.mutate('stop')
    })
    await waitFor(() => expect(h.writes()).toHaveLength(1))
    expect(h.writes()[0]!.request).toEqual({ method: 'POST', path: '/service/start' })
    await waitFor(() => {
      expect(first.result.current.isPending).toBe(true)
      expect(second.result.current.isPending).toBe(true)
    })
    expect(h.cache.getQueryData(['service'])).toEqual(status('stopped'))
    await act(async () =>
      h.writes()[0]!.result.resolve(response({ action: 'start', status: status('ready') })),
    )
    await h.read(0, status('ready'))
    await h.settled()
    await waitFor(() => {
      expect(first.result.current.isUncertain).toBe(false)
      expect(second.result.current.isPending).toBe(false)
    })
    expect(h.writes()).toHaveLength(1)
  })

  it.each(['start', 'stop'] as const)(
    'requires a fresh GET after %s and never publishes the POST snapshot optimistically',
    async (action) => {
      const h = setup(opposite(action))
      const hook = h.mount()
      const before = h.cache.getQueryData(['service'])
      act(() => hook.result.current.mutate(action))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      expect(h.cache.getQueryData(['service'])).toBe(before)
      const posted = { ...status(target(action)), memory_bytes: 101 }
      await act(async () => h.writes()[0]!.result.resolve(response({ action, status: posted })))
      await waitFor(() => expect(h.reads()).toHaveLength(1))
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
      expect(hook.result.current.isConfirming).toBe(true)
      expect(h.cache.getQueryData(['service'])).toBe(before)
      const confirmed = { ...status(target(action)), memory_bytes: 202 }
      await h.read(0, confirmed)
      await h.settled()
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
      expect(hook.result.current.isPending).toBe(false)
      expect(hook.result.current.error).toBeNull()
      expect(h.cache.getQueryData(['service'])).toEqual(confirmed)
      expect(h.writes()).toHaveLength(1)
    },
  )

  it('keeps an accepted write alive across unmount and blocks a remounted caller until verification', async () => {
    const h = setup()
    const original = h.mount()
    act(() => original.result.current.mutate('start'))
    await waitFor(() => expect(h.writes()).toHaveLength(1))
    const accepted = h.writes()[0]!
    original.unmount()
    expect(accepted.signal?.aborted ?? false).toBe(false)
    expect(accepted.aborted).not.toHaveBeenCalled()
    const next = h.mount()
    await waitFor(() => expect(next.result.current.isPending).toBe(true))
    act(() => {
      next.result.current.mutate('start')
      next.result.current.mutate('stop')
    })
    expect(h.writes()).toHaveLength(1)
    await act(async () =>
      accepted.result.resolve(response({ action: 'start', status: status('ready') })),
    )
    await h.read(0, status('ready'))
    await h.settled()
    await waitFor(() => expect(next.result.current.isUncertain).toBe(false))
    expect(next.result.current.isPending).toBe(false)
    expect(h.cache.getQueryData(['service'])).toEqual(status('ready'))
    expect(accepted.aborted).not.toHaveBeenCalled()
  })

  it.each([401, 403, 409, 429])(
    'treats HTTP %s as explicit rejection and overrides nonzero mutation retry defaults',
    async (httpStatus) => {
      const h = setup()
      const hook = h.mount()
      act(() => hook.result.current.mutate('start'))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () => h.writes()[0]!.result.resolve(rejection(httpStatus)))
      await h.read(0, status('stopped'))
      await h.settled()
      await waitFor(() => expect(hook.result.current.error).toBeInstanceOf(ApiError))
      expect(hook.result.current.error).toMatchObject({
        status: httpStatus,
        code: 'service.rejected',
      })
      expect(hook.result.current.isPending).toBe(false)
      expect(hook.result.current.isUncertain).toBe(false)
      expect(hook.result.current.persisted).toBe(false)
      expect(h.writes()).toHaveLength(1)
    },
  )

  it.each([
    ['HTTP 500', () => rejection(500), 500],
    ['HTTP 502', () => rejection(502), 502],
    ['HTTP 504', () => rejection(504), 504],
    ['HTTP 408', () => rejection(408), 408],
    ['gateway HTTP 502 without an envelope', () => ({ status: 502, body: 'Bad gateway' }), null],
    [
      'gateway HTTP 504 without an envelope',
      () => ({ status: 504, body: 'Gateway timeout' }),
      null,
    ],
    [
      'native timeout',
      () => new ApiError('client.timeout', 'Native response timed out', 504),
      null,
    ],
    ['disconnect', () => new TypeError('Connection lost after acceptance'), null],
    ['cancellation', () => new DOMException('Connection cancelled', 'AbortError'), null],
    ['invalid 409 envelope', () => ({ status: 409, body: { schema: 2 } }), null],
  ] as const)(
    'keeps %s uncertain without replay and verifies with GET only',
    async (_name, failure, retainedErrorStatus) => {
      const h = setup()
      const hook = h.mount()
      act(() => hook.result.current.mutate('start'))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () => {
        const outcome = failure()
        if (outcome instanceof Error) h.writes()[0]!.result.reject(outcome)
        else h.writes()[0]!.result.resolve(outcome)
      })
      await h.read(0, status('starting'))
      await h.settled()
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
      expect(hook.result.current.isPending).toBe(false)
      expect(hook.result.current.error).toBeNull()
      act(() => {
        hook.result.current.mutate('start')
        hook.result.current.mutate('stop')
      })
      let verification!: Promise<void>
      act(() => {
        verification = hook.result.current.verify()
      })
      await h.read(1, status('ready'))
      await act(async () => verification)
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
      if (retainedErrorStatus === null) expect(hook.result.current.error).toBeNull()
      else
        expect(hook.result.current.error).toMatchObject({
          status: retainedErrorStatus,
          code: 'service.rejected',
          message: 'Fixture rejection',
          responseReceived: true,
        })
      expect(hook.result.current.persisted).toBe(false)
      expect(h.writes()).toHaveLength(1)
      expect(h.reads()).toHaveLength(2)
    },
  )

  it('does not turn POST success plus an unavailable GET into success or another write', async () => {
    const h = setup()
    const hook = h.mount()
    act(() => hook.result.current.mutate('start'))
    await waitFor(() => expect(h.writes()).toHaveLength(1))
    await act(async () =>
      h.writes()[0]!.result.resolve(response({ action: 'start', status: status('ready') })),
    )
    await waitFor(() => expect(h.reads()).toHaveLength(1))
    await act(async () => h.reads()[0]!.result.reject(new TypeError('Read unavailable')))
    await h.settled()
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
    expect(h.cache.getQueryData(['service'])).toEqual(status('stopped'))
    let verification!: Promise<void>
    act(() => {
      verification = hook.result.current.verify()
    })
    await h.read(1, status('starting'))
    await act(async () => verification)
    expect(hook.result.current.isUncertain).toBe(true)
    act(() => hook.result.current.mutate('stop'))
    expect(h.writes()).toHaveLength(1)
  })

  it.each([502, 504, 408])(
    'accepts a failed terminal GET after a completed backend HTTP %s envelope without replaying',
    async (httpStatus) => {
      const h = setup()
      const hook = h.mount()
      act(() => hook.result.current.mutate('start'))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () => h.writes()[0]!.result.resolve(rejection(httpStatus)))
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
      await h.read(0, status('failed'))
      await h.settled()
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
      expect(hook.result.current.error).toMatchObject({
        status: httpStatus,
        responseReceived: true,
      })
      expect(h.writes()).toHaveLength(1)
    },
  )

  it('retains persisted=true and the original error even when GET confirms the desired state', async () => {
    const h = setup()
    const hook = h.mount()
    act(() => hook.result.current.mutate('start'))
    await waitFor(() => expect(h.writes()).toHaveLength(1))
    await act(async () =>
      h
        .writes()[0]!
        .result.resolve(rejection(409, { persisted: true, detail: 'runtime sync failed' })),
    )
    await h.read(0, status('starting'))
    await h.settled()
    await waitFor(() => expect(hook.result.current.persisted).toBe(true))
    expect(hook.result.current.isUncertain).toBe(true)
    let verification!: Promise<void>
    act(() => {
      verification = hook.result.current.verify()
    })
    await h.read(1, status('ready'))
    await act(async () => verification)
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
    expect(hook.result.current.persisted).toBe(true)
    expect(hook.result.current.error).toMatchObject({
      status: 409,
      code: 'service.rejected',
      data: { persisted: true },
    })
    expect(h.writes()).toHaveLength(1)
  })

  it.each(['start', 'stop'] as const)(
    'keeps uncertain %s blocked on the wrong target until 150 seconds AND a terminal observation',
    async (action) => {
      const now = 1_800_000_000_000
      const clock = vi.spyOn(Date, 'now').mockReturnValue(now)
      const h = setup(opposite(action))
      const hook = h.mount()
      const failure = new TypeError('Write outcome unknown')
      act(() => hook.result.current.mutate(action))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () => h.writes()[0]!.result.reject(failure))
      await h.read(0, status(opposite(action)))
      await h.settled()
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
      const verify = async (index: number, state: ServiceStatus['state']) => {
        let pending!: Promise<void>
        act(() => {
          pending = hook.result.current.verify()
        })
        await h.read(index, status(state))
        await act(async () => pending)
      }
      clock.mockReturnValue(now + 149_999)
      await verify(1, opposite(action))
      expect(hook.result.current.isUncertain).toBe(true)
      await verify(2, 'failed')
      expect(hook.result.current.isUncertain).toBe(true)
      act(() => hook.result.current.mutate(action))
      expect(h.writes()).toHaveLength(1)
      clock.mockReturnValue(now + 150_000)
      expect(hook.result.current.isUncertain).toBe(true)
      await verify(3, action === 'start' ? 'starting' : 'stopping')
      expect(hook.result.current.isUncertain).toBe(true)
      await verify(4, opposite(action))
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
      expect(hook.result.current.error).toBe(failure)
      expect(h.writes()).toHaveLength(1)
      expect(h.reads()).toHaveLength(5)
    },
  )

  it.each(['start', 'stop'] as const)(
    'does not mistake transient failed status for completed %s after losing the write response',
    async (action) => {
      const h = setup(opposite(action))
      const hook = h.mount()
      const failure = new TypeError('Write outcome unknown')
      act(() => hook.result.current.mutate(action))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () => h.writes()[0]!.result.reject(failure))
      await h.read(0, status('failed'))
      await h.settled()
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
      expect(hook.result.current.error).toBeNull()
      act(() => hook.result.current.mutate(action))
      expect(h.writes()).toHaveLength(1)
      let verification!: Promise<void>
      act(() => {
        verification = hook.result.current.verify()
      })
      await h.read(1, status(target(action)))
      await act(async () => verification)
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
      expect(hook.result.current.error).toBeNull()
      expect(h.writes()).toHaveLength(1)
    },
  )

  it('releases unknown failed status only after the 150-second deadline, without polling or replay', async () => {
    const now = 1_800_000_000_000
    const clock = vi.spyOn(Date, 'now').mockReturnValue(now)
    const h = setup()
    const hook = h.mount()
    const failure = new TypeError('Write response lost')
    act(() => hook.result.current.mutate('start'))
    await waitFor(() => expect(h.writes()).toHaveLength(1))
    await act(async () => h.writes()[0]!.result.reject(failure))
    await h.read(0, status('failed'))
    await h.settled()
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
    clock.mockReturnValue(now + 150_000)
    act(() => {
      hook.result.current.mutate('start')
      hook.result.current.mutate('stop')
    })
    expect(h.writes()).toHaveLength(1)
    expect(h.reads()).toHaveLength(1)
    let verification!: Promise<void>
    act(() => {
      verification = hook.result.current.verify()
    })
    await h.read(1, status('failed'))
    await act(async () => verification)
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
    expect(hook.result.current.error).toBe(failure)
    expect(h.writes()).toHaveLength(1)
    expect(h.reads()).toHaveLength(2)
  })

  it('rejects old SSE frames synchronously at both action epoch changes and accepts only the replacement session', async () => {
    const h = setup()
    const hook = h.mount()
    h.events()
    expect(h.sessions).toHaveLength(1)
    const old = h.sessions[0]!
    act(() => {
      hook.result.current.mutate('start')
      old.consume(frame('status', status('ready')))
      old.fail(new ApiError('api.unauthorized', 'Old session', 401))
      expect(h.cache.getQueryData(['service'])).toEqual(status('stopped'))
      expect(h.cache.getQueryData(['events-connection'])).not.toBe('offline')
    })
    await waitFor(() => expect(h.sessions).toHaveLength(2))
    expect(old.close).toHaveBeenCalledOnce()
    const duringWrite = h.sessions[1]!
    await waitFor(() => expect(h.writes()).toHaveLength(1))
    // Deliver a queued frame directly from the cache notification, before React can replace the effect.
    let deliveredAtCompletion = false
    const unsubscribe = h.cache.getQueryCache().subscribe((event) => {
      if (
        event.query.queryKey[0] !== 'service-operation' ||
        deliveredAtCompletion ||
        h.cache.getQueryData<{ phase: string }>(['service-operation'])?.phase !== 'uncertain'
      )
        return
      deliveredAtCompletion = true
      duringWrite.consume(frame('status', status('ready')))
    })
    await act(async () =>
      h.writes()[0]!.result.resolve(response({ action: 'start', status: status('ready') })),
    )
    unsubscribe()
    expect(deliveredAtCompletion).toBe(true)
    await waitFor(() => expect(duringWrite.close).toHaveBeenCalledOnce())
    expect(h.cache.getQueryData(['service'])).toEqual(status('stopped'))
    await waitFor(() => expect(h.reads()).toHaveLength(1))
    expect(h.sessions).toHaveLength(2)
    act(() => {
      old.consume(frame('status', status('ready')))
      duringWrite.consume(frame('status', status('ready')))
    })
    expect(h.sessions).toHaveLength(2)
    expect(h.cache.getQueryData(['service'])).toEqual(status('stopped'))
    expect(h.reads()[0]!.aborted).not.toHaveBeenCalled()
    await act(async () => h.reads()[0]!.result.reject(new TypeError('Verification disconnected')))
    await h.settled()
    await waitFor(() => expect(h.sessions).toHaveLength(3))
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
    act(() => {
      old.consume(frame('status', status('ready')))
      duringWrite.consume(frame('status', status('ready')))
    })
    expect(h.cache.getQueryData(['service'])).toEqual(status('stopped'))
    expect(hook.result.current.isUncertain).toBe(true)
    act(() => h.sessions[2]!.consume(frame('status', status('ready'))))
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
    expect(h.cache.getQueryData(['service'])).toEqual(status('ready'))
    expect(h.writes()).toHaveLength(1)
  })

  it.each(['start', 'stop'] as const)(
    'opens the post-%s stream only after the authoritative GET and ignores late old-stream status afterward',
    async (action) => {
      const h = setup(opposite(action))
      const hook = h.mount()
      h.events()
      h.complete(status(opposite(action)))
      await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('live'))
      expect(h.reads()).toHaveLength(0)
      const original = h.sessions[0]!
      act(() => hook.result.current.mutate(action))
      await waitFor(() => expect(h.sessions).toHaveLength(2))
      const duringWrite = h.sessions[1]!
      h.complete(status(opposite(action)), duringWrite)
      expect(h.reads()).toHaveLength(0)
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () =>
        h.writes()[0]!.result.resolve(response({ action, status: status(target(action)) })),
      )
      await waitFor(() => expect(duringWrite.close).toHaveBeenCalledOnce())
      await waitFor(() => expect(h.reads()).toHaveLength(1))
      expect(h.sessions).toHaveLength(2)
      expect(hook.result.current.isUncertain).toBe(true)
      act(() => duringWrite.consume(frame('status', status(target(action)))))
      expect(h.cache.getQueryData(['service'])).toEqual(status(opposite(action)))
      expect(h.reads()[0]!.aborted).not.toHaveBeenCalled()
      expect(h.sessions).toHaveLength(2)
      const authoritative = { ...status(target(action)), memory_bytes: 202 }
      await h.read(0, authoritative)
      await h.settled()
      await waitFor(() => expect(h.sessions).toHaveLength(3))
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
      expect(h.cache.getQueryData(['service'])).toEqual(authoritative)
      act(() => {
        original.consume(frame('status', status(opposite(action))))
        duringWrite.consume(frame('status', status('failed')))
        duringWrite.fail(new ApiError('api.unauthorized', 'Late old error', 401))
      })
      expect(h.cache.getQueryData(['service'])).toEqual(authoritative)
      expect(h.cache.getQueryData(['events-connection'])).not.toBe('offline')
      const current = { ...authoritative, memory_bytes: 303 }
      h.complete(current)
      await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('live'))
      expect(h.cache.getQueryData(['service'])).toEqual(current)
      expect(h.reads()).toHaveLength(1)
      expect(h.writes()).toHaveLength(1)
      expect(h.sessions).toHaveLength(3)
    },
  )

  it.each(['complete', 'timeout'] as const)(
    'gives SSE a full 30-second snapshot window after a 29-second pre-GET (%s)',
    async (finish) => {
      const h = setup()
      const hook = h.mount()
      act(() => hook.result.current.mutate('start'))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () => h.writes()[0]!.result.reject(new TypeError('Write outcome unknown')))
      await h.read(0, status('stopped'))
      await h.settled()
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
      vi.useFakeTimers()
      h.events()
      expect(h.reads()).toHaveLength(2)
      expect(h.sessions).toHaveLength(0)
      await act(async () => vi.advanceTimersByTimeAsync(29_000))
      expect(h.sessions).toHaveLength(0)
      expect(h.reads()[1]!.aborted).not.toHaveBeenCalled()
      await act(async () => h.reads()[1]!.result.resolve(response(status('stopped'))))
      expect(h.sessions).toHaveLength(1)
      const session = h.sessions[0]!
      await act(async () => vi.advanceTimersByTimeAsync(29_999))
      expect(session.close).not.toHaveBeenCalled()
      expect(h.sessions).toHaveLength(1)
      expect(h.reads()).toHaveLength(2)
      expect(h.cache.getQueryData(['events-connection'])).toBe('connecting')
      if (finish === 'complete') h.complete(status('stopped'), session)
      await act(async () => vi.advanceTimersByTimeAsync(1))
      if (finish === 'complete') {
        expect(h.cache.getQueryData(['events-connection'])).toBe('live')
        expect(session.close).not.toHaveBeenCalled()
        await act(async () => vi.advanceTimersByTimeAsync(30_000))
        expect(session.close).not.toHaveBeenCalled()
      } else {
        expect(session.close).toHaveBeenCalledOnce()
        expect(h.cache.getQueryData(['events-connection'])).toBe('reconnecting')
      }
      expect(h.sessions).toHaveLength(1)
      expect(h.reads()).toHaveLength(2)
      expect(h.writes()).toHaveLength(1)
    },
  )

  it.each(['settled', 'uncertain'] as const)(
    'orders initial opening, manual verify, and reconnect after GET for an existing %s operation',
    async (phase) => {
      const h = setup()
      const hook = h.mount()
      act(() => hook.result.current.mutate('start'))
      await waitFor(() => expect(h.writes()).toHaveLength(1))
      await act(async () => {
        if (phase === 'settled')
          h.writes()[0]!.result.resolve(response({ action: 'start', status: status('ready') }))
        else h.writes()[0]!.result.reject(new TypeError('Write outcome unknown'))
      })
      const observed = status(phase === 'settled' ? 'ready' : 'stopped')
      await h.read(0, observed)
      await h.settled()
      await waitFor(() => expect(hook.result.current.isUncertain).toBe(phase === 'uncertain'))
      h.events()
      await waitFor(() => expect(h.reads()).toHaveLength(2))
      expect(h.sessions).toHaveLength(0)
      await h.read(1, observed)
      await waitFor(() => expect(h.sessions).toHaveLength(1))
      h.complete(observed)
      await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('live'))
      expect(h.reads()).toHaveLength(2)
      const old = h.sessions[0]!
      let verification!: Promise<void>
      act(() => {
        verification = hook.result.current.verify()
        old.consume(frame('status', { ...status('ready'), memory_bytes: 999 }))
        expect(h.cache.getQueryData(['service'])).toEqual(observed)
      })
      await waitFor(() => expect(old.close).toHaveBeenCalledOnce())
      await waitFor(() => expect(h.reads()).toHaveLength(3))
      expect(h.sessions).toHaveLength(1)
      expect(hook.result.current.isUncertain).toBe(phase === 'uncertain')
      const authoritative = { ...status('ready'), memory_bytes: 202 }
      await h.read(2, authoritative)
      await act(async () => verification)
      await waitFor(() => expect(h.sessions).toHaveLength(2))
      act(() => old.consume(frame('status', status('stopped'))))
      expect(h.cache.getQueryData(['service'])).toEqual(authoritative)
      h.complete(authoritative)
      await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('live'))
      expect(h.reads()).toHaveLength(3)
      const disconnected = h.sessions[1]!
      act(() => disconnected.fail(new TypeError('Stream disconnected')))
      fireEvent.click(screen.getByLabelText(/^Management connection:/))
      fireEvent.click(await screen.findByRole('button', { name: 'Reconnect' }))
      await waitFor(() => expect(h.reads()).toHaveLength(4))
      expect(h.sessions).toHaveLength(2)
      act(() => disconnected.consume(frame('status', status('stopped'))))
      expect(h.cache.getQueryData(['service'])).toEqual(authoritative)
      expect(h.reads()[3]!.aborted).not.toHaveBeenCalled()
      await h.read(3, authoritative)
      await waitFor(() => expect(h.sessions).toHaveLength(3))
      h.complete(authoritative)
      await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('live'))
      expect(h.reads()).toHaveLength(4)
      expect(h.writes()).toHaveLength(1)
    },
  )
})

it('allows native start/stop capabilities without enabling mode, selection, or configuration writes', async () => {
  const fixtures = createFixtures()
  let service = status('stopped')
  const request = vi.fn<ApiTransport['request']>(async ({ method, path }) => {
    if (method === 'POST' && (path === '/service/start' || path === '/service/stop')) {
      const action = path === '/service/start' ? 'start' : 'stop'
      service = status(target(action))
      return response({ action, status: service })
    }
    if (method === 'GET' && path === '/service') return response(service)
    if (method === 'GET' && path === '/info')
      return response({
        api_version: 1,
        version: 'fixture',
        version_code: '1',
        instance_id: 'fixture',
        pid: 1,
        platform: 'android',
        capabilities: ['events'],
        memory: {
          heap_alloc_bytes: 1,
          heap_inuse_bytes: 1,
          heap_released_bytes: 0,
          stack_inuse_bytes: 1,
        },
      })
    throw new Error(`Unexpected request: ${method} ${path}`)
  })
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: ['info.read', 'service.read', 'service.start', 'service.stop'],
    request,
    subscribe: (_topics, consume) => {
      consume(frame('status', service))
      consume(frame('groups', fixtures.policy))
      consume(
        frame(
          'subscriptions',
          fixtures.snapshot.groups.map(({ group }) => group),
        ),
      )
      return () => {}
    },
  }
  const root = createRootRoute({
    component: () => (
      <>
        <ModuleEvents />
        <Outlet />
      </>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([
      createRoute({ getParentRoute: () => root, path: '/', component: DashboardPage }),
      createRoute({ getParentRoute: () => root, path: '/nodes', component: NodesPage }),
      createRoute({ getParentRoute: () => root, path: '/settings', component: SettingsPage }),
    ]),
    history: createMemoryHistory(),
  })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale: 'en' }))
  render(
    <AppProviders api={new ApiClient(transport)}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  const start = await screen.findByRole('button', { name: 'Start service' })
  await waitFor(() => expect(start).toBeEnabled())
  expect(screen.getByRole('combobox', { name: 'Outbound mode' })).toBeDisabled()
  fireEvent.click(start)
  const stop = await screen.findByRole('button', { name: 'Stop service' })
  await waitFor(() => expect(stop).toBeEnabled())
  fireEvent.click(stop)
  const dialog = await screen.findByRole('dialog')
  expect(request.mock.calls.filter(([req]) => req.method === 'POST')).toHaveLength(1)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Start service' })).toBeEnabled())
  expect(screen.getByRole('combobox', { name: 'Outbound mode' })).toBeDisabled()
  await act(async () => router.navigate({ to: '/nodes' }))
  expect(await screen.findByRole('button', { name: 'Select Tokyo 01' })).toBeDisabled()
  for (const button of screen.getAllByRole('button', { name: /^Select / }))
    expect(button).toBeDisabled()
  await act(async () => router.navigate({ to: '/settings' }))
  expect(await screen.findByRole('heading', { name: 'General' })).toBeVisible()
  expect(screen.queryByRole('switch', { name: 'Start on boot' })).not.toBeInTheDocument()
  expect(request.mock.calls.filter(([req]) => req.method !== 'GET').map(([req]) => req)).toEqual([
    { method: 'POST', path: '/service/start' },
    { method: 'POST', path: '/service/stop' },
  ])
  expect(
    request.mock.calls.every(([req]) =>
      ['/info', '/service', '/service/start', '/service/stop'].includes(req.path),
    ),
  ).toBe(true)
})
