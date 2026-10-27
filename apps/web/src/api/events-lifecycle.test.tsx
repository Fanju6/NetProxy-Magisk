import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi, type Mock } from 'vitest'
import { ApiContext, PreferencesContext } from '@/app/context'
import { translator } from '@/app/i18n'
import { createFixtures, demoTraffic } from '@/mocks/fixtures'
import { ApiClient } from './client'
import {
  ApiError,
  apiOperations,
  type ApiEvent,
  type ApiTransport,
  type EventTopic,
} from './contracts'
import { ModuleEvents } from './events'
import { DashboardPage } from '@/features/dashboard'

type Session = {
  consume: (event: ApiEvent) => void
  fail: (error: Error) => void
  close: Mock<() => void>
}
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const frame = (name: ApiEvent['name'], data: unknown): ApiEvent => ({
  name,
  body: { schema: 1, ok: true, code: 'fixture', message: '', data },
})

function reconnect() {
  act(() => screen.getByLabelText(/^Management connection:/).click())
  act(() => screen.getByRole('button', { name: 'Reconnect' }).click())
}

it('stages policy frames before status and rejects old-core runtime data', () => {
  const h = setup()
  const { service, policy } = createFixtures()
  h.emit('groups', policy)
  expect(h.cache.getQueryData(['policy-groups'])).toBeUndefined()
  h.emit('status', service)
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(policy)
  const next = { ...policy, core_started_at_millis: policy.core_started_at_millis + 1000 }
  h.emit('groups', next)
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(policy)
  h.emit('status', { ...service, core_started_at_millis: next.core_started_at_millis })
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(next)
  h.emit('groups', policy)
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(next)
})

it('does not use Catalog membership to filter or manufacture policy groups', () => {
  const h = setup()
  const fixture = createFixtures()
  h.cache.setQueryData(['subscriptions'], [])
  h.emit('status', fixture.service)
  h.emit('groups', fixture.policy)
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(fixture.policy)
  h.emit('subscriptions', [])
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(fixture.policy)
  expect(h.cache.getQueryData(['selection'])).toBeUndefined()
})

it('keeps actual status and traffic during selection, fences old groups, and hands over once', async () => {
  const h = setup()
  const fixture = createFixtures()
  h.complete()
  h.emit('traffic', demoTraffic(fixture.service))
  const old = h.sessions[0]!
  const target = {
    kind: 'policy' as const,
    value: { group_tag: 'Internet', member_tag: 'Hong Kong 01' },
  }
  const operation = {
    epoch: 1,
    eventEpoch: 0,
    target,
    phase: 'pending',
    deadline: Date.now() + 150000,
    responseReceived: false,
    error: null,
  }
  act(() => h.cache.setQueryData(['routing-operation'], operation))
  expect(h.sessions).toHaveLength(1)
  const selected = structuredClone(fixture.policy)
  selected.groups[0]!.selected = target.value.member_tag
  h.emit('groups', selected)
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(fixture.policy)
  h.emit('traffic', demoTraffic(fixture.service))
  expect(h.cache.getQueryData(['traffic-history'])).toHaveLength(2)
  expect(h.cache.getQueryData(['events-connection'])).toBe('live')
  vi.mocked(h.transport.request).mockImplementation(async (request) => ({
    status: 200,
    body: {
      schema: 1,
      ok: true,
      code: 'fixture',
      message: '',
      data:
        request.path === '/service'
          ? fixture.service
          : request.path === '/mode'
            ? {
                mode: fixture.service.configured_outbound_mode,
                available: fixture.service.available_outbound_modes,
              }
            : selected,
    },
  }))
  act(() => {
    h.cache.setQueryData(['policy-groups'], selected)
    h.cache.setQueryData(['routing-operation'], {
      ...operation,
      epoch: 2,
      eventEpoch: 2,
      phase: 'idle',
      responseReceived: true,
    })
  })
  await waitFor(() => expect(h.sessions).toHaveLength(2))
  expect(old.close).toHaveBeenCalledOnce()
  expect(h.cache.getQueryData(['events-connection'])).toBe('refreshing')
  expect(h.cache.getQueryData(['traffic-history'])).toHaveLength(2)
  h.emit('groups', fixture.policy, old)
  h.emit('status', { ...fixture.service, state: 'stopped' }, old)
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(selected)
  h.complete()
  await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('live'))
  expect(h.cache.getQueryData(['policy-groups'])).toEqual(selected)
  expect(h.cache.getQueryData(['traffic-history'])).toHaveLength(2)
  act(() => h.sessions[1]!.fail(new Error('real disconnect')))
  expect(h.cache.getQueryData(['traffic-history'])).toEqual([])
  expect(h.cache.getQueryData(['events-connection'])).toBe('reconnecting')
  h.view.unmount()
})

