interface CaptureBody {
  data: string
  format: string
  size?: number
  captured_size?: number
  content_type?: string
  content_encoding?: string
  decoded?: boolean
  truncated: boolean
  blob?: boolean
  filename?: string
  content_range?: string
}

interface CaptureMessage {
  headers: Record<string, string[]> | null
  headers_truncated: boolean
  body: CaptureBody
}

interface RewriteMatch {
  rule_index: number
  pattern: string
  phase: string
  state: string
  actions?: string[]
  redirect?: string
  original_status?: number
  duration_us: number
  headers?: { name: string, before?: string[], after?: string[], truncated: boolean }[]
  body?: { before: number, after: number }
  error?: string
}

export interface CaptureDetail {
  method: string
  host: string
  status: number
  outbound: string
  duration_ms: number
  passthrough_reason?: string
  source_app?: { uid: number, user_id: number, package?: string }
  exit?: { inbound?: string, chain?: { tag: string, type: string }[], remote?: string, package?: string, uid?: number, process?: string }
  http?: {
    url: string, url_truncated: boolean, request: CaptureMessage, response: CaptureMessage, rewrites?: RewriteMatch[]
    websocket?: { frames: { direction: string, opcode: number, size: number, text?: string, truncated: boolean }[], truncated: boolean }
  }
}

const states: Record<string, string> = {
  applied: '已重写', no_change: '未修改', shadowed: '前序规则优先', not_reached: '未进入阶段', error: '重写错误',
}
const errors: Record<string, string> = {
  busy: '脚本执行器繁忙', deadline: '脚本超时', body_unavailable: '正文无法重写',
  invalid_result: '脚本结果无效', script_error: '脚本执行失败',
}
const passthroughReasons: Record<string, string> = {
  certificate_rejected: '证书固定或不信任 · 已跳过', excluded_host: '主机在排除名单中',
  host_not_selected: '主机未选中解密', decryption_disabled: 'HTTPS 解密已关闭',
  decryption_unavailable: 'HTTPS 解密不可用', non_tls: '非 TLS 流量',
  sniff_timeout: '协议识别超时', unsupported_alpn: '不支持的 TLS 应用协议',
  protocol_unidentified: '协议无法识别', tls_hello_unavailable: '无法识别 TLS ClientHello',
  tls_host_unavailable: '无法识别 TLS 主机',
}

function element<K extends keyof HTMLElementTagNameMap>(tag: K, text?: string): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  if (text !== undefined) node.textContent = text
  return node
}

function section(title: string, open = false): HTMLDetailsElement {
  const node = element('details')
  node.open = open
  node.append(element('summary', title))
  return node
}

function line(parent: HTMLElement, key: string, value: string | number | undefined) {
  if (value === undefined || value === '') return
  const row = element('div')
  row.className = 'capture-line'
  row.append(element('strong', key), element('span', String(value)))
  parent.append(row)
}

function copy(label: string, value: string) {
  const button = element('button', label)
  button.type = 'button'
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(value)
      button.textContent = '已复制'
    } catch {
      button.textContent = '复制失败'
    }
  })
  return button
}

export function formatCaptureBody(data: string, type?: string): string {
  const mime = type?.split(';', 1)[0].trim().toLowerCase()
  try {
    if (mime === 'application/json' || mime?.endsWith('+json')) return JSON.stringify(JSON.parse(data), null, 2)
    if (mime === 'application/x-www-form-urlencoded') {
      return Array.from(new URLSearchParams(data), ([key, value]) => `${key} = ${value}`).join('\n')
    }
    if (mime && (['text/html', 'text/xml', 'application/xml', 'image/svg+xml'].includes(mime) || mime.endsWith('+xml'))) {
      return data.replace(/>\s*(?=<)/g, '>\n')
    }
    return data
  } catch {
    return data
  }
}

export function captureHex(data: string): string {
  try {
    return Array.from(atob(data), (value, index) =>
      `${index ? index % 16 === 0 ? '\n' : ' ' : ''}${value.charCodeAt(0).toString(16).padStart(2, '0')}`).join('')
  } catch {
    return data
  }
}

