import { expect, test } from '@playwright/test'

test('editor actions remain above a keyboard that only resizes the visual viewport', async ({
  page,
}) => {
  await page.goto('/#/settings/configuration')
  await page.getByRole('button', { name: /开机启动 module\/auto_start/ }).click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('.cm-content').click()
  const height = 420
  await page.evaluate((height) => {
    Object.defineProperty(window.visualViewport, 'height', { configurable: true, value: height })
    window.visualViewport?.dispatchEvent(new Event('resize'))
  }, height)
  await expect(page.locator('html')).toHaveAttribute('data-keyboard-open')
  const footer = dialog.locator('.editor-toolbar')
  await expect
    .poll(async () => (await footer.boundingBox())!.y + (await footer.boundingBox())!.height)
    .toBeLessThanOrEqual(height + 1)
  await expect(dialog.getByRole('button', { name: '保存', exact: true })).toBeInViewport()
  await page.evaluate(() => {
    Object.defineProperty(window.visualViewport, 'height', {
      configurable: true,
      value: window.innerHeight,
    })
    window.visualViewport?.dispatchEvent(new Event('resize'))
  })
  await expect(page.locator('html')).not.toHaveAttribute('data-keyboard-open')
  await expect.poll(async () => (await dialog.boundingBox())!.height).toBeGreaterThan(height)
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.goto('/#/subscriptions')
  await page.locator('header').getByRole('button', { name: '资源操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '导入文本', exact: true }).click()
  await dialog.getByRole('textbox', { name: '节点内容', exact: true }).focus()
  await page.evaluate((height) => {
    Object.defineProperty(window.visualViewport, 'height', { configurable: true, value: height })
    window.visualViewport?.dispatchEvent(new Event('resize'))
  }, height)
  await expect(page.locator('html')).toHaveAttribute('data-keyboard-open')
  const actions = dialog.locator('[data-slot="drawer-footer"], [data-slot="dialog-footer"]')
  await expect
    .poll(async () => {
      const bounds = (await actions.boundingBox())!
      return bounds.y + bounds.height
    })
    .toBeLessThanOrEqual(height + 1)
})

test('configuration JSON validation and local rule editing use real-shaped revisions', async ({
  page,
}) => {
  await page.goto('/#/settings/configuration')
  await page.getByRole('button', { name: /开机启动 module\/auto_start/ }).click()
  let dialog = page.getByRole('dialog').filter({ has: page.locator('.cm-content') })
  const editor = dialog.locator('.cm-content')
  await editor.click()
  await page.keyboard.press('ControlOrMeta+A')
  await page.keyboard.insertText('{"auto_start":true}')
  await expect(editor).toHaveText('{"auto_start":true}')
  await dialog.getByRole('button', { name: '校验', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('校验通过')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('已保存')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: /代理规则 singbox\/rules\/local\/proxy.json/ }).click()
  dialog = page.getByRole('dialog', { name: 'proxy.json', exact: true })
  await dialog.getByRole('button', { name: '添加规则', exact: true }).click()
  const rule = page.getByRole('dialog', { name: '添加规则', exact: true })
  await rule.getByRole('combobox', { name: '匹配方式', exact: true }).click()
  await page.getByRole('option', { name: 'IP 地址或网段', exact: true }).click()
  await rule.getByRole('textbox', { name: '每行一个值', exact: true }).fill('192.168.10.0/24')
  await rule.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByText('192.168.10.0/24', { exact: true })).toBeVisible()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('已保存')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('application and Wi-Fi policies preserve selections and offer native inbound controls', async ({
  page,
}) => {
  await page.goto('/#/settings/policies?section=apps')
  await page.getByRole('combobox', { name: '代理模式', exact: true }).click()
  await page.getByRole('option', { name: '白名单', exact: true }).click()
  await page.getByRole('searchbox', { name: '搜索应用或包名', exact: true }).fill('termux')
  await page.getByRole('button', { name: /Termux com.termux/ }).click()
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('已保存')
  await page.goto('/#/settings/policies?section=wifi')
  await page.getByRole('combobox', { name: '网络匹配', exact: true }).click()
  await page.getByRole('option', { name: '黑名单', exact: true }).click()
  await page.getByRole('textbox', { name: 'Wi-Fi 名称', exact: true }).fill(' WiFi, Name ')
  await page.getByRole('button', { name: '添加 Wi-Fi 名称', exact: true }).click()
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('已保存')
  await page.goto('/#/settings/policies?section=inbound')
  await page.getByRole('combobox', { name: '入站后端', exact: true }).click()
  await page.getByRole('option', { name: 'TUN', exact: true }).click()
  await expect(page.getByText('接口地址，每行一个', { exact: true })).toBeVisible()
  await expect(page.getByRole('combobox', { name: 'Root 进程', exact: true })).toBeVisible()
  await expect(page.locator('[role="dialog"][data-panel]')).toHaveCount(0)
  await page.screenshot({
    path: `test-results/${test.info().project.name}-tun-editor.png`,
    fullPage: true,
  })
})

test('local node import and logs are usable without platform file permissions', async ({
  page,
}) => {
  await page.goto('/#/subscriptions')
  await page.locator('header').getByRole('button', { name: '资源操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '导入文本', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '导入文本', exact: true })
  await dialog
    .getByRole('textbox', { name: '节点内容', exact: true })
    .fill(
      '{"outbounds":[{"tag":"Anonymous fixture","type":"socks","server":"127.0.0.1","server_port":1080}]}',
    )
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('操作完成')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: '查看详情 本地配置', exact: true }).click()
  await expect(
    page.getByRole('dialog').getByText('Anonymous fixture', { exact: true }),
  ).toBeVisible()
  await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click()
  await page.goto('/#/settings/logs')
  await expect(page.getByText(/Service ready/)).toBeVisible()
  await page.getByRole('button', { name: '日志选项', exact: true }).click()
  await page.getByRole('menuitem', { name: 'eBPF 能力检查', exact: true }).click()
  await expect(page.getByText(/does not prove that traffic is intercepted/)).toBeVisible()
  await page.getByRole('button', { name: '日志选项', exact: true }).click()
  await page.getByRole('menuitem', { name: '清空', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '确认', exact: true }).click()
  await expect(page.getByText('暂无日志内容', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