function setup(
  onSubscribe?: (session: Session, index: number) => void,
  { dashboard = false, strict = false } = {},
  observeVisibility?: ApiTransport['observeVisibility'],
) {
  const sessions: Session[] = []
  const active = new Set<Session>()
  const transport: ApiTransport = {
    kind: 'mock',
    capabilities: apiOperations,
    request: vi.fn(),
    subscribe: (_topics: readonly EventTopic[], consume, fail) => {
      const session: Session = {
        consume,
        fail,
        close: vi.fn(() => {
          active.delete(session)
        }),
      }
      sessions.push(session)
      active.add(session)
      onSubscribe?.(session, sessions.length - 1)
      return session.close
    },
    ...(observeVisibility ? { observeVisibility } : {}),
  }
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity, staleTime: Infinity } },
  })
  const router = dashboard
    ? createRouter({
        routeTree: createRootRoute({ component: DashboardPage }),
        history: createMemoryHistory(),
      })
    : undefined
  if (dashboard) {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    cache.setQueryData(['service'], createFixtures().service)
    cache.setQueryData(['info'], { version: 'v8.6.1' })
  }
  const tree = (
    <PreferencesContext
      value={{
        preferences: { theme: 'light', locale: 'en' },
        t: translator('en'),
        update: vi.fn(),
      }}
    >
      <ApiContext value={new ApiClient(transport)}>
        <QueryClientProvider client={cache}>
          <ModuleEvents />
          {router && <RouterProvider router={router} />}
        </QueryClientProvider>
      </ApiContext>
    </PreferencesContext>
  )
  const view = render(strict ? <StrictMode>{tree}</StrictMode> : tree)
  const emit = (name: ApiEvent['name'], data: unknown, session = sessions.at(-1)!) => {
    act(() => session.consume(frame(name, data)))
  }
  const complete = (service = createFixtures().service) => {
    const { snapshot, policy } = createFixtures()
    emit('status', service)
    emit('groups', {
      ...policy,
      live: service.state === 'ready',
      core_started_at_millis: service.core_started_at_millis,
    })
    emit(
      'subscriptions',
      snapshot.groups.map((item) => item.group),
    )
  }
  return { cache, sessions, active, transport, emit, complete, view }
}

it('waits for asynchronous native visibility during handover without pretending the app went background', async () => {
  const listeners: ((visible: boolean) => void)[] = []
  const h = setup(undefined, {}, (listener) => {
    listeners.push(listener)
    return vi.fn()
  })
  act(() => listeners[0]!(true))
  h.complete()
  h.emit('traffic', demoTraffic(createFixtures().service))
  act(() =>
    h.cache.setQueryData(['routing-operation'], {
      epoch: 2,
      eventEpoch: 2,
      phase: 'idle',
      target: { kind: 'policy', value: { group_tag: 'Internet', member_tag: 'Hong Kong 01' } },
      deadline: Date.now() + 150000,
      responseReceived: true,
      error: null,
    }),
  )
  await waitFor(() => expect(listeners).toHaveLength(2))
  expect(h.cache.getQueryData(['events-connection'])).toBe('refreshing')
  expect(h.cache.getQueryData(['traffic-history'])).toHaveLength(1)
  expect(h.cache.getQueryData(['policy-groups'])).toMatchObject({ live: true })
  expect(h.sessions).toHaveLength(1)
  act(() => listeners[1]!(false))
  expect(h.cache.getQueryData(['events-connection'])).toBe('paused')
  expect(h.cache.getQueryData(['traffic-history'])).toEqual([])
  act(() => listeners[1]!(true))
  expect(h.cache.getQueryData(['events-connection'])).toBe('connecting')
  h.view.unmount()
})

