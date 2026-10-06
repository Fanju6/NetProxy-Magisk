import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(import.meta.dirname, '../public/mascots')
const checkOnly = process.argv.includes('--check')
const base = readFileSync(`${root}/dragon/base.svg`, 'utf8')
const body = base.replace(/^<svg[^>]*><title[^>]*>[^<]*<\/title>/, '').replace(/<\/svg>$/, '')
const ink = '#15212b'
const mint = '#91dacd'
const cream = '#fff3d7'
const blue = '#246fe2'
const pink = '#fb9b92'
const path = (d, fill = ink) => `<path fill="${fill}" d="${d}"/>`
const rect = (x, y, w, h, fill = ink) => `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="${fill}"/>`
const heart = (x, y, fill = pink) => `<g transform="translate(${x} ${y})">${path('M0 12h12V0h24v12h12V0h24v12h12v24H72v12H60v12H48v12H36V60H24V48H12V36H0z', fill)}</g>`
const frame = (content, label, viewBox = '0 0 512 512') => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}" shape-rendering="crispEdges" role="img" aria-labelledby="title"><title id="title">${label}</title>${content}</svg>\n`

function write(file, content) {
  const target = `${root}/${file}`
  if (checkOnly) {
    if (readFileSync(target, 'utf8') !== content) throw new Error(`角色生成物需更新：${file}`)
  } else {
    mkdirSync(resolve(target, '..'), { recursive: true })
    writeFileSync(target, content)
  }
}

function face(expression) {
  const eye = (x, y) => {
    if (expression === 'sleep') return path(`M${x} ${y + 38}h64v16h-64z`)
    if (expression === 'tired') return path(`M${x} ${y + 30}h64v34h-64z`)
    if (expression === 'happy') return path(`M${x} ${y + 24}h16v-16h32v16h16v16h-16v-16h-32v16h-16z`)
    if (expression === 'angry') return path(`M${x} ${y}h32v16h32v48h-64z`)
    if (expression === 'heart') return `<g transform="translate(${x} ${y}) scale(.76)">${heart(0, 0)}</g>`
    if (expression === 'surprised') return rect(x, y - 10, 64, 84) + rect(x + 20, y + 10, 24, 44, cream)
    if (expression === 'wink' && x < 500) return path(`M${x} ${y + 30}h64v16h-64z`)
    return rect(x, y, 64, 64)
  }
  const mouth = expression === 'surprised'
    ? rect(481, 606, 62, 70) + rect(497, 622, 30, 38, pink)
    : ['sleep', 'tired', 'angry'].includes(expression)
      ? rect(477, 626, 62, 14)
      : path('M463 598h18v8h64v-10h19v22h-17v22h-17v21h-33v-19h-17v-23h-17z') + rect(498, 622, 33, 25, pink)
  return `<g id="expression">${eye(361, 551)}${eye(606, 562)}${mouth}</g>`
}

