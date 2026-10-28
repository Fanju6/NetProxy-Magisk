import { expect, test } from '@playwright/test'

test('inbound settings are one revisioned form with retained backend preferences', async ({
  page,
}) => {
  await page.goto('/#/settings')
  await expect(page.getByRole('link', { name: 'Root 进程', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: '入站设置', exact: true }).click()
  await expect(page.locator('header h1')).toHaveText('入站设置')
  const backend = page.getByRole('combobox', { name: '入站后端', exact: true })
  await expect(backend).toContainText('eBPF')
  await expect(page.getByRole('link', { name: '应用代理', exact: true })).toBeVisible()
  await expect(page.getByRole('combobox', { name: '数据平面', exact: true })).toHaveCount(0)
  await page.screenshot({
    path: `test-results/${test.info().project.name}-inbound-ebpf.png`,
  })
  await backend.click()
  await page.getByRole('option', { name: 'TUN', exact: true }).click()
  const root = page.getByRole('combobox', { name: 'Root 进程', exact: true })
  await root.click()
  await page.getByRole('option', { name: '绕过', exact: true }).click()
  const name = page.getByRole('textbox', { name: '接口名称', exact: true })
  await name.fill('netproxy-test')
  await name.blur()
  await backend.click()
  await page.getByRole('option', { name: 'eBPF', exact: true }).click()
  await expect(page.getByText('本机网络', { exact: true })).toBeVisible()
  await backend.click()
  await page.getByRole('option', { name: 'TUN', exact: true }).click()
  await expect(name).toHaveValue('netproxy-test')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('已保存')
  await page.screenshot({
    path: `test-results/${test.info().project.name}-inbound-tun.png`,
  })
  await page.locator('header').getByRole('button', { name: '返回', exact: true }).click()
  await page.getByRole('combobox', { name: '主题', exact: true }).click()
  await page.getByRole('option', { name: '深色', exact: true }).click()
  await page.getByRole('link', { name: '入站设置', exact: true }).click()
  await expect(backend).toContainText('TUN')
  await expect(root).toContainText('绕过')
  await expect(name).toHaveValue('netproxy-test')
  await page.getByText('高级参数', { exact: true }).click()
  await expect(page.getByRole('spinbutton', { name: 'MTU', exact: true })).toBeVisible()
  await page.screenshot({
    path: `test-results/${test.info().project.name}-inbound-dark.png`,
  })
  await page.getByRole('button', { name: 'JSON', exact: true }).click()
  const editor = page
    .getByRole('dialog')
    .filter({ has: page.getByRole('button', { name: '表单', exact: true }) })
  await expect(editor.getByRole('button', { name: '保存', exact: true })).toBeVisible()
  await editor.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(backend).toContainText('TUN')
})
