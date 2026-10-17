import { expect, test } from '@playwright/test'

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
  await page.getByRole('button', { name: '列表视图' }).click()
  await page.getByRole('searchbox').fill('Tokyo')
  await expect(page.getByRole('button', { name: '选择 Hong Kong 01', exact: true })).toHaveCount(0)
  await page.getByRole('searchbox').clear()
  await page.getByRole('button', { name: 'Global Connect', exact: false }).first().click()
  await expect(page.getByRole('button', { name: '选择 Tokyo 01', exact: true })).not.toBeVisible()
  await navigation.getByRole('link', { name: '订阅', exact: true }).click()
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
  await page.getByRole('button', { name: '主题', exact: true }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.getByRole('button', { name: '语言', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
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
  await expect(manual).toContainText('53 ms')
  const single = group.getByRole('button', { name: '测试 Hong Kong 01 的延迟' })
  await expect(single).toBeEnabled()
  await single.click()
  await expect(single).toBeEnabled()
  await expect(group.getByRole('button', { name: '选择 Hong Kong 01', exact: true })).toContainText(
    '32 ms',
  )
  await expect(manual).toContainText('53 ms')
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
  await expect(page.getByText('已保存的默认模式', { exact: true })).toBeVisible()
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
  await expect(page.getByText('v8.6.1', { exact: true })).toBeVisible()
  await page.screenshot({
    path: `test-results/${test.info().project.name}-dashboard-light.png`,
    fullPage: true,
  })
  await page.getByRole('button', { name: '主题', exact: true }).click()
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
