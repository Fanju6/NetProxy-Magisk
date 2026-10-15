import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { ApiClient, unavailableTransport } from '@/api/client'
import { ApiError } from '@/api/contracts'
import { AppProviders } from '@/app/providers'
import { EmptyState, ErrorState, LoadingState } from './states'

it('shows persisted and conflict outcomes without leaking backend exception text', () => {
  render(
    <AppProviders api={new ApiClient(unavailableTransport)}>
      <ErrorState
        error={
          new ApiError('node.runtime_sync_failed', 'secret=private-fixture', 502, {
            persisted: true,
          })
        }
      />
      <ErrorState error={new ApiError('config.conflict', 'private-revision-fixture', 409)} />
    </AppProviders>,
  )
  expect(screen.getByText(/Saved, but runtime application failed/)).toBeVisible()
  expect(screen.getByText(/configuration changed elsewhere/)).toBeVisible()
  expect(screen.queryByText(/private-fixture|private-revision-fixture/)).not.toBeInTheDocument()
})
it('distinguishes loading, empty and connection failure', () => {
  render(
    <AppProviders api={new ApiClient(unavailableTransport)}>
      <LoadingState />
      <EmptyState />
      <ErrorState error={new ApiError('client.transport_unavailable', '', 0)} />
    </AppProviders>,
  )
  expect(screen.getByRole('status')).toHaveAccessibleName('Loading…')
  expect(screen.getByText('Nothing here yet')).toBeVisible()
  expect(screen.getByRole('alert')).toHaveTextContent('A trusted connection is not configured')
})
