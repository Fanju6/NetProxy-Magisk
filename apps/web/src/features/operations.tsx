import { useEffect, useState } from 'react'
import { z } from 'zod'
import { RefreshCw, Download, Ellipsis, Trash2, Stethoscope, ListFilter } from 'lucide-react'
import { Details } from '@/components/details'
import { useApi, usePreferences } from '@/app/context'
import { useManagementAction } from '@/api/management'
import { serviceSchema, type ApiRequest } from '@/api/contracts'
import { Choice } from '@/components/choice'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ErrorState, LoadingState } from '@/components/states'
import { useResultToast } from '@/components/ui/toast'

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
  const [view, setView] = useState<'entries' | 'raw'>('entries')
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const [diagnostic, setDiagnostic] = useState<string>()
  const [notice, setNotice] = useState('')
  useResultToast('tools-result', notice, notice)
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
        z.object({ prepared: z.literal(true) }),
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
  const entries =
    kind === 'service'
      ? (logs?.entries ?? []).map((entry) => ({
          ...entry,
          raw: `[${entry.timestamp}] [${entry.level}] [${entry.component}] [${entry.event}] [${entry.result}] [${entry.error_code}] ${entry.message}`,
        }))
      : (logs?.content.split('\n').filter(Boolean) ?? []).map((line) => ({
          timestamp: '',
          level: '',
          component: '',
          event: '',
          result: '',
          error_code: '',
          message: line,
          raw: line,
        }))
  const visible = entries.filter(
    (entry) =>
      (level === 'all' ||
        (kind === 'service'
          ? entry.level.toUpperCase() === level
          : entry.raw.toUpperCase().includes(level))) &&
      entry.raw.toLocaleLowerCase().includes(filter.toLocaleLowerCase()),
  )
  const options = (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            className="size-11 shrink-0"
            aria-label={text('日志选项', 'Log options')}
          />
        }
      >
        <Ellipsis />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel>{text('服务维护', 'Service maintenance')}</DropdownMenuLabel>
          {(['restart', 'reload', 'check'] as const).map((value) => (
            <DropdownMenuItem
              key={value}
              disabled={action.pending || !api.supports(`service.${value}`)}
              onClick={() => (value === 'check' ? void service(value) : setConfirm(value))}
            >
              {value === 'restart'
                ? text('重启服务', 'Restart')
                : value === 'reload'
                  ? text('重载服务', 'Reload')
                  : text('检查配置', 'Check configuration')}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuLabel>{text('诊断', 'Diagnostics')}</DropdownMenuLabel>
          <DropdownMenuItem
            disabled={action.pending || !api.supports('diagnostics.ebpf')}
            onClick={() => void probe()}
          >
            <Stethoscope />
            {text('eBPF 能力检查', 'eBPF capabilities')}
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={action.pending || !api.supports('diagnostics.export')}
            onClick={() => void exportDiagnostics()}
          >
            <Download />
            {text('导出诊断包', 'Export diagnostics')}
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          disabled={action.pending || !api.supports('logs.clear')}
          onClick={() => setConfirm('clear')}
        >
          <Trash2 />
          {text('清空', 'Clear')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
  const feedback = (
    <div className="space-y-3">
      {action.error != null && <ErrorState compact error={action.error} />}
      {diagnostic !== undefined && (
        <pre className="max-h-80 overflow-auto overscroll-contain whitespace-pre-wrap break-words rounded-md border p-4 font-mono text-xs leading-5">
          {diagnostic}
        </pre>
      )}
    </div>
  )
  return (
    <div className="max-w-5xl space-y-5">
      <section className="min-w-0 space-y-3" aria-label={text('日志', 'Logs')}>
        <div className="flex min-w-0 items-center gap-2">
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
          <div className="flex-1" />
          <Button
            variant="ghost"
            size="icon"
            aria-label={text('筛选选项', 'Filter options')}
            onClick={() => setFiltersOpen(true)}
          >
            <ListFilter />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-11 shrink-0"
            aria-label={text('刷新日志', 'Refresh logs')}
            onClick={reload}
          >
            <RefreshCw />
          </Button>
          {options}
        </div>
        <Input
          type="search"
          className="h-11 text-base md:text-sm"
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
          <div
            className="min-h-64 max-h-[60dvh] overflow-auto overscroll-contain border-y"
            tabIndex={0}
            aria-label={text('日志内容', 'Log content')}
          >
            {!visible.length ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                {text('暂无日志内容', 'No log entries')}
              </p>
            ) : view === 'raw' ? (
              <pre className="whitespace-pre py-3 font-mono text-xs leading-5">
                {visible.map((entry) => entry.raw).join('\n')}
              </pre>
            ) : (
              <ol className="divide-y">
                {visible.map((entry, index) => (
                  <li key={index} className="space-y-1.5 py-3">
                    <p className="whitespace-pre-wrap break-words text-sm leading-6">
                      {entry.message}
                    </p>
                    {kind === 'service' && (
                      <div className="flex flex-wrap gap-x-3 gap-y-1 break-all text-xs text-muted-foreground">
                        <time>{entry.timestamp}</time>
                        <span
                          className={
                            ['ERROR', 'FATAL'].includes(entry.level.toUpperCase())
                              ? 'font-medium text-destructive'
                              : 'font-medium'
                          }
                        >
                          {entry.level}
                        </span>
                        <span>{entry.component}</span>
                        <span>{entry.event}</span>
                        {entry.result !== '-' && <span>{entry.result}</span>}
                        {entry.error_code !== '-' && <span>{entry.error_code}</span>}
                      </div>
                    )}
                  </li>
                ))}
              </ol>
            )}
          </div>
        )}
      </section>
      {feedback}
      <Details
        open={filtersOpen}
        onOpenChange={setFiltersOpen}
        title={text('筛选选项', 'Filter options')}
      >
        <div className="settings-group">
          <div className="setting-row">
            <span>{text('显示方式', 'Log view')}</span>
            <Choice
              variant="ghost"
              label={text('显示方式', 'Log view')}
              value={view}
              items={[
                { value: 'entries', label: text('条目', 'Entries') },
                { value: 'raw', label: text('原始', 'Raw') },
              ]}
              onChange={(value) => {
                if (value === 'entries' || value === 'raw') setView(value)
              }}
            />
          </div>
          <div className="setting-row">
            <span>{text('级别', 'Level')}</span>
            <Choice
              variant="ghost"
              label={text('级别', 'Level')}
              value={level}
              onChange={setLevel}
              items={['all', 'DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL'].map((value) => ({
                value,
                label: value === 'all' ? text('全部级别', 'All levels') : value,
              }))}
            />
          </div>
        </div>
      </Details>
      <Dialog
        open={confirm !== undefined}
        onOpenChange={(open) => {
          if (!open) setConfirm(undefined)
        }}
      >
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>{text('确认操作', 'Confirm operation')}</DialogTitle>
            <DialogDescription>
              {confirm === 'clear'
                ? text('清空当前日志，无法撤销。', 'Clear the selected log. This cannot be undone.')
                : text(
                    '操作可能短暂中断代理连接。',
                    'Proxy connections may be interrupted briefly.',
                  )}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirm(undefined)}>
              {text('取消', 'Cancel')}
            </Button>
            <Button
              variant="destructive"
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
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
