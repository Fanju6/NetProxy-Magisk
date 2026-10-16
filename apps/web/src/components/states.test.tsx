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

it('describes missing native credentials without claiming pairing or exposing details', () => {
  render(
    <AppProviders api={new ApiClient(unavailableTransport)}>
      <ErrorState error={new ApiError('api.connection_unavailable', 'private-native-fixture', 0)} />
    </AppProviders>,
  )
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Trusted connection details are not available',
  )
  expect(screen.getByRole('alert')).toHaveTextContent('Trusted pairing did not complete.')
  expect(screen.queryByText(/private-native-fixture/)).not.toBeInTheDocument()
})

it.each([
  [401, 'api.unauthorized', 'Connection credentials are invalid'],
  [403, 'client.operation_denied', 'This operation is not authorized.'],
  [409, 'api.busy', 'Another operation is in progress.'],
] as const)('classifies operation rejection %s without backend text', (status, code, title) => {
  render(
    <AppProviders api={new ApiClient(unavailableTransport)}>
      <ErrorState error={new ApiError(code, 'private-fixture', status)} />
    </AppProviders>,
  )
  expect(screen.getByRole('alert')).toHaveTextContent(title)
  expect(screen.queryByText(/private-fixture/)).not.toBeInTheDocument()
})
