import { readFileSync, statSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

export function checkBuild(directory) {
  const checked = new Set()
  const check = reference => {
    if (/^(?:https?:|data:|#)/.test(reference)) return
    const name = reference.split(/[?#]/, 1)[0]
    if (checked.has(name)) return
    checked.add(name)
    const path = resolve(directory, name)
    const info = statSync(path)
    if (!info.isFile() || info.size === 0) {
      throw new Error(`WebUI 构建资源无效: ${name}`)
    }
    if (name.endsWith('.js')) {
      const code = readFileSync(path, 'utf8')
      for (const match of code.matchAll(/["'](\.\/mascots\/[^"']+)["']/g)) check(match[1])
    }
  }
  check('index.html')
  const html = readFileSync(resolve(directory, 'index.html'), 'utf8')
  for (const match of html.matchAll(/\b(?:src|href)=["']([^"']+)["']/g)) check(match[1])
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  checkBuild(resolve(import.meta.dirname, '../../module/webroot/netproxy'))
  console.log('WebUI 构建资源完整')
}
