import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import {
  apiOperations,
  type ApiTransport,
  type PolicySnapshot,
  type ServiceStatus,
} from '@/api/contracts'
import { ApiContext, PreferencesContext } from '@/app/context'
import { HeaderActionsProvider } from '@/app/header-actions'
import { translator } from '@/app/i18n'
import { createFixtures, demoTraffic } from '@/mocks/fixtures'
import { DashboardPage } from './dashboard'

function setup(
  options: {
    service?: Partial<ServiceStatus>
    policy?: Partial<PolicySnapshot>
    connection?: 'live' | 'offline'
    kind?: 'native' | 'mock'
  } = {},
) {
  const fixture = createFixtures()
  const service = { ...fixture.service, ...options.service }
  const policy = { ...fixture.policy, ...options.policy }
  const request = vi.fn<ApiTransport['request']>(async (input) => {
    if (input.method !== 'GET' || input.path !== '/service')
      throw new Error('Unexpected dashboard request')
    return {
      status: 200,
      body: { schema: 1, ok: true, code: 'service.status', message: '', data: service },
    }
  })
  const api = new ApiClient({
    kind: options.kind ?? 'native',
    capabilities: apiOperations,
    request,
    subscribe: () => () => {},
  })
  const cache = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  cache.setQueryData(['service'], service)
  cache.setQueryData(['mode'], {
    mode: service.configured_outbound_mode,
    runtime_mode: service.outbound_mode,
    available: service.available_outbound_modes,
  })
  cache.setQueryData(['policy-groups'], policy)
  cache.setQueryData(['events-connection'], options.connection ?? 'live')
  cache.setQueryData(['traffic'], demoTraffic(fixture.service))
  cache.setQueryData(['traffic-history'], [1024, 2048, 3072])
  function Page() {
    const [target, setTarget] = useState<HTMLElement | null>(null)
    return (
      <HeaderActionsProvider target={target}>
        <header ref={setTarget} />
        <DashboardPage />
      </HeaderActionsProvider>
    )
  }
  const router = createRouter({
    routeTree: createRootRoute({ component: Page }),
    history: createMemoryHistory(),
  })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const view = render(
    <PreferencesContext
      value={{
        preferences: { locale: 'en', theme: 'light' },
        update: () => {},
        t: translator('en'),
      }}
    >
      <ApiContext value={api}>
        <QueryClientProvider client={cache}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ApiContext>
    </PreferencesContext>,
  )
  return { cache, request, service, view }
}

function value(label: string) {
  return screen.getByText(label, { selector: 'dt' }).nextElementSibling!
}

it('keeps only the chart space during a same-core reconnect and drops it for a new core', async () => {
  const h = setup()
  await screen.findByRole('img', { name: 'Live traffic' })
  act(() => {
    h.cache.setQueryData(['events-connection'], 'reconnecting')
    h.cache.setQueryData(['traffic'], { available: false })
    h.cache.setQueryData(['traffic-history'], [])
  })
  expect(await screen.findByText('Waiting for live traffic')).toHaveClass('h-20')
  expect(screen.queryByRole('img', { name: 'Live traffic' })).not.toBeInTheDocument()
  expect(screen.queryByText(/MB\/s/)).not.toBeInTheDocument()
  act(() =>
    h.cache.setQueryData(['service'], {
      ...h.service,
      core_started_at_millis: h.service.core_started_at_millis + 1,
    }),
  )
  await waitFor(() =>
    expect(screen.queryByText('Waiting for live traffic')).not.toBeInTheDocument(),
  )
})

it('orders service and uptime, speeds, SSE chart, statistics, mode, then native outbound details', async () => {
  setup({ kind: 'mock' })
  await screen.findByRole('heading', { name: 'Running' })
  const order = [
    screen.getByRole('heading', { name: 'Running' }),
    screen.getByText(/^Uptime/),
    screen.getByText('Download', { exact: true }),
    screen.getByRole('img', { name: 'Live traffic' }),
    screen.getByText('Total download'),
    screen.getByRole('heading', { name: 'Outbound mode' }),
    screen.getByText('Current node'),
    screen.getByRole('note'),
  ]
  for (let index = 1; index < order.length; index++)
    expect(
      order[index - 1]!.compareDocumentPosition(order[index]!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  expect(value('Default outbound')).toHaveTextContent('Internet')
  expect(value('Selected')).toHaveTextContent('Fastest')
  expect(value('Current node')).toHaveTextContent('Hong Kong 01')
  for (const name of ['Transfer speeds', 'Service statistics', 'Outbound mode', 'Current node'])
    expect(screen.getByRole('region', { name })).toBeVisible()
})

it.each(['stopped', 'preparing', 'starting', 'stopping', 'failed'] as const)(
  'hides uptime and runtime selections for %s even if old cached fields remain',
  async (state) => {
    setup({ service: { state } })
    await screen.findByText('Current node')
    expect(screen.queryByText(/^Uptime/)).not.toBeInTheDocument()
    expect(screen.queryByRole('img', { name: 'Live traffic' })).not.toBeInTheDocument()
    expect(value('Current node')).toHaveTextContent('Not confirmed')
    expect(value('Selected')).toHaveTextContent('Not confirmed')
    expect(value('Default outbound')).toHaveTextContent('Internet')
  },
)

it.each(['offline', 'old-core', 'zero-core'] as const)(
  'does not display runtime node, selection or chart with %s snapshots',
  async (snapshot) => {
    setup(
      snapshot === 'offline'
        ? { connection: 'offline' }
        : { service: { core_started_at_millis: snapshot === 'zero-core' ? 0 : 1792065600001 } },
    )
    await screen.findByText('Current node')
    expect(value('Current node')).toHaveTextContent('Not confirmed')
    expect(value('Selected')).toHaveTextContent('Not confirmed')
    expect(screen.queryByRole('img', { name: 'Live traffic' })).not.toBeInTheDocument()
    expect(screen.queryByText(/MB\/s/)).not.toBeInTheDocument()
  },
)

it('never substitutes a selected group or its type for an unconfirmed final node', async () => {
  setup({ service: { runtime_selected: '' } })
  await screen.findByText('Current node')
  expect(value('Selected')).toHaveTextContent('Fastest')
  expect(value('Current node')).toHaveTextContent('Not confirmed')
})

it('does not invent a default Proxy group or final node for native outbounds without groups', async () => {
  setup({
    service: { default_outbound: 'direct', runtime_selected: '' },
    policy: { default_outbound: 'direct', groups: [] },
  })
  await screen.findByText('Current node')
  expect(value('Default outbound')).toHaveTextContent('direct')
  expect(value('Current node')).toHaveTextContent('Not confirmed')
  expect(screen.queryByText('Selected', { selector: 'dt' })).not.toBeInTheDocument()
})

it('refreshes only service status from the header menu without replacing SSE traffic history', async () => {
  const h = setup()
  fireEvent.click(await screen.findByRole('button', { name: 'Dashboard' }))
  expect(await screen.findByRole('menuitem', { name: 'Logs & diagnostics' })).toHaveAttribute(
    'href',
    '/settings/logs',
  )
  fireEvent.click(screen.getByRole('menuitem', { name: 'Check status' }))
  await waitFor(() => expect(h.request).toHaveBeenCalledTimes(1))
  expect(h.request.mock.calls[0]![0]).toEqual({ method: 'GET', path: '/service' })
  expect(h.cache.getQueryData(['traffic-history'])).toEqual([1024, 2048, 3072])
  await act(async () => h.view.unmount())
})
