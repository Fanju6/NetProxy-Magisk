import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider } from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import type { ApiEvent, ApiTransport, ServiceStatus } from '@/api/contracts'
import { NativeTransport } from '@/api/native'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import { MockTransport } from '@/mocks/transport'

afterEach(() => vi.restoreAllMocks())

const success = (data: unknown) => ({
  status: 200,
  body: { schema: 1, ok: true, code: 'fixture.current', message: '', data },
})

// 仅替换数据源，保留实际原生能力集合、ApiClient、查询、事件和完整页面。
function nativeFixture(customMode = false, failRuntime = false) {
  const backend = new MockTransport(0)
  let savedMode = 'Office network'
  const runtimeMode = 'Direct'
  const available = ['Office network', 'Direct', 'Travel network']
  const patchStatus = (status: ServiceStatus) =>
    customMode
      ? {
          ...status,
          configured_outbound_mode: savedMode,
          outbound_mode: runtimeMode,
          available_outbound_modes: available,
        }
      : status
  const request = vi.fn<ApiTransport['request']>(async (input, signal) => {
    if (customMode && input.method === 'GET' && input.path === '/mode')
      return success({ mode: savedMode, runtime_mode: runtimeMode, available })
    if (customMode && input.method === 'PUT' && input.path === '/mode') {
      savedMode = (input.body as { mode: string }).mode
      if (failRuntime)
        return {
          status: 502,
          body: {
            schema: 1,
            ok: false,
            code: 'mode.runtime_sync_failed',
            message: 'Saved but not synchronized',
            data: { persisted: true, runtime_synced: false },
          },
        }
      return success({ mode: savedMode, runtime_mode: runtimeMode, available })
    }
    const result = await backend.request(input, signal)
    if (input.path === '/service' && input.method === 'GET')
      return {
        ...result,
        body: { ...result.body, data: patchStatus(result.body.data as ServiceStatus) },
      }
    return result
  })
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: new NativeTransport().capabilities,
    request,
    subscribe: (topics, consume) =>
      backend.subscribe(topics, (event) => {
        if (event.name !== 'status') return consume(event)
        const body = event.body as { data: ServiceStatus }
        const patched: ApiEvent = {
          ...event,
          body: { ...body, data: patchStatus(body.data) },
        }
        consume(patched)
      }),
  }
  const api = new ApiClient(transport)
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale: 'en' }))
  window.location.hash = '#/'
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const router = makeRouter()
  const view = render(
    <AppProviders api={api}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return { request, router, view }
}

it('uses GET mode options and distinguishes the saved default from runtime network policy', async () => {
  const fixture = nativeFixture(true)
  const choice = await screen.findByRole('combobox', { name: 'Outbound mode' })
  await waitFor(() => expect(choice).toBeEnabled())
  expect(choice).toHaveTextContent('Office network')
  expect(screen.getByText('Runtime mode · Direct')).toBeVisible()
  expect(screen.getByText('Network policy may use a different runtime mode.')).toBeVisible()
  expect(
    fixture.request.mock.calls.some(([input]) => input.path === '/mode' && input.method === 'GET'),
  ).toBe(true)
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  const user = userEvent.setup()
  await user.click(choice)
  expect(await screen.findByRole('option', { name: 'Travel network' })).toBeVisible()
  expect(screen.queryByRole('option', { name: 'Rule' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('option', { name: 'Travel network' }))
  await waitFor(() => expect(choice).toHaveTextContent('Travel network'))
  await waitFor(() => expect(choice).toBeEnabled())
  expect(
    fixture.request.mock.calls.filter(
      ([input]) => input.method === 'PUT' && input.path === '/mode',
    ),
  ).toHaveLength(1)
  expect(fixture.request).toHaveBeenCalledWith(
    { method: 'PUT', path: '/mode', body: { mode: 'Travel network' } },
    undefined,
  )
  fixture.view.unmount()
})

it('keeps a persisted mode failure visible after confirming the saved default', async () => {
  const fixture = nativeFixture(true, true)
  const choice = await screen.findByRole('combobox', { name: 'Outbound mode' })
  await waitFor(() => expect(choice).toBeEnabled())
  const user = userEvent.setup()
  await user.click(choice)
  await user.click(await screen.findByRole('option', { name: 'Travel network' }))
  await waitFor(() => expect(choice).toHaveTextContent('Travel network'))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Saved, but runtime application failed',
  )
  expect(screen.getByText('Runtime mode · Direct')).toBeVisible()
  await waitFor(() => expect(choice).toBeEnabled())
  expect(
    fixture.request.mock.calls.filter(
      ([input]) => input.method === 'PUT' && input.path === '/mode',
    ),
  ).toHaveLength(1)
  fixture.view.unmount()
})

it('permits native node and delay controls without enabling subscription or configuration writes', async () => {
  const fixture = nativeFixture()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Stop service' })).toBeEnabled())
  await act(() => fixture.router.navigate({ to: '/nodes' }))
  const manual = await screen.findByRole('button', { name: 'Select Tokyo 01' })
  await waitFor(() => expect(manual).toBeEnabled())
  fireEvent.click(manual)
  await waitFor(() => expect(manual).toHaveAttribute('aria-pressed', 'true'))
  await waitFor(() => expect(manual).toBeEnabled())
  const group = screen.getByTestId('group-demo-global')
  const auto = within(group).getByRole('button', { name: 'Select Auto-Fastest' })
  fireEvent.click(auto)
  await waitFor(() => expect(auto).toHaveAttribute('aria-pressed', 'true'))
  const singleTest = screen.getByRole('button', { name: 'Test latency for Tokyo 01' })
  await waitFor(() => expect(singleTest).toBeEnabled())
  fireEvent.click(singleTest)
  expect(
    await screen.findByText('Latest available results received; some may be cached.'),
  ).toBeVisible()
  expect(within(manual).getByText('32 ms')).toBeVisible()
  expect(singleTest.closest('button')?.parentElement?.closest('button')).toBeNull()
  await act(() => fixture.router.navigate({ to: '/settings' }))
  await screen.findByRole('heading', { name: 'Settings' })
  expect(screen.queryByRole('switch', { name: 'Start on boot' })).not.toBeInTheDocument()
  await act(() => fixture.router.navigate({ to: '/subscriptions' }))
  await screen.findByRole('heading', { name: '本地配置' })
  expect(screen.queryByRole('button', { name: /update|save|delete/i })).not.toBeInTheDocument()
  const writes = fixture.request.mock.calls
    .map(([input]) => input)
    .filter((input) => input.method !== 'GET')
  expect(writes).toEqual([
    { method: 'PUT', path: '/selection', body: { group_id: 'demo-global', node_tag: 'Tokyo 01' } },
    { method: 'PUT', path: '/selection', body: { group_id: 'demo-global', node_tag: '' } },
    {
      method: 'POST',
      path: '/nodes/delay',
      body: { group_id: 'demo-global', node_tag: 'Tokyo 01' },
    },
  ])
  fixture.view.unmount()
})
