<template>
  <section class="tool-shell gacha-tool" aria-label="莫奈色彩祈愿模拟器">
    <header class="gacha-header">
      <div><span aria-hidden="true">◆</span> {{ crystals.toLocaleString() }} 原石</div>
      <button type="button" class="icon-button" :aria-label="muted ? '开启声音' : '关闭声音'" :aria-pressed="muted" @click="toggleSound">
        {{ muted ? '静音' : '声音' }}
      </button>
    </header>

    <div class="gacha-stage">
      <section v-if="gameState === 'lobby'" class="gacha-lobby" aria-labelledby="gacha-banner-title">
        <div class="gacha-banner">
          <div>
            <span class="gacha-tag">限时祈愿</span>
            <h2 id="gacha-banner-title">莫奈的幻想空间</h2>
            <p>极光幻夜与红莲劫火登场。5 星概率 1.6%，80 抽保底；4 星概率 8.4%，10 抽保底。</p>
          </div>
          <div class="featured-grid" aria-label="本期配色">
            <article v-for="card in featuredCards" :key="card.id">
              <LogoMark :colors="card.colors" :uid="`featured-${card.id}`" :size="76" />
              <strong>{{ card.name }}</strong>
              <span>{{ '★'.repeat(card.rarity) }}</span>
            </article>
          </div>
        </div>

        <div class="gacha-actions">
          <button ref="wishButton" type="button" :disabled="busy" @click="performWish(1)">
            <strong>祈愿 1 次</strong><span>160 原石</span>
          </button>
          <button type="button" :disabled="busy" @click="performWish(10)">
            <strong>祈愿 10 次</strong><span>1600 原石</span>
          </button>
        </div>
        <p class="pity-line">距离 5 星保底还有 <strong>{{ 80 - pity.pity5 }}</strong> 抽，距离 4 星保底还有 <strong>{{ 10 - pity.pity4 }}</strong> 抽。</p>
      </section>

      <button
        v-else-if="gameState === 'animation'"
        type="button"
        :class="['wish-animation', `rarity-${highestRarity}`]"
        aria-label="跳过祈愿动画"
        @click="skipAnimation"
      >
        <span class="meteor" aria-hidden="true"></span>
        <span>点击或按 Escape 跳过</span>
      </button>

      <section v-else class="wish-results" role="dialog" aria-modal="true" aria-labelledby="wish-result-title">
        <h2 id="wish-result-title">祈愿结果</h2>

        <article v-if="gameState === 'single'" :class="['result-card', `rarity-${results[0].rarity}`]">
          <LogoMark :colors="results[0].colors" :uid="`single-${results[0].id}`" :size="210" />
          <p class="stars">{{ '★'.repeat(results[0].rarity) }}</p>
          <h3>{{ results[0].name }}</h3>
          <p>{{ results[0].desc }}</p>
          <div class="tool-actions">
            <button type="button" @click="copyCard(results[0], 'svg')">复制 SVG</button>
            <button type="button" class="secondary" @click="copyCard(results[0], 'vector')">复制 Vector XML</button>
          </div>
        </article>

        <div v-else class="ten-result-grid">
          <button
            v-for="(card, index) in results"
            :key="`${card.id}-${index}`"
            type="button"
            :class="[`rarity-${card.rarity}`]"
            @click="openDetail(card, $event.currentTarget as HTMLElement)"
          >
            <LogoMark :colors="card.colors" :uid="`result-${index}-${card.id}`" :size="72" />
            <strong>{{ card.name }}</strong>
            <span>{{ '★'.repeat(card.rarity) }}</span>
          </button>
        </div>

        <button type="button" class="confirm-button" @click="closeResults">确定</button>
      </section>
    </div>

    <section class="collection" aria-labelledby="collection-title">
      <h2 id="collection-title">色彩图鉴 <small>{{ unlocked.length }} 种</small></h2>
      <p v-if="unlocked.length === 0">完成祈愿后，抽到的配色会出现在这里。</p>
      <div v-else class="collection-grid">
        <button v-for="(card, index) in unlocked" :key="card.name" type="button" @click="openDetail(card, $event.currentTarget as HTMLElement)">
          <LogoMark :colors="card.colors" :uid="`collection-${index}-${card.id}`" :size="48" />
          <span>{{ card.name }}</span>
        </button>
      </div>
    </section>

    <div class="sr-status" aria-live="polite" role="status">{{ status }}</div>

    <div v-if="detailCard" class="tool-modal-backdrop" @click.self="closeDetail">
      <section ref="detailDialog" class="tool-modal" role="dialog" aria-modal="true" aria-labelledby="detail-title" tabindex="-1">
        <button type="button" class="modal-close" aria-label="关闭详情" @click="closeDetail">关闭</button>
        <LogoMark :colors="detailCard.colors" :uid="`detail-${detailCard.id}`" :size="180" />
        <p class="stars">{{ '★'.repeat(detailCard.rarity) }}</p>
        <h2 id="detail-title">{{ detailCard.name }}</h2>
        <p>{{ detailCard.desc }}</p>
        <div class="tool-actions">
          <button type="button" @click="copyCard(detailCard, 'svg')">复制 SVG</button>
          <button type="button" class="secondary" @click="copyCard(detailCard, 'vector')">复制 Vector XML</button>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import LogoMark from './LogoMark.vue'
