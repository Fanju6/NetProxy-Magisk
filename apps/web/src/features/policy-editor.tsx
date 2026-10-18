import { useEffect, useRef, useState } from 'react'
import { z } from 'zod'
import { useApi, usePreferences } from '@/app/context'
import { ApiError } from '@/api/contracts'
import { readConfig, useManagementAction } from '@/api/management'
import { safeManagementError } from '@/api/management-data'
import {
  outboundObjectSchema,
  readPolicyObject,
  setPolicyProperty,
  replacePolicyObject,
} from '@/api/policy-config'
import { EditorPanel } from '@/components/details'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { JsonEditor } from '@/components/json-editor'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { LoadingState } from '@/components/states'
import { ManagementSelect } from './configuration'
import { useManagementCopy } from './management-copy'

export function PolicyEditor({
  tag,
  remove = false,
  onClose,
}: {
  tag: string | null
  remove?: boolean
  onClose: () => void
}) {
  const api = useApi()
  const { preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const copy = useManagementCopy()
  const action = useManagementAction()
  const readonly = !api.supports('config.write')
  const [document, setDocument] = useState<{ content: string; revision: string } | null>(null)
  const [draft, setDraft] = useState('')
  const [initial, setInitial] = useState('')
  const [error, setError] = useState<ApiError | null>(null)
  const [blocked, setBlocked] = useState(false)
  const [generation, setGeneration] = useState(0)
  const [json, setJson] = useState(false)
  const [notice, setNotice] = useState('')
  const writing = useRef(false)
  const mounted = useRef(false)
  useEffect(() => {
    mounted.current = true
    const controller = new AbortController()
    void readConfig(api, 'singbox/outbounds', controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return
        const content =
          tag === null
            ? '{"type":"selector","tag":"","outbounds":[]}'
            : readPolicyObject(result.content, tag)
        const object = JSON.parse(content) as { type: string }
        setDocument(result)
        setDraft(content)
        setInitial(content)
        setJson(!['selector', 'urltest'].includes(object.type))
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(safeManagementError(failure))
      })
    return () => {
      mounted.current = false
      controller.abort()
    }
  }, [api, tag, generation])
  const parsed = outboundObjectSchema.extend({ tag: z.string() }).safeParse(
    (() => {
      try {
        return JSON.parse(draft) as unknown
      } catch {
        return null
      }
    })(),
  )
  const object = parsed.success ? parsed.data : null
  const form = !json && (object === null || ['selector', 'urltest'].includes(object.type))
  const change = (key: string, value: unknown) => {
    try {
      setDraft(setPolicyProperty(draft, key, value))
      setNotice('')
    } catch {
      setJson(true)
    }
  }
  async function apply(validate: boolean) {
    if (!document || blocked || writing.current) return
    writing.current = true
    setError(null)
    setNotice('')
    try {
      let content: string
      try {
        content = replacePolicyObject(document.content, tag, remove ? null : draft)
      } catch {
        throw new ApiError('config.invalid', 'Invalid native outbound object', 422, null, true)
      }
      await action.run(
        {
          method: validate ? 'POST' : 'PUT',
          path: validate ? '/config/validate' : '/config',
          query: { target: 'singbox/outbounds' },
          body: { content, revision: document.revision },
        },
        z.object({ target: z.literal('singbox/outbounds'), revision: z.string().min(1) }),
      )
      if (!mounted.current) return
      if (validate) setNotice(copy.validated)
      else onClose()
    } catch (failure) {
      if (!mounted.current) return
      const safe = safeManagementError(failure)
      setError(safe)
      if (safe.code === 'config.conflict' || safe.persisted || !safe.responseReceived)
        setBlocked(true)
    } finally {
      writing.current = false
    }
  }
  const reload = () => {
    if (!writing.current) {
      setDocument(null)
      setDraft('')
      setInitial('')
      setError(null)
      setBlocked(false)
      setGeneration((value) => value + 1)
    }
  }
  const failure = error && (
    <div role="alert" className="space-y-2 text-sm text-destructive">
      <p>
        {error.code === 'config.conflict'
          ? copy.conflict
          : error.persisted
            ? copy.persisted
            : blocked
              ? copy.unknown
              : copy.failed}
      </p>
      <p className="break-all font-mono text-xs">{error.code}</p>
      <Button variant="outline" disabled={action.pending} onClick={reload}>
        {copy.discard}
      </Button>
    </div>
  )
  if (remove)
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open && !action.pending) onClose()
        }}
      >
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>{text('删除策略组', 'Delete policy group')}</DialogTitle>
            <DialogDescription>{tag}</DialogDescription>
          </DialogHeader>
          <p className="text-sm">
            {text(
              '确认删除？不会自动修改其他出站、路由或默认出站的引用，内核校验失败将保留配置。',
              'Delete this group? References in other outbounds, routes and the default outbound will not be rewritten. Invalid references prevent saving.',
            )}
          </p>
          {failure}
          {!document && !error && <LoadingState />}
          <DialogFooter>
            <Button variant="outline" disabled={action.pending} onClick={onClose}>
              {copy.cancel}
            </Button>
            <Button
              variant="destructive"
              disabled={!document || blocked || action.pending || readonly}
              onClick={() => void apply(false)}
            >
              {copy.remove}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  return (
    <EditorPanel
      open
      onOpenChange={(open) => {
        if (!open && !action.pending) onClose()
      }}
      title={tag ?? text('新增策略组', 'Add policy group')}
      dirty={draft !== initial}
      headerActions={
        !readonly && (
          <Button variant="ghost" onClick={() => setJson((value) => !value)}>
            {json ? copy.form : copy.json}
          </Button>
        )
      }
      footer={
        <>
          <Button
            variant="ghost"
            disabled={!document || blocked || action.pending || !api.supports('config.validate')}
            onClick={() => void apply(true)}
          >
            {copy.validate}
          </Button>
          <Button
            disabled={!document || blocked || action.pending || !api.supports('config.write')}
            onClick={() => void apply(false)}
          >
            {copy.save}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {failure}
        {notice && <p role="status">{notice}</p>}
        {!document && !error && <LoadingState />}
        {document &&
          (form ? (
            <fieldset disabled={readonly || action.pending || blocked} className="space-y-4">
              <label className="block space-y-2 text-sm">
                <span>Tag</span>
                <Input
                  value={typeof object?.tag === 'string' ? object.tag : ''}
                  onChange={(event) => change('tag', event.target.value)}
                />
              </label>
              <ManagementSelect
                label={text('类型', 'Type')}
                value={object?.type ?? 'selector'}
                choices={[
                  { value: 'selector', label: 'selector' },
                  { value: 'urltest', label: 'urltest' },
                ]}
                onChange={(value) => change('type', value)}
              />
              {['outbounds', 'providers'].map((key) => (
                <label key={key} className="block space-y-2 text-sm">
                  <span>{key}</span>
                  <textarea
                    className="min-h-24 w-full rounded-md border bg-background p-3 text-base"
                    value={Array.isArray(object?.[key]) ? (object[key] as string[]).join('\n') : ''}
                    onChange={(event) =>
                      change(
                        key,
                        event.target.value === '' ? undefined : event.target.value.split('\n'),
                      )
                    }
                  />
                </label>
              ))}
              <label className="flex min-h-11 items-center justify-between gap-4 text-sm">
                <span>use_all_providers</span>
                <Switch
                  checked={object?.use_all_providers === true}
                  onCheckedChange={(value) => change('use_all_providers', value)}
                />
              </label>
              {(object?.type === 'urltest'
                ? ['url', 'interval', 'tolerance', 'idle_timeout']
                : ['default']
              ).map((key) => (
                <label key={key} className="block space-y-2 text-sm">
                  <span>{key === 'url' ? 'URL' : key}</span>
                  <Input
                    type={key === 'tolerance' ? 'number' : 'text'}
                    min={0}
                    value={
                      typeof object?.[key] === 'string' || typeof object?.[key] === 'number'
                        ? String(object[key])
                        : ''
                    }
                    onChange={(event) =>
                      change(
                        key,
                        event.target.value === ''
                          ? undefined
                          : key === 'tolerance'
                            ? Number(event.target.value)
                            : event.target.value,
                      )
                    }
                  />
                </label>
              ))}
            </fieldset>
          ) : (
            <JsonEditor
              value={draft}
              onChange={setDraft}
              readOnly={readonly || action.pending || blocked}
              label="Outbound JSON"
            />
          ))}
      </div>
    </EditorPanel>
  )
}
