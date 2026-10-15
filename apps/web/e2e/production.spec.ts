import { expect, test } from '@playwright/test'

test('production fails closed without a trusted transport and never controls a backend', async ({
  page,
}) => {
  const connections: string[] = []
  page.on('request', (request) => {
    if (!request.url().startsWith('http://127.0.0.1:5183/')) connections.push(request.url())
  })
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('尚未配置可信连接')
  await expect(page.getByRole('note')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '开启服务' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '停止服务' })).toHaveCount(0)
  expect(connections).toEqual([])
})
