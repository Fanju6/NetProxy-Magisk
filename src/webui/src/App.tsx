import { useState, useRef, useEffect, useCallback, type KeyboardEvent, type MouseEvent } from 'react'
import { ctl, ctlJson, shell, inKsu, completions as fetchCompletions } from './exec'
import { complete } from './autocomplete'
import { parseCommandLine } from './command'
import { getHelp } from './help'

const PROMPT = '❯ '
const STATE_MAP: Record<string, { label: string; color: string }> = {
  ready:     { label: '运行中', color: 'var(--good)' },
  stopped:   { label: '未运行', color: 'var(--text-faint)' },
  failed:    { label: '启动失败', color: 'var(--danger)' },
  starting:  { label: '启动中', color: 'var(--medium)' },
  stopping:  { label: '停止中', color: 'var(--medium)' },
  preparing: { label: '准备中', color: 'var(--medium)' },
}
type Line = { t: 'i' | 'o' | 'e' | 'h' | 'c'; text: string }

export default function App() {
  const [lines, setLines] = useState<Line[]>([{ t: 'h', text: getHelp() }])
  const [input, setInput] = useState('')
  const [hist, setHist] = useState<string[]>([])
  const [hIdx, setHIdx] = useState(-1)
  const [busy, setBusy] = useState(false)
  const [groups, setGroups] = useState<string[]>([])
  const [subs, setSubs] = useState<string[]>([])
  const [tabCnt, setTabCnt] = useState(0)
  const [svcState, setSvcState] = useState<string>()
  const outRef = useRef<HTMLDivElement>(null)
  const inRef = useRef<HTMLInputElement>(null)

  const append = (...items: Line[]) => {
    setLines(p => [...p, ...items])
    requestAnimationFrame(() => { outRef.current && (outRef.current.scrollTop = outRef.current.scrollHeight) })
  }

  const refresh = useCallback(async () => {
    try { const { groups: g, subs: s } = await fetchCompletions(); setGroups(g); setSubs(s) } catch {}
  }, [])

  const pollStatus = useCallback(() => {
    ctlJson<{ state?: string }>(['service', 'status']).then(j => j.ok && setSvcState(j.data?.state))
  }, [])

  useEffect(() => { pollStatus(); refresh() }, [pollStatus, refresh])

  useEffect(() => {
    let id: ReturnType<typeof setInterval>
    const start = () => { id = setInterval(pollStatus, 5000) }
    const stop = () => clearInterval(id)
    const onVis = () => document.hidden ? stop() : (pollStatus(), start())
    start()
    document.addEventListener('visibilitychange', onVis)
    return () => { stop(); document.removeEventListener('visibilitychange', onVis) }
  }, [pollStatus])

  const run = (raw: string) => {
    const cmd = raw.trim()
    if (!cmd || busy) return
    setHist(p => [...p, cmd]); setHIdx(-1); setTabCnt(0); setInput('')
    append({ t: 'i', text: PROMPT + cmd })
    setBusy(true)
    requestAnimationFrame(async () => {
      try {
        let out = '', err = '', code = 0
        if (cmd === 'clear') { setLines([]); setBusy(false); return }
        if (cmd === 'exit') { err = 'WebView 中无法退出，请关闭页面' }
        else if (cmd === 'help') { append({ t: 'h', text: getHelp() }) }
        else if (cmd.startsWith('help ')) { append({ t: 'h', text: getHelp(cmd.slice(5).trim()) }) }
        else if (cmd.startsWith('!')) {
          const s = cmd.slice(1).trim()
          if (s) ({ out, err, code } = await shell(s))
        } else {
          const args = parseCommandLine(cmd)
          ;({ out, err, code } = await ctl(args))
          if (['service', 'sub', 'node', 'catalog'].includes(args[0])) {
            refresh()
            if (args[0] === 'service') pollStatus()
          }
        }
        if (out) append({ t: 'o', text: out })
        if (err) append({ t: 'e', text: err })
        if (code !== 0 && !out && !err) append({ t: 'e', text: `退出码: ${code}` })
      } catch (e: any) { append({ t: 'e', text: `异常: ${e?.message || e}` }) }
      finally { setBusy(false) }
    })
  }

  const doComplete = () => {
    if (busy) return
    const r = complete(input, groups, subs)
    if (!r.candidates.length) return
    if (r.candidates.length === 1) { setInput(r.completed); setTabCnt(0) }
    else if (tabCnt === 0) { setInput(r.completed); setTabCnt(1) }
    else { append({ t: 'c', text: r.candidates.join('  ') }); setTabCnt(0) }
  }

  // 历史导航：上一条 / 下一条，键盘与虚拟按键共用
  const histPrev = () => {
    if (busy || !hist.length) return
    const i = hIdx === -1 ? hist.length - 1 : Math.max(0, hIdx - 1)
    setHIdx(i); setInput(hist[i]); setTabCnt(0)
  }

  const histNext = () => {
    if (busy || hIdx === -1) return
    const i = hIdx + 1
    if (i >= hist.length) { setHIdx(-1); setInput('') } else { setHIdx(i); setInput(hist[i]) }
    setTabCnt(0)
  }

  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (busy && e.key === 'Enter') { e.preventDefault(); return }
    if (e.key === 'Enter') { e.preventDefault(); run(input); return }
    if (e.key === 'Tab') { e.preventDefault(); doComplete(); return }
    if (e.key === 'ArrowUp') { e.preventDefault(); histPrev(); return }
    if (e.key === 'ArrowDown') { e.preventDefault(); histNext(); return }
    setTabCnt(0)
  }

  const svc = svcState ? STATE_MAP[svcState] || { label: svcState, color: 'var(--text-faint)' } : { label: '检测中', color: 'var(--text-faint)' }
  const showStatus = (e: MouseEvent) => { e.stopPropagation(); run('service status') }

  return (
    <div className="term" onClick={() => inRef.current?.focus()}>
      <div className="out" ref={outRef}>
        {lines.map((l, i) =>
          l.t === 'h' ? <pre key={i} className="help">{l.text}</pre> :
          l.t === 'c' ? <div key={i} className="cands">{l.text.split('  ').map((c, j) => <span key={j}>{c}</span>)}</div> :
          <pre key={i} className={l.t}>{l.text}</pre>
        )}
        {busy && <pre className="busy"><span className="spinner" /></pre>}
      </div>
      <div className="iline">
        <span className="prompt">{PROMPT}</span>
        <input
          ref={inRef}
          className="inp"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={onKey}
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          spellCheck={false}
        />
      </div>
      <div className="vk">
        <button className="vkbtn" onClick={doComplete} disabled={busy}>Tab</button>
        <button className="vkbtn" onClick={() => { histPrev(); inRef.current?.focus() }} disabled={busy}>↑</button>
        <button className="vkbtn" onClick={() => { histNext(); inRef.current?.focus() }} disabled={busy}>↓</button>
        <button className="vkbtn run" onClick={() => run(input)} disabled={busy}>{busy ? '…' : '↵'}</button>
      </div>
      <div className="bar">
        <span className="bar-status" onClick={showStatus}>服务: <b style={{ color: svc.color }}>{svc.label}</b></span>
        <span className="dim">{inKsu ? 'KernelSU' : '预览'}</span>
      </div>
    </div>
  )
}
