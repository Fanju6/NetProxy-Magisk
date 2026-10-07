import { complete, replaceCompletion } from './autocomplete'
import { parseCommandLine } from './command'
import { decodeCtlResult } from './contract'
import { ctl, ctlJson, shell, inKsu, completions as fetchCompletions } from './exec'
import { formatCtlOutput } from './format'
import { getHelp } from './help'
import { createPoller } from './polling'
import './style.css'

// WebUI 持有自身的奶屁龙素材，模块构建不依赖 docs 目录。
const dragonDigRaised = './mascots/dragon/dig-raised.svg'
const dragonDigStrike = './mascots/dragon/dig-strike.svg'
const dragonSleepFirst = './mascots/dragon/sleep-a.svg'
const dragonSleepSecond = './mascots/dragon/sleep-b.svg'
const dragonFailedFirst = './mascots/dragon/failed-a.svg'
const dragonFailedSecond = './mascots/dragon/failed-b.svg'
const dragonWaitingFirst = './mascots/dragon/waiting-a.svg'
const dragonWaitingSecond = './mascots/dragon/waiting-b.svg'

type MascotState = { label: string; first: string; second: string }

const STATE_MAP: Record<string, MascotState> = {
  ready: { label: '服务运行中', first: dragonDigRaised, second: dragonDigStrike },
  stopped: { label: '服务已停止', first: dragonSleepFirst, second: dragonSleepSecond },
  failed: { label: '服务启动失败', first: dragonFailedFirst, second: dragonFailedSecond },
  preparing: { label: '服务准备中', first: dragonWaitingFirst, second: dragonWaitingSecond },
  starting: { label: '服务启动中', first: dragonWaitingFirst, second: dragonWaitingSecond },
  stopping: { label: '服务停止中', first: dragonWaitingFirst, second: dragonWaitingSecond },
}

function byId<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id)
  if (!element) throw new Error(`缺少页面元素: ${id}`)
  return element as T
}

const output = byId<HTMLElement>('output')
const welcome = byId<HTMLElement>('welcome')
const form = byId<HTMLFormElement>('command-form')
const input = byId<HTMLInputElement>('command-input')
const suggestions = byId<HTMLElement>('suggestions')
const completeButton = byId<HTMLButtonElement>('complete')
const previousButton = byId<HTMLButtonElement>('history-prev')
const nextButton = byId<HTMLButtonElement>('history-next')
const runButton = byId<HTMLButtonElement>('run')
const copyButton = byId<HTMLButtonElement>('copy')
const clearButton = byId<HTMLButtonElement>('clear')
const latestButton = byId<HTMLButtonElement>('latest')
const mascot = byId<HTMLElement>('mascot')
const mascotAction = byId<HTMLImageElement>('mascot-action')
const mascotImpact = byId<HTMLImageElement>('mascot-impact')
const announcement = byId<HTMLElement>('announcement')
const serviceState = byId<HTMLElement>('service-state')
const entryTemplate = byId<HTMLTemplateElement>('command-entry')
const history: string[] = []
let historyIndex = -1
let draft = ''
let busy = false
let composing = false
let followOutput = true
let scrollFrame = 0
let lastResult = ''
let knownGroups: string[] = []
let knownSubscriptions: string[] = []
let knownModes: string[] = []
let completionRevision = 0
let mascotActionSource = ''
let mascotImpactSource = ''

function announce(message: string) {
  announcement.textContent = message
}

function updateMascotLayer(image: HTMLImageElement, source: string | undefined, currentSource: string): string {
  image.hidden = !source
  if (source && source !== currentSource) image.src = source
  return source ?? currentSource
}

function updateMascot(state?: MascotState, stateName?: string) {
  const label = state?.label ?? '服务状态不可用'
  if (serviceState.textContent !== label) serviceState.textContent = label
  if (stateName) mascot.dataset.state = stateName
  else delete mascot.dataset.state
  if (state) mascot.dataset.animated = 'true'
  else delete mascot.dataset.animated
  mascotActionSource = updateMascotLayer(mascotAction, state?.first, mascotActionSource)
  mascotImpactSource = updateMascotLayer(mascotImpact, state?.second, mascotImpactSource)
}

function updateControls() {
  runButton.disabled = busy || !input.value.trim()
  completeButton.disabled = busy
  previousButton.disabled = busy || !history.length || historyIndex === 0
  nextButton.disabled = busy || historyIndex === -1
  clearButton.disabled = busy || !output.querySelector('.entry')
  copyButton.disabled = !lastResult
}

