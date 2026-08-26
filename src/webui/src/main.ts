import { complete } from './autocomplete'
import { parseCommandLine } from './command'
import { ctl, ctlJson, shell, inKsu, completions as fetchCompletions } from './exec'
import { formatCtlOutput } from './format'
import { getHelp } from './help'
import './style.css'

const PROMPT = '❯ '
const STATE_MAP: Record<string, { label: string; color: string }> = {
  ready: { label: '运行中', color: 'var(--good)' },
  stopped: { label: '未运行', color: 'var(--text-faint)' },
  failed: { label: '启动失败', color: 'var(--danger)' },
  starting: { label: '启动中', color: 'var(--medium)' },
  stopping: { label: '停止中', color: 'var(--medium)' },
  preparing: { label: '准备中', color: 'var(--medium)' },
}

function byId<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id)
  if (!element) throw new Error(`缺少页面元素: ${id}`)
  return element as T
}

const terminal = byId<HTMLDivElement>('terminal')
const output = byId<HTMLDivElement>('output')
const input = byId<HTMLInputElement>('command-input')
const completeButton = byId<HTMLButtonElement>('complete')
const previousButton = byId<HTMLButtonElement>('history-prev')
const nextButton = byId<HTMLButtonElement>('history-next')
const runButton = byId<HTMLButtonElement>('run')
const serviceStatus = byId<HTMLSpanElement>('service-status')
const serviceState = byId<HTMLElement>('service-state')
const environment = byId<HTMLSpanElement>('environment')
const buttons = [completeButton, previousButton, nextButton, runButton]
const busyLine = document.createElement('pre')
busyLine.className = 'busy'
busyLine.append(Object.assign(document.createElement('span'), { className: 'spinner' }))

let history: string[] = []
let historyIndex = -1
let tabCount = 0
let busy = false
let knownGroups: string[] = []
let knownSubscriptions: string[] = []

function scrollToBottom() {
  requestAnimationFrame(() => { output.scrollTop = output.scrollHeight })
}

function append(kind: 'i' | 'o' | 'e' | 'help', text: string) {
  const line = document.createElement('pre')
  line.className = kind
  line.textContent = text
  output.insertBefore(line, busyLine.isConnected ? busyLine : null)
  scrollToBottom()
}

function appendCandidates(candidates: string[]) {
  const line = document.createElement('div')
  line.className = 'cands'
  for (const candidate of candidates) {
    const item = document.createElement('span')
    item.textContent = candidate
    line.append(item)
  }
  output.insertBefore(line, busyLine.isConnected ? busyLine : null)
  scrollToBottom()
}

function setBusy(value: boolean) {
  busy = value
  for (const button of buttons) button.disabled = value
  runButton.textContent = value ? '…' : '↵'
  if (value) output.append(busyLine)
  else busyLine.remove()
  scrollToBottom()
}

async function refreshCompletions() {
  try {
    const result = await fetchCompletions()
    knownGroups = result.groups
    knownSubscriptions = result.subs
  } catch {
    // 补全失败不影响终端命令执行。
  }
}

function renderServiceState(state?: string) {
  const status = state ? STATE_MAP[state] || { label: state, color: 'var(--text-faint)' } : { label: '检测中', color: 'var(--text-faint)' }
  serviceState.textContent = status.label
  serviceState.style.color = status.color
}

async function pollStatus() {
  const result = await ctlJson<{ state?: string }>(['service', 'status'])
  if (result.ok) renderServiceState(result.data?.state)
}

async function run(raw: string) {
  const command = raw.trim()
  if (!command || busy) return

  history = [...history, command]
  historyIndex = -1
  tabCount = 0
  input.value = ''
  append('i', PROMPT + command)
  setBusy(true)
  await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))

  try {
    let out = ''
    let err = ''
    let code = 0

    if (command === 'clear') {
      output.replaceChildren()
      return
    }
    if (command === 'exit') {
      err = 'WebView 中无法退出，请关闭页面'
    } else if (command === 'help') {
      append('help', getHelp())
    } else if (command.startsWith('help ')) {
      append('help', getHelp(command.slice(5).trim()))
    } else if (command.startsWith('!')) {
      const value = command.slice(1).trim()
      if (value) ({ out, err, code } = await shell(value))
    } else {
      const args = parseCommandLine(command)
      ;({ out, err, code } = await ctl(args))
      if (!args.includes('--raw')) out = formatCtlOutput(out)
      if (['service', 'sub', 'node', 'catalog'].includes(args[0])) {
        void refreshCompletions()
        if (args[0] === 'service') void pollStatus()
      }
    }

    if (out) append('o', out)
    if (err) append('e', err)
    if (code !== 0 && !out && !err) append('e', `退出码: ${code}`)
  } catch (error) {
    append('e', `异常: ${error instanceof Error ? error.message : String(error)}`)
  } finally {
    setBusy(false)
  }
}

function completeInput() {
  if (busy) return
  const result = complete(input.value, knownGroups, knownSubscriptions)
  if (!result.candidates.length) return
  if (result.candidates.length === 1) {
    input.value = result.completed
    tabCount = 0
  } else if (tabCount === 0) {
    input.value = result.completed
    tabCount = 1
  } else {
    appendCandidates(result.candidates)
    tabCount = 0
  }
}

function historyPrevious() {
  if (busy || !history.length) return
  historyIndex = historyIndex === -1 ? history.length - 1 : Math.max(0, historyIndex - 1)
  input.value = history[historyIndex]
  tabCount = 0
}

function historyNext() {
  if (busy || historyIndex === -1) return
  historyIndex += 1
  if (historyIndex >= history.length) {
    historyIndex = -1
    input.value = ''
  } else {
    input.value = history[historyIndex]
  }
  tabCount = 0
}

input.addEventListener('keydown', event => {
  if (event.key === 'Enter') {
    event.preventDefault()
    if (!busy) void run(input.value)
  } else if (event.key === 'Tab') {
    event.preventDefault()
    completeInput()
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    historyPrevious()
  } else if (event.key === 'ArrowDown') {
    event.preventDefault()
    historyNext()
  } else {
    tabCount = 0
  }
})

terminal.addEventListener('click', () => input.focus())
completeButton.addEventListener('click', completeInput)
previousButton.addEventListener('click', () => { historyPrevious(); input.focus() })
nextButton.addEventListener('click', () => { historyNext(); input.focus() })
runButton.addEventListener('click', () => { void run(input.value) })
serviceStatus.addEventListener('click', event => { event.stopPropagation(); void run('service status') })

let pollTimer: number | undefined
function stopPolling() {
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
  pollTimer = undefined
}
function startPolling() {
  stopPolling()
  pollTimer = window.setInterval(() => { void pollStatus() }, 5000)
}

document.addEventListener('visibilitychange', () => {
  if (document.hidden) stopPolling()
  else { void pollStatus(); startPolling() }
})

environment.textContent = inKsu ? 'KernelSU' : '预览'
append('help', getHelp())
void pollStatus()
void refreshCompletions()
startPolling()
