import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider } from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import type { ApiTransport } from '@/api/contracts'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import { MockTransport } from '@/mocks/transport'
import { z } from 'zod'

vi.mock('@/components/json-editor', () => ({
  JsonEditor: ({
    value,
    onChange,
    label,
  }: {
    value: string
    onChange?: (value: string) => void
    label?: string
  }) => (
    <textarea
      aria-label={label}
      value={value}
      onChange={(event) => onChange?.(event.target.value)}
    />
  ),
}))
afterEach(() => vi.restoreAllMocks())
async function setup(locale: 'en' | 'zh' = 'en', stopped = false, missingSources = false) {
  const backend = new MockTransport(0)
  if (stopped)
    await new ApiClient(backend).request({ method: 'POST', path: '/service/stop' }, z.unknown())
  const request = vi.fn<ApiTransport['request']>(async (input, signal) => {
    const response = await backend.request(input, signal)
    return missingSources && input.path === '/subscriptions'
      ? {
          status: 200,
          body: { schema: 1, ok: true, code: 'subscription.list', message: '', data: [] },
        }
      : response
  })
  const transport: ApiTransport = {
    kind: 'mock',
    capabilities: backend.capabilities,
    request,
    subscribe: (topics, consume) => backend.subscribe(topics, consume),
  }
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale }))
  window.location.hash = '#/nodes'
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const router = makeRouter()
  const view = render(
    <AppProviders api={new ApiClient(transport)}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  await screen.findByTestId('policy-Internet', {}, { timeout: 10000 })
  return { request, backend, ...view }
}
it('shows configured Provider members while stopped without claiming a live choice or delay', async () => {
  const h = await setup('en', true)
  const manual = within(screen.getByTestId('policy-Internet'))
  expect(manual.getByText(/Configured default/)).toBeVisible()
  const auto = within(screen.getByTestId('policy-Fastest'))
  for (const name of ['Hong Kong 01', 'Tokyo 01']) {
    const button = auto.getByRole('button', { name: `Select ${name}` })
    expect(button).toBeDisabled()
    expect(button).toHaveAttribute('aria-pressed', 'false')
  }
  expect(screen.queryByText('Last snapshot · not live')).not.toBeInTheDocument()
  expect(screen.queryByText(/\d+ ms/)).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Test group Fastest' })).toBeDisabled()
  expect(h.request.mock.calls.every(([request]) => request.method === 'GET')).toBe(true)
  expect(auto.queryByText('Local default')).not.toBeInTheDocument()
})
it('defaults to expanded native groups and selects the exact member tag', async () => {
  const h = await setup()
  expect(await screen.findByRole('heading', { name: 'Proxies' })).toBeVisible()
  for (const grid of screen.getAllByTestId('node-grid')) expect(grid).toHaveClass('grid-cols-1')
  const group = within(screen.getByTestId('policy-Internet'))
  const member = group.getByRole('button', { name: 'Select Hong Kong 01' })
  await waitFor(() => expect(member).toBeEnabled())
  fireEvent.click(member)
  await waitFor(() => expect(member).toHaveAttribute('aria-pressed', 'true'))
  const card = member.closest('[data-slot="node-card"]')!
  await waitFor(() => expect(card).toHaveAttribute('data-selected', 'true'))
  expect(card).toHaveClass('border', 'rounded-lg', 'bg-accent')
  expect(member.querySelector('[data-slot="node-choice"] svg')).toBeInTheDocument()
  const unselected = group.getByRole('button', { name: 'Select direct' })
  expect(unselected.querySelector('[data-slot="node-choice"] svg')).not.toBeInTheDocument()
  expect(
    h.request.mock.calls
      .filter(([input]) => input.method === 'POST' && input.path === '/policy-groups/select')
      .map(([input]) => input.body),
  ).toEqual([{ group_tag: 'Internet', member_tag: 'Hong Kong 01' }])
  expect(screen.queryByText('Global Connect')).not.toBeInTheDocument()
})
it('keeps urltest read-only, resolves nested groups and offers tabs and grid search', async () => {
  await setup()
  expect(
    within(screen.getByTestId('policy-Fastest')).getByRole('button', { name: 'Select Tokyo 01' }),
  ).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Open group Fastest' }))
  expect(screen.getByRole('tab', { name: 'Fastest' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.queryByTestId('policy-Internet')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'View options' }))
  fireEvent.click(await screen.findByRole('menuitemradio', { name: 'Grid view' }))
  expect(screen.getByTestId('node-grid')).toHaveClass('grid-cols-2')
  fireEvent.change(screen.getByRole('searchbox', { name: 'Search proxies' }), {
    target: { value: 'Tokyo' },
  })
  expect(screen.getByRole('button', { name: 'Select Tokyo 01' })).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Select Hong Kong 01' })).not.toBeInTheDocument()
})
it('tests a whole native group and retains seconds-based result timestamps', async () => {
  const h = await setup()
  const test = screen.getByRole('button', { name: 'Test group Fastest' })
  await waitFor(() => expect(test).toBeEnabled())
  const latency = within(screen.getByTestId('policy-Fastest')).getByText('68 ms')
  expect(latency).toHaveAttribute('title', new Date(1792065600 * 1000).toLocaleString())
  fireEvent.click(test)
  await waitFor(() => expect(test).toBeEnabled())
  expect(screen.queryByText(/some may be cached/)).not.toBeInTheDocument()
  expect(document.querySelector('[data-slot="toast"]')).not.toBeInTheDocument()
  expect(
    h.request.mock.calls
      .filter(([input]) => input.path === '/policy-groups/delay')
      .map(([input]) => input.body),
  ).toEqual([{ group_tag: 'Fastest' }])
})

