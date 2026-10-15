import { act, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { ApiContext, PreferencesContext } from '@/app/context'
import { translator } from '@/app/i18n'
import { createFixtures, demoTraffic } from '@/mocks/fixtures'
import { ApiClient } from './client'
import type { ApiEvent, ApiTransport, EventTopic } from './contracts'
import { ModuleEvents } from './events'

function setup() {
  const sessions: {
    consume: (event: ApiEvent) => void
    fail: (error: Error) => void
    close: ReturnType<typeof vi.fn>
  }[] = []
  const transport: ApiTransport = {
    kind: 'mock',
    request: vi.fn(),
    subscribe: (_topics: readonly EventTopic[], consume, fail) => {
      const session = { consume, fail, close: vi.fn() }
      sessions.push(session)
      return session.close
    },
  }
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
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
        </QueryClientProvider>
      </ApiContext>
    </PreferencesContext>,
  )
  const emit = (name: ApiEvent['name'], data: unknown, session = sessions.at(-1)!) => {
    act(() =>
      session.consume({ name, body: { schema: 1, ok: true, code: 'fixture', message: '', data } }),
    )
  }
  return { cache, sessions, emit, view }
}

it('clears traffic on disconnect and rejects old-session frames after reconnect', () => {
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
  expect(screen.getByRole('alert')).toBeVisible()
  act(() => screen.getByRole('button', { name: 'Reconnect' }).click())
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
  await waitFor(() => expect(finish).toBeTypeOf('function'))
  emit('status', service)
  finish({ ...service, state: 'stopped' })
  await pending
  expect(cache.getQueryData(['service'])).toEqual(service)
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
