import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useQueryClient } from '@tanstack/react-query'
import { expect, it } from 'vitest'
import { AppProviders } from './providers'
import { ApiClient } from '@/api/client'
import { MockTransport } from '@/mocks/transport'
import { subscriptionOperationKey, type SubscriptionOperation } from '@/api/subscription-control'

function Controls() {
  const cache = useQueryClient()
  const set = (intent: number, pending = false) =>
    cache.setQueryData<SubscriptionOperation>(subscriptionOperationKey, {
      epoch: intent + 1,
      intent,
      action: 'update',
      groupId: 'demo-global',
      phase: 'idle',
      deadline: Date.now() + 1000,
      error: null,
      cancel: 'idle',
      result: { group_id: 'demo-global', runtime_sync_pending: pending },
    })
  return (
    <>
      <button onClick={() => set(1)}>complete</button>
      <button onClick={() => set(1)}>same result</button>
      <button onClick={() => set(2, true)}>runtime pending</button>
      <button onClick={() => set(3)}>next result</button>
    </>
  )
}
it('announces each successful intent once, even after dismissal, and never calls pending runtime success', async () => {
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ locale: 'en', theme: 'light' }))
  render(
    <AppProviders api={new ApiClient(new MockTransport(0))}>
      <Controls />
    </AppProviders>,
  )
  fireEvent.click(screen.getByText('complete'))
  expect(await screen.findByText('Subscription updated')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Close' }))
  await waitFor(() => expect(screen.queryByText('Subscription updated')).not.toBeInTheDocument())
  fireEvent.click(screen.getByText('same result'))
  expect(screen.queryByText('Subscription updated')).not.toBeInTheDocument()
  fireEvent.click(screen.getByText('runtime pending'))
  expect(screen.queryByText('Subscription updated')).not.toBeInTheDocument()
  fireEvent.click(screen.getByText('next result'))
  expect(await screen.findByText('Subscription updated')).toBeVisible()
})
