import { hslToHex, type LogoColors } from './color'

export interface GachaCard {
  id: string
  name: string
  rarity: 3 | 4 | 5
  desc: string
  colors: LogoColors
}

export interface PityState {
  pity5: number
  pity4: number
}

export interface PullResult {
  cards: GachaCard[]
  pity: PityState
  highestRarity: 3 | 4 | 5
}

export const pool5Star: readonly GachaCard[] = [
  {
    id: 'aurora', name: '极光幻夜', rarity: 5,
    desc: '模拟北极夜空的魔幻极光，青与紫的交织如梦如幻。',
    colors: { leftColor: '#80DEEA', rightColor: '#E1BEE7', gradStart: '#006064', gradStop2: '#00838F', gradStop3: '#8E24AA', gradEnd: '#4A148C' }
  },
  {
    id: 'inferno', name: '红莲劫火', rarity: 5,
    desc: '凝聚了火山深处最炽烈的火种，具有极致的侵略性与热情。',
    colors: { leftColor: '#FF8A80', rightColor: '#FF5252', gradStart: '#3700B3', gradStop2: '#D50000', gradStop3: '#FF1744', gradEnd: '#800808' }
  },
  {
    id: 'amber', name: '金枫圣芒', rarity: 5,
    desc: '融汇了秋日落叶与黄金余晖的圣神光芒，代表极致的高贵与防御。',
    colors: { leftColor: '#FFE082', rightColor: '#FFB74D', gradStart: '#FFF8E1', gradStop2: '#FFD54F', gradStop3: '#FFB300', gradEnd: '#FF8F00' }
  }
]

export const pool4Star: readonly GachaCard[] = [
  {
    id: 'lavender', name: '薰衣草幻境', rarity: 4,
    desc: '迷雾中若隐若现的薰衣草田，带来静谧、幽雅与舒适的视觉享受。',
    colors: { leftColor: '#D0BCFF', rightColor: '#EFB8C8', gradStart: '#4F378B', gradStop2: '#6750A4', gradStop3: '#9A82DB', gradEnd: '#381E72' }
  },
  {
    id: 'mint', name: '夏日清荷', rarity: 4,
    desc: '清凉薄荷色系交织，仿佛夏日池塘中盛开的青绿荷叶与微风。',
    colors: { leftColor: '#A7F3D0', rightColor: '#FDE047', gradStart: '#064E3B', gradStop2: '#059669', gradStop3: '#34D399', gradEnd: '#022C22' }
  },
  {
    id: 'watcher', name: '深渊之眼', rarity: 4,
    desc: '蔚蓝深邃的海洋与星辰之光的呼应，科技感十足。',
    colors: { leftColor: '#93C5FD', rightColor: '#C084FC', gradStart: '#1E3A8A', gradStop2: '#2563EB', gradStop3: '#60A5FA', gradEnd: '#172554' }
  },
  {
    id: 'sakura', name: '绯樱落羽', rarity: 4,
    desc: '漫天飞舞的樱花花瓣，如同初春的恋爱般细腻温暖。',
    colors: { leftColor: '#FBCFE8', rightColor: '#FCA5A5', gradStart: '#881337', gradStop2: '#DB2777', gradStop3: '#F472B6', gradEnd: '#4C0519' }
  }
]

const threeStarSuffixes = ['幽绿', '赤红', '晶蓝', '松石', '冷灰', '金砂', '暮紫', '幻青']

export function generateThreeStar(random: () => number = Math.random): GachaCard {
  const suffix = threeStarSuffixes[Math.floor(random() * threeStarSuffixes.length)]
  const hue = Math.floor(random() * 360)
  return {
    id: `star3_${Math.floor(random() * Number.MAX_SAFE_INTEGER).toString(36)}`,
    name: `量子${suffix}`,
    rarity: 3,
    desc: '通过量子引擎生成的普通等级常驻莫奈配色。',
    colors: {
      leftColor: hslToHex(hue - 32, 75, 62),
      rightColor: hslToHex(hue + 38, 70, 70),
      gradStart: hslToHex(hue, 80, 42),
      gradStop2: hslToHex(hue + 8, 75, 55),
      gradStop3: hslToHex(hue + 24, 65, 68),
      gradEnd: hslToHex(hue - 15, 80, 30)
    }
  }
}

function pick<T>(items: readonly T[], random: () => number): T {
  return items[Math.floor(random() * items.length)]
}

export function rollOne(state: PityState, random: () => number = Math.random): { card: GachaCard, pity: PityState } {
  let pity5 = state.pity5 + 1
  let pity4 = state.pity4 + 1

  if (pity5 >= 80) {
    return { card: { ...pick(pool5Star, random) }, pity: { pity5: 0, pity4: 0 } }
  }
  if (pity4 >= 10) {
    return { card: { ...pick(pool4Star, random) }, pity: { pity5, pity4: 0 } }
  }

  const chance = random() * 100
  if (chance < 1.6) {
    return { card: { ...pick(pool5Star, random) }, pity: { pity5: 0, pity4: 0 } }
  }
  if (chance < 10) {
    return { card: { ...pick(pool4Star, random) }, pity: { pity5, pity4: 0 } }
  }
  return { card: generateThreeStar(random), pity: { pity5, pity4 } }
}

export function pull(count: 1 | 10, state: PityState, random: () => number = Math.random): PullResult {
  const cards: GachaCard[] = []
  let pity = { ...state }
  for (let index = 0; index < count; index += 1) {
    const result = rollOne(pity, random)
    cards.push(result.card)
    pity = result.pity
  }
  return {
    cards,
    pity,
    highestRarity: Math.max(...cards.map((card) => card.rarity)) as 3 | 4 | 5
  }
}
