import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import type { ApiTransport } from '@/api/contracts'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import { MockTransport } from '@/mocks/transport'
import { NativeInboundForm } from './native-inbound-form'
import { OperationsPage } from './operations'

vi.mock('@/components/json-editor', () => ({
  JsonEditor: ({ value, label }: { value: string; label: string }) => (
    <textarea aria-label={label} value={value} readOnly />
  ),
}))

function fixture(override?: ApiTransport['request']) {
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale: 'en' }))
  const backend = new MockTransport(0)
  const request = vi.fn<ApiTransport['request']>(
    override ?? ((input, signal) => backend.request(input, signal)),
  )
  const api = new ApiClient({
    kind: 'mock',
    capabilities: backend.capabilities,
    request,
    subscribe: () => () => {},
  })
  return { api, request }
}

function page(path: string) {
  const data = fixture()
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const router = makeRouter()
  router.update({ history: createMemoryHistory({ initialEntries: [path] }) })
  const view = render(
    <AppProviders api={data.api}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return { ...data, router, view }
}

async function choose(label: string, option: string) {
  const user = userEvent.setup()
  await user.click(await screen.findByRole('combobox', { name: label }))
  await user.click(await screen.findByRole('option', { name: option }))
}

it('groups settings and links directly to each policy and configuration section', async () => {
  page('/settings')
  expect(await screen.findByText('NETPROXY V9')).toBeVisible()
  expect(screen.getAllByRole('heading', { name: 'General' })).toHaveLength(1)
  expect(screen.getByRole('heading', { name: 'Proxy & network' })).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Advanced' })).toBeVisible()
  for (const [name, section] of [
    ['Root processes', 'root'],
    ['Application proxy', 'apps'],
    ['Wi-Fi', 'wifi'],
  ] as const)
    expect(screen.getByRole('link', { name })).toHaveAttribute(
      'href',
      expect.stringContaining(`section=${section}`),
    )
  expect(screen.getByRole('link', { name: 'Routing rules' })).toHaveAttribute(
    'href',
    expect.stringContaining('section=rules'),
  )
  expect(await screen.findByText(/v8\.6\.1.*1088/)).toBeVisible()
})

it.each([
  ['root', 'inbound/root_policy', 'Root processes'],
  ['apps', 'inbound/app', 'Proxy mode'],
  ['wifi', 'module/wifi', 'Network matching'],
])('opens the %s form inline without a second mixed list', async (section, target, label) => {
  const data = page(`/settings/policies?section=${section}`)
  expect(await screen.findByRole('combobox', { name: label })).toBeVisible()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Inbound backend' })).not.toBeInTheDocument()
  expect(
    data.request.mock.calls
      .filter(([input]) => input.path === '/config')
      .map(([input]) => input.query?.target),
  ).toEqual([target])
  await act(() => data.router.navigate({ to: '/settings' }))
  expect(await screen.findByText('NETPROXY V9')).toBeVisible()
})

it('keeps the unsectioned policy entry grouped and usable', async () => {
  page('/settings/policies')
  expect(await screen.findByRole('button', { name: 'Inbound backend' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Network matching' })).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Root processes' }))
  expect(await screen.findByRole('combobox', { name: 'Root processes' })).toBeVisible()
})

it('preserves exact Wi-Fi names and inactive lists when switching the policy off', async () => {
  const data = page('/settings/policies?section=wifi')
  await choose('Network matching', 'Blacklist')
  const name = ' Office, Wi-Fi '
  fireEvent.change(screen.getByRole('textbox', { name: 'Wi-Fi name' }), { target: { value: name } })
  fireEvent.click(screen.getByRole('button', { name: 'Add Wi-Fi name' }))
  expect(
    within(screen.getByRole('list', { name: 'Blacklist' })).getByText(name, {
      normalizer: (value) => value,
    }).textContent,
  ).toBe(name)
  await choose('Network matching', 'Off')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(data.request.mock.calls.some(([input]) => input.method === 'PUT')).toBe(true),
  )
  const write = data.request.mock.calls.find(([input]) => input.method === 'PUT')![0]
  const body = write.body as { revision: string; content: string }
  expect(body.revision).not.toBe('')
  expect(JSON.parse(body.content)).toEqual({
    wifi: {
      enabled: false,
      mode: 'blacklist',
      blacklist: [name],
      whitelist: [],
      proxy_on_non_wifi: true,
    },
  })
  expect(await screen.findByText('Saved')).toBeVisible()
})