it('clears traffic on disconnect and rejects old-session frames after reconnect', async () => {
  const { service } = createFixtures()
  const { cache, sessions, emit } = setup()
  emit('traffic', demoTraffic(service))
  expect(cache.getQueryData(['traffic'])).toEqual({ available: false })
  emit('status', service)
  emit('traffic', demoTraffic(service))
  expect(cache.getQueryData(['traffic-history'])).toHaveLength(1)
  const old = sessions[0]!
  act(() => old.fail(new Error('disconnected')))
  expect(cache.getQueryData(['traffic'])).toEqual({ available: false })
  expect(cache.getQueryData(['traffic-history'])).toEqual([])
  expect(await screen.findByRole('status')).toBeVisible()
  reconnect()
  expect(sessions).toHaveLength(2)
  emit('status', service, old)
  emit('traffic', demoTraffic(service))
  expect(cache.getQueryData(['traffic'])).toEqual({ available: false })
  emit('status', service)
  emit('traffic', demoTraffic(service))
  expect(cache.getQueryData(['traffic-history'])).toHaveLength(1)
})

it('keeps a new event snapshot when an earlier HTTP read resolves later', async () => {
  const { service } = createFixtures()
  const { cache, emit } = setup()
  let finish!: (value: unknown) => void
  const pending = cache
    .fetchQuery({
      queryKey: ['service'],
      queryFn: () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    })
    .catch(() => undefined)
  expect(finish).toBeTypeOf('function')
  emit('status', service)
  finish({ ...service, state: 'stopped' })
  await pending
  expect(cache.getQueryData(['service'])).toEqual(service)
})

it('discards the initial resource replay after reconnect while accepting newer subsequent summaries', () => {
  const h = setup()
  const fresh = createFixtures().snapshot.groups.map(({ group }) => ({ ...group, revision: 5 }))
  h.cache.setQueryData(['subscriptions'], fresh)
  h.complete()
  expect(h.cache.getQueryData(['subscriptions'])).toEqual(fresh)
  act(() => h.sessions[0]!.fail(new Error('offline')))
  reconnect()
  h.complete()
  expect(h.cache.getQueryData(['subscriptions'])).toEqual(fresh)
  const latest = fresh.map((group) => ({ ...group, revision: 6 }))
  h.emit('subscriptions', latest)
  expect(h.cache.getQueryData(['subscriptions'])).toEqual(latest)
  h.emit('subscriptions', fresh, h.sessions[0]!)
  expect(h.cache.getQueryData(['subscriptions'])).toEqual(latest)
})

it('clears traffic and history when ready core A changes directly to ready core B', () => {
  const { cache, emit, complete } = setup()
  const { service: coreA } = createFixtures()
  complete(coreA)
  emit('traffic', demoTraffic(coreA))
  emit('traffic', demoTraffic(coreA, 1))
  expect(cache.getQueryData(['traffic-history'])).toHaveLength(2)
  emit('status', coreA)
  expect(cache.getQueryData(['traffic-history'])).toHaveLength(2)
  const coreB = { ...coreA, core_started_at_millis: coreA.core_started_at_millis + 1000 }
  emit('status', coreB)
  expect(cache.getQueryData(['traffic'])).toEqual({ available: false })
  expect(cache.getQueryData(['traffic-history'])).toEqual([])
  emit('traffic', demoTraffic(coreA))
  expect(cache.getQueryData(['traffic-history'])).toEqual([])
  emit('traffic', demoTraffic(coreB))
  expect(cache.getQueryData(['traffic-history'])).toHaveLength(1)
})

