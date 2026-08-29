<template>
  <section class="tool-shell monet-tool" aria-label="NetProxy 莫奈调色器">
    <div class="tool-workspace">
      <section class="tool-preview" aria-labelledby="monet-preview-title">
        <div class="tool-heading-row">
          <h2 id="monet-preview-title">效果预览</h2>
          <div class="segmented-control" aria-label="预览背景">
            <button
              v-for="background in backgrounds"
              :key="background.id"
              type="button"
              :class="{ active: backgroundType === background.id }"
              :aria-pressed="backgroundType === background.id"
              @click="backgroundType = background.id"
            >{{ background.name }}</button>
          </div>
        </div>

        <div :class="['logo-canvas', `canvas-${backgroundType}`]">
          <LogoMark :colors="colors" uid="monet-preview" :stroke-width="strokeWidth" :opacity="opacity" :stroke-linecap="strokeLinecap" />
        </div>

        <h3>预设</h3>
        <div class="preset-grid">
          <button v-for="preset in presets" :key="preset.name" type="button" class="preset-button" @click="applyPreset(preset)">
            <span class="preset-swatches" aria-hidden="true">
              <i :style="{ backgroundColor: preset.leftColor }"></i>
              <i :style="{ backgroundColor: preset.gradStop2 }"></i>
              <i :style="{ backgroundColor: preset.rightColor }"></i>
            </span>
            {{ preset.name }}
          </button>
        </div>
      </section>

      <section class="tool-controls" aria-labelledby="monet-controls-title">
        <h2 id="monet-controls-title">调色参数</h2>

        <fieldset>
          <legend>种子色</legend>
          <div class="color-row">
            <input type="color" :value="seedColor" aria-label="种子色" @input="setSeed(($event.target as HTMLInputElement).value)" />
            <input :value="seedColor" inputmode="text" aria-label="种子色 Hex" @input="setSeed(($event.target as HTMLInputElement).value)" />
            <button type="button" @click="randomize">随机</button>
          </div>
          <label class="check-row"><input v-model="autoGenerate" type="checkbox" @change="autoGenerate && applySeed()" /> 种子色变化时自动生成</label>
        </fieldset>

        <fieldset>
          <legend>线条与渐变</legend>
          <label v-for="field in colorFields" :key="field.key" class="field-row">
            <span>{{ field.label }}</span>
            <input type="color" :value="colors[field.key]" @input="setColor(field.key, ($event.target as HTMLInputElement).value)" />
            <input :value="colors[field.key]" inputmode="text" @input="setColor(field.key, ($event.target as HTMLInputElement).value)" />
          </label>
        </fieldset>

        <fieldset>
          <legend>几何</legend>
          <label class="range-row">
            <span>线宽</span>
            <input v-model.number="strokeWidth" type="range" min="40" max="130" step="1" />
            <output>{{ strokeWidth }} px</output>
          </label>
          <label class="range-row">
            <span>透明度</span>
            <input v-model.number="opacity" type="range" min="0.1" max="1" step="0.05" />
            <output>{{ opacity.toFixed(2) }}</output>
          </label>
          <label class="select-row">
            <span>端点</span>
            <select v-model="strokeLinecap">
              <option value="round">round</option>
              <option value="butt">butt</option>
              <option value="square">square</option>
            </select>
          </label>
        </fieldset>

        <p class="tool-status" role="status" aria-live="polite">{{ status }}</p>
      </section>
    </div>

    <section class="export-surface" aria-labelledby="monet-export-title">
      <div class="tool-heading-row">
        <h2 id="monet-export-title">导出</h2>
        <div class="segmented-control" aria-label="导出格式">
          <button type="button" :class="{ active: activeFormat === 'svg' }" :aria-pressed="activeFormat === 'svg'" @click="activeFormat = 'svg'">SVG</button>
          <button type="button" :class="{ active: activeFormat === 'vector' }" :aria-pressed="activeFormat === 'vector'" @click="activeFormat = 'vector'">Vector XML</button>
        </div>
      </div>
      <pre tabindex="0"><code>{{ activeCode }}</code></pre>
      <div class="tool-actions">
        <button type="button" @click="copyCode">复制 {{ activeFormat === 'svg' ? 'SVG' : 'Vector XML' }}</button>
        <button v-if="activeFormat === 'svg'" type="button" class="secondary" @click="downloadSvg">下载 SVG</button>
      </div>
    </section>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref } from 'vue'
import LogoMark from './LogoMark.vue'
import { createMonetScheme, normalizeHex, randomHex, type LogoColors } from '../tools/color'
import { defaultLogoOptions, generateSvg, generateVector, type StrokeLinecap } from '../tools/logo'

interface Preset extends LogoColors { name: string, seedColor: string }

