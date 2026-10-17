import { expect, test } from '@playwright/test'

test('one compact title and owned navigation across primary and settings pages', async ({
  page,
}) => {
  for (const [path, title] of [
    ['/', '仪表盘'],
    ['/nodes', '节点'],
    ['/subscriptions', '资源'],
    ['/settings', '设置'],
  ] as const) {
    await page.goto(`/#${path}`)
    await expect(page.locator('h1')).toHaveCount(1)
    await expect(page.locator('header h1')).toHaveText(title)
    await expect(page.locator('main h1')).toHaveCount(0)
    await expect(page.locator('header button')).toHaveCount(0)
    await expect(page.getByRole('note')).toHaveCount(path === '/' ? 1 : 0)
    await expect(page.getByText('原生连接 · 核心管理', { exact: true })).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({
      path: `test-results/${test.info().project.name}-${title}-redesign.png`,
      fullPage: true,
    })
  }
  await page.getByRole('link', { name: 'Wi-Fi', exact: true }).click()
  await expect(page).toHaveURL(/section=wifi/)
  await expect(page.locator('header h1')).toHaveText('网络匹配')
  await expect(page.getByRole('combobox', { name: '网络匹配', exact: true })).toBeVisible()
  const navigation = page.getByRole('navigation', { name: '主导航' }).filter({ visible: true })
  await expect(navigation.getByRole('link', { name: '设置', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  )
  await page.locator('header').getByRole('button', { name: '返回', exact: true }).click()
  await expect(page.locator('header h1')).toHaveText('设置')
})

test('full JSON editor guards drafts and browser back closes the editor before its page', async ({
  page,
}) => {
  await page.goto('/#/settings')
  await page.getByRole('link', { name: '内核配置', exact: true }).click()
  await page.getByRole('button', { name: /完整主配置 singbox\/config.json/ }).click()
  const editor = page.getByRole('dialog', { name: 'config.json', exact: true })
  await expect(editor).toBeVisible()
  if (page.viewportSize()!.width < 768) {
    const box = await editor.boundingBox()
    expect(box!.width).toBeGreaterThanOrEqual(page.viewportSize()!.width - 1)
    expect(box!.height).toBeGreaterThanOrEqual(page.viewportSize()!.height - 1)
  }
  const content = editor.locator('.cm-content')
  await content.click()
  await page.keyboard.press('ControlOrMeta+A')
  await page.keyboard.insertText('{"ui_draft_fixture":"not-persisted"}')
  await editor.getByRole('button', { name: '关闭', exact: true }).click()
  const prompt = page.getByRole('dialog', { name: '放弃未保存的修改？', exact: true })
  await expect(prompt).toBeVisible()
  await prompt.getByRole('button', { name: '继续编辑', exact: true }).click()
  await expect(content).toHaveText('{"ui_draft_fixture":"not-persisted"}')
  await page.evaluate(() => window.history.back())
  await expect(prompt).toBeVisible()
  await prompt.getByRole('button', { name: '放弃修改', exact: true }).click()
  await expect(editor).toHaveCount(0)
  await expect(page.locator('header h1')).toHaveText('内核配置')
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain('ui_draft_fixture')
  await page.locator('header').getByRole('button', { name: '返回', exact: true }).click()
  await expect(page.locator('header h1')).toHaveText('设置')
})
