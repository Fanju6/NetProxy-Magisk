import { expect, test } from '@playwright/test'

test('configuration JSON validation and local rule editing use real-shaped revisions', async ({
  page,
}) => {
  await page.goto('/#/settings/configuration')
  await page.getByRole('button', { name: 'auto_start', exact: true }).click()
  let dialog = page.getByRole('dialog')
  await dialog.locator('.cm-content').fill('{"auto_start":true}')
  await dialog.getByRole('button', { name: '校验', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('校验通过')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('已保存')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: 'proxy.json', exact: true }).click()
  dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '添加规则', exact: true }).click()
  await dialog.getByRole('combobox', { name: '匹配方式', exact: true }).click()
  await page.getByRole('option', { name: 'IP 地址或网段', exact: true }).click()
  await dialog.getByRole('textbox', { name: '每行一个值', exact: true }).fill('192.168.10.0/24')
  await dialog.getByRole('button', { name: '保存', exact: true }).first().click()
  await expect(dialog.getByText('192.168.10.0/24', { exact: true })).toBeVisible()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('已保存')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('application and Wi-Fi policies preserve selections and offer native inbound controls', async ({
  page,
}) => {
  await page.goto('/#/settings/policies')
  await page.getByRole('button', { name: '应用代理', exact: true }).click()
  let dialog = page.getByRole('dialog')
  await dialog.getByRole('combobox', { name: '代理模式', exact: true }).click()
  await page.getByRole('option', { name: '白名单', exact: true }).click()
  await dialog.getByRole('textbox', { name: '搜索应用或包名', exact: true }).fill('termux')
  await dialog.getByRole('button', { name: /Termux com.termux/ }).click()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('已保存')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: '网络匹配', exact: true }).click()
  dialog = page.getByRole('dialog')
  await dialog.getByRole('combobox', { name: '网络匹配', exact: true }).click()
  await page.getByRole('option', { name: '黑名单', exact: true }).click()
  await dialog.getByRole('textbox', { name: 'Wi-Fi 名称', exact: true }).fill(' WiFi, Name ')
  await dialog.getByRole('button', { name: '添加 Wi-Fi 名称', exact: true }).click()
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('已保存')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: 'TUN', exact: true }).click()
  await expect(
    page.getByRole('dialog').getByText('接口地址，每行一个', { exact: true }),
  ).toBeVisible()
  await page.screenshot({
    path: `test-results/${test.info().project.name}-tun-editor.png`,
    fullPage: true,
  })
})

test('local node import and logs are usable without platform file permissions', async ({
  page,
}) => {
  await page.goto('/#/nodes')
  await page.getByRole('button', { name: '管理本地节点', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog
    .locator('.cm-content')
    .fill(
      '{"outbounds":[{"tag":"Anonymous fixture","type":"socks","server":"127.0.0.1","server_port":1080}]}',
    )
  await dialog.getByRole('button', { name: '执行', exact: true }).click()
  await expect(dialog.getByRole('status')).toContainText('操作完成')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '选择 Anonymous fixture', exact: true }),
  ).toBeVisible()
  await page.goto('/#/settings/logs')
  await expect(page.getByText(/Service ready/)).toBeVisible()
  await page.getByRole('button', { name: 'eBPF 能力检查', exact: true }).click()
  await expect(page.getByText(/does not prove that traffic is intercepted/)).toBeVisible()
  await page.getByRole('button', { name: '清空', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '确认', exact: true }).click()
  await expect(page.getByText('暂无日志内容', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