it('accepts synchronous complete snapshots and owns their disposer until unmount', () => {
  const { service, snapshot, policy } = createFixtures()
  const { cache, active, sessions, view } = setup((session) => {
    session.consume(frame('status', service))
    session.consume(frame('groups', policy))
    session.consume(
      frame(
        'subscriptions',
        snapshot.groups.map((item) => item.group),
      ),
    )
  })
  expect(cache.getQueryData(['events-connection'])).toBe('live')
  expect(active.size).toBe(1)
  expect(sessions[0]!.close).not.toHaveBeenCalled()
  view.unmount()
  expect(active.size).toBe(0)
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
})

it('does not overwrite a nested replacement subscription with the late disposer', () => {
  const { service } = createFixtures()
  const { active, sessions, cache, view } = setup((session, index) => {
    session.consume(frame('status', service))
    if (index !== 0) return
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
  })
  expect(sessions).toHaveLength(2)
  expect(active.size).toBe(1)
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  expect(sessions[1]!.close).not.toHaveBeenCalled()
  act(() => sessions[0]!.consume(frame('status', { ...service, state: 'failed' })))
  expect(cache.getQueryData(['service'])).toMatchObject({ state: 'ready' })
  view.unmount()
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  expect(sessions[1]!.close).toHaveBeenCalledOnce()
  expect(active.size).toBe(0)
})

it.each([400, 401, 403])(
  'does not automatically retry HTTP %s authentication or contract errors',
  (status) => {
    vi.useFakeTimers()
    const { sessions, cache, view } = setup()
    act(() => sessions[0]!.fail(new ApiError('fixture.rejected', '', status)))
    expect(cache.getQueryData(['events-connection'])).toBe('offline')
    act(() => vi.advanceTimersByTime(120_000))
    expect(sessions).toHaveLength(1)
    view.unmount()
  },
)

it('does not retry a synchronous authentication exception or double-release its resources', () => {
  vi.useFakeTimers()
  const { sessions, active, cache, view } = setup((session) => {
    session.close()
    throw new ApiError('api.unauthorized', '', 401)
  })
  expect(cache.getQueryData(['events-connection'])).toBe('offline')
  expect(active.size).toBe(0)
  act(() => vi.advanceTimersByTime(120_000))
  expect(sessions).toHaveLength(1)
  view.unmount()
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
})

it('resets the retry budget only after a stable complete-snapshot session', () => {
  vi.useFakeTimers()
  const { sessions, complete, view } = setup()
  act(() => sessions[0]!.fail(new Error('network')))
  act(() => vi.advanceTimersByTime(1000))
  complete()
  act(() => vi.advanceTimersByTime(30_000))
  act(() => sessions[1]!.fail(new Error('network again')))
  act(() => vi.advanceTimersByTime(999))
  expect(sessions).toHaveLength(2)
  act(() => vi.advanceTimersByTime(1))
  expect(sessions).toHaveLength(3)
  view.unmount()
})

it.each(['ready', 'stopped'] as const)(
  'shows a non-live %s snapshot without claiming zero speed or changing proxy state',
  async (state) => {
    const { sessions, complete, emit, view } = setup(undefined, { dashboard: true })
    const service = { ...createFixtures().service, state }
    const label = state === 'ready' ? 'Running' : 'Stopped'
    complete(service)
    emit('traffic', demoTraffic(service))
    expect(await screen.findByRole('heading', { name: label })).toBeVisible()
    expect(screen.queryByText('Last snapshot · not live')).not.toBeInTheDocument()
    if (state === 'ready') expect(await screen.findByText('2.1 MB/s')).toBeVisible()
    else expect(screen.getAllByText('0 B/s')).toHaveLength(2)
    act(() => sessions[0]!.fail(new ApiError('api.unauthorized', 'fixture', 401)))
    expect(await screen.findByText('Last snapshot · not live')).toBeVisible()
    expect(screen.getByRole('heading', { name: label })).toBeVisible()
    expect(screen.queryByText('2.1 MB/s')).not.toBeInTheDocument()
    expect(screen.getAllByText('—')).toHaveLength(2)
    reconnect()
    emit('status', service)
    expect(screen.getByLabelText(/^Management connection:/)).toBeVisible()
    expect(screen.getByText('Last snapshot · not live')).toBeVisible()
    complete(service)
    await waitFor(() =>
      expect(screen.queryByText('Last snapshot · not live')).not.toBeInTheDocument(),
    )
    view.unmount()
  },
)

