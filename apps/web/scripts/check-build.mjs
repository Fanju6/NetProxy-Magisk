import assert from 'node:assert/strict'
import { readFile, readdir, stat } from 'node:fs/promises'
import { resolve, sep } from 'node:path'
import { JSDOM } from 'jsdom'

const native = process.argv.includes('--native')
const root = resolve(native ? 'dist-native' : 'dist')
const html = await readFile(resolve(root, 'index.html'), 'utf8')
const document = new JSDOM(html).window.document
assert.ok(document.querySelector('meta[http-equiv="Content-Security-Policy"]'))
assert.ok(
  document
    .querySelector('meta[name="viewport"]')
    ?.getAttribute('content')
    ?.includes('viewport-fit=cover'),
)
for (const element of document.querySelectorAll('script[src], link[href]')) {
  const value = element.getAttribute('src') ?? element.getAttribute('href')
  assert.ok(value && !value.startsWith('http') && !value.startsWith('//'))
  const path = resolve(root, value.replace(/^\.\//, '').replace(/^\//, ''))
  assert.ok(path.startsWith(root + sep))
  assert.ok((await stat(path)).size > 0)
}
const assets = await readdir(resolve(root, 'assets'))
const code = (
  await Promise.all(
    assets
      .filter((name) => name.endsWith('.js'))
      .map((name) => readFile(resolve(root, 'assets', name), 'utf8')),
  )
).join('\n')
assert.ok(code.length > 0)
const demo = process.argv.includes('--demo')
assert.equal(code.includes('plugin:module-api|'), native, '原生 IPC 只能出现在显式 Native 构建中')
assert.equal(html.includes('http://ipc.localhost'), native, '仅原生构建允许 Tauri IPC CSP')
assert.equal(
  code.includes('demo-worker-1'),
  demo,
  '普通生产包不得包含 Mock 数据；演示包必须显式包含 Mock',
)
console.log(`V9 ${native ? '原生受限' : demo ? '演示' : '生产'}产物检查通过`)
