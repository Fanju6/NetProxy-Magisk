import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import {
  androidImportPayload,
  detectImportKind,
  loadAndroidInput,
  type AndroidInputAdapter,
  type AndroidImportPayload,
  type DocumentKind,
  type ImportKind,
} from '@/api/android-input'
import { useEventConnection } from '@/api/events'
import { useSubscriptionActions } from '@/api/subscription-control'
import type { SubscriptionInput } from '@/api/subscription-input'
import { ApiError } from '@/api/contracts'
import { useApi, usePreferences } from '@/app/context'
import { AndroidImportContext } from '@/app/android-import-context'
import { Details } from '@/components/details'
import { Button } from '@/components/ui/button'
import { ConfigurationEditor } from './configuration'
import { ResourceNodeManager } from './resource-nodes'
import { SubscriptionEditor } from './subscription-editor'
import { SubscriptionFeedback } from './subscriptions'

export function AndroidImports({
  children,
  adapter: injected,
}: {
  children: ReactNode
  adapter?: AndroidInputAdapter | null
}) {
  const api = useApi()
  const connection = useEventConnection()
  const { preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const [adapter, setAdapter] = useState(injected ?? null)
  const [current, setCurrent] = useState<{
    payload: AndroidImportPayload
    kind: ImportKind | null
    editing: boolean
  } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [picking, setPicking] = useState(false)
  const [receiving, setReceiving] = useState(false)
  const [finishing, setFinishing] = useState(false)
  const [foreground, setForeground] = useState(false)
  const occupied = useRef(false)
  const activeImport = useRef<{ adapter: AndroidInputAdapter; id: string } | null>(null)
  const completing = useRef(false)
  const mounted = useRef(false)
  const wake = useRef(() => {})
  useEffect(() => {
    let disposed = false
    if (injected === undefined)
      void loadAndroidInput()
        .then((value) => {
          if (!disposed) setAdapter(value)
        })
        .catch(() => {})
    return () => {
      disposed = true
    }
  }, [injected])
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  useEffect(() => {
    if (!adapter) return
    let disposed = false
    let ready = false
    let visible = false
    let pending = true
    let taking = false
    let release: (() => void) | undefined
    const isForeground = () => visible && document.visibilityState !== 'hidden'
    const drain = async () => {
      if (disposed || !ready || !isForeground() || occupied.current || taking || !pending) return
      pending = false
      taking = true
      occupied.current = true
      setReceiving(true)
      let received = false
      try {
        const payload = androidImportPayload(await adapter.takeImport())
        if (!disposed && payload) {
          received = true
          occupied.current = true
          activeImport.current = { adapter, id: payload.id }
          setError(null)
          setCurrent({ payload, kind: detectImportKind(payload.content), editing: false })
        }
      } catch (failure) {
        if (!disposed) setError(failure instanceof ApiError ? failure.code : 'api.import_invalid')
      } finally {
        taking = false
        if (!disposed) {
          setReceiving(false)
          if (!received) occupied.current = false
        }
        if (!disposed && pending) void drain()
      }
    }
    const updateVisibility = () => {
      if (disposed) return
      setForeground(isForeground())
      if (isForeground()) {
        pending = true
        void drain()
      }
    }
    wake.current = () => {
      pending = true
      void drain()
    }
    document.addEventListener('visibilitychange', updateVisibility)
    void adapter
      .listen(
        () => {
          pending = true
          void drain()
        },
        (value) => {
          visible = value
          setForeground(isForeground())
        },
      )
      .then((close) => {
        if (disposed) {
          close()
          return
        }
        release = close
        ready = true
        void drain()
      })
      .catch(() => {
        if (!disposed) setError('api.import_unreadable')
      })
    return () => {
      disposed = true
      wake.current = () => {}
      document.removeEventListener('visibilitychange', updateVisibility)
      release?.()
    }
  }, [adapter])
  const close = useCallback(() => {
    if (completing.current) return
    setCurrent(null)
    setError(null)
    const active = activeImport.current
    if (!active) {
      occupied.current = false
      wake.current()
      return
    }
    completing.current = true
    setFinishing(true)
    void active.adapter
      .completeImport(active.id)
      .then(() => {
        if (!mounted.current) return
        activeImport.current = null
        occupied.current = false
        wake.current()
      })
      .catch(() => {
        if (mounted.current) setError('import.close_failed')
      })
      .finally(() => {
        completing.current = false
        if (mounted.current) setFinishing(false)
      })
  }, [])
  async function pick(kind: DocumentKind) {
    if (!adapter || occupied.current || !foreground) return
    occupied.current = true
    setPicking(true)
    setError(null)
    try {
      const payload = androidImportPayload(await adapter.pickDocument(kind))
      if (!mounted.current) return
      if (payload) {
        activeImport.current = { adapter, id: payload.id }
        setCurrent({ payload, kind: detectImportKind(payload.content) ?? kind, editing: false })
      } else close()
    } catch (failure) {
      if (mounted.current) {
        occupied.current = false
        setError(failure instanceof ApiError ? failure.code : 'api.import_invalid')
        wake.current()
      }
    } finally {
      if (mounted.current) setPicking(false)
    }
  }
  const supported = (kind: ImportKind) =>
    kind === 'config'
      ? api.supports('config.read') &&
        api.supports('config.validate') &&
        api.supports('config.write')
      : kind === 'nodes'
        ? api.supports('nodes.read') && api.supports('nodes.import')
        : api.supports('subscriptions.add')
  return (
    <AndroidImportContext
      value={{
        available: adapter !== null,
        busy:
          picking || receiving || finishing || error !== null || current !== null || !foreground,
        pick: (kind) => {
          void pick(kind)
        },
      }}
    >
      {children}
      {error && (
        <Details
          open
          title={text('导入失败', 'Import failed')}
          onOpenChange={(open) => {
            if (!open) close()
          }}
        >
          <p role="alert">
            {error === 'import.close_failed'
              ? text(
                  '未能完成关闭导入。请关闭此提示以重试。',
                  'Could not finish closing the import. Close this message to retry.',
                )
              : error === 'api.busy'
                ? text(
                    '另一个导入正在进行，请稍后重试。',
                    'Another import is in progress. Try again shortly.',
                  )
                : error === 'api.body_too_large'
                  ? text(
                      '文件超过 1 MiB，请选择更小的文件。',
                      'The file exceeds 1 MiB. Choose a smaller file.',
                    )
                  : error === 'api.import_unreadable'
                    ? text(
                        '无法读取文件，请重新选择。',
                        'Could not read the file. Choose it again.',
                      )
                    : text('输入无效，请选择 UTF-8 文本。', 'Invalid input. Choose UTF-8 text.')}
          </p>
        </Details>
      )}
      {current && !current.editing && foreground && (
        <Details
          open
          title={text('确认导入', 'Review import')}
          onOpenChange={(open) => {
            if (!open) close()
          }}
          description={text(
            '原文可能包含凭据。确认后继续编辑，不会自动保存或下载。',
            'Content may contain credentials. Continue to edit; nothing is saved or downloaded automatically.',
          )}
          footer={
            <>
              <Button variant="outline" onClick={close}>
                {text('取消', 'Cancel')}
              </Button>
              <Button
                disabled={!current.kind || !supported(current.kind)}
                onClick={() => setCurrent({ ...current, editing: true })}
              >
                {text('继续', 'Continue')}
              </Button>
            </>
          }
        >
          <div className="min-w-0 space-y-4">
            <p className="break-all text-sm">{current.payload.name}</p>
            <label className="block space-y-2 text-sm">
              <span>{text('导入内容', 'Import content')}</span>
              <textarea
                readOnly
                value={current.payload.content}
                rows={6}
                autoComplete="off"
                spellCheck={false}
                className="w-full resize-y rounded-md border p-3 font-mono text-base"
              />
            </label>
            <fieldset className="flex flex-wrap gap-3">
              <legend className="mb-2 text-sm">{text('导入为', 'Import as')}</legend>
              {(['config', 'nodes', 'subscription'] as const).map((kind) => (
                <label key={kind} className="flex min-h-11 items-center gap-2 text-sm">
                  <input
                    type="radio"
                    name="android-import-kind"
                    checked={current.kind === kind}
                    disabled={!supported(kind)}
                    onChange={() => setCurrent({ ...current, kind })}
                  />
                  {kind === 'config'
                    ? text('主配置 JSON', 'Core configuration JSON')
                    : kind === 'nodes'
                      ? text('节点文本', 'Node text')
                      : text('订阅 URL', 'Subscription URL')}
                </label>
              ))}
            </fieldset>
          </div>
        </Details>
      )}
      {current?.editing && current.kind === 'config' && (
        <ConfigurationEditor
          key={current.payload.id}
          target="singbox/config.json"
          title={text('导入主配置', 'Import core configuration')}
          initialContent={current.payload.content}
          onClose={close}
        />
      )}
      {current?.editing && current.kind === 'nodes' && (
        <ResourceNodeManager
          key={current.payload.id}
          target={{ operation: 'import' }}
          initialContent={current.payload.content}
          onClose={close}
        />
      )}
      {current?.editing && current.kind === 'subscription' && (
        <ImportedSubscription
          key={current.payload.id}
          initialURL={current.payload.content.trim()}
          disabled={connection !== 'live'}
          onClose={close}
        />
      )}
    </AndroidImportContext>
  )
}

function ImportedSubscription({
  initialURL,
  disabled,
  onClose,
}: {
  initialURL: string
  disabled: boolean
  onClose: () => void
}) {
  const actions = useSubscriptionActions()
  const { t } = usePreferences()
  const admitted = useRef(false)
  const add = useRef(actions.add)
  const [submitted, setSubmitted] = useState(false)
  useEffect(() => {
    add.current = actions.add
  }, [actions.add])
  const submit = useCallback((input: SubscriptionInput) => {
    admitted.current = add.current(input)
    if (admitted.current) setSubmitted(true)
    return admitted.current
  }, [])
  const dismiss = useCallback(() => {
    if (!admitted.current) onClose()
  }, [onClose])
  const feedback = <SubscriptionFeedback actions={actions} groups={[]} />
  if (submitted)
    return (
      <Details
        open
        title={t('subscriptionOperation')}
        onOpenChange={(open) => {
          if (!open) onClose()
        }}
      >
        {feedback}
      </Details>
    )
  return (
    <SubscriptionEditor
      initialURL={initialURL}
      busy={actions.operation?.phase === 'pending' || actions.operation?.phase === 'uncertain'}
      disabled={disabled}
      feedback={feedback}
      onSubmit={submit}
      onClose={dismiss}
    />
  )
}
