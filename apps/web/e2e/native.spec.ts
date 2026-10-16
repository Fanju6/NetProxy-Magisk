import { expect, test } from '@playwright/test'

test('native-only build fails closed in a browser and never falls back to Mock or HTTPS', async ({
  page,
}) => {
  const outside: string[] = []
  page.on('request', (request) => {
    if (!request.url().startsWith('http://127.0.0.1:5184/')) outside.push(request.url())
  })
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('尚未配置可信连接')
  await expect(page.getByRole('note')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '开启服务', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '停止服务', exact: true })).toHaveCount(0)
  await page
    .getByRole('navigation', { name: '主导航' })
    .filter({ visible: true })
    .getByRole('link', { name: '设置', exact: true })
    .click()
  await expect(page.getByRole('heading', { name: '设置', exact: true })).toBeVisible()
  await expect(page.getByRole('switch', { name: '开机自启', exact: true })).toHaveCount(0)
  expect(outside).toEqual([])
})
