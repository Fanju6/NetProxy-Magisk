import { expect, test } from '@playwright/test'

test('service transitions retain chart and mode positions without replaying writes', async ({
  page,
}) => {
  await page.route('**/src/mocks/transport.ts', async (route) => {
    const response = await route.fetch()
    await route.fulfill({
      response,
      body: `${await response.text()}
      const original = MockTransport.prototype.request;
      window.transitionEvidence = { writes: [], states: [], streams: 0 };
      const subscribe = MockTransport.prototype.subscribe;
      MockTransport.prototype.subscribe = function(...args) {
        window.transitionEvidence.streams++;
        return subscribe.apply(this, args);
      };
      MockTransport.prototype.request = async function(request, signal) {
        if (request.method === 'POST' && /service\\/(start|stop)$/.test(request.path)) {
          window.transitionEvidence.writes.push(request.path);
          for (const state of request.path.endsWith('start') ? ['preparing', 'starting'] : ['stopping']) {
            this.state.service.state = state;
            window.transitionEvidence.states.push(state);
            this.emit('status');
            await new Promise(resolve => setTimeout(resolve, 150));
          }
        }
        return original.call(this, request, signal);
      };`,
    })
  })
  await page.goto('/')
  await expect(page.getByRole('img', { name: '实时流量', exact: true })).toBeVisible()
  const positions = () =>
    page.evaluate(() => ({
      speed: document.querySelector('[aria-label="实时速率"]')!.getBoundingClientRect().top,
      mode: document.querySelector('[aria-label="出站模式"]')!.getBoundingClientRect().top,
    }))
  const before = await positions()
  await page.evaluate(() => {
    const probe = window as unknown as {
      transitionFrames: number[][]
      transitionErrors: string[]
      frameTimer: number
    }
    probe.transitionFrames = []
    probe.transitionErrors = []
    probe.frameTimer = window.setInterval(() => {
      for (const alert of document.querySelectorAll('main [role="alert"]'))
        probe.transitionErrors.push(alert.textContent ?? '')
      if (document.querySelector('main h2')?.textContent === '启动失败')
        probe.transitionErrors.push('Unexpected failed service state')
      probe.transitionFrames.push(
        ['实时速率', '出站模式'].map(
          (name) => document.querySelector(`[aria-label="${name}"]`)!.getBoundingClientRect().top,
        ),
      )
    }, 20)
  })
  await page.getByRole('button', { name: '停止服务', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '确定', exact: true }).click()
  await expect(page.getByRole('heading', { name: '已停止', exact: true })).toBeVisible()
  expect(await positions()).toEqual(before)
  await page.getByRole('button', { name: '开启服务', exact: true }).click()
  await expect(page.getByRole('heading', { name: '运行中', exact: true })).toBeVisible()
  expect(await positions()).toEqual(before)
  const evidence = await page.evaluate(
    () =>
      (
        window as unknown as {
          transitionEvidence: { writes: string[]; states: string[]; streams: number }
        }
      ).transitionEvidence,
  )
  expect(evidence.writes).toEqual(['/service/stop', '/service/start'])
  expect(evidence.states).toEqual(['stopping', 'preparing', 'starting'])
  const frames = await page.evaluate(() => {
    const probe = window as unknown as { transitionFrames: number[][]; frameTimer: number }
    clearInterval(probe.frameTimer)
    return probe.transitionFrames
  })
  expect(frames.length).toBeGreaterThan(5)
  for (const [speed, mode] of frames) {
    expect(speed).toBeCloseTo(before.speed, 0)
    expect(mode).toBeCloseTo(before.mode, 0)
  }
  expect(
    await page.evaluate(
      () => (window as unknown as { transitionErrors: string[] }).transitionErrors,
    ),
  ).toEqual([])
})
