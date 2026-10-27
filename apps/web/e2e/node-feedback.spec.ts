import { expect, test } from '@playwright/test'

test('selection preserves policy cards and traffic while confirming one write', async ({
  page,
}) => {
  await page.route('**/src/mocks/transport.ts', async (route) => {
    const response = await route.fetch()
    await route.fulfill({
      response,
      body: `${await response.text()}
      window.selectionEvidence = { streams: 0, writes: 0, frames: [] };
      const subscribe = MockTransport.prototype.subscribe;
      MockTransport.prototype.subscribe = function(...args) {
        window.selectionEvidence.streams++;
        return subscribe.apply(this, args);
      };
      const request = MockTransport.prototype.request;
      MockTransport.prototype.request = function(input, signal) {
        if (input.method === 'POST' && input.path === '/policy-groups/select') window.selectionEvidence.writes++;
        return request.call(this, input, signal);
      };`,
    })
  })
  await page.goto('/#/nodes')
  const group = page.getByTestId('policy-Internet')
  const target = group.getByRole('button', { name: '选择 Hong Kong 01', exact: true })
  await expect(target).toBeEnabled()
  const before = await page.evaluate(
    () =>
      (window as unknown as { selectionEvidence: { streams: number } }).selectionEvidence.streams,
  )
  await page.evaluate(() => {
    const state = window as unknown as {
      selectionEvidence: { frames: { top: number; warning: boolean }[] }
      sample: number
    }
    state.sample = window.setInterval(() => {
      state.selectionEvidence.frames.push({
        top: document.querySelector('[data-testid="policy-Internet"]')!.getBoundingClientRect().top,
        warning: /历史快照|实时状态暂不可用|结果待确认/.test(
          document.querySelector('main')!.textContent!,
        ),
      })
    }, 20)
  })
  const top = (await group.boundingBox())!.y
  await target.click()
  await expect(target).toHaveAttribute('aria-pressed', 'true')
  await expect(target).toBeEnabled()
  const after = await page.evaluate(() => {
    const state = window as unknown as {
      selectionEvidence: {
        streams: number
        writes: number
        frames: { top: number; warning: boolean }[]
      }
      sample: number
    }
    clearInterval(state.sample)
    return state.selectionEvidence
  })
  expect(after.writes).toBe(1)
  expect(after.streams - before).toBeLessThanOrEqual(1)
  expect(after.frames.length).toBeGreaterThan(2)
  for (const frame of after.frames) {
    expect(frame.warning).toBe(false)
    expect(frame.top).toBe(top)
  }
})

test('unknown selection stays unconfirmed with one actionable toast and never replays POST', async ({
  page,
}) => {
  await page.route('**/src/mocks/transport.ts', async (route) => {
    const response = await route.fetch()
    await route.fulfill({
      response,
      body: `${await response.text()}
      window.unknownWrites = 0;
      const request = MockTransport.prototype.request;
      MockTransport.prototype.request = function(input,signal) {
        if (input.method === 'POST' && input.path === '/policy-groups/select') {
          window.unknownWrites++;
          return Promise.resolve({status:502,body:{schema:1,ok:false,code:'policy.api_failed',message:'PRIVATE_UNKNOWN_MARKER',data:{accepted:true}}});
        }
        return request.call(this,input,signal);
      };`,
    })
  })
  await page.goto('/#/nodes')
  const group = page.getByTestId('policy-Internet')
  const member = group.getByRole('button', { name: '选择 Hong Kong 01', exact: true })
  await expect(member).toBeEnabled()
  const top = (await group.boundingBox())!.y
  await member.click()
  const toast = page.locator('[data-slot="toast"]')
  await expect(toast).toHaveCount(1)
  await expect(toast).toContainText('策略组选择结果待确认')
  await expect(member).toHaveAttribute('aria-pressed', 'false')
  await expect(member).toBeDisabled()
  await toast.getByRole('button', { name: '检查状态', exact: true }).click()
  await expect(toast).toHaveCount(1)
  expect(
    await page.evaluate(() => (window as unknown as { unknownWrites: number }).unknownWrites),
  ).toBe(1)
  expect((await group.boundingBox())!.y).toBe(top)
  await expect(page.getByText('PRIVATE_UNKNOWN_MARKER')).toHaveCount(0)
  await toast.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(toast).toHaveCount(0)
  await expect(
    page.locator('header').getByRole('button', { name: '检查状态', exact: true }),
  ).toBeVisible()
})

test('rejected selection uses one sanitized toast without moving the list or replaying writes', async ({
  page,
}) => {
  await page.route('**/src/mocks/transport.ts', async (route) => {
    const response = await route.fetch()
    await route.fulfill({
      response,
      body: `${await response.text()}
      window.rejectedWrites = 0;
      const request = MockTransport.prototype.request;
      MockTransport.prototype.request = function(input, signal) {
        if (input.method === 'POST' && input.path === '/policy-groups/select') {
          window.rejectedWrites++;
          return Promise.resolve({status:403,body:{schema:1,ok:false,code:'policy.not_selectable',message:'PRIVATE_MARKER_NOT_FOR_UI',data:{}}});
        }
        return request.call(this,input,signal);
      };`,
    })
  })
  await page.goto('/#/nodes')
  const group = page.getByTestId('policy-Internet')
  const member = group.getByRole('button', { name: '选择 Hong Kong 01', exact: true })
  await expect(member).toBeEnabled()
  const top = (await group.boundingBox())!.y
  await member.click()
  const toast = page.locator('[data-slot="toast"]')
  await expect(toast).toHaveCount(1)
  await expect(toast).toContainText('当前连接未授权')
  await expect(page.getByText('PRIVATE_MARKER_NOT_FOR_UI')).toHaveCount(0)
  await expect(member).toHaveAttribute('aria-pressed', 'false')
  await expect(member).toBeEnabled()
  expect((await group.boundingBox())!.y).toBe(top)
  expect(
    await page.evaluate(() => (window as unknown as { rejectedWrites: number }).rejectedWrites),
  ).toBe(1)
  await page.screenshot({
    path: `test-results/${test.info().project.name}-selection-error-toast.png`,
  })
  await toast.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(toast).toHaveCount(0)
  await page.getByRole('searchbox', { name: '搜索代理' }).fill('Hong')
  await expect(toast).toHaveCount(0)
})