function message(title: string, data: CaptureMessage) {
  const node = section(title)
  const headers = section('头字段', true)
  const lines: string[] = []
  for (const [name, values] of Object.entries(data.headers ?? {})) {
    for (const value of values) {
      const full = `${name}: ${value}`
      const row = section(full)
      row.append(copy('复制名称', name), copy('复制值', value), copy('复制整条', full))
      headers.append(row)
      lines.push(full)
    }
  }
  if (lines.length) headers.append(copy('复制所有头字段', lines.join('\n')))
  else headers.append(element('p', '没有头字段'))
  if (data.headers_truncated) headers.append(element('p', '部分头字段已截断'))
  node.append(headers)

  const body = data.body
  const payload = section(['正文', body.content_type, body.size === undefined ? '' : `${body.size} 字节`,
    body.content_encoding, body.decoded ? '已解压' : '', body.format === 'base64' ? 'Base64' : '', body.truncated ? '已截断' : ''].filter(Boolean).join(' · '))
  if (body.truncated || body.blob) line(payload, '已捕获大小', body.captured_size)
  line(payload, '文件名', body.filename)
  line(payload, '响应分段', body.content_range)
  if (body.blob) payload.append(element('p', '二进制正文已保存在本地；可在 Android 管理器中预览或导出已保留的字节。'))
  else if (!body.data) payload.append(element('p', body.format === 'binary' ? '未保留二进制正文' : '没有正文'))
  else {
    payload.addEventListener('toggle', () => {
      if (!payload.open || payload.querySelector('pre')) return
      const pretty = body.format === 'text' ? formatCaptureBody(body.data, body.content_type) : captureHex(body.data)
      const code = element('pre', pretty)
      if (pretty !== body.data) {
        const toggle = element('button', '显示原始正文')
        toggle.type = 'button'
        toggle.addEventListener('click', () => {
          const raw = code.textContent === pretty
          code.textContent = raw ? body.data : pretty
          toggle.textContent = raw ? '格式化正文' : '显示原始正文'
        })
        payload.append(toggle)
      }
      payload.append(copy('复制正文', body.data), code)
    })
  }
  node.append(payload)
  return node
}

export function renderCaptureDetail(data: CaptureDetail): HTMLElement {
  const article = element('article')
  article.className = 'capture-detail'
  const overview = section('概览与代理出口', true)
  line(overview, '会话', `${data.method} ${data.host}${data.status > 0 ? ` · ${data.status}` : ''}`)
  line(overview, '耗时', `${data.duration_ms} ms`)
  line(overview, '完整 URL', data.http?.url)
  if (data.http?.url_truncated) overview.append(element('p', 'URL 已截断'))
  line(overview, '入站', data.exit?.inbound)
  line(overview, '匹配的出站', data.outbound)
  if (data.passthrough_reason) line(overview, '透传原因', passthroughReasons[data.passthrough_reason] ?? data.passthrough_reason)
  line(overview, '代理出口链', data.exit?.chain?.map(hop => `${hop.tag} (${hop.type})`).join(' → '))
  line(overview, '传输对端', data.exit?.remote)
  line(overview, '来源应用', data.source_app?.package ?? data.exit?.package)
  line(overview, 'UID', data.source_app?.uid ?? data.exit?.uid)
  line(overview, 'Android 用户', data.source_app?.user_id)
  line(overview, '进程', data.exit?.process)
  article.append(overview)
  if (!data.http) {
    article.append(element('p', '该连接仅捕获元数据。'))
    return article
  }
  const matches = data.http.rewrites ?? []
  const rewrites = section(`重写规则 · ${matches.length}`, matches.length > 0)
  if (!matches.length) rewrites.append(element('p', '该会话没有匹配的重写规则。'))
  for (const rule of matches) {
    const row = section(`规则 ${rule.rule_index + 1} · ${rule.phase === 'request' ? '请求' : '响应'} · ${states[rule.state] ?? rule.state}`)
    line(row, '匹配表达式', rule.pattern)
    if (rule.state !== 'shadowed' && rule.state !== 'not_reached') line(row, '脚本耗时', `${rule.duration_us} μs`)
    if (rule.error) line(row, '错误', errors[rule.error] ?? rule.error)
    if (rule.actions?.includes('reject')) row.append(element('p', '请求已拦截'))
    line(row, '重定向目标', rule.redirect)
    if (rule.original_status !== undefined) line(row, 'HTTP 状态', `${rule.original_status} → ${data.status}`)
    if (rule.body) line(row, '正文大小', `${rule.body.before} → ${rule.body.after} 字节`)
    for (const change of rule.headers ?? []) {
      const before = change.before?.join('\n') ?? '—'
      const after = change.after?.join('\n') ?? '—'
      line(row, change.name, `${before} → ${after}${change.truncated ? '（已截断）' : ''}`)
    }
    rewrites.append(row)
  }
  article.append(rewrites, message('请求', data.http.request), message('响应', data.http.response))
  if (data.http.websocket) {
    const socket = section('WebSocket 帧', true)
    for (const frame of data.http.websocket.frames) {
      line(socket, frame.direction === 'send' ? '发送' : '接收', `0x${frame.opcode.toString(16)} · ${frame.size} 字节`)
      if (frame.text) socket.append(element('pre', frame.text))
      if (frame.truncated) socket.append(element('p', '文本预览已截断。'))
    }
    if (data.http.websocket.truncated) socket.append(element('p', '仅显示前 64 帧。'))
    article.append(socket)
  }
  return article
}
