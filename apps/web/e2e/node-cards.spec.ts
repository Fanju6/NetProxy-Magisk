import { expect, test } from '@playwright/test'

test('node cards share boundaries and right-aligned selection in list, grid and tabs', async ({
  page,
}) => {
  await page.goto('/#/nodes')
  const group = page.getByTestId('policy-Internet')
  const member = group.getByRole('button', { name: '选择 Hong Kong 01', exact: true })
  await expect(member).toBeEnabled()
  await member.click()
  await expect(member).toHaveAttribute('aria-pressed', 'true')
  const selected = group.locator('[data-slot="node-card"][data-selected="true"]')
  const inspect = async () => {
    const measurements = await page.locator('[data-slot="node-card"]').evaluateAll((cards) =>
      cards.map((card) => {
        const rect = card.getBoundingClientRect()
        const name = card.querySelector('[data-slot="node-name"]')!.getBoundingClientRect()
        const choice = card.querySelector('[data-slot="node-choice"]')!.getBoundingClientRect()
        const style = getComputedStyle(card)
        return {
          border: style.borderTopWidth,
          radius: parseFloat(style.borderRadius),
          inset: name.left - rect.left,
          rightOfName: choice.left >= name.right,
          overflow: card.scrollWidth > card.clientWidth,
        }
      }),
    )
    for (const item of measurements) {
      expect(item.border).toBe('1px')
      expect(item.radius).toBeLessThanOrEqual(8)
      expect(item.inset).toBeGreaterThanOrEqual(12)
      expect(item.inset).toBeLessThanOrEqual(14)
      expect(item.rightOfName).toBe(true)
      expect(item.overflow).toBe(false)
    }
    expect(await selected.locator('[data-slot="node-choice"] svg').count()).toBe(1)
    await expect(
      group
        .getByRole('button', { name: '选择 direct', exact: true })
        .locator('[data-slot="node-choice"] svg'),
    ).toHaveCount(0)
  }
  await inspect()
  await page.screenshot({ path: `test-results/${test.info().project.name}-cards-list-light.png` })
  const initial = await selected.boundingBox()
  await selected.hover()
  expect(await selected.boundingBox()).toEqual(initial)
  await page.evaluate(() => document.documentElement.classList.add('dark'))
  await page.getByRole('button', { name: '显示方式', exact: true }).click()
  await page.getByRole('menuitemradio', { name: '网格视图', exact: true }).click()
  await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(0)
  await inspect()
  await page.screenshot({ path: `test-results/${test.info().project.name}-cards-grid-dark.png` })
  await page.getByRole('button', { name: '显示方式', exact: true }).click()
  await page.getByRole('menuitemradio', { name: '标签视图', exact: true }).click()
  await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCount(0)
  await expect(page.getByRole('tab', { name: 'Internet', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await inspect()
  await page.screenshot({ path: `test-results/${test.info().project.name}-cards-tabs-dark.png` })
  await expect(member).toHaveAttribute('aria-pressed', 'true')
})
