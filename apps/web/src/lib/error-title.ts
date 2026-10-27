import { ApiError } from '@/api/contracts'
import type { MessageKey } from '@/app/i18n'

export function errorTitle(error: unknown, t: (key: MessageKey) => string) {
  const code = error instanceof ApiError ? error.code : ''
  return code === 'config.conflict'
    ? t('conflict')
    : error instanceof ApiError && error.persisted
      ? t('persisted')
      : code === 'client.transport_unavailable'
        ? t('unavailable')
        : code === 'api.connection_unavailable'
          ? t('connectionUnavailable')
          : code === 'api.unauthorized'
            ? t('unauthorized')
            : error instanceof ApiError && error.status === 403
              ? t('operationDenied')
              : error instanceof ApiError && [409, 429].includes(error.status)
                ? t('operationBusy')
                : t('requestFailed')
}
