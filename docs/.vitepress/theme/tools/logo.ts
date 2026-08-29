import { normalizeHex, type LogoColors } from './color'

export type StrokeLinecap = 'round' | 'butt' | 'square'

export interface LogoOptions extends LogoColors {
  strokeWidth: number
  opacity: number
  strokeLinecap: StrokeLinecap
}

export const defaultLogoOptions: LogoOptions = {
  leftColor: '#0DD8F5',
  rightColor: '#79F5A9',
  gradStart: '#003BCC',
  gradStop2: '#28CBE6',
  gradStop3: '#55E6AA',
  gradEnd: '#004A8F',
  strokeWidth: 88,
  opacity: 0.85,
  strokeLinecap: 'round'
}

export function validateLogoOptions(options: LogoOptions): LogoOptions {
  const colors = Object.fromEntries(
    ['leftColor', 'rightColor', 'gradStart', 'gradStop2', 'gradStop3', 'gradEnd'].map((key) => {
      const color = normalizeHex(options[key as keyof LogoColors])
      if (!color) throw new Error(`${key} 不是有效 Hex 颜色`)
      return [key, color]
    })
  ) as unknown as LogoColors

  if (!Number.isFinite(options.strokeWidth) || options.strokeWidth < 1 || options.strokeWidth > 200) {
    throw new Error('线宽必须在 1 到 200 之间')
  }
  if (!Number.isFinite(options.opacity) || options.opacity < 0 || options.opacity > 1) {
    throw new Error('透明度必须在 0 到 1 之间')
  }
  if (!['round', 'butt', 'square'].includes(options.strokeLinecap)) {
    throw new Error('端点类型无效')
  }

  return { ...colors, strokeWidth: options.strokeWidth, opacity: options.opacity, strokeLinecap: options.strokeLinecap }
}

export function generateSvg(input: LogoOptions): string {
  const options = validateLogoOptions(input)
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400" width="100%" height="100%">
  <defs>
    <linearGradient id="diagGrad" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="${options.gradStart}" />
      <stop offset="35%" stop-color="${options.gradStop2}" />
      <stop offset="65%" stop-color="${options.gradStop3}" />
      <stop offset="100%" stop-color="${options.gradEnd}" />
    </linearGradient>
  </defs>
  <line x1="100" y1="80" x2="100" y2="320" stroke="${options.leftColor}" stroke-width="${options.strokeWidth}" stroke-linecap="${options.strokeLinecap}" />
  <line x1="300" y1="80" x2="300" y2="320" stroke="${options.rightColor}" stroke-width="${options.strokeWidth}" stroke-linecap="${options.strokeLinecap}" />
  <line x1="100" y1="80" x2="300" y2="320" stroke="url(#diagGrad)" stroke-width="${options.strokeWidth}" stroke-linecap="${options.strokeLinecap}" opacity="${options.opacity}" />
</svg>`
}

export function generateVector(input: LogoOptions): string {
  const options = validateLogoOptions(input)
  return `<vector xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:aapt="http://schemas.android.com/aapt"
    android:width="400dp"
    android:height="400dp"
    android:viewportWidth="400"
    android:viewportHeight="400">
    <path android:pathData="M 100,80 L 100,320" android:strokeColor="${options.leftColor}" android:strokeWidth="${options.strokeWidth}" android:strokeLineCap="${options.strokeLinecap}" />
    <path android:pathData="M 300,80 L 300,320" android:strokeColor="${options.rightColor}" android:strokeWidth="${options.strokeWidth}" android:strokeLineCap="${options.strokeLinecap}" />
    <path android:pathData="M 100,80 L 300,320" android:strokeWidth="${options.strokeWidth}" android:strokeLineCap="${options.strokeLinecap}" android:strokeAlpha="${options.opacity}">
        <aapt:attr name="android:strokeColor">
            <gradient android:startX="100" android:startY="80" android:endX="300" android:endY="320" android:type="linear">
                <item android:offset="0.0" android:color="${options.gradStart}" />
                <item android:offset="0.35" android:color="${options.gradStop2}" />
                <item android:offset="0.65" android:color="${options.gradStop3}" />
                <item android:offset="1.0" android:color="${options.gradEnd}" />
            </gradient>
        </aapt:attr>
    </path>
</vector>`
}