const dragon = [
  ['received', '收到', 'wink', rect(874, 280, 18, 42, blue) + rect(892, 304, 40, 18, blue)],
  ['digging', '开挖！', 'happy', path('M272 910h32v16h-32zM310 931h48v16h-48zM821 930h32v16h-32z', blue)],
  ['connected', '通了！', 'happy', path('M925 345h22v22h22v22h-22v22h-22v-22h-22v-22h22z', cream)],
  ['rest', '先歇会', 'sleep', rect(864, 315, 20, 65, blue) + rect(904, 315, 20, 65, blue)],
  ['check', '我来康康', 'wink', path('M850 290h80v16h16v64h-16v16h-80v-16h-16v-64h16z', blue) + rect(854, 310, 72, 52, cream) + path('M922 378h20v20h20v20h-20v-20h-20z', blue)],
  ['speedtest', '正在测速', 'normal', path('M190 710h88v16h-88zM210 750h68v16h-68zM190 790h88v16h-88z', blue)],
  ['waiting', '怎么还没好', 'tired', path('M850 280h96v18h-14v30h-18v18h18v30h14v18h-96v-18h14v-30h18v-18h-18v-30h-14z', blue)],
  ['offline', '没网了', 'surprised', path('M870 290h24v56h-24zM870 365h24v24h-24z', '#d65368')],
  ['caught', '不许偷跑', 'angry', path('M858 290h66v18h18v66h-18v18h-66v-18h-18v-66h18z', '#d65368') + rect(860, 331, 61, 18, cream)],
  ['yay', '好耶！', 'happy', path('M860 300h20v20h20v20h-20v20h-20v-20h-20v-20h20z', blue) + heart(234, 311)],
  ['thanks', '谢谢投喂', 'heart', heart(841, 300) + path('M812 743h75v68h-75zM887 758h24v37h-24z', cream)],
  ['goodnight', '晚安', 'sleep', path('M870 280h54v20h-34v34h34v20h-54v-18h-18v-38h18z', cream)],
  ['confused', '啊？', 'surprised', path('M855 281h58v18h18v34h-18v18h-18v22h-20v-40h18v-18h-38zM875 390h20v20h-20z', blue)],
  ['panic', '等一下！', 'surprised', path('M429 596h18v50h-18zM677 610h18v50h-18z', blue)],
  ['done', '收工', 'wink', path('M858 313h20v20h20v-42h20v62h-20v20h-20v-20h-20z', blue)],
  ['love', '贴贴', 'heart', heart(832, 310) + heart(235, 380)],
]
const niang = [
  ['hmph', '哼'], ['really', '就这？'], ['repeat', '你再说一遍'], ['my-turn', '本小姐出手'],
  ['update', '还不更新？'], ['care', '才不是担心你'], ['config', '给我看看配置'], ['all', '我全都要'],
  ['later', '下次一定'], ['done', '优雅收工'], ['leave', '退下吧'], ['received', '收到'],
  ['logs', '日志呢？'], ['thanks', '谢谢投喂'], ['goodnight', '晚安'], ['leave-it', '交给我'],
]
const entries = []
for (const [index, [slug, caption, expression, prop]] of dragon.entries()) {
  const name = `${String(index + 1).padStart(2, '0')}-${slug}`
  const hiddenFace = '<style>#eye-left,#eye-right,#mouth{display:none}</style>'
  const illustration = `${hiddenFace}<g transform="translate(-48 -57) scale(.43)">${body}${face(expression)}<g id="props">${prop}</g></g>`
  const text = `<text x="256" y="469" text-anchor="middle" fill="${ink}" stroke="white" stroke-width="10" paint-order="stroke" font-family="Microsoft YaHei, Noto Sans CJK SC, sans-serif" font-size="46" font-weight="900">${caption}</text>`
  write(`dragon/stickers/${name}.svg`, frame(illustration + text, `奶屁龙：${caption}`))
  entries.push({ character: 'dragon', name: '奶屁龙', caption, slug, vector: `dragon/stickers/${name}.svg`, file: `dragon/stickers/${name}.png` })
}
for (const [index, [slug, caption]] of niang.entries()) {
  entries.push({ character: 'niang', name: '奶屁娘', caption, slug, file: `niang/stickers/${String(index + 1).padStart(2, '0')}-${slug}.png` })
}
write('stickers.json', JSON.stringify(entries, null, 2) + '\n')

const faceUnderlay = body.match(/<g id="face-underlay"[^>]*>[\s\S]*?<\/g>/)?.[0] ?? ''
const wood = '#d6a866'
const toolHandle = points => points.map(([x, y]) => rect(x, y, 24, 24, ink) + rect(x + 4, y + 4, 16, 16, wood)).join('')
const toolHead = (x, y) => path(`M${x} ${y}h76v16h16v20h-28v-8h-52v8h-28v-20h16z`, ink) + path(`M${x + 4} ${y + 4}h68v12h16v8h-24v-8h-52v8h-24v-8h16z`, blue)
const zMark = (x, y) => path(`M${x} ${y}h48v16h-20v16h-16v16h36v16h-48V48h20V32h16V16h-36z`, blue)
const alertMark = (x, y) => rect(x, y, 24, 56, '#d65368') + rect(x, y + 76, 24, 24, '#d65368')
const waitingDots = count => Array.from({ length: count }, (_, index) => rect(838 + index * 28, 302, 16, 16, blue)).join('')
const diggingRaised = faceUnderlay + face('happy')
  + toolHandle([[281, 682], [297, 706], [313, 730], [329, 754], [345, 778], [361, 802]])
  + toolHead(241, 650)
const diggingStrike = faceUnderlay + face('happy')
  + toolHandle([[361, 802], [337, 826], [313, 850], [289, 874], [265, 898]])
  + toolHead(225, 902)
  + path('M272 950h28v16h-28zM316 966h40v16h-40zM376 950h24v16h-24z', blue)
