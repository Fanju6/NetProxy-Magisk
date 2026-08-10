export interface CommandToken {
  value: string
  start: number
  end: number
}

// 只解析终端传给 netproxyctl 的参数，不执行 Shell 展开或环境变量替换。
export function parseCommandTokens(input: string): CommandToken[] {
  const tokens: CommandToken[] = []
  let value = ''
  let start = -1
  let quote = ''
  let escaped = false

  const flush = (end: number) => {
    if (start < 0) return
    tokens.push({ value, start, end })
    value = ''
    start = -1
  }

  for (let index = 0; index < input.length; index += 1) {
    const char = input[index]
    if (escaped) {
      value += char
      escaped = false
      continue
    }
    if (char === '\\') {
      if (start < 0) start = index
      escaped = true
      continue
    }
    if (quote) {
      if (char === quote) {
        quote = ''
      } else {
        value += char
      }
      continue
    }
    if (char === '"' || char === "'") {
      if (start < 0) start = index
      quote = char
    } else if (/\s/.test(char)) {
      flush(index)
    } else {
      if (start < 0) start = index
      value += char
    }
  }

  if (escaped) value += '\\'
  flush(input.length)
  return tokens
}

export function parseCommandLine(input: string): string[] {
  return parseCommandTokens(input).map(token => token.value)
}

export function quoteCommandToken(value: string): string {
  if (!/[\s"'\\]/.test(value)) return value
  return `"${value.replace(/["\\]/g, char => `\\${char}`)}"`
}
