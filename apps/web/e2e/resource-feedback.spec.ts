import { expect, test } from '@playwright/test'

test('resource success is one expiring toast, not persistent detail or home content', async ({
  page,
}) => {
  await page.goto('/#/subscriptions')
  await page.getByRole('button', { name: '查看详情 Global Connect', exact: true }).click()
  const details = page.getByRole('dialog', { name: 'Global Connect', exact: true })
  await details.getByRole('button', { name: '更新订阅', exact: true }).click()
  const toast = page.locator('[data-slot="toast"]')
  await expect(toast).toHaveCount(1)
  await expect(toast).toContainText('订阅已更新')
  await expect(details.getByText('订阅已更新', { exact: true })).toHaveCount(0)
  await details.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.locator('main').getByText('订阅已更新', { exact: true })).toHaveCount(0)
  await expect(toast).toHaveCount(0, { timeout: 8000 })
  await page.getByRole('button', { name: '查看详情 Global Connect', exact: true }).click()
  await details.getByRole('heading', { name: 'Global Connect', exact: true }).waitFor()
  await expect(toast).toHaveCount(0)
  await details.getByRole('button', { name: '关闭', exact: true }).click()
  await page.locator('header').getByRole('button', { name: '资源操作', exact: true }).click()
  for (const [name, icon] of [
    ['添加订阅', 'rss'],
    ['新建本地分组', 'folder-plus'],
    ['手动添加节点', 'server'],
    ['导入文本', 'clipboard-paste'],
    ['更新全部订阅', 'refresh-cw'],
    ['刷新资源', 'rotate-cw'],
  ]) {
    await expect(
      page.getByRole('menuitem', { name, exact: true }).locator(`svg.lucide-${icon}`),
    ).toHaveCount(1)
  }
  await page.screenshot({ path: `test-results/${test.info().project.name}-resource-icons.png` })
})

test('policy members show resource names, while runtime member identity is unchanged', async ({
  page,
}) => {
  await page.goto('/#/nodes')
  const member = page
    .getByTestId('policy-Fastest')
    .getByRole('button', { name: '选择 Tokyo 01', exact: true })
  await expect(member).toContainText('Global Connect')
  await expect(member).not.toContainText('np/demo-global')
  await expect(member.locator('[data-slot="node-name"]')).toHaveAttribute(
    'title',
    'np/demo-global/Tokyo 01',
  )
  await page.screenshot({ path: `test-results/${test.info().project.name}-resource-names.png` })
})

test('delay results stay on cards and Provider references stay in advanced details', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.goto('/#/nodes')
  const direct = page
    .getByTestId('policy-Internet')
    .getByRole('button', { name: '选择 direct', exact: true })
  await expect(direct).not.toContainText('未测速')
  await expect(direct.locator('.tabular-nums')).toBeEmpty()
  const group = page.getByTestId('policy-Fastest')
  const testDelay = group.getByRole('button', { name: '测试分组 Fastest' })
  await expect(testDelay).toBeEnabled()
  await testDelay.click()
  await expect(testDelay).toBeEnabled()
  await expect(group).toContainText(/\d+ ms/)
  await expect(page.locator('[data-slot="toast"]')).toHaveCount(0)
  await page.screenshot({ path: `test-results/${test.info().project.name}-quiet-delay.png` })
  await page.goto('/#/subscriptions')
  await page.getByRole('button', { name: '查看详情 Global Connect', exact: true }).click()
  const details = page.getByRole('dialog', { name: 'Global Connect', exact: true })
  const tag = details.getByText('np/demo-global', { exact: true })
  await expect(tag).not.toBeVisible()
  await details.getByText('高级信息', { exact: true }).click()
  await expect(tag).toBeVisible()
  await details.getByRole('button', { name: '复制 ProviderTag', exact: true }).click()
  await expect(page.locator('[data-slot="toast"]')).toContainText('Provider Tag 已复制')
  await expect(details.getByText('Provider Tag 已复制', { exact: true })).toHaveCount(0)
  await page.mouse.move(0, 0)
  await expect(page.locator('[data-slot="toast"]')).toHaveCount(0, { timeout: 8000 })
  await page.screenshot({ path: `test-results/${test.info().project.name}-advanced-provider.png` })
})