const seedColor = ref('#003BCC')
const colors = reactive<LogoColors>({ ...defaultLogoOptions })
const strokeWidth = ref(defaultLogoOptions.strokeWidth)
const opacity = ref(defaultLogoOptions.opacity)
const strokeLinecap = ref<StrokeLinecap>(defaultLogoOptions.strokeLinecap)
const backgroundType = ref('checker')
const activeFormat = ref<'svg' | 'vector'>('svg')
const autoGenerate = ref(true)
const status = ref('可继续调色或导出代码。')
let statusTimer: ReturnType<typeof setTimeout> | undefined

const backgrounds = [
  { id: 'checker', name: '透明' },
  { id: 'light', name: '浅色' },
  { id: 'dark', name: '深色' }
]

const colorFields: Array<{ key: keyof LogoColors, label: string }> = [
  { key: 'leftColor', label: '左侧线条' },
  { key: 'rightColor', label: '右侧线条' },
  { key: 'gradStart', label: '渐变 0%' },
  { key: 'gradStop2', label: '渐变 35%' },
  { key: 'gradStop3', label: '渐变 65%' },
  { key: 'gradEnd', label: '渐变 100%' }
]

const presets: Preset[] = [
  { name: 'NetProxy 原版', seedColor: '#003BCC', leftColor: defaultLogoOptions.leftColor, rightColor: defaultLogoOptions.rightColor, gradStart: defaultLogoOptions.gradStart, gradStop2: defaultLogoOptions.gradStop2, gradStop3: defaultLogoOptions.gradStop3, gradEnd: defaultLogoOptions.gradEnd },
  { name: '薰衣草', seedColor: '#6750A4', leftColor: '#D0BCFF', rightColor: '#EFB8C8', gradStart: '#4F378B', gradStop2: '#6750A4', gradStop3: '#9A82DB', gradEnd: '#381E72' },
  { name: '夏日薄荷', seedColor: '#059669', leftColor: '#A7F3D0', rightColor: '#FDE047', gradStart: '#064E3B', gradStop2: '#059669', gradStop3: '#34D399', gradEnd: '#022C22' },
  { name: '皇家蓝', seedColor: '#1D4ED8', leftColor: '#93C5FD', rightColor: '#C084FC', gradStart: '#1E3A8A', gradStop2: '#2563EB', gradStop3: '#60A5FA', gradEnd: '#172554' },
  { name: '浅粉樱花', seedColor: '#DB2777', leftColor: '#FBCFE8', rightColor: '#FCA5A5', gradStart: '#881337', gradStop2: '#DB2777', gradStop3: '#F472B6', gradEnd: '#4C0519' },
  { name: '秋日暖枫', seedColor: '#D97706', leftColor: '#FDE68A', rightColor: '#FCA5A5', gradStart: '#78350F', gradStop2: '#D97706', gradStop3: '#FBBF24', gradEnd: '#451A03' }
]

const options = computed(() => ({ ...colors, strokeWidth: strokeWidth.value, opacity: opacity.value, strokeLinecap: strokeLinecap.value }))
const svgCode = computed(() => generateSvg(options.value))
const vectorCode = computed(() => generateVector(options.value))
const activeCode = computed(() => activeFormat.value === 'svg' ? svgCode.value : vectorCode.value)

function announce(message: string, reset = true) {
  if (statusTimer) clearTimeout(statusTimer)
  status.value = message
  if (reset) statusTimer = setTimeout(() => { status.value = '可继续调色或导出代码。' }, 2400)
}

function applySeed() {
  Object.assign(colors, createMonetScheme(seedColor.value))
}

function setSeed(value: string) {
  const normalized = normalizeHex(value)
  if (!normalized) return announce('种子色无效，请输入 3 位或 6 位 Hex。')
  seedColor.value = normalized
  if (autoGenerate.value) applySeed()
  announce('已更新种子色。')
}

function setColor(key: keyof LogoColors, value: string) {
  const normalized = normalizeHex(value)
  if (!normalized) return announce(`${colorFields.find((field) => field.key === key)?.label}不是有效 Hex。`)
  colors[key] = normalized
  autoGenerate.value = false
  announce('已更新手动配色。')
}

function randomize() {
  seedColor.value = randomHex()
  autoGenerate.value = true
  applySeed()
  announce('已生成随机种子色。')
}

function applyPreset(preset: Preset) {
  seedColor.value = preset.seedColor
  for (const field of colorFields) colors[field.key] = preset[field.key]
  autoGenerate.value = false
  announce(`已应用“${preset.name}”。`)
}

async function copyCode() {
  try {
    await navigator.clipboard.writeText(activeCode.value)
    announce(`${activeFormat.value === 'svg' ? 'SVG' : 'Vector XML'} 已复制。`)
  } catch {
    announce('复制失败，请检查浏览器剪贴板权限。', false)
  }
}

function downloadSvg() {
  const url = URL.createObjectURL(new Blob([svgCode.value], { type: 'image/svg+xml' }))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'N.svg'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
  announce('SVG 已下载。')
}

onUnmounted(() => {
  if (statusTimer) clearTimeout(statusTimer)
})
</script>