function scrollToLatest(force = false) {
  if (force) followOutput = true
  if (scrollFrame) cancelAnimationFrame(scrollFrame)
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = 0
    if (followOutput) output.scrollTop = output.scrollHeight
    latestButton.hidden = output.scrollHeight - output.clientHeight - output.scrollTop < 24
  })
}

function closeSuggestions() {
  suggestions.hidden = true
  suggestions.replaceChildren()
}

function focusInput() {
  input.focus({ preventScroll: true })
  input.setSelectionRange(input.value.length, input.value.length)
}

function setInput(value: string) {
  input.value = value
  historyIndex = -1
  closeSuggestions()
  updateControls()
  focusInput()
}

async function refreshCompletions() {
  const revision = ++completionRevision
  try {
    const result = await fetchCompletions()
    if (revision !== completionRevision) return
    knownGroups = result.groups
    knownSubscriptions = result.subs
  } catch {
    // 补全失败不影响终端命令执行。
  }
}

const statusPoller = createPoller(
  () => ctlJson<{ state?: string; available_outbound_modes?: string[] }>(['service', 'status']),
  result => {
    const stateName = result.ok ? result.data?.state : undefined
    const state = stateName ? STATE_MAP[stateName] : undefined
    updateMascot(state, stateName)
    if (result.ok && Array.isArray(result.data?.available_outbound_modes)) knownModes = result.data.available_outbound_modes
  },
)

function clearOutput() {
  output.replaceChildren(welcome)
  lastResult = ''
  copyButton.textContent = '复制结果'
  latestButton.hidden = true
  followOutput = true
  closeSuggestions()
  updateControls()
  announce('终端输出已清空')
}

function append(entry: HTMLElement, kind: 'o' | 'e' | 'help', text: string) {
  if (!text) return
  entry.append(Object.assign(document.createElement('pre'), { className: kind, textContent: text }))
}

async function run(raw: string) {
  const command = raw.trim()
  if (!command || busy || composing) return
  if (history[history.length - 1] !== command) history.push(command)
  if (history.length > 100) history.shift()
  historyIndex = -1
  input.value = ''
  draft = ''
  closeSuggestions()
  if (command === 'clear') {
    clearOutput()
    return
  }

  const entry = entryTemplate.content.firstElementChild!.cloneNode(true) as HTMLElement
  entry.querySelector('pre')!.textContent = '❯ ' + command
  const state = entry.querySelector<HTMLElement>('.entry-state')!
  output.append(entry)
  // 只在当前页面保留有限记录，不持久化可能含凭据的输入和输出。
  const entries = output.querySelectorAll('.entry')
  if (entries.length > 100) entries[0].remove()
  busy = true
  output.setAttribute('aria-busy', 'true')
  statusPoller.setActive(false)
  updateControls()
  scrollToLatest(true)
  announce('命令执行中')

  let failed = false
  let resultText = ''
  try {
    let out = ''
    let err = ''
    let code = 0
    let kind: 'o' | 'help' = 'o'
    if (command === 'exit') {
      err = '请关闭模块 WebUI 页面退出终端'
      code = 1
    } else if (command === 'help' || command.startsWith('help ')) {
      out = getHelp(command.slice(4).trim())
      kind = 'help'
    } else if (command.startsWith('!')) {
      const value = command.slice(1).trim()
      if (value) ({ out, err, code } = await shell(value))
      else { err = '请在 ! 后输入 Shell 命令'; code = 1 }
    } else {
      const args = parseCommandLine(command)
      const result = await ctl(args)
      ;({ out, err, code } = result)
      // 命令仍显示原有结果，但成功标记必须同时满足 JSON 契约与退出码。
      const decoded = decodeCtlResult(result)
      failed = !decoded.ok
      if (failed && !err && (decoded.code.startsWith('transport.') || !out)) err = decoded.message
      if (!args.includes('--raw')) out = formatCtlOutput(out)
      if (['service', 'sub', 'node', 'catalog'].includes(args[0])) void refreshCompletions()
    }
    failed ||= code !== 0
    append(entry, kind, out)
    append(entry, 'e', err)
    resultText = [out, err].filter(Boolean).join('\n')
  } catch (error) {
    failed = true
    resultText = `异常: ${error instanceof Error ? error.message : String(error)}`
    append(entry, 'e', resultText)
  } finally {
    lastResult = resultText
    copyButton.textContent = '复制结果'
    entry.dataset.state = failed ? 'error' : 'success'
    state.textContent = failed ? '失败' : '完成'
    busy = false
    output.setAttribute('aria-busy', 'false')
    updateControls()
    scrollToLatest()
    announce(failed ? '命令执行失败，请查看输出' : '命令执行完成')
    statusPoller.setActive(!document.hidden)
  }
}

