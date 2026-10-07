<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import data from '../data/supporters.json'

const query = ref('')
const selected = ref<number | null>(null)
const cheering = ref(false)
const replies = ['股东来了，今天认真开挖。', '收到投喂，饼干库存增加。', '这次不挖饼干柜了。']
const total = data.supporters.reduce((sum, person) => sum + person.count, 0)
const visible = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return data.supporters.filter(person => person.name.toLowerCase().includes(needle))
})
const guest = computed(() => data.supporters.find(person => person.id === selected.value))
const message = computed(() => guest.value ? `谢谢投喂！${replies[(guest.value.id - 1) % replies.length]}` : '工坊能一直开门，多亏各位股东。')
let timer: ReturnType<typeof setTimeout> | undefined

function thank(id: number, event: MouseEvent) {
  selected.value = id
  clearTimeout(timer)
  cheering.value = event.detail > 0
  if (cheering.value) timer = setTimeout(() => { cheering.value = false }, 800)
}

onUnmounted(() => clearTimeout(timer))
</script>

<template>
  <section class="supporter-wall" aria-label="奶屁股东名册">
    <div class="welcome">
      <div class="dragon" :class="{ cheering }" aria-hidden="true">
        <img src="/mascots/dragon/base.svg" alt="" width="966" height="848" />
        <span class="thanks-sign">谢谢投喂</span>
      </div>
      <div class="greeting">
        <span class="speaker">奶屁龙 · 今日值班</span>
        <p role="status" aria-live="polite" aria-atomic="true">{{ message }}</p>
      </div>
    </div>

    <div class="ledger">
      <span><strong>{{ data.supporters.length }}</strong> 位股东</span>
      <span><strong>{{ total }}</strong> 次支持</span>
      <span class="updated">更新于 {{ data.updatedAt }}</span>
    </div>

    <div class="search">
      <label for="supporter-search">找找自己的名字</label>
      <div class="search-field">
        <input id="supporter-search" v-model="query" type="text" placeholder="搜索昵称" autocomplete="off" autocapitalize="none" spellcheck="false" enterkeyhint="search" />
        <button type="button" :disabled="!query" @click="query = ''">清除</button>
      </div>
      <span class="result-count" role="status">{{ visible.length }} / {{ data.supporters.length }} 位</span>
    </div>

    <ul class="nameplates" aria-label="支持者">
      <li v-for="person in visible" :key="person.id">
        <button class="nameplate" type="button" :aria-pressed="selected === person.id" @click="thank(person.id, $event)">
          <span class="name">{{ person.name }}</span>
          <span class="visits">支持了 {{ person.count }} 次</span>
          <span class="since">{{ selected === person.id ? '奶屁龙：谢谢投喂！' : `${person.since} 首次支持` }}</span>
        </button>
      </li>
    </ul>
    <p v-if="!visible.length" class="empty">这次没有找到。换个名字试试？</p>
    <p class="coverage">收录 {{ data.period.from }} 至 {{ data.period.to }} 的爱发电记录，按首次支持顺序排列。</p>

    <div class="closing">
      <img src="/mascots/niang/stickers/14-thanks.png" alt="奶屁娘：谢谢投喂" width="512" height="512" loading="lazy" />
      <div>
        <span class="speaker">奶屁娘 · 工坊主人</span>
        <p>本小姐都记着呢。谢谢你们照顾这间工坊。</p>
        <a href="https://afdian.com/a/fanju" target="_blank" rel="noopener noreferrer">请作者喝杯咖啡 <span aria-hidden="true">↗</span></a>
      </div>
    </div>
  </section>
</template>

