import { expect, test } from '@playwright/test'

for (const width of [360, 900, 1920]) {
  for (const theme of ['light', 'dark'] as const) {
    test(`main pages share layout at ${width}px in ${theme}`, async ({ page }, info) => {
      test.skip(info.project.name !== 'desktop', 'Explicit viewport matrix covers both navigations')
      await page.setViewportSize({ width, height: 900 })
      await page.addInitScript((theme) => {
        localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme, locale: 'zh' }))
      }, theme)
      const measurements = []
      for (const [route, title, ready] of [
        ['/', '仪表盘', 'main dl'],
        ['/nodes', '代理', '[data-testid="policy-Internet"]'],
        ['/subscriptions', '资源', 'main .page-section'],
        ['/settings', '设置', '#general-title'],
      ]) {
        await page.goto(`/#${route}`)
        await expect(page.locator('header h1')).toHaveText(title)
        await expect(page.locator(ready).first()).toBeVisible()
        await expect(
          page.locator('header').getByRole('button', { name: /^管理连接:/ }),
        ).toBeHidden()
        if (route === '/')
          await expect(page.getByRole('img', { name: '实时流量', exact: true })).toBeVisible()
        await page.evaluate(() => document.fonts.ready)
        const layout = await page.evaluate(() => {
          const main = document.querySelector('main')!
          const stack = main.querySelector('.page-stack')!
          const header = document.querySelector('.app-header-content')!
          const heading = document.querySelector('header h1')!
          const style = getComputedStyle(main)
          const contentLeft = main.getBoundingClientRect().left + parseFloat(style.paddingLeft)
          const label = main.querySelector('.section-label')
          const labelStyle = label ? getComputedStyle(label) : null
          return {
            top: main.getBoundingClientRect().top,
            contentTop: stack.getBoundingClientRect().top,
            left: stack.getBoundingClientRect().left,
            width: stack.getBoundingClientRect().width,
            titleLeft: heading.getBoundingClientRect().left,
            contentLeft,
            headerWidth: header.getBoundingClientRect().width,
            mainWidth: main.getBoundingClientRect().width,
            gap: getComputedStyle(stack).gap,
            label: labelStyle && {
              font: labelStyle.fontSize,
              weight: labelStyle.fontWeight,
              height: labelStyle.lineHeight,
              color: labelStyle.color,
              padding: labelStyle.padding,
            },
            sections: [...main.querySelectorAll('.page-section')].map((section) => ({
              gap: getComputedStyle(section).gap,
              left: section.getBoundingClientRect().left,
            })),
            buttons: [...header.querySelectorAll('button')].map((button) => ({
              width: button.getBoundingClientRect().width,
              height: button.getBoundingClientRect().height,
            })),
            overflow: document.documentElement.scrollWidth > window.innerWidth,
          }
        })
        expect(layout.overflow).toBe(false)
        expect(layout.contentTop - layout.top).toBe(16)
        expect(layout.titleLeft).toBe(layout.contentLeft)
        expect(layout.left).toBe(layout.contentLeft)
        expect(layout.headerWidth).toBe(layout.mainWidth)
        expect(layout.gap).toBe('24px')
        for (const section of layout.sections) {
          expect(section.gap).toBe('12px')
          expect(section.left).toBe(layout.left)
        }
        for (const button of layout.buttons) expect(button).toEqual({ width: 48, height: 48 })
        measurements.push(layout)
        await page.screenshot({
          path: `../../.agents/runs/v9-layout-consistency/screenshots/${width}-${theme}-${route.slice(1) || 'dashboard'}.png`,
          fullPage: true,
          animations: 'disabled',
        })
      }
      for (const layout of measurements) {
        expect(layout.top).toBe(measurements[0].top)
        expect(layout.width).toBe(measurements[0].width)
        expect(layout.left).toBe(measurements[0].left)
      }
      expect(measurements[2].label).toEqual(measurements[3].label)
      expect(measurements[3].label).toMatchObject({
        font: '14px',
        weight: '500',
        height: '20px',
        padding: '0px',
      })
      await info.attach('layout-measurements', {
        body: JSON.stringify(measurements, null, 2),
        contentType: 'application/json',
      })
    })
  }
}
