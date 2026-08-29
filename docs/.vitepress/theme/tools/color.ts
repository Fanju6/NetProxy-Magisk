export interface HslColor {
  h: number
  s: number
  l: number
}

export interface LogoColors {
  leftColor: string
  rightColor: string
  gradStart: string
  gradStop2: string
  gradStop3: string
  gradEnd: string
}

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))

export function normalizeHex(value: string): string | null {
  const input = value.trim()
  const short = /^#?([0-9a-f]{3})$/i.exec(input)
  if (short) {
    const expanded = [...short[1]].map((part) => part + part).join('')
    return `#${expanded.toUpperCase()}`
  }
  const full = /^#?([0-9a-f]{6})$/i.exec(input)
  return full ? `#${full[1].toUpperCase()}` : null
}

export function hexToHsl(value: string): HslColor {
  const hex = normalizeHex(value)
  if (!hex) throw new Error('颜色必须是 3 位或 6 位 Hex')

  const red = Number.parseInt(hex.slice(1, 3), 16) / 255
  const green = Number.parseInt(hex.slice(3, 5), 16) / 255
  const blue = Number.parseInt(hex.slice(5, 7), 16) / 255
  const max = Math.max(red, green, blue)
  const min = Math.min(red, green, blue)
  const lightness = (max + min) / 2

  if (max === min) return { h: 0, s: 0, l: Math.round(lightness * 100) }

  const delta = max - min
  const saturation = lightness > 0.5 ? delta / (2 - max - min) : delta / (max + min)
  let hue = 0
  if (max === red) hue = (green - blue) / delta + (green < blue ? 6 : 0)
  if (max === green) hue = (blue - red) / delta + 2
  if (max === blue) hue = (red - green) / delta + 4

  return {
    h: Math.round((hue / 6) * 360),
    s: Math.round(saturation * 100),
    l: Math.round(lightness * 100)
  }
}

export function hslToHex(hue: number, saturation: number, lightness: number): string {
  const h = ((hue % 360) + 360) % 360 / 360
  const s = clamp(saturation, 0, 100) / 100
  const l = clamp(lightness, 0, 100) / 100

  if (s === 0) {
    const part = Math.round(l * 255).toString(16).padStart(2, '0')
    return `#${part}${part}${part}`.toUpperCase()
  }

  const q = l < 0.5 ? l * (1 + s) : l + s - l * s
  const p = 2 * l - q
  const channel = (offset: number) => {
    let t = h + offset
    if (t < 0) t += 1
    if (t > 1) t -= 1
    let value = p
    if (t < 1 / 6) value = p + (q - p) * 6 * t
    else if (t < 1 / 2) value = q
    else if (t < 2 / 3) value = p + (q - p) * (2 / 3 - t) * 6
    return Math.round(value * 255).toString(16).padStart(2, '0')
  }

  return `#${channel(1 / 3)}${channel(0)}${channel(-1 / 3)}`.toUpperCase()
}

export function createMonetScheme(seed: string): LogoColors {
  const { h, s, l } = hexToHsl(seed)
  return {
    leftColor: hslToHex(h - 32, Math.min(s + 10, 95), Math.max(l, 58)),
    rightColor: hslToHex(h + 38, Math.min(s + 5, 90), Math.max(l + 10, 68)),
    gradStart: hslToHex(h, Math.max(s, 75), Math.max(l - 18, 25)),
    gradStop2: hslToHex(h + 8, Math.max(s, 70), Math.max(l - 5, 45)),
    gradStop3: hslToHex(h + 24, Math.max(s - 10, 60), Math.min(l + 12, 75)),
    gradEnd: hslToHex(h - 15, Math.max(s, 80), Math.max(l - 25, 20))
  }
}

export function randomHex(random: () => number = Math.random): string {
  return `#${Math.floor(random() * 0x1000000).toString(16).padStart(6, '0')}`.toUpperCase()
}