it('selects applications by Android user while keeping names and packages visible', async () => {
  const data = page('/settings/policies?section=apps')
  await choose('Proxy mode', 'Whitelist')
  const search = await screen.findByRole('searchbox', { name: 'Search apps or package names' })
  expect(await screen.findByText('Chrome')).toBeVisible()
  expect(screen.getByText('com.android.chrome')).toBeVisible()
  await choose('Android user', 'Work')
  fireEvent.change(search, { target: { value: 'org.example' } })
  fireEvent.click(await screen.findByRole('button', { name: /Work app/ }))
  expect(screen.getByRole('button', { name: /Work app/ })).toHaveAttribute('aria-pressed', 'true')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(data.request.mock.calls.some(([input]) => input.method === 'PUT')).toBe(true),
  )
  const body = data.request.mock.calls.find(([input]) => input.method === 'PUT')![0].body as {
    content: string
  }
  expect(JSON.parse(body.content).app.proxy_apps).toEqual(['10:org.example.work'])
  expect(await screen.findByText('Saved')).toBeVisible()
})

it('keeps advanced TUN preferences untouched when editing the interface', async () => {
  const data = fixture()
  function Form() {
    const [content, setContent] = useState(
      JSON.stringify({
        tun: {
          type: 'tun',
          tag: 'netproxy-in',
          auto_route: true,
          auto_redirect: true,
          address: ['172.19.0.1/30'],
        },
      }),
    )
    return (
      <>
        <NativeInboundForm backend="tun" content={content} change={setContent} />
        <output data-testid="draft">{content}</output>
      </>
    )
  }
  render(
    <AppProviders api={data.api}>
      <Form />
    </AppProviders>,
  )
  const input = screen.getByRole('textbox', { name: 'Interface name' })
  fireEvent.change(input, { target: { value: 'netproxy-test' } })
  fireEvent.blur(input)
  const draft = JSON.parse(screen.getByTestId('draft').textContent!)
  expect(draft.tun.interface_name).toBe('netproxy-test')
  expect(draft.tun.auto_route).toBe(true)
  expect(draft.tun.auto_redirect).toBe(true)
  expect(draft.tun).not.toHaveProperty('mtu')
  expect(draft.tun).not.toHaveProperty('stack')
  expect(draft.tun).not.toHaveProperty('strict_route')
})

const logEntry = {
  timestamp: '2026-10-17 12:00:00',
  level: 'INFO',
  component: 'service',
  event: 'start',
  result: 'success',
  error_code: '-',
  message: 'Ready for traffic',
}

function logsPage(empty = false) {
  const backend = new MockTransport(0)
  const data = fixture(async (input, signal) =>
    input.path === '/logs' && input.method === 'GET'
      ? {
          status: 200,
          body: {
            schema: 1,
            ok: true,
            code: 'logs.show',
            message: '',
            data: {
              kind: input.query?.kind,
              content: 'DO NOT INFER SERVICE ENTRIES',
              entries: empty ? [] : [logEntry],
            },
          },
        }
      : backend.request(input, signal),
  )
  render(
    <AppProviders api={data.api}>
      <OperationsPage />
    </AppProviders>,
  )
  return data
}

async function logMenu() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Log options' }))
  return user
}

it('shows readable service entries and a filtered monospace raw view', async () => {
  logsPage()
  expect(await screen.findByText('Ready for traffic')).toBeVisible()
  expect(screen.getByRole('list')).toBeVisible()
  expect(screen.queryByText('DO NOT INFER SERVICE ENTRIES')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Filter options' }))
  await choose('Log view', 'Raw')
  fireEvent.click(screen.getByRole('button', { name: 'Close' }))
  expect(screen.getByText(/\[INFO\] \[service\]/).tagName).toBe('PRE')
  fireEvent.change(screen.getByRole('searchbox', { name: 'Filter logs' }), {
    target: { value: 'not a match' },
  })
  expect(screen.getByText('No log entries')).toBeVisible()
})

it('does not infer service entries from legacy content when entries is empty', async () => {
  logsPage(true)
  expect(await screen.findByText('No log entries')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Filter options' }))
  await choose('Log view', 'Raw')
  fireEvent.click(screen.getByRole('button', { name: 'Close' }))
  expect(screen.queryByText('DO NOT INFER SERVICE ENTRIES')).not.toBeInTheDocument()
})

it('requires confirmation before clearing logs or restarting and groups diagnostic tools', async () => {
  const data = logsPage()
  await screen.findByText('Ready for traffic')
  const user = await logMenu()
  await user.click(await screen.findByRole('menuitem', { name: 'Clear' }))
  expect(await screen.findByText('Clear the selected log. This cannot be undone.')).toBeVisible()
  expect(data.request.mock.calls.every(([input]) => input.method === 'GET')).toBe(true)
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() =>
    expect(data.request.mock.calls.filter(([input]) => input.method === 'DELETE')).toHaveLength(1),
  )
  await logMenu()
  expect(await screen.findByRole('menuitem', { name: 'Export diagnostics' })).toBeVisible()
  const restart = await screen.findByRole('menuitem', { name: 'Restart' })
  await waitFor(() => expect(restart).not.toHaveAttribute('aria-disabled', 'true'))
  await user.click(restart)
  expect(await screen.findByText('Proxy connections may be interrupted briefly.')).toBeVisible()
  expect(data.request.mock.calls.some(([input]) => input.path === '/service/restart')).toBe(false)
})
