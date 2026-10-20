import { expect, test } from '@playwright/test'

test('event reconnect, offline and background transitions keep the header and content in place', async ({
  page,
}) => {
  await page.route('**/src/mocks/transport.ts', async (route) => {
    const response = await route.fetch()
    const body = await response.text()
    await route.fulfill({
      response,
      body: `${body}\nimport { ApiError as EventFixtureError } from '/src/api/contracts.ts';
        const originalSubscribe = MockTransport.prototype.subscribe;
        MockTransport.prototype.subscribe = function(topics, consume, fail) {
          window.disconnectFixture = (terminal = false) => fail(terminal
            ? new EventFixtureError('api.unauthorized', 'fixture', 401) : new Error('fixture'));
          return originalSubscribe.call(this, topics, consume);
        };`,
    })
  })
  await page.goto('/#/nodes')
  const search = page.getByRole('searchbox', { name: '搜索代理' })
  await expect(
    page.getByRole('button', { name: '选择 Hong Kong 01', exact: true }).first(),
  ).toBeEnabled()
  const initial = await page.evaluate(() => ({
    main: document.querySelector('main')!.getBoundingClientRect().top,
    title: document.querySelector('header h1')!.getBoundingClientRect().left,
    search: document.querySelector('input[type=search]')!.getBoundingClientRect().top,
  }))
  const verifyPosition = async () =>
    expect(
      await page.evaluate(() => ({
        main: document.querySelector('main')!.getBoundingClientRect().top,
        title: document.querySelector('header h1')!.getBoundingClientRect().left,
        search: document.querySelector('input[type=search]')!.getBoundingClientRect().top,
      })),
    ).toEqual(initial)
  const disconnect = (terminal: boolean) =>
    page.evaluate((terminal) => {
      ;(window as unknown as { disconnectFixture: (terminal: boolean) => void }).disconnectFixture(
        terminal,
      )
    }, terminal)
  await disconnect(false)
  const indicator = page.locator('header').getByRole('button', { name: /^管理连接:/ })
  await expect(indicator).toBeVisible()
  await verifyPosition()
  await expect(indicator).toBeHidden()
  await verifyPosition()
  await disconnect(true)
  await expect(indicator).toBeVisible()
  await verifyPosition()
  await page.screenshot({ path: `test-results/${test.info().project.name}-connection-offline.png` })
  await indicator.click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText('显示的上次快照并非实时状态')
  await dialog.getByRole('button', { name: '重新连接', exact: true }).click()
  await expect(indicator).toBeHidden()
  await verifyPosition()
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await expect(indicator).toBeVisible()
  await verifyPosition()
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await expect(indicator).toBeHidden()
  await expect(search).toBeVisible()
  await verifyPosition()
})

test('page actions stay in the header, dashboard order is stable and group tabs scroll horizontally', async ({
  page,
}) => {
  await page.goto('/')
  await expect(page.getByRole('img', { name: '实时流量', exact: true })).toBeVisible()
  const regions = ['实时速率', '服务统计', '出站模式', '当前节点']
  let previous = 0
  for (const name of regions) {
    const top = (await page.getByRole('region', { name, exact: true }).boundingBox())!.y
    expect(top).toBeGreaterThan(previous)
    previous = top
  }
  await expect(page.getByText(/^运行时长/)).toHaveCount(1)
  const nav = page.getByRole('navigation', { name: '主导航' }).filter({ visible: true })
  await nav.getByRole('link', { name: '代理', exact: true }).click()
  await expect(page.locator('header').getByRole('button', { name: '新增策略组' })).toBeVisible()
  await expect(page.locator('main').getByRole('button', { name: '新增策略组' })).toHaveCount(0)
  const search = page.getByRole('searchbox', { name: '搜索代理' })
  expect((await search.boundingBox())!.width).toBeCloseTo(
    await page.locator('main > div').evaluate((element) => element.getBoundingClientRect().width),
    0,
  )
  await page.locator('header').getByRole('button', { name: '显示方式' }).click()
  await page.getByRole('menuitemradio', { name: '标签视图' }).click()
  const tabs = page.getByRole('tablist')
  await expect(tabs).toHaveCSS('flex-wrap', 'nowrap')
  await expect(tabs).toHaveCSS('overflow-x', 'auto')
  await page.getByRole('tab', { name: 'Internet', exact: true }).focus()
  await page.keyboard.press('ArrowRight')
  await expect(page.getByRole('tab', { name: 'Fastest', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await nav.getByRole('link', { name: '资源', exact: true }).click()
  await expect(page.locator('main').getByRole('button', { name: '添加订阅' })).toHaveCount(0)
  await page.locator('header').getByRole('button', { name: '资源操作' }).click()
  for (const name of [
    '添加订阅',
    '新建本地分组',
    '手动添加节点',
    '导入文本',
    '更新全部订阅',
    '刷新资源',
  ])
    await expect(page.getByRole('menuitem', { name, exact: true })).toBeVisible()
  await page.screenshot({ path: `test-results/${test.info().project.name}-resource-actions.png` })
  await page.keyboard.press('Escape')
  await nav.getByRole('link', { name: '设置', exact: true }).click()
  await expect(page.locator('header').getByRole('button')).toHaveCount(0)
})