it.each(['error', 'invalid frame'] as const)(
  'releases a synchronous %s subscription exactly once',
  (failure) => {
    vi.useFakeTimers()
    const { sessions, active, view } = setup((session) => {
      if (failure === 'error') session.fail(new Error('synchronous error'))
      else session.consume({ name: 'status', body: { schema: 2 } })
    })
    expect(active.size).toBe(0)
    expect(sessions[0]!.close).toHaveBeenCalledOnce()
    view.unmount()
    act(() => vi.advanceTimersByTime(60_000))
    expect(sessions).toHaveLength(1)
    expect(sessions[0]!.close).toHaveBeenCalledOnce()
  },
)

it('releases a subscription when visibility requests disposal before subscribe returns', () => {
  const { sessions, active, view } = setup(() => {
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
  })
  expect(active.size).toBe(0)
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  view.unmount()
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
})

it('does not release twice when the close callback reports another error', () => {
  const { sessions, active, view } = setup((session) => {
    const release = session.close.getMockImplementation()!
    session.close.mockImplementation(() => {
      release()
      session.fail(new Error('closed'))
    })
  })
  act(() => sessions[0]!.fail(new Error('disconnected')))
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  expect(active.size).toBe(0)
  view.unmount()
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
})

it('releases Strict Mode probe subscriptions and rejects their queued callbacks after cleanup', async () => {
  const { service, snapshot, policy } = createFixtures()
  const { sessions, active, cache, view } = setup(
    (session, index) => {
      session.consume(frame('status', service))
      session.consume(frame('groups', policy))
      session.consume(
        frame(
          'subscriptions',
          snapshot.groups.map((item) => item.group),
        ),
      )
      if (index === 0)
        queueMicrotask(() => session.consume(frame('status', { ...service, state: 'failed' })))
    },
    { strict: true },
  )
  await act(async () => {
    await Promise.resolve()
  })
  expect(sessions).toHaveLength(2)
  expect(active.size).toBe(1)
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  expect(sessions[1]!.close).not.toHaveBeenCalled()
  expect(cache.getQueryData(['service'])).toMatchObject({ state: 'ready' })
  expect(cache.getQueryData(['events-connection'])).toBe('live')
  view.unmount()
  expect(active.size).toBe(0)
  expect(sessions[1]!.close).toHaveBeenCalledOnce()
})

it('retries only the event subscription with a finite budget and fresh snapshots', () => {
  vi.useFakeTimers()
  const { sessions, cache, transport, emit, complete, view } = setup()
  complete()
  expect(cache.getQueryData(['events-connection'])).toBe('live')
  for (const [index, delay] of [1000, 2000, 4000].entries()) {
    act(() => sessions.at(-1)!.fail(new Error('network')))
    expect(cache.getQueryData(['events-connection'])).toBe('reconnecting')
    act(() => vi.advanceTimersByTime(delay - 1))
    expect(sessions).toHaveLength(index + 1)
    act(() => vi.advanceTimersByTime(1))
    expect(sessions).toHaveLength(index + 2)
    emit('traffic', demoTraffic(createFixtures().service))
    expect(cache.getQueryData(['traffic'])).toEqual({ available: false })
    complete()
    expect(cache.getQueryData(['events-connection'])).toBe('live')
  }
  act(() => sessions.at(-1)!.fail(new Error('still broken')))
  expect(cache.getQueryData(['events-connection'])).toBe('offline')
  act(() => vi.advanceTimersByTime(120_000))
  expect(sessions).toHaveLength(4)
  expect(transport.request).not.toHaveBeenCalled()
  reconnect()
  expect(sessions).toHaveLength(5)
  emit('status', createFixtures().service)
  expect(cache.getQueryData(['events-connection'])).not.toBe('live')
  complete()
  expect(cache.getQueryData(['events-connection'])).toBe('live')
  view.unmount()
})

