import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import type { GroupSummary } from '@/api/contracts'
import {
  subscriptionCreateSchema,
  subscriptionInputSchema,
  type SubscriptionInput,
} from '@/api/subscription-input'
import { usePreferences } from '@/app/context'
import type { MessageKey } from '@/app/i18n'
import { Details } from '@/components/details'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { ResourceProviderTag } from './resource-provider-tag'

export interface SubscriptionEditorProps {
  group?: GroupSummary
  initialURL?: string
  busy: boolean
  disabled?: boolean
  feedback?: ReactNode
  // Return admission promptly, not the eventual transaction result. Never cache input as mutation variables.
  onSubmit: (input: SubscriptionInput) => boolean | Promise<boolean>
  onClose: () => void
}

const replacementFields = [
  ['url', 'subscriptionUrl'],
  ['user_agent', 'subscriptionUserAgent'],
  ['hwid', 'subscriptionHwid'],
  ['custom_headers', 'subscriptionHeaders'],
  ['include', 'subscriptionInclude'],
  ['exclude', 'subscriptionExclude'],
  ['allow_insecure', 'subscriptionAllowInsecure'],
  ['download_timeout', 'subscriptionTimeout'],
] as const satisfies readonly (readonly [keyof SubscriptionInput, MessageKey])[]
type Replacement = (typeof replacementFields)[number][0]

/** Mount only while editing. Changing group identity discards the previous draft. */
export function SubscriptionEditor(props: SubscriptionEditorProps) {
  if (props.group?.type === 'local') return null
  return <Editor key={props.group?.id ?? 'new'} {...props} />
}