it('leaves unmeasured latency blank without hiding existing results', async () => {
  await setup()
  const direct = within(screen.getByTestId('policy-Internet')).getByRole('button', {
    name: 'Select direct',
  })
  expect(direct).not.toHaveTextContent('Not measured')
  expect(direct.querySelector('.tabular-nums')).toBeEmptyDOMElement()
  expect(within(screen.getByTestId('policy-Fastest')).getByText('68 ms')).toBeVisible()
})

it('keeps delay errors visible as a toast without replacing the group list', async () => {
  const h = await setup()
  const original = h.request.getMockImplementation()!
  h.request.mockImplementation((input, signal) =>
    input.path === '/policy-groups/delay'
      ? Promise.reject(new Error('test-only delay failure'))
      : original(input, signal),
  )
  const test = screen.getByRole('button', { name: 'Test group Fastest' })
  await waitFor(() => expect(test).toBeEnabled())
  fireEvent.click(test)
  await waitFor(() => expect(document.querySelector('[data-slot="toast"]')).toBeInTheDocument())
  await waitFor(() => expect(test).toBeEnabled())
  expect(within(screen.getByTestId('policy-Fastest')).getByText('68 ms')).toBeVisible()
  expect(
    h.request.mock.calls.filter(([input]) => input.path === '/policy-groups/delay'),
  ).toHaveLength(1)
})

it('shows card progress during a single delay request and restores measured values quietly', async () => {
  const h = await setup()
  const original = h.request.getMockImplementation()!
  let finish = () => {}
  const waiting = new Promise<void>((resolve) => {
    finish = resolve
  })
  h.request.mockImplementation(async (input, signal) => {
    if (input.path === '/policy-groups/delay') await waiting
    return original(input, signal)
  })
  const group = screen.getByTestId('policy-Fastest')
  const test = screen.getByRole('button', { name: 'Test group Fastest' })
  await waitFor(() => expect(test).toBeEnabled())
  fireEvent.click(test)
  await waitFor(() =>
    expect(group.querySelector('[data-slot="node-card"] .tabular-nums svg')).toBeInTheDocument(),
  )
  expect(screen.getByRole('button', { name: 'Stop waiting Fastest' })).toBeEnabled()
  await act(async () => finish())
  await waitFor(() => expect(test).toBeEnabled())
  await waitFor(() =>
    expect(
      group.querySelector('[data-slot="node-card"] .tabular-nums svg'),
    ).not.toBeInTheDocument(),
  )
  expect(document.querySelector('[data-slot="toast"]')).not.toBeInTheDocument()
})