it('times out incomplete snapshots and does not treat traffic as connection readiness', () => {
  vi.useFakeTimers()
  const { sessions, cache, emit, view } = setup()
  emit('status', createFixtures().service)
  emit('traffic', demoTraffic(createFixtures().service))
  expect(cache.getQueryData(['events-connection'])).toBe('connecting')
  act(() => vi.advanceTimersByTime(30_000))
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  expect(cache.getQueryData(['events-connection'])).toBe('reconnecting')
  view.unmount()
  act(() => vi.advanceTimersByTime(60_000))
  expect(sessions).toHaveLength(1)
})

it('keeps proxy state independent from the disconnected stream and ignores frames after unmount', async () => {
  const { sessions, cache, complete, emit, view } = setup()
  complete()
  act(() => sessions[0]!.fail(new Error('network')))
  expect(cache.getQueryData(['service'])).toMatchObject({ state: 'ready' })
  expect(await screen.findByRole('status')).toHaveTextContent(/snapshot.*not live/i)
  view.unmount()
  emit('status', { ...createFixtures().service, state: 'failed' })
  expect(cache.getQueryData(['service'])).toMatchObject({ state: 'ready' })
})

it('closes the stream in the background and releases it on unmount', () => {
  const { sessions, view } = setup()
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  expect(sessions).toHaveLength(2)
  view.unmount()
  expect(sessions[1]!.close).toHaveBeenCalledOnce()
  vi.restoreAllMocks()
})

it('marks a hidden connection as paused even when the visibility event has not arrived', () => {
  vi.useFakeTimers()
  const { sessions, cache, complete, view } = setup()
  complete()
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
  act(() => sessions[0]!.fail(new Error('hidden disconnect')))
  expect(cache.getQueryData(['events-connection'])).toBe('paused')
  act(() => vi.advanceTimersByTime(120_000))
  expect(sessions).toHaveLength(1)
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  expect(sessions).toHaveLength(2)
  view.unmount()
})

it('waits for native visibility, combines it with document visibility and ignores duplicate initial callbacks', () => {
  let notify!: (visible: boolean) => void
  const release = vi.fn()
  const { sessions, cache, view, complete, emit } = setup(undefined, {}, (listener) => {
    notify = listener
    return release
  })
  expect(sessions).toHaveLength(0)
  expect(cache.getQueryData(['events-connection'])).toBe('paused')
  act(() => {
    notify(true)
    notify(true)
  })
  expect(sessions).toHaveLength(1)
  complete()
  emit('traffic', demoTraffic(createFixtures().service))
  act(() => notify(false))
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  expect(cache.getQueryData(['traffic-history'])).toEqual([])
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
  act(() => {
    document.dispatchEvent(new Event('visibilitychange'))
    notify(true)
  })
  expect(sessions).toHaveLength(1)
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  expect(sessions).toHaveLength(2)
  expect(cache.getQueryData(['events-connection'])).toBe('connecting')
  emit('traffic', demoTraffic(createFixtures().service))
  expect(cache.getQueryData(['traffic-history'])).toEqual([])
  complete()
  expect(cache.getQueryData(['events-connection'])).toBe('live')
  view.unmount()
  expect(release).toHaveBeenCalledOnce()
  act(() => notify(true))
  expect(sessions).toHaveLength(2)
})

it('does not open twice when native visibility initializes synchronously', () => {
  const release = vi.fn()
  const { sessions, view } = setup(undefined, { strict: true }, (listener) => {
    listener(true)
    return release
  })
  expect(sessions).toHaveLength(2)
  expect(sessions[0]!.close).toHaveBeenCalledOnce()
  expect(sessions[1]!.close).not.toHaveBeenCalled()
  view.unmount()
  expect(release).toHaveBeenCalledTimes(2)
  expect(sessions[1]!.close).toHaveBeenCalledOnce()
})
