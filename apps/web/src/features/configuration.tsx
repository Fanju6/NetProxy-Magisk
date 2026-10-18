import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { z } from 'zod'
import { Link, useSearch } from '@tanstack/react-router'
import { Braces, FileJson, Plus, RefreshCw } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { ApiError } from '@/api/contracts'
import { configTargetsSchema, readConfig, useManagementAction } from '@/api/management'
import {
  makeSimpleRule,
  parseDocument,
  replaceRule,
  ruleDocumentSchema,
  ruleFields,
  safeManagementError,
  simpleRule,
  type RuleField,
} from '@/api/management-data'
import { Details, DraftGuard, EditorPanel } from '@/components/details'
import { JsonEditor } from '@/components/json-editor'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { EmptyState, ErrorState, LoadingState } from '@/components/states'
import { useManagementCopy } from './management-copy'

export interface ConfigurationEditorProps {
  target: string
  title: string
  readOnly?: boolean
  inline?: boolean
  renderForm?: (content: string, change: (content: string) => void) => ReactNode
  onClose: () => void
}

export function ManagementSelect({
  label,
  value,
  choices,
  onChange,
}: {
  label: string
  value: string
  choices: readonly { value: string; label: string }[]
  onChange: (value: string) => void
}) {
  const id = useId()
  return (
    <div className="space-y-2">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <Select
        value={value}
        onValueChange={(next) => {
          if (next !== null) onChange(next)
        }}
      >
        <SelectTrigger id={id} className="min-h-11 w-full text-base md:text-sm">
          <SelectValue>
            {(selected: unknown) =>
              choices.find((choice) => choice.value === selected)?.label ?? ''
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {choices.map((choice) => (
            <SelectItem key={choice.value} value={choice.value}>
              {choice.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

export function ConfigurationEditor({
  target,
  title,
  readOnly = false,
  inline = false,
  renderForm,
  onClose,
}: ConfigurationEditorProps) {
  const api = useApi()
  const copy = useManagementCopy()
  const action = useManagementAction()
  const readonly = readOnly || target.startsWith('runtime/')
  const rules = /^singbox\/rules\/local\/(proxy|direct|block)\.json$/.test(target)
  const [draft, setDraft] = useState<{ revision: string; content: string } | null>(null)
  const [savedContent, setSavedContent] = useState('')
  const [error, setError] = useState<ApiError | null>(null)
  const [generation, setGeneration] = useState(0)
  const [form, setForm] = useState(!readonly && (rules || renderForm !== undefined))
  const [notice, setNotice] = useState<'validated' | 'saved' | null>(null)
  const [blocked, setBlocked] = useState(false)
  const writing = useRef(false)
  const mounted = useRef(false)
  const calls = useRef(new Set<AbortController>())
  useEffect(() => {
    mounted.current = true
    const controller = new AbortController()
    void readConfig(api, target, controller.signal)
      .then((document) => {
        if (controller.signal.aborted) return
        if (document.target !== target)
          throw new ApiError('client.invalid_response', 'Invalid target', 502)
        setDraft({ revision: document.revision, content: document.content })
        setSavedContent(document.content)
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(safeManagementError(failure))
      })
    const pending = calls.current
    return () => {
      mounted.current = false
      controller.abort()
      for (const call of pending) call.abort()
    }
  }, [api, target, generation])
  function reload() {
    if (writing.current) return
    setDraft(null)
    setError(null)
    setBlocked(false)
    setNotice(null)
    setGeneration((value) => value + 1)
  }
  function change(content: string) {
    setDraft((current) => (current ? { ...current, content } : null))
    setNotice(null)
  }
  async function apply(validateOnly: boolean) {
    if (!draft || readonly || blocked || writing.current) return
    const controller = new AbortController()
    calls.current.add(controller)
    writing.current = true
    setError(null)
    setNotice(null)
    try {
      const result = await action.run(
        {
          method: validateOnly ? 'POST' : 'PUT',
          path: validateOnly ? '/config/validate' : '/config',
          query: { target },
          body: { content: draft.content, revision: draft.revision },
        },
        z.object({ target: z.literal(target), revision: z.string().min(1) }),
        controller.signal,
      )
      if (!mounted.current) return
      if (!validateOnly) {
        setDraft((current) => (current ? { ...current, revision: result.revision } : null))
        setSavedContent(draft.content)
      }
      setNotice(validateOnly ? 'validated' : 'saved')
    } catch (failure) {
      if (!mounted.current) return
      const safe = safeManagementError(failure)
      setError(safe)
      if (safe.code === 'config.conflict' || safe.persisted || !safe.responseReceived)
        setBlocked(true)
    } finally {
      writing.current = false
      calls.current.delete(controller)
    }
  }
  const conflict = error?.code === 'config.conflict'
  const footer = draft && !readonly && (
    <>
      <Button
        variant="ghost"
        size="sm"
        disabled={action.pending || blocked}
        onClick={() => void apply(true)}
      >
        {copy.validate}
      </Button>
      <Button disabled={action.pending || blocked} onClick={() => void apply(false)}>
        {copy.save}
      </Button>
    </>
  )
  const body = (
    <div className={form ? 'space-y-4' : 'flex min-h-0 flex-1 flex-col'}>
      {error && (
        <div role="alert" className="space-y-3 text-sm text-destructive">
          <p>
            {conflict
              ? copy.conflict
              : error.persisted
                ? copy.persisted
                : blocked
                  ? copy.unknown
                  : copy.failed}
          </p>
          <Button variant="outline" className="min-h-11" disabled={action.pending} onClick={reload}>
            {draft ? copy.discard : copy.retry}
          </Button>
        </div>
      )}
      {!draft && !error && <LoadingState />}
      {draft && (
        <>
          <fieldset
            disabled={action.pending || blocked}
            className={form ? 'min-w-0' : 'flex min-h-0 flex-1 flex-col'}
          >
            {form && !readonly ? (
              renderForm ? (
                renderForm(draft.content, change)
              ) : (
                <RuleForm content={draft.content} change={change} editJson={() => setForm(false)} />
              )
            ) : (
              <JsonEditor
                value={draft.content}
                onChange={change}
                readOnly={readonly || action.pending || blocked}
                label={`${title} JSON`}
              />
            )}
          </fieldset>
          {notice && (
            <p role="status" className="text-sm text-muted-foreground">
              {copy[notice]}
            </p>
          )}
        </>
      )}
    </div>
  )
  const dirty = !readonly && draft !== null && draft.content !== savedContent
  const switchView = !readonly && (renderForm || rules) && (
    <Button variant="ghost" size="sm" disabled={action.pending} onClick={() => setForm(!form)}>
      <Braces />
      {form ? copy.json : copy.form}
    </Button>
  )
  if (inline && form)
    return (
      <DraftGuard dirty={dirty}>
        {body}
        <div className="panel-inline-actions mt-4 flex items-center gap-2 bg-background py-3">
          {switchView}
          <div className="flex-1" />
          {footer}
        </div>
      </DraftGuard>
    )
  const Panel = form ? Details : EditorPanel
  return (
    <Panel
      open
      title={title}
      description={form ? undefined : readonly ? copy.readOnly : copy.sensitive}
      dirty={dirty && !(inline && !form)}
      footer={footer}
      headerActions={switchView}
      onOpenChange={(open) => {
        if (!open) {
          if (inline && !form) setForm(true)
          else onClose()
        }
      }}
    >
      {body}
    </Panel>
  )
}

function RuleForm({
  content,
  change,
  editJson,
}: {
  content: string
  change: (content: string) => void
  editJson: () => void
}) {
  const copy = useManagementCopy()
  const document = parseDocument(content, ruleDocumentSchema)
  const [editing, setEditing] = useState<{
    index: number | null
    field: RuleField
    text: string
  } | null>(null)
  const [invalid, setInvalid] = useState(false)
  if (!document) return <EmptyState title={copy.invalid} />
  function save() {
    if (!editing || !document) return
    const rule = makeSimpleRule(editing.field, editing.text)
    if (!rule) {
      setInvalid(true)
      return
    }
    change(JSON.stringify(replaceRule(document, editing.index, rule), null, 2))
    setEditing(null)
    setInvalid(false)
  }
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{copy.matchRules}</h3>
        <Button
          variant="outline"
          className="min-h-11"
          onClick={() => {
            setEditing({ index: null, field: 'domain_suffix', text: '' })
            setInvalid(false)
          }}
        >
          <Plus />
          {copy.addRule}
        </Button>
      </div>
      {editing && (
        <Details
          open
          title={editing.index === null ? copy.addRule : copy.editRule}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
          dirty={Boolean(editing.text)}
          footer={<Button onClick={save}>{copy.save}</Button>}
        >
          <div className="space-y-3">
            <ManagementSelect
              label={copy.match}
              value={editing.field}
              choices={ruleFields.map((field) => ({ value: field, label: copy[field] }))}
              onChange={(value) => {
                if (ruleFields.includes(value as RuleField))
                  setEditing({ ...editing, field: value as RuleField })
              }}
            />
            <label className="block space-y-2 text-sm">
              <span>{copy.values}</span>
              <textarea
                rows={4}
                className="w-full rounded-md border bg-transparent p-3 text-base"
                autoCapitalize="none"
                spellCheck={false}
                value={editing.text}
                onChange={(event) => setEditing({ ...editing, text: event.target.value })}
              />
            </label>
            {invalid && (
              <p role="alert" className="text-sm text-destructive">
                {copy.invalidRule}
              </p>
            )}
          </div>
        </Details>
      )}
      {!document.rules.length && <EmptyState title={copy.noRules} />}
      <ol className="divide-y">
        {document.rules.map((rule, index) => {
          const simple = simpleRule(rule)
          return (
            <li key={index} className="flex min-w-0 items-center justify-between gap-3 py-3">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">
                  {simple ? copy[simple.field] : copy.complexRule}
                </p>
                {simple && (
                  <p className="break-words text-sm text-muted-foreground">
                    {simple.values.join(', ')}
                  </p>
                )}
              </div>
              {simple ? (
                <div className="flex shrink-0 gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="min-h-11"
                    onClick={() => {
                      setEditing({ index, field: simple.field, text: simple.values.join('\n') })
                      setInvalid(false)
                    }}
                  >
                    {copy.editRule}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="min-h-11 text-destructive"
                    onClick={() =>
                      change(
                        JSON.stringify(
                          {
                            ...document,
                            rules: document.rules.filter((_, item) => item !== index),
                          },
                          null,
                          2,
                        ),
                      )
                    }
                  >
                    {copy.remove}
                  </Button>
                </div>
              ) : (
                <Button variant="outline" size="sm" className="min-h-11" onClick={editJson}>
                  {copy.json}
                </Button>
              )}
            </li>
          )
        })}
      </ol>
      <p className="text-xs text-muted-foreground">{copy.ruleOrder}</p>
    </div>
  )
}

export function ConfigurationPage() {
  const api = useApi()
  const { preferences } = usePreferences()
  const section = useSearch({ from: '/settings/configuration' }).section
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const copy = useManagementCopy()
  const labels: Record<string, string> = {
    module: text('模块设置', 'Module settings'),
    'module/auto_start': text('开机启动', 'Start on boot'),
    'module/wifi': text('Wi-Fi 策略', 'Wi-Fi policy'),
    inbound: text('完整入站配置', 'Full inbound configuration'),
    'inbound/backend': copy.backend,
    'inbound/root_policy': copy.root,
    'inbound/app': copy.apps,
    'inbound/ebpf': 'eBPF',
    'inbound/tun': 'TUN',
    'singbox/config.json': text('完整主配置', 'Full core configuration'),
    'singbox/rules/local/proxy.json': text('代理规则', 'Proxy rules'),
    'singbox/rules/local/direct.json': text('直连规则', 'Direct rules'),
    'singbox/rules/local/block.json': text('拦截规则', 'Block rules'),
    'runtime/inbound.json': text('运行时入站', 'Runtime inbound'),
    'runtime/providers.json': text('运行时节点', 'Runtime providers'),
    ...Object.fromEntries(
      [
        'log',
        'dns',
        'ntp',
        'certificate',
        'certificate_providers',
        'http_clients',
        'network_namespaces',
        'endpoints',
        'inbounds',
        'outbounds',
        'providers',
        'route',
        'services',
        'experimental',
      ].map((key, index) => [
        `singbox/${key}`,
        preferences.locale === 'zh'
          ? [
              '日志',
              'DNS',
              '时间同步',
              '证书',
              '证书提供器',
              'HTTP 客户端',
              '网络命名空间',
              '端点',
              '入站',
              '出站',
              '节点提供器',
              '路由',
              '服务',
              '实验功能',
            ][index]!
          : key.replaceAll('_', ' '),
      ]),
    ),
  }
  const [targets, setTargets] = useState<z.infer<typeof configTargetsSchema> | null>(null)
  const [error, setError] = useState<ApiError | null>(null)
  const [generation, setGeneration] = useState(0)
  const [selected, setSelected] = useState<z.infer<typeof configTargetsSchema>[number] | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    void api
      .request({ method: 'GET', path: '/configs' }, configTargetsSchema, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setTargets(result)
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(safeManagementError(failure))
      })
    return () => controller.abort()
  }, [api, generation])
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-end gap-3">
        {section === 'core' && (
          <Link
            to="/settings/configuration"
            search={{ section: 'all' }}
            className="mr-auto flex min-h-12 items-center text-sm text-muted-foreground underline underline-offset-4"
          >
            {text('全部配置文件', 'All configuration files')}
          </Link>
        )}
        <Button
          variant="ghost"
          size="icon"
          className="size-11 shrink-0"
          aria-label={copy.retry}
          onClick={() => {
            setError(null)
            setGeneration((value) => value + 1)
          }}
        >
          <RefreshCw />
        </Button>
      </div>
      {error && <ErrorState error={error} compact />}
      {!targets && !error && <LoadingState />}
      {targets?.length === 0 && <EmptyState title={copy.noConfigurations} />}
      {(['module', 'config', 'rules', 'inbound', 'runtime'] as const).map((category) => {
        if (section === 'rules' && category !== 'rules') return null
        if (section === 'core' && !['config', 'runtime'].includes(category)) return null
        const entries = targets?.filter((target) => target.category === category) ?? []
        if (!entries.length) return null
        return (
          <section key={category} className="space-y-2">
            <h2 className="section-label">
              {category === 'module'
                ? 'NetProxy'
                : category === 'config'
                  ? copy.core
                  : copy[category]}
            </h2>
            <div className="settings-group">
              {entries.map((target) => (
                <Button
                  key={target.id}
                  variant="ghost"
                  className="min-h-14 w-full justify-start gap-3 rounded-none whitespace-normal text-left"
                  onClick={() => setSelected(target)}
                >
                  <FileJson className="size-4 shrink-0" />
                  <span className="min-w-0 flex-1 space-y-1">
                    <span className="block break-words">
                      {labels[target.id] ?? target.section ?? target.filename}
                    </span>
                    <span className="block truncate text-xs font-normal text-muted-foreground">
                      {target.id}
                    </span>
                  </span>
                  {!target.editable && (
                    <span className="text-xs text-muted-foreground">{copy.readOnly}</span>
                  )}
                  <Braces className="size-4 shrink-0" />
                </Button>
              ))}
            </div>
          </section>
        )
      })}
      {selected && (
        <ConfigurationEditor
          key={selected.id}
          target={selected.id}
          title={selected.filename}
          readOnly={!selected.editable}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  )
}
