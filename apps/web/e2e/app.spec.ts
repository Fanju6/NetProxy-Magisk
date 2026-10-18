import { expect, test, type Page } from '@playwright/test'

async function resourceAction(page: Page, name: string) {
  await page.getByRole('dialog').getByRole('button', { name: '更多操作', exact: true }).click()
  await page.getByRole('menuitem', { name, exact: true }).click()
}

test('resources show public details in both themes and restrict URL actions to subscriptions', async ({
  page,
}) => {
  const failures: string[] = []
  page.on('pageerror', (error) => failures.push(error.message))
  await page.goto('/#/subscriptions')
  await expect(page.getByRole('heading', { name: '资源', exact: true })).toBeVisible()
  await expect(page.locator('main h1')).toHaveCount(0)
  await expect(page.getByText('订阅与本地配置，集中管理。', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: '本地分组', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'URL 订阅', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '查看详情 Global Connect', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: '订阅流量', exact: true })).toBeVisible()
  await expect(dialog.getByText('100 GB', { exact: true })).toBeVisible()
  await expect(dialog.getByText('运行时已同步', { exact: true })).toBeVisible()
  await expect(dialog.getByText('失败', { exact: true })).toBeVisible()
  await expect(dialog.getByText('1440 分钟', { exact: true })).toBeVisible()
  await expect(
    dialog.locator('[data-slot="drawer-footer"], [data-slot="dialog-footer"]').getByRole('button'),
  ).toHaveCount(1)
  await expect(dialog.getByRole('button', { name: '编辑订阅', exact: true })).toHaveCount(0)
  await dialog.getByRole('button', { name: '更多操作', exact: true }).click()
  await expect(page.getByRole('menuitem', { name: '编辑订阅', exact: true })).toBeVisible()
  await expect(page.getByRole('menuitem', { name: '删除订阅', exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({
    path: `test-results/${test.info().project.name}-resource-detail.png`,
    fullPage: true,
  })
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.goto('/#/settings')
  await page.getByRole('combobox', { name: '主题', exact: true }).click()
  await page.getByRole('option', { name: '深色', exact: true }).click()
  await page.goto('/#/subscriptions')
  await page.getByRole('button', { name: '查看详情 备用线路', exact: true }).click()
  await expect(dialog.getByText(/下载中/)).toBeVisible()
  await expect(dialog.getByText('阶段不代表完成百分比，最终结果请查看历史。')).toBeVisible()
  await page.screenshot({
    path: `test-results/${test.info().project.name}-resource-dark.png`,
    fullPage: true,
  })
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: '查看详情 本地配置', exact: true }).click()
  await expect(dialog.getByText('节点', { exact: true })).toBeVisible()
  await expect(dialog.getByText('更新历史', { exact: true })).toHaveCount(0)
  await expect(dialog.getByText('自动更新', { exact: true })).toHaveCount(0)
  await expect(
    dialog.getByRole('button', { name: /编辑订阅|删除订阅|更新订阅|请求取消更新/ }),
  ).toHaveCount(0)
  expect(failures).toEqual([])
})

test('subscription form creates, patches and deletes a demo source without revealing private values', async ({
  page,
}) => {
  const failures: string[] = []
  page.on('pageerror', (error) => failures.push(error.message))
  await page.goto('/#/subscriptions')
  await page.getByRole('button', { name: '添加订阅', exact: true }).click()
  let dialog = page.getByRole('dialog')
  await dialog.getByRole('textbox', { name: '名称', exact: true }).fill('端到端订阅')
  const url = dialog.getByRole('textbox', { name: '订阅 URL', exact: true })
  await url.fill('https://example.invalid/feed?key=anonymous')
  if (page.viewportSize()!.width < 768)
    expect(
      await url.evaluate((element) => Number.parseFloat(getComputedStyle(element).fontSize)),
    ).toBeGreaterThanOrEqual(16)
  await dialog.getByRole('spinbutton', { name: '更新周期（秒，至少 900）' }).fill('1800')
  await page.setViewportSize({ width: page.viewportSize()!.width, height: 430 })
  const save = dialog.getByRole('button', { name: '保存', exact: true })
  await save.scrollIntoViewIfNeeded()
  await page.screenshot({
    path: `test-results/${test.info().project.name}-subscription-form-short.png`,
    fullPage: true,
  })
  await save.click()
  await expect(page.getByRole('heading', { name: '端到端订阅', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '查看详情 端到端订阅', exact: true }).click()
  await resourceAction(page, '编辑订阅')
  await expect(page.getByRole('dialog')).toHaveCount(1)
  dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('textbox', { name: '订阅 URL', exact: true })).toHaveCount(0)
  await dialog.getByRole('textbox', { name: '名称', exact: true }).fill('已编辑订阅')
  await dialog.getByRole('button', { name: '保存', exact: true }).scrollIntoViewIfNeeded()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已编辑订阅', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '查看详情 已编辑订阅', exact: true }).click()
  await resourceAction(page, '删除订阅')
  await expect(dialog.getByText('已编辑订阅', { exact: true })).toBeVisible()
  await expect(
    dialog.getByText('删除此订阅及其节点？此操作无法撤销。', { exact: true }),
  ).toBeVisible()
  await dialog.getByRole('button', { name: '删除订阅', exact: true }).click()
  await expect(page.getByText('订阅已删除', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: '已编辑订阅', exact: true })).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain('example.invalid')
  expect(failures).toEqual([])
})