<style scoped>
.supporter-wall { margin-top: 24px; }
.welcome { display: grid; grid-template-columns: 136px minmax(0, 1fr); align-items: center; gap: 24px; min-height: 152px; }
.dragon { position: relative; width: 136px; aspect-ratio: 1; }
.supporter-wall .dragon img { width: 100%; height: 100%; object-fit: contain; border: 0; border-radius: 0; }
.thanks-sign { position: absolute; bottom: 14px; left: 22px; padding: 3px 10px; background: var(--vp-c-bg); border: 2px solid #246fe2; border-radius: 4px; color: var(--vp-c-text-1); font-size: 12px; font-weight: 600; line-height: 20px; transform: rotate(3deg); transition: transform 180ms cubic-bezier(0.23, 1, 0.32, 1); }
.cheering .thanks-sign { transform: translateY(-8px) rotate(-6deg); }
.speaker { color: var(--vp-c-text-2); font-size: 13px; }
.greeting p { margin: 8px 0 0; min-height: 64px; font-size: 18px; line-height: 1.65; overflow-wrap: anywhere; }
.ledger { display: flex; flex-wrap: wrap; align-items: baseline; gap: 12px 24px; padding: 20px 0; margin: 8px 0 24px; border-block: 1px solid var(--vp-c-divider); color: var(--vp-c-text-2); font-size: 14px; }
.ledger strong { color: var(--vp-c-text-1); font-size: 24px; font-weight: 600; font-variant-numeric: tabular-nums; }
.updated { margin-left: auto; font-size: 12px; }
.search { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px 16px; margin-bottom: 20px; }
.search label { grid-column: 1 / -1; font-size: 14px; font-weight: 500; }
.search-field { display: flex; min-width: 0; border: 1px solid var(--vp-c-divider); border-radius: 6px; background: var(--vp-c-bg); }
.search-field:focus-within { outline: 2px solid var(--vp-c-brand-1); outline-offset: 2px; }
.search input { width: 100%; min-width: 0; padding: 10px 12px; outline: none; font-size: 16px; line-height: 24px; }
.search-field button { flex: 0 0 52px; color: var(--vp-c-brand-1); font-size: 13px; }
.search-field button:disabled { opacity: 0.4; cursor: default; }
.result-count { align-self: center; min-width: 72px; color: var(--vp-c-text-2); text-align: right; font-size: 12px; font-variant-numeric: tabular-nums; }
.supporter-wall .nameplates { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; padding: 0; margin: 0; list-style: none; }
.supporter-wall .nameplates li { min-width: 0; margin: 0; }
.nameplate { display: flex; flex-direction: column; gap: 4px; width: 100%; min-height: 124px; height: 100%; padding: 16px; border: 1px solid var(--vp-c-divider); border-radius: 6px; background: var(--vp-c-bg); text-align: left; transition: transform 100ms cubic-bezier(0.23, 1, 0.32, 1), border-color 100ms; }
.nameplate[aria-pressed="true"] { border-color: var(--vp-c-brand-1); background: var(--vp-c-brand-soft); }
.name { min-height: 44px; color: var(--vp-c-text-1); font-size: 15px; font-weight: 600; line-height: 22px; overflow-wrap: anywhere; }
.visits { color: var(--vp-c-text-2); font-size: 13px; line-height: 20px; }
.since { min-height: 36px; color: var(--vp-c-text-2); font-size: 12px; line-height: 18px; word-break: keep-all; overflow-wrap: anywhere; }
.nameplate[aria-pressed="true"] .since { color: var(--vp-c-brand-1); }
.supporter-wall button { cursor: pointer; touch-action: manipulation; -webkit-tap-highlight-color: transparent; }
.supporter-wall button:focus-visible, .closing a:focus-visible { outline: 2px solid var(--vp-c-brand-1); outline-offset: 3px; }
.nameplate:active:not(:focus-visible) { transform: scale(0.98); }
.empty { padding: 32px 0; text-align: center; color: var(--vp-c-text-2); }
.coverage { margin: 16px 0 0; color: var(--vp-c-text-2); font-size: 12px; line-height: 1.7; }
.closing { display: grid; grid-template-columns: 88px minmax(0, 1fr); align-items: center; gap: 20px; padding-top: 24px; margin-top: 32px; border-top: 1px solid var(--vp-c-divider); }
.supporter-wall .closing img { width: 88px; height: 88px; border: 0; border-radius: 0; }
.closing p { margin: 6px 0 10px; font-size: 14px; line-height: 1.7; }
.closing a { font-size: 14px; }
@media (hover: hover) and (pointer: fine) { .nameplate:hover { border-color: var(--vp-c-brand-1); } }
@media (min-width: 641px) and (max-width: 1200px) { .supporter-wall .nameplates { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 640px) {
  .welcome { grid-template-columns: 104px minmax(0, 1fr); gap: 16px; }
  .dragon { width: 104px; }
  .thanks-sign { left: 8px; bottom: 6px; }
  .greeting p { font-size: 16px; min-height: 80px; }
  .ledger { gap: 8px 20px; }
  .updated { flex-basis: 100%; margin-left: 0; }
  .supporter-wall .nameplates { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  .nameplate { padding: 12px; }
  .closing { grid-template-columns: 64px minmax(0, 1fr); gap: 12px; }
  .supporter-wall .closing img { width: 64px; height: 64px; }
}
@media (prefers-reduced-motion: reduce) { .thanks-sign, .nameplate { transition: none; transform: none; } .cheering .thanks-sign { transform: none; } }
</style>
