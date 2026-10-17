import { useEffect, useState } from 'react'
import { z } from 'zod'
import { RefreshCw, Download, Trash2, Stethoscope } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { useManagementAction } from '@/api/management'
import { serviceSchema, type ApiRequest } from '@/api/contracts'
import { Details } from '@/components/details'
import { Choice } from '@/components/choice'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ErrorState, LoadingState } from '@/components/states'

const logsSchema = z.object({
  kind: z.enum(['service', 'core']),
  content: z.string(),
  entries: z.array(
    z.object({
      timestamp: z.string(),
      level: z.string(),
      component: z.string(),
      event: z.string(),
      result: z.string(),
      error_code: z.string(),
      message: z.string(),
    }),
  ),
})
type Logs = z.infer<typeof logsSchema>
export function OperationsPage() {
  const api = useApi()
  const { preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const action = useManagementAction()
  const [kind, setKind] = useState<'service' | 'core'>('service')
  const [logs, setLogs] = useState<Logs>()
  const [error, setError] = useState<unknown>(null)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const [level, setLevel] = useState('all')
  const [refresh, setRefresh] = useState(0)
  const [diagnostic, setDiagnostic] = useState<string>()
  const [notice, setNotice] = useState('')
  const [confirm, setConfirm] = useState<'clear' | 'restart' | 'reload'>()
  useEffect(() => {
    const controller = new AbortController()
    void api
      .request(
        { method: 'GET', path: '/logs', query: { kind, lines: '500' } },
        logsSchema,
        controller.signal,
      )
      .then((value) => {
        if (!controller.signal.aborted) setLogs(value)
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(failure)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [api, kind, refresh])
  const reload = () => {
    setLoading(true)
    setError(null)
    setLogs(undefined)
    setRefresh((value) => value + 1)
  }
  const service = async (value: 'restart' | 'reload' | 'check') => {
    setNotice('')
    try {
      const result = await action.run(
        { method: 'POST', path: `/service/${value}` },
        z.object({ action: z.literal(value), status: serviceSchema }),
      )
      setNotice(`${text('服务状态', 'Service status')}: ${result.status.state}`)
      reload()
    } catch {
      /* The single action error below retains persisted and uncertain results. */
    }
  }
  const probe = async () => {
    try {
      const result = await action.run(
        { method: 'POST', path: '/diagnostics/ebpf', body: { mode: 'configured', raw: false } },
        z.object({ content: z.string() }),
      )
      setDiagnostic(result.content)
    } catch (failure) {
      const content =
        failure && typeof failure === 'object' && 'data' in failure
          ? z.object({ content: z.string() }).safeParse(failure.data)
          : undefined
      if (content?.success) setDiagnostic(content.data.content)
    }
  }
  const exportDiagnostics = async () => {
    try {
      await action.run(
        { method: 'GET', path: '/diagnostics/export' },
        z.object({ shared: z.literal(true) }),
      )
      setNotice(
        text(
          api.transport.kind === 'mock'
            ? '演示：诊断包已准备'
            : '诊断包已准备，请在系统分享面板中保存或发送',
          api.transport.kind === 'mock'
            ? 'Demo: diagnostics prepared'
            : 'Diagnostics prepared. Save or send from the system share sheet.',
        ),
      )
    } catch {
      /* Report through the action error. */
    }
  }
  const lines =
    kind === 'service' && logs?.entries.length
      ? logs.entries
          .filter((entry) => level === 'all' || entry.level.toUpperCase() === level)
          .map(
            (entry) =>
              `[${entry.timestamp}] [${entry.level}] [${entry.component}] [${entry.event}] [${entry.result}] [${entry.error_code}] ${entry.message}`,
          )
      : (logs?.content.split('\n') ?? []).filter(
          (line) => level === 'all' || line.toUpperCase().includes(level),
        )
  const visible = lines
    .filter((line) => line.toLocaleLowerCase().includes(filter.toLocaleLowerCase()))
    .join('\n')
  return (
    <div className="max-w-5xl space-y-6">
      <section className="space-y-3">
        <h2 className="section-label">{text('服务维护', 'Service maintenance')}</h2>
        <div className="flex flex-wrap gap-2">
          {(['restart', 'reload', 'check'] as const).map((value) => (
            <Button
              key={value}
              variant="outline"
              disabled={action.pending || !api.supports(`service.${value}`)}
              onClick={() => (value === 'check' ? void service(value) : setConfirm(value))}
            >
              {value === 'restart'
                ? text('重启服务', 'Restart')
                : value === 'reload'
                  ? text('重载服务', 'Reload')
                  : text('检查配置', 'Check configuration')}
            </Button>
          ))}
          <Button
            variant="outline"
            disabled={action.pending || !api.supports('diagnostics.ebpf')}
            onClick={() => void probe()}
          >
            <Stethoscope />
            {text('eBPF 能力检查', 'eBPF capabilities')}
          </Button>
          <Button
            variant="outline"
            disabled={action.pending || !api.supports('diagnostics.export')}
            onClick={() => void exportDiagnostics()}
          >
            <Download />
            {text('导出诊断包', 'Export diagnostics')}
          </Button>
        </div>
        {notice && (
          <p role="status" className="text-sm text-muted-foreground">
            {notice}
          </p>
        )}
        {action.error != null && <ErrorState compact error={action.error} />}
        {diagnostic !== undefined && (
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-md border p-3 text-xs">
            {diagnostic}
          </pre>
        )}
      </section>
      <section className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Choice
            label={text('日志', 'Logs')}
            value={kind}
            items={[
              { value: 'service', label: text('模块服务', 'Module service') },
              { value: 'core', label: 'sing-box' },
            ]}
            onChange={(value) => {
              if (value === 'service' || value === 'core') {
                setKind(value)
                reload()
              }
            }}
          />
          <Choice
            label={text('级别', 'Level')}
            value={level}
            items={['all', 'DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL'].map((value) => ({
              value,
              label: value === 'all' ? text('全部级别', 'All levels') : value,
            }))}
            onChange={setLevel}
          />
          <Button
            variant="outline"
            size="icon"
            aria-label={text('刷新日志', 'Refresh logs')}
            onClick={reload}
          >
            <RefreshCw />
          </Button>
          <Button
            variant="outline"
            disabled={action.pending || !api.supports('logs.clear')}
            onClick={() => setConfirm('clear')}
          >
            <Trash2 />
            {text('清空', 'Clear')}
          </Button>
        </div>
        <Input
          type="search"
          aria-label={text('筛选日志', 'Filter logs')}
          placeholder={text('筛选日志', 'Filter logs')}
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        {loading ? (
          <LoadingState />
        ) : error != null ? (
          <ErrorState error={error} retry={reload} />
        ) : (
          <pre
            className="min-h-48 max-h-[55dvh] overflow-auto whitespace-pre rounded-md border p-3 font-mono text-xs leading-5"
            tabIndex={0}
          >
            {visible || text('暂无日志内容', 'No log entries')}
          </pre>
        )}
      </section>
      <Details
        open={confirm !== undefined}
        onOpenChange={(open) => {
          if (!open) setConfirm(undefined)
        }}
        title={text('确认操作', 'Confirm operation')}
        description={
          confirm === 'clear'
            ? text('清空当前日志，无法撤销。', 'Clear the selected log. This cannot be undone.')
            : text('操作可能短暂中断代理连接。', 'Proxy connections may be interrupted briefly.')
        }
      >
        <Button
          disabled={action.pending}
          onClick={() => {
            const target = confirm
            setConfirm(undefined)
            if (target === 'clear')
              void action
                .run(
                  { method: 'DELETE', path: '/logs', query: { kind } } satisfies ApiRequest,
                  z.object({ kind: z.literal(kind) }),
                )
                .then(reload)
                .catch(() => {})
            else if (target) void service(target)
          }}
        >
          {text('确认', 'Confirm')}
        </Button>
      </Details>
    </div>
  )
}