test('active subscription deletion requires a named replacement and update-all reports partial results', async ({
  page,
}) => {
  await page.goto('/#/subscriptions')
  await page.getByRole('button', { name: '更多操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '更新全部订阅', exact: true }).click()
  await expect(page.getByText('部分订阅未能更新，请逐项查看结果。', { exact: true })).toBeVisible()
  const operation = page.getByRole('region', { name: '订阅操作' })
  await expect(operation.getByText('Global Connect', { exact: true })).toBeVisible()
  await expect(operation.getByText('备用线路', { exact: true })).toBeVisible()
  await expect(operation.getByText('成功', { exact: true })).toBeVisible()
  await expect(operation.getByText('失败', { exact: true })).toBeVisible()
  await expect(page.getByText('全部订阅已更新', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '查看详情 Global Connect', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await resourceAction(page, '删除订阅')
  const confirm = dialog.getByRole('button', { name: '删除订阅', exact: true })
  await expect(confirm).toBeDisabled()
  await dialog.getByRole('combobox', { name: '替代分组', exact: true }).click()
  await expect(page.getByRole('option', { name: 'Travel backup', exact: true })).toHaveCount(0)
  await page.getByRole('option', { name: '本地配置', exact: true }).click()
  await expect(confirm).toBeEnabled()
  await confirm.click()
  await expect(page.getByText('订阅已删除', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Global Connect', exact: true })).toHaveCount(0)
})

test('service confirmation, stopped Catalog and selection work without a backend', async ({
  page,
}) => {
  const failures: string[] = []
  page.on('pageerror', (error) => failures.push(error.message))
  await page.goto('/')
  await expect(page.getByRole('note')).toContainText('演示模式')
  await expect(page.getByRole('heading', { name: '运行中', exact: true })).toBeVisible()
  await expect(page.getByText('历史快照 · 非实时', { exact: true })).toHaveCount(0)
  await expect(page.getByText('正在获取实时快照', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '停止服务', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.getByRole('heading', { name: '运行中', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '停止服务', exact: true }).click()
  await page.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已停止', exact: true })).toBeVisible()
  const navigation = page.getByRole('navigation', { name: '主导航' }).filter({ visible: true })
  await navigation.getByRole('link', { name: '节点', exact: true }).click()
  await expect(page.getByRole('heading', { name: '节点', exact: true })).toBeVisible()
  await expect(
    page.getByTestId('group-demo-empty').getByRole('button', { name: /Auto-Fastest/ }),
  ).toHaveCount(0)
  await page.getByRole('button', { name: '选择 Tokyo 01', exact: true }).click()
  await expect(page.getByRole('button', { name: '选择 Tokyo 01', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await page.getByRole('button', { name: '显示方式', exact: true }).click()
  await page.getByRole('menuitemradio', { name: '列表视图' }).click()
  await expect(page.getByRole('menu')).toHaveCount(0)
  await page.getByRole('searchbox').fill('Tokyo')
  await expect(page.getByRole('button', { name: '选择 Hong Kong 01', exact: true })).toHaveCount(0)
  await page.getByRole('searchbox').clear()
  await page.getByRole('button', { name: 'Global Connect', exact: false }).first().click()
  await expect(page.getByRole('button', { name: '选择 Tokyo 01', exact: true })).not.toBeVisible()
  await navigation.getByRole('link', { name: '资源', exact: true }).click()
  await expect(page.getByRole('heading', { name: '本地配置', exact: true })).toBeVisible()
  await navigation.getByRole('link', { name: '仪表盘', exact: true }).click()
  await page.getByRole('button', { name: '开启服务', exact: true }).click()
  await expect(page.getByRole('heading', { name: '运行中', exact: true })).toBeVisible()
  await page.getByRole('combobox', { name: '出站模式', exact: true }).click()
  await page.getByRole('option', { name: '直连', exact: true }).click()
  await expect(page.getByRole('combobox', { name: '出站模式', exact: true })).toContainText('直连')
  expect(failures).toEqual([])
})

test('theme, language and configuration survive page navigation; preferences survive reload', async ({
  page,
}) => {
  await page.goto('/#/settings')
  await expect(page.getByRole('switch', { name: '开机自启', exact: true })).toBeVisible()
  await page.getByRole('switch', { name: '开机自启', exact: true }).click()
  await expect(page.getByRole('switch', { name: '开机自启', exact: true })).toBeChecked()
  await page.getByRole('combobox', { name: '主题', exact: true }).click()
  await page.getByRole('option', { name: '深色', exact: true }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.getByRole('combobox', { name: '语言', exact: true }).click()
  await page.getByRole('option', { name: 'English', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await expect(page.getByRole('note')).toHaveCount(0)
  await page.goto('/')
  await expect(page.getByRole('note')).toContainText('Demo mode')
})

test('node testing and saved routing changes remain usable while the core is stopped', async ({
  page,
}) => {
  const failures: string[] = []
  page.on('pageerror', (error) => failures.push(error.message))
  await page.goto('/')
  await page.getByRole('button', { name: '停止服务', exact: true }).click()
  await page.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已停止', exact: true })).toBeVisible()
  const navigation = page.getByRole('navigation', { name: '主导航' }).filter({ visible: true })
  await navigation.getByRole('link', { name: '节点', exact: true }).click()
  const group = page.getByTestId('group-demo-global')
  const manual = group.getByRole('button', { name: '选择 Tokyo 01', exact: true })
  await expect(manual).toBeEnabled()
  await manual.click()
  await expect(manual).toHaveAttribute('aria-pressed', 'true')
  await expect(manual).toBeEnabled()
  const auto = group.getByRole('button', { name: '选择 Auto-Fastest', exact: true })
  await auto.click()
  await expect(auto).toHaveAttribute('aria-pressed', 'true')
  await expect(group.getByRole('button', { name: '测试分组 Global Connect' })).toBeEnabled()
  await group.getByRole('button', { name: '测试分组 Global Connect' }).click()
  await expect(page.getByText('已收到最近可用结果，其中可能包含缓存。')).toBeVisible()
  const measured = group.getByRole('button', { name: '测试 Tokyo 01 的延迟', exact: true })
  await expect(measured).toContainText('53 ms')
  const single = group.getByRole('button', { name: '测试 Hong Kong 01 的延迟' })
  await expect(single).toBeEnabled()
  await single.click()
  await expect(single).toBeEnabled()
  await expect(single).toContainText('32 ms')
  await expect(measured).toContainText('53 ms')
  expect(await single.locator('xpath=ancestor::button').count()).toBe(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await navigation.getByRole('link', { name: '仪表盘', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已停止', exact: true })).toBeVisible()
  const mode = page.getByRole('combobox', { name: '出站模式', exact: true })
  await expect(mode).toBeEnabled()
  await mode.click()
  await page.getByRole('option', { name: '代理', exact: true }).click()
  await expect(mode).toContainText('代理')
  await expect(mode).toBeEnabled()
  await expect(page.getByText(/运行时模式 ·/)).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(failures).toEqual([])
})

test('details, keyboard navigation and responsive content stay inside the viewport', async ({
  page,
}) => {
  await page.goto('/#/nodes')
  await page.getByRole('button', { name: '查看详情 Tokyo 01', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('dialog')).toContainText('jp-01.example.invalid')
  await expect(
    page.getByRole('dialog').locator('[data-slot="drawer-footer"], [data-slot="dialog-footer"]'),
  ).toHaveCount(0)
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.getByRole('dialog')).not.toBeVisible()
  await expect(page.getByRole('button', { name: '查看详情 Tokyo 01', exact: true })).toBeFocused()
  await page.getByRole('searchbox').fill('🇬🇧')
  await expect(page.getByRole('button', { name: /选择 🇬🇧/ })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.getByRole('searchbox').clear()
  await page.screenshot({
    path: `test-results/${test.info().project.name}-nodes.png`,
    fullPage: true,
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '运行中', exact: true })).toBeVisible()
  await expect(page.getByText('v8.6.1', { exact: true })).toHaveCount(0)
  await page.screenshot({
    path: `test-results/${test.info().project.name}-dashboard-light.png`,
    fullPage: true,
  })
  await page.goto('/#/settings')
  await page.getByRole('combobox', { name: '主题', exact: true }).click()
  await page.getByRole('option', { name: '深色', exact: true }).click()
  await page.goto('/')
  await page.screenshot({
    path: `test-results/${test.info().project.name}-dashboard-dark.png`,
    fullPage: true,
  })
})

test('narrow layouts, reduced motion and keyboard focus stay usable', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/#/nodes')
  await expect(page.getByRole('button', { name: '选择 Tokyo 01', exact: true })).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: '跳到正文' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('main')).toBeFocused()
  await expect(page.getByRole('heading', { name: '节点', exact: true })).toBeVisible()
  const search = page.getByRole('searchbox')
  await search.fill('Tokyo')
  await page.setViewportSize({ width: 320, height: 400 })
  await expect(page.getByRole('button', { name: '选择 Tokyo 01', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({
    path: `test-results/${test.info().project.name}-narrow.png`,
    fullPage: true,
  })
})
