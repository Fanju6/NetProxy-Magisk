import { useId, useRef, useState } from 'react'
import { z } from 'zod'
import { ApiError, groupIdSchema, type GroupSummary, type ApiRequest } from '@/api/contracts'
import { managementRequest, useManagementAction } from '@/api/management'
import { useEventConnection } from '@/api/events'
import { useApi, usePreferences } from '@/app/context'
import { Details } from '@/components/details'
import { ErrorState } from '@/components/states'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ResourceProviderTag } from './resource-provider-tag'
import { resourceWriteUnconfirmed } from './resource-write'
import { useResultToast } from '@/components/ui/toast'

export type ResourceGroupTarget =
  { operation: 'create' } | { operation: 'rename' | 'remove'; group: GroupSummary }

export function ResourceGroupManager({
  target,
  groups,
  onClose,
}: {
  target: ResourceGroupTarget
  groups: GroupSummary[]
  onClose: (result?: { id: string; removed: boolean }) => void
}) {
  const api = useApi()
  const connection = useEventConnection()
  const { t, preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const action = useManagementAction()
  const prefix = useId()
  const submitting = useRef(false)
  const [id, setId] = useState(target.operation === 'create' ? '' : target.group.id)
  const [name, setName] = useState(target.operation === 'create' ? '' : target.group.name)
  const [invalid, setInvalid] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useResultToast(
    `resource-group-${prefix}`,
    saved,
    target.operation === 'remove'
      ? text('分组已删除', 'Group deleted')
      : text('分组已保存', 'Group saved'),
  )
  const [uncertain, setUncertain] = useState(false)
  const current = groups.find((group) => group.id === id)
  const validTarget =
    target.operation === 'create' ||
    (current?.type === 'local' && (target.operation !== 'remove' || id !== 'default'))
  const blocked = action.pending || connection !== 'live' || uncertain || !validTarget
  const close = () => onClose(saved ? { id, removed: target.operation === 'remove' } : undefined)
  const submit = async () => {
    if (submitting.current || blocked || saved) return
    if (
      !groupIdSchema.safeParse(id).success ||
      (target.operation !== 'remove' && !name.trim()) ||
      (target.operation === 'create' && current)
    ) {
      setInvalid(true)
      return
    }
    const request: ApiRequest =
      target.operation === 'create'
        ? { method: 'POST', path: '/catalog', body: { id, name: name.trim() } }
        : target.operation === 'rename'
          ? { method: 'PUT', path: `/catalog/${id}`, body: { name: name.trim() } }
          : { method: 'DELETE', path: `/catalog/${id}` }
    if (!managementRequest(request)) {
      setInvalid(true)
      return
    }
    submitting.current = true
    setInvalid(false)
    setError(null)
    try {
      await action.run(request, z.object({ id: z.literal(id) }))
      setSaved(true)
    } catch (failure) {
      setError(failure)
      setUncertain(resourceWriteUnconfirmed(failure))
    } finally {
      submitting.current = false
    }
  }
  if (!api.supports('catalog.write')) return null
  const title =
    target.operation === 'create'
      ? text('新建本地分组', 'Create local group')
      : target.operation === 'rename'
        ? text('重命名分组', 'Rename group')
        : text('删除分组', 'Delete group')
  const description =
    target.operation === 'remove'
      ? text(
          '删除后无法恢复。仍被策略引用的分组不能删除。',
          'This cannot be undone. Groups referenced by a policy cannot be deleted.',
        )
      : text(
          '分组显示名可以更改，ProviderTag 保持不变。',
          'The display name can change. The ProviderTag stays the same.',
        )
  const footer = saved ? (
    <Button className="min-h-11" onClick={close}>
      {t('close')}
    </Button>
  ) : (
    <>
      <Button variant="outline" className="min-h-11" onClick={close}>
        {t('cancel')}
      </Button>
      <Button
        className="min-h-11"
        variant={target.operation === 'remove' ? 'destructive' : 'default'}
        disabled={blocked}
        onClick={() => void submit()}
      >
        {action.pending
          ? t('working')
          : target.operation === 'remove'
            ? text('确认删除', 'Confirm delete')
            : t('save')}
      </Button>
    </>
  )
  const content = (
    <div className="min-w-0 space-y-4">
      {target.operation !== 'create' && (
        <>
          <p className="break-all font-medium">{target.group.name}</p>
          <ResourceProviderTag group={target.group} />
        </>
      )}
      {!saved && target.operation !== 'remove' && (
        <fieldset disabled={blocked} className="min-w-0 space-y-4">
          {target.operation === 'create' && (
            <div className="space-y-2">
              <label htmlFor={`${prefix}-id`} className="text-sm font-medium">
                {text('分组标识', 'Group ID')}
              </label>
              <Input
                id={`${prefix}-id`}
                value={id}
                onChange={(event) => setId(event.target.value)}
                autoComplete="off"
                autoCapitalize="none"
                spellCheck={false}
                className="h-12"
              />
            </div>
          )}
          <div className="space-y-2">
            <label htmlFor={`${prefix}-name`} className="text-sm font-medium">
              {text('分组名称', 'Group name')}
            </label>
            <Input
              id={`${prefix}-name`}
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
              className="h-12"
            />
          </div>
        </fieldset>
      )}
      {!validTarget && !saved && (
        <p role="alert" className="text-sm text-destructive">
          {text(
            '分组已变更，请关闭后重新读取。',
            'The group changed. Close and refresh resources.',
          )}
        </p>
      )}
      {invalid && (
        <p role="alert" className="text-sm text-destructive">
          {text(
            '请填写有效且未占用的分组标识和名称。',
            'Enter an available, valid group ID and name.',
          )}
        </p>
      )}
      {error instanceof ApiError && error.code === 'catalog.provider_in_use' ? (
        <p role="alert" className="text-sm text-destructive">
          {text(
            '分组仍被策略组或出站引用，请先移除这些引用。',
            'The group is still referenced by a policy or outbound. Remove those references first.',
          )}
        </p>
      ) : error != null ? (
        <ErrorState error={error} compact />
      ) : null}
      {uncertain && (
        <p role="status" className="text-sm">
          {text(
            '结果未确认，请刷新资源后核对。',
            'Result not confirmed. Refresh resources to check.',
          )}
        </p>
      )}
    </div>
  )
  if (target.operation === 'create')
    return (
      <Details
        open
        dirty={!saved && (!!id || !!name)}
        onOpenChange={(open) => !open && close()}
        title={title}
        description={description}
        footer={footer}
      >
        {content}
      </Details>
    )
  return (
    <Dialog open onOpenChange={(open) => !open && close()}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto overscroll-contain rounded-lg">
        <DialogHeader className="pr-8">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {content}
        <DialogFooter>{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