function completeInput(): boolean {
  if (busy || composing) return false
  const result = complete(input.value, knownGroups, knownSubscriptions, knownModes)
  if (!result.candidates.length) { closeSuggestions(); return false }
  setInput(result.completed)
  if (result.candidates.length > 1) {
    for (const candidate of result.candidates) {
      const button = Object.assign(document.createElement('button'), { type: 'button', textContent: candidate })
      button.dataset.completion = candidate
      suggestions.append(button)
    }
    suggestions.hidden = false
    announce(`${result.candidates.length} 个补全候选，向下键选择`)
  }
  return true
}

function moveHistory(direction: -1 | 1) {
  if (busy || !history.length || (direction === 1 && historyIndex === -1)) return
  if (historyIndex === -1) { draft = input.value; historyIndex = history.length }
  historyIndex = Math.max(0, historyIndex + direction)
  if (historyIndex >= history.length) { historyIndex = -1; input.value = draft }
  else input.value = history[historyIndex]
  closeSuggestions()
  updateControls()
  focusInput()
}

form.addEventListener('submit', event => { event.preventDefault(); void run(input.value) })
input.addEventListener('compositionstart', () => { composing = true })
input.addEventListener('compositionend', () => { composing = false; updateControls() })
input.addEventListener('input', () => {
  historyIndex = -1
  closeSuggestions()
  updateControls()
})
input.addEventListener('keydown', event => {
  if (event.isComposing || composing || event.keyCode === 229) return
  if (event.key === 'Tab' && !event.shiftKey && completeInput()) event.preventDefault()
  else if (event.key === 'ArrowDown' && !suggestions.hidden) {
    event.preventDefault()
    suggestions.querySelector<HTMLButtonElement>('button')?.focus()
  } else if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
    event.preventDefault()
    moveHistory(event.key === 'ArrowUp' ? -1 : 1)
  } else if (event.key === 'Enter') {
    event.preventDefault()
    if (!event.repeat) void run(input.value)
  } else if (event.key === 'l' && event.ctrlKey && !busy) {
    event.preventDefault()
    clearOutput()
  }
})
suggestions.addEventListener('keydown', event => {
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
  event.preventDefault()
  const buttons = Array.from(suggestions.querySelectorAll('button'))
  const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
  const next = index + (event.key === 'ArrowDown' ? 1 : -1)
  if (next < 0) focusInput()
  else buttons[Math.min(next, buttons.length - 1)]?.focus()
})
document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && !suggestions.hidden) { closeSuggestions(); focusInput() }
})
document.addEventListener('pointerdown', event => {
  if (!form.contains(event.target as Node)) closeSuggestions()
})
document.addEventListener('click', event => {
  const button = (event.target as Element).closest<HTMLButtonElement>('[data-command], [data-completion]')
  if (!button || busy) return
  setInput(button.dataset.command ?? replaceCompletion(input.value, button.dataset.completion!))
})
completeButton.addEventListener('click', completeInput)
previousButton.addEventListener('click', () => moveHistory(-1))
nextButton.addEventListener('click', () => moveHistory(1))
clearButton.addEventListener('click', clearOutput)
copyButton.addEventListener('click', async () => {
  const value = lastResult
  let copied = false
  try {
    await navigator.clipboard.writeText(value)
    copied = true
  } catch {}
  if (value !== lastResult) return
  copyButton.textContent = copied ? '已复制' : '复制失败'
  announce(copied ? '最近一次命令结果已复制' : '复制失败，请选中输出手动复制')
})
output.addEventListener('scroll', () => {
  followOutput = output.scrollHeight - output.clientHeight - output.scrollTop < 24
  latestButton.hidden = followOutput
}, { passive: true })
latestButton.addEventListener('click', () => scrollToLatest(true))
const resizeObserver = new ResizeObserver(() => scrollToLatest())
resizeObserver.observe(output)
document.addEventListener('visibilitychange', () => statusPoller.setActive(!document.hidden && !busy))
window.addEventListener('pagehide', () => {
  statusPoller.setActive(false)
  if (scrollFrame) cancelAnimationFrame(scrollFrame)
  resizeObserver.disconnect()
})
window.addEventListener('pageshow', () => {
  resizeObserver.observe(output)
  statusPoller.setActive(!document.hidden && !busy)
})

byId('environment').hidden = !(import.meta.env.DEV && !inKsu)
void refreshCompletions()
updateControls()
statusPoller.setActive(!document.hidden)