function Editor({
  group,
  initialURL,
  busy,
  disabled = false,
  feedback,
  onSubmit,
  onClose,
}: SubscriptionEditorProps) {
  const { t } = usePreferences()
  const prefix = useId()
  const form = useRef<HTMLFormElement>(null)
  const mounted = useRef(false)
  const closed = useRef(false)
  const submitting = useRef(false)
  const [open, setOpen] = useState(true)
  const [waiting, setWaiting] = useState(false)
  const [invalid, setInvalid] = useState(false)
  const [dirty, setDirty] = useState(!group && !!initialURL)
  const [replacements, setReplacements] = useState<Replacement[]>([])
  const [initial] = useState(() => ({
    name: group?.name ?? '',
    auto_update: group?.auto_update ?? true,
    update_interval: group?.update_interval ?? 86400,
    update_via_proxy: group?.update_via_proxy ?? 'auto',
  }))
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  function close() {
    if (closed.current) return
    closed.current = true
    form.current?.reset()
    setReplacements([])
    setOpen(false)
    onClose()
  }

  async function submit(element: HTMLFormElement) {
    if (closed.current || submitting.current || busy || disabled) return
    const data = new FormData(element)
    const input: Record<string, unknown> = {
      name: data.get('name'),
      auto_update: data.has('auto_update'),
      update_interval: Number(data.get('update_interval')),
      update_via_proxy: data.get('update_via_proxy'),
    }
    if (group) {
      for (const key of Object.keys(initial) as (keyof typeof initial)[])
        if (input[key] === initial[key]) delete input[key]
    } else if (input.name === '') delete input.name
    try {
      for (const [key] of replacementFields) {
        if (!(key === 'url' && !group) && !replacements.includes(key)) continue
        const value = data.get(key)
        input[key] =
          key === 'custom_headers'
            ? JSON.parse(String(value || '{}'))
            : key === 'allow_insecure'
              ? data.has(key)
              : key === 'download_timeout'
                ? Number(value)
                : value
      }
      const parsed = (group ? subscriptionInputSchema : subscriptionCreateSchema).safeParse(input)
      if (!parsed.success) {
        setInvalid(true)
        return
      }
      submitting.current = true
      setWaiting(true)
      setInvalid(false)
      const admitted = await onSubmit(parsed.data)
      if (!mounted.current) return
      if (admitted) close()
    } catch {
      // Neither validation nor callback failures may render user-supplied values.
      if (mounted.current) setInvalid(true)
    } finally {
      submitting.current = false
      if (mounted.current) setWaiting(false)
    }
  }

  function field(key: Replacement) {
    const id = `${prefix}-${key}`
    if (key === 'allow_insecure')
      return (
        <label htmlFor={id} className="flex min-h-12 items-center justify-between gap-4 text-sm">
          {t('subscriptionAllowInsecureValue')}
          <Switch id={id} name={key} />
        </label>
      )
    if (key === 'custom_headers')
      return (
        <textarea
          id={id}
          name={key}
          aria-label={t('subscriptionHeaders')}
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          defaultValue="{}"
          rows={4}
          className="w-full resize-y rounded-md border border-input bg-transparent px-2.5 py-2 text-base outline-none focus-visible:ring-2 focus-visible:ring-ring md:text-sm"
        />
      )
    return (
      <Input
        id={id}
        name={key}
        aria-label={t(replacementFields.find(([name]) => name === key)![1])}
        type={key === 'download_timeout' ? 'number' : key === 'url' ? 'url' : 'text'}
        inputMode={key === 'download_timeout' ? 'numeric' : key === 'url' ? 'url' : 'text'}
        min={key === 'download_timeout' ? 1 : undefined}
        step={key === 'download_timeout' ? 1 : undefined}
        defaultValue={
          key === 'download_timeout' ? 60 : key === 'url' && !group ? (initialURL ?? '') : ''
        }
        autoComplete="off"
        autoCapitalize="none"
        spellCheck={false}
        className="h-12"
      />
    )
  }

  return (
    <Details
      open={open}
      dirty={dirty}
      onOpenChange={(value) => {
        if (!value) close()
      }}
      title={t(group ? 'editSubscription' : 'addSubscription')}
      description={t('subscriptionWriteOnly')}
      footer={
        open && (
          <Button
            type="submit"
            form={`${prefix}-form`}
            disabled={busy || disabled || waiting}
            className="min-h-11 flex-1"
          >
            {t(waiting ? 'working' : 'save')}
          </Button>
        )
      }
    >
      {open && (
        <form
          id={`${prefix}-form`}
          ref={form}
          autoComplete="off"
          noValidate
          onInput={() => setDirty(true)}
          onChange={() => setDirty(true)}
          onSubmit={(event) => {
            event.preventDefault()
            void submit(event.currentTarget)
          }}
          className="space-y-4 overscroll-contain"
        >
          {feedback}
          <fieldset disabled={busy || disabled || waiting} className="min-w-0 space-y-4">
            <div className="space-y-4">
              <Labeled label={t('subscriptionName')} htmlFor={`${prefix}-name`}>
                <Input
                  id={`${prefix}-name`}
                  name="name"
                  defaultValue={initial.name}
                  className="h-12"
                />
              </Labeled>
              {group && <ResourceProviderTag group={group} />}
              {!group && (
                <Labeled label={t('subscriptionUrl')} htmlFor={`${prefix}-url`}>
                  {field('url')}
                </Labeled>
              )}
            </div>
            <div className="settings-group divide-y">
              <label className="setting-row" htmlFor={`${prefix}-auto`}>
                {t('autoUpdate')}
                <Switch
                  id={`${prefix}-auto`}
                  name="auto_update"
                  defaultChecked={initial.auto_update}
                  onCheckedChange={() => setDirty(true)}
                />
              </label>
              <div className="min-w-0 space-y-4 py-4">
                <Labeled label={t('subscriptionIntervalSeconds')} htmlFor={`${prefix}-interval`}>
                  <Input
                    id={`${prefix}-interval`}
                    name="update_interval"
                    type="number"
                    inputMode="numeric"
                    min={900}
                    step={1}
                    defaultValue={initial.update_interval}
                    className="h-12"
                  />
                </Labeled>
                <Labeled label={t('subscriptionViaProxy')} htmlFor={`${prefix}-via`}>
                  <Select
                    name="update_via_proxy"
                    defaultValue={initial.update_via_proxy}
                    onValueChange={() => setDirty(true)}
                  >
                    <SelectTrigger
                      id={`${prefix}-via`}
                      className="min-h-12 w-full text-base md:text-sm"
                    >
                      <SelectValue>
                        {(value: unknown) =>
                          t(
                            value === 'always'
                              ? 'Proxy'
                              : value === 'never'
                                ? 'Direct'
                                : 'subscriptionViaAuto',
                          )
                        }
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="auto" className="min-h-12">
                        {t('subscriptionViaAuto')}
                      </SelectItem>
                      <SelectItem value="always" className="min-h-12">
                        {t('Proxy')}
                      </SelectItem>
                      <SelectItem value="never" className="min-h-12">
                        {t('Direct')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </Labeled>
              </div>
            </div>
            <details className="settings-group">
              <summary className="min-h-12 cursor-pointer py-3 text-sm font-medium">
                {t('subscriptionAdvanced')}
              </summary>
              <p className="pb-3 text-sm text-muted-foreground">{t('subscriptionUnchanged')}</p>
              <div className="divide-y border-t">
                {replacementFields
                  .filter(([key]) => key !== 'url' || group)
                  .map(([key, label]) => (
                    <div key={key} className="space-y-2 py-2">
                      <label
                        className="flex min-h-12 items-center justify-between gap-4 text-sm"
                        htmlFor={`${prefix}-modify-${key}`}
                      >
                        {t('subscriptionModify', { field: t(label) })}
                        <Switch
                          id={`${prefix}-modify-${key}`}
                          checked={replacements.includes(key)}
                          onCheckedChange={(checked) => {
                            setDirty(true)
                            setReplacements((current) =>
                              checked
                                ? [...current, key]
                                : current.filter((entry) => entry !== key),
                            )
                          }}
                        />
                      </label>
                      {replacements.includes(key) && field(key)}
                    </div>
                  ))}
              </div>
            </details>
            {invalid && (
              <p role="alert" className="text-sm text-destructive">
                {t('subscriptionInputInvalid')}
              </p>
            )}
          </fieldset>
        </form>
      )}
    </Details>
  )
}

function Labeled({
  label,
  htmlFor,
  children,
}: {
  label: string
  htmlFor: string
  children: ReactNode
}) {
  return (
    <div className="min-w-0 space-y-2">
      <label htmlFor={htmlFor} className="text-sm font-medium">
        {label}
      </label>
      {children}
    </div>
  )
}