import { pool5Star, pull, type GachaCard, type PityState } from '../tools/gacha'
import { generateSvg, generateVector } from '../tools/logo'

type GameState = 'lobby' | 'animation' | 'single' | 'ten'

const crystals = ref(16000)
const pity = ref<PityState>({ pity5: 0, pity4: 0 })
const gameState = ref<GameState>('lobby')
const results = ref<GachaCard[]>([])
const unlocked = ref<GachaCard[]>([])
const detailCard = ref<GachaCard | null>(null)
const highestRarity = ref<3 | 4 | 5>(3)
const muted = ref(false)
const status = ref('准备祈愿。')
const reduceMotion = ref(false)
const detailDialog = ref<HTMLElement | null>(null)
const wishButton = ref<HTMLButtonElement | null>(null)
const featuredCards = pool5Star.slice(0, 2)
const busy = computed(() => gameState.value !== 'lobby')

let audioContext: AudioContext | null = null
let previousFocus: HTMLElement | null = null
let motionQuery: MediaQueryList | null = null
const timers = new Set<ReturnType<typeof setTimeout>>()

function schedule(callback: () => void, delay: number) {
  const timer = setTimeout(() => {
    timers.delete(timer)
    callback()
  }, delay)
  timers.add(timer)
}

function ensureAudio() {
  if (!audioContext) audioContext = new AudioContext()
  if (audioContext.state === 'suspended') void audioContext.resume()
}

function tone(frequency: number, duration = 0.16, type: OscillatorType = 'sine') {
  if (muted.value) return
  try {
    ensureAudio()
    if (!audioContext) return
    const oscillator = audioContext.createOscillator()
    const gain = audioContext.createGain()
    oscillator.type = type
    oscillator.frequency.value = frequency
    gain.gain.setValueAtTime(0.12, audioContext.currentTime)
    gain.gain.exponentialRampToValueAtTime(0.0001, audioContext.currentTime + duration)
    oscillator.connect(gain).connect(audioContext.destination)
    oscillator.start()
    oscillator.stop(audioContext.currentTime + duration)
  } catch {
    muted.value = true
    status.value = '浏览器无法播放声音，已自动静音。'
  }
}

function playReveal() {
  const notes = highestRarity.value === 5 ? [262, 330, 392, 523] : highestRarity.value === 4 ? [440, 554, 659] : [330, 392]
  notes.forEach((note, index) => schedule(() => tone(note, index === notes.length - 1 ? 0.5 : 0.3, index === notes.length - 1 ? 'triangle' : 'sine'), index * 120))
}

function toggleSound() {
  muted.value = !muted.value
  if (!muted.value) tone(523, 0.1)
  status.value = muted.value ? '声音已关闭。' : '声音已开启。'
}

function performWish(count: 1 | 10) {
  if (busy.value) return
  const cost = count === 10 ? 1600 : 160
  if (crystals.value < cost) {
    crystals.value += 16000
    status.value = '原石不足，已补充 16000 原石供继续体验。'
  }
  crystals.value -= cost
  const outcome = pull(count, pity.value)
  pity.value = outcome.pity
  results.value = outcome.cards
  highestRarity.value = outcome.highestRarity
  gameState.value = 'animation'
  tone(523, 0.1)
  schedule(skipAnimation, reduceMotion.value ? 10 : 2200)
}

function skipAnimation() {
  if (gameState.value !== 'animation') return
  gameState.value = results.value.length === 1 ? 'single' : 'ten'
  playReveal()
  status.value = `祈愿完成，最高获得 ${highestRarity.value} 星配色。`
}

function closeResults() {
  for (const card of results.value) {
    if (!unlocked.value.some((item) => item.name === card.name)) unlocked.value.push(card)
  }
  results.value = []
  gameState.value = 'lobby'
  status.value = '结果已加入色彩图鉴。'
  nextTick(() => wishButton.value?.focus())
}

function openDetail(card: GachaCard, trigger: HTMLElement) {
  previousFocus = trigger
  detailCard.value = card
  nextTick(() => detailDialog.value?.focus())
}

function closeDetail() {
  detailCard.value = null
  nextTick(() => previousFocus?.focus())
}

async function copyCard(card: GachaCard, format: 'svg' | 'vector') {
  const options = { ...card.colors, strokeWidth: 88, opacity: 0.85, strokeLinecap: 'round' as const }
  try {
    await navigator.clipboard.writeText(format === 'svg' ? generateSvg(options) : generateVector(options))
    status.value = `${format === 'svg' ? 'SVG' : 'Vector XML'} 已复制。`
    tone(880, 0.12)
  } catch {
    status.value = '复制失败，请检查浏览器剪贴板权限。'
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  if (detailCard.value) closeDetail()
  else if (gameState.value === 'animation') skipAnimation()
  else if (gameState.value === 'single' || gameState.value === 'ten') closeResults()
}

function updateMotion(event: MediaQueryListEvent | MediaQueryList) {
  reduceMotion.value = event.matches
}

onMounted(() => {
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  updateMotion(motionQuery)
  motionQuery.addEventListener('change', updateMotion)
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  timers.forEach(clearTimeout)
  timers.clear()
  window.removeEventListener('keydown', handleKeydown)
  motionQuery?.removeEventListener('change', updateMotion)
  void audioContext?.close()
  audioContext = null
})
</script>