write('dragon/dig-raised.svg', frame(diggingRaised, '奶屁龙开挖抬镐', '225 206 966 848'))
write('dragon/dig-strike.svg', frame(diggingStrike, '奶屁龙开挖落镐', '225 206 966 848'))
write('dragon/sleep-a.svg', frame(faceUnderlay + face('sleep') + zMark(854, 304), '奶屁龙打呼噜', '225 206 966 848'))
write('dragon/sleep-b.svg', frame(faceUnderlay + face('sleep') + zMark(838, 326) + zMark(878, 266), '奶屁龙打呼噜冒泡', '225 206 966 848'))
write('dragon/failed-a.svg', frame(alertMark(870, 290), '奶屁龙异常提醒', '225 206 966 848'))
write('dragon/failed-b.svg', frame(alertMark(890, 270) + rect(850, 294, 16, 16, '#d65368') + rect(938, 294, 16, 16, '#d65368'), '奶屁龙异常提醒闪烁', '225 206 966 848'))
write('dragon/waiting-a.svg', frame(waitingDots(1), '奶屁龙等待中', '225 206 966 848'))
write('dragon/waiting-b.svg', frame(waitingDots(3), '奶屁龙等待中提示', '225 206 966 848'))

const torso = path('M15 3h12v2h5v3h4v3h3v4h2v9h-3v3h-2v9h-3v3H11v-3H8V27H5V24H3v-9h2v-4h3V8h4V5h3z')
  + path('M15 5h12v2h5v3h4v5h3v9h-3v3h-2v9h-3v1H13v-1h-3v-9H7v-3H5v-9h2v-5h5V7h3z', mint)
  + path('M12 29h3v7h18v-7h2v8h-3v2H13v-2h-1z', '#59aaa6')
const horns = path('M10 4h3v2h2v5h-6V6h1zM30 4h3v2h1v5h-6V6h2z')
  + path('M10 6h3v4h-3zM30 6h3v4h-3z', cream)
const feet = path('M11 37h7v4h-8v-3h1zM27 37h7v4h-8v-3h1z')
  + path('M11 38h6v2h-6zM27 38h6v2h-6z', cream)
const tail = path('M33 31h5v-3h3v-4h3v2h3v-2h2v8h-3v3h-4v1h-9z')
  + path('M34 32h5v-3h3v4h-3v2h-5z', mint)
  + path('M42 26h2v2h3v-2h1v5h-3v2h-2v-2h-1z', blue)
const mark = path('M17 12h2v2h2v2h2v-4h2v6h-4v-2h-2v2h-2z', cream)
const front = tail + torso + horns + feet + mark
  + rect(13, 20, 3, 3) + rect(28, 20, 3, 3) + path('M20 24h2v1h2v-1h2v2h-2v2h-2v-2h-2z')
  + rect(22, 26, 2, 1, pink) + path('M17 28h9v2h2v5h-2v2h-7v-2h-2z', cream)
  + path('M9 29h3v2h2v2h-4v-1H9zM31 29h3v3h-1v1h-4v-2h2z', cream)
const back = tail + torso + horns + feet + path('M21 3h3v4h-3zM21 12h3v4h-3zM21 22h3v4h-3zM21 31h3v5h-3z', blue)
const side = path('M23 4h7v3h4v4h3v6h3v8h-4v4h-2v7h-2v4H17v-3h-3v-4H8v-3H5v-6h3v-5h4v-7h4V8h7z')
  + path('M23 6h6v3h4v3h2v7h3v5h-4v5h-2v7h-2v2H19v-3h-3v-4h-6v-3H7v-2h3v-6h4v-7h4V10h5z', mint)
  + path('M17 5h4v2h2v5h-7V7h1z') + path('M17 7h4v4h-4z', cream)
  + path('M17 28h4v7h-4zM17 37h7v3h-7z', cream) + rect(15, 19, 3, 3)
  + path('M9 25h4v1h-4zM34 12h3v4h-3zM36 21h3v4h-3zM33 30h3v4h-3z', blue)
  + path('M31 32h5v-4h3v-4h3v2h3v-2h2v8h-3v2h-3v2H31z')
  + path('M32 33h5v-4h3v4h-3v2h-5z', mint) + path('M40 26h2v2h3v-2h1v5h-3v2h-2v-2h-1z', blue)
write('dragon/front.svg', frame(front, '奶屁龙正面补全', '0 0 50 44'))
write('dragon/side.svg', frame(side, '奶屁龙侧面补全', '0 0 50 44'))
write('dragon/back.svg', frame(back, '奶屁龙背面补全', '0 0 50 44'))
const views = [front, side, back].map((view, index) => `<g transform="translate(${index * 54} 0)">${view}</g>`).join('')
write('dragon/turnaround.svg', frame(views, '奶屁龙正面、侧面、背面补全', '0 0 162 44'))

console.log(`角色素材${checkOnly ? '检查' : '生成'}完成：奶屁龙 16 张，奶屁娘 16 张。`)