it('resolves resource display names by stable Provider identity and follows renames', async () => {
  const h = await setup()
  const auto = within(screen.getByTestId('policy-Fastest'))
  const member = auto.getByRole('button', { name: 'Select Tokyo 01' })
  await waitFor(() => expect(member).toHaveTextContent('Global Connect'))
  expect(member).not.toHaveTextContent('np/demo-global')
  const api = new ApiClient(h.backend)
  await api.request(
    { method: 'PUT', path: '/subscriptions/demo-global', body: { name: 'My renamed source' } },
    z.unknown(),
  )
  await waitFor(() => expect(member).toHaveTextContent('My renamed source'))
  expect(member.querySelector('[title]')).toHaveAttribute('title', 'np/demo-global/Tokyo 01')
  await act(async () => {
    await api.request(
      { method: 'PUT', path: '/subscriptions/demo-empty', body: { name: 'My renamed source' } },
      z.unknown(),
    )
  })
  expect(member).toHaveTextContent('My renamed source')
})

it('keeps Provider member identity when the resource summary is unavailable', async () => {
  await setup('en', false, true)
  const member = within(screen.getByTestId('policy-Fastest')).getByRole('button', {
    name: 'Select Tokyo 01',
  })
  await waitFor(() => expect(member).toHaveTextContent('Source unavailable'))
  expect(member).not.toHaveTextContent('np/demo-global')
  expect(member.querySelector('[title]')).toHaveAttribute('title', 'np/demo-global/Tokyo 01')
})
it('edits only a native outbound object with the partition revision', async () => {
  const h = await setup()
  fireEvent.click(screen.getByRole('button', { name: 'Edit policy group Fastest' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Native configuration' }))
  await screen.findByRole('textbox', { name: 'interval' })
  fireEvent.change(screen.getByRole('textbox', { name: 'interval' }), { target: { value: '5m' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  const writes = h.request.mock.calls.filter(
    ([input]) => input.method === 'PUT' && input.path === '/config',
  )
  expect(writes).toHaveLength(1)
  const request = writes[0]![0]
  expect(request.query).toEqual({ target: 'singbox/outbounds' })
  const body = request.body as { content: string; revision: string }
  expect(body.revision).toHaveLength(64)
  const outbounds = JSON.parse(body.content).outbounds as Record<string, unknown>[]
  expect(outbounds.find((item) => item.tag === 'Fastest')).toMatchObject({
    providers: ['np/demo-global'],
    interval: '5m',
    tolerance: 50,
    url: 'https://example.invalid/generate_204',
  })
  expect(outbounds[0]).toEqual({
    type: 'selector',
    tag: 'Internet',
    outbounds: ['Fastest', 'Hong Kong 01', 'direct'],
    default: 'Fastest',
  })
})
it('confirms deletion and retains a native check failure without rewriting references', async () => {
  const h = await setup()
  fireEvent.click(screen.getByRole('button', { name: 'Edit policy group Fastest' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete policy group' }))
  const remove = await screen.findByRole('button', { name: 'Remove' })
  await waitFor(() => expect(remove).toBeEnabled())
  expect(h.request.mock.calls.filter(([input]) => input.method === 'PUT')).toHaveLength(0)
  fireEvent.click(remove)
  expect(await screen.findByRole('alert')).toHaveTextContent('config.invalid')
  expect(screen.getByRole('dialog')).toBeVisible()
  const config = await new ApiClient(h.backend).request(
    { method: 'GET', path: '/config', query: { target: 'singbox/outbounds' } },
    (await import('@/api/contracts')).configSchema,
  )
  expect(config.content).toContain('"Fastest"')
})
it('keeps Chinese proxy labels', async () => {
  await setup('zh')
  expect(await screen.findByRole('heading', { name: '代理' })).toBeVisible()
  expect(screen.getByRole('button', { name: '新增策略组' })).toBeEnabled()
})

it('creates a native urltest with providers and does not reset its type before a tag is entered', async () => {
  const h = await setup()
  fireEvent.click(screen.getByRole('button', { name: 'Add policy group' }))
  const type = await screen.findByRole('combobox', { name: 'Type' })
  const user = userEvent.setup()
  await user.click(type)
  await user.click(await screen.findByRole('option', { name: /^urltest$/ }))
  expect(await screen.findByRole('textbox', { name: 'URL' })).toBeVisible()
  fireEvent.change(screen.getByRole('textbox', { name: 'Tag' }), {
    target: { value: 'Travel / Auto' },
  })
  fireEvent.change(screen.getByRole('textbox', { name: 'providers' }), {
    target: { value: 'np/demo-global' },
  })
  fireEvent.click(screen.getByRole('switch', { name: 'use_all_providers' }))
  fireEvent.change(screen.getByRole('textbox', { name: 'URL' }), {
    target: { value: 'https://example.invalid/check' },
  })
  fireEvent.change(screen.getByRole('textbox', { name: 'interval' }), { target: { value: '5m' } })
  fireEvent.change(screen.getByRole('spinbutton', { name: 'tolerance' }), {
    target: { value: '100' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByTestId('policy-Travel / Auto')).toBeVisible()
  const write = h.request.mock.calls.find(
    ([request]) => request.path === '/config' && request.method === 'PUT',
  )![0]
  expect(JSON.parse((write.body as { content: string }).content).outbounds.at(-1)).toMatchObject({
    tag: 'Travel / Auto',
    type: 'urltest',
    providers: ['np/demo-global'],
    use_all_providers: true,
    url: 'https://example.invalid/check',
    interval: '5m',
    tolerance: 100,
  })
})

it('keeps the policy draft on a revision conflict and requires explicit discard before reloading', async () => {
  const h = await setup()
  fireEvent.click(screen.getByRole('button', { name: 'Edit policy group Fastest' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Native configuration' }))
  fireEvent.change(await screen.findByRole('textbox', { name: 'interval' }), {
    target: { value: '7m' },
  })
  const api = new ApiClient(h.backend)
  const { configSchema } = await import('@/api/contracts')
  const before = await api.request(
    { method: 'GET', path: '/config', query: { target: 'singbox/outbounds' } },
    configSchema,
  )
  await api.request(
    {
      method: 'PUT',
      path: '/config',
      query: { target: 'singbox/outbounds' },
      body: {
        content: before.content.replace('"interval":"3m"', '"interval":"9m"'),
        revision: before.revision,
      },
    },
    configSchema.pick({ target: true, revision: true }),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('config.conflict')
  expect(screen.getByRole('textbox', { name: 'interval' })).toHaveValue('7m')
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(h.request.mock.calls.filter(([request]) => request.method === 'PUT')).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Discard draft and reload' }))
  await waitFor(() => expect(screen.getByRole('textbox', { name: 'interval' })).toHaveValue('9m'))
})

it('keeps raw target unknown fields, unnamed siblings and large integer literals through form editing', async () => {
  const h = await setup()
  const api = new ApiClient(h.backend)
  const { configSchema } = await import('@/api/contracts')
  const original = await api.request(
    { method: 'GET', path: '/config', query: { target: 'singbox/outbounds' } },
    configSchema,
  )
  const seeded = original.content
    .replace('"tolerance":50', '"tolerance":50,"future_integer":9007199254740993')
    .replace('"outbounds":[', '"outbounds":[{"type":"direct","unknown":18446744073709551615},')
  await api.request(
    {
      method: 'PUT',
      path: '/config',
      query: { target: 'singbox/outbounds' },
      body: { content: seeded, revision: original.revision },
    },
    configSchema.pick({ target: true, revision: true }),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Edit policy group Fastest' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Native configuration' }))
  const input = await screen.findByRole('textbox', { name: 'interval' })
  fireEvent.change(input, { target: { value: '7m' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  const content = (
    h.request.mock.calls
      .filter(([request]) => request.method === 'PUT' && request.path === '/config')
      .at(-1)![0].body as { content: string }
  ).content
  expect(content).toBe(seeded.replace('"interval":"3m"', '"interval":"7m"'))
  expect(
    (
      await api.request(
        { method: 'GET', path: '/config', query: { target: 'singbox/outbounds' } },
        configSchema,
      )
    ).content,
  ).toBe(content)
})
