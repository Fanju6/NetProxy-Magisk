import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import { type ApiTransport } from '@/api/contracts'
import { AppProviders } from '@/app/providers'
import { MockTransport } from '@/mocks/transport'
import { ConfigurationEditor } from './configuration'

vi.mock('@/components/json-editor', () => ({
  JsonEditor: ({
    value,
    onChange,
    label,
  }: {
    value: string
    onChange: (value: string) => void
    label: string
  }) => (
    <textarea aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} />
  ),
}))

it('preserves a conflicting private draft until explicit discard without caching it', async () => {
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale: 'en' }))
  const backend = new MockTransport(0)
  const request = vi.fn<ApiTransport['request']>((input, signal) =>
    input.method === 'PUT'
      ? Promise.resolve({
          status: 409,
          body: {
            schema: 1,
            ok: false,
            code: 'config.conflict',
            message: 'Conflicting change',
            data: {},
          },
        })
      : backend.request(input, signal),
  )
  const transport: ApiTransport = {
    kind: 'mock',
    capabilities: backend.capabilities,
    request,
    subscribe: () => () => {},
  }
  let cache: QueryClient | undefined
  function Probe() {
    cache = useQueryClient()
    return null
  }
  render(
    <AppProviders api={new ApiClient(transport)}>
      <Probe />
      <ConfigurationEditor target="module" title="Module" onClose={() => {}} />
    </AppProviders>,
  )
  const editor = await screen.findByRole('textbox', { name: 'Module JSON' })
  fireEvent.change(editor, { target: { value: '{"private_fixture":"never-cache-this"}' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText(/This partition changed elsewhere/)
  expect(editor).toHaveValue('{"private_fixture":"never-cache-this"}')
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(request.mock.calls.filter(([input]) => input.method === 'GET')).toHaveLength(1)
  expect(
    JSON.stringify(
      cache!
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data),
    ),
  ).not.toContain('never-cache-this')
  expect(cache!.getMutationCache().getAll()).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Discard draft and reload' }))
  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'Module JSON' })).not.toHaveValue(
      '{"private_fixture":"never-cache-this"}',
    ),
  )
  expect(request.mock.calls.filter(([input]) => input.method === 'PUT')).toHaveLength(1)
})
