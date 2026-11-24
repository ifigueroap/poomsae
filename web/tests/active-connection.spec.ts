import { expect, test } from './fixtures'

test.use({ colorScheme: 'dark', reducedMotion: 'reduce' })

test('read-only notices remain visible', async ({ page, poomsae }) => {
  let inventoryReads = 0
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/v1/tokens') inventoryReads++
  })
  const credentials = await poomsae.launch(page)
  const issue = await poomsae.seedIssue(page, credentials, { title: 'Review example release' })
  await page.goto(`${poomsae.origin}/poomsae?view=all-open`)
  await expect(page.getByRole('button', { name: /Review example release/ })).toBeVisible()
  const summary = page.getByRole('status', { name: 'Active connection' })

  await poomsae.restartReadonly()
  try {
    await page.goto(`${poomsae.origin}/poomsae?issue=${issue.uid}`)
    await expect(summary).toContainText('Credential: Read-only')
    await page.setViewportSize({ width: 1440, height: 900 })
    await expect(summary).not.toContainText('example-local-token')
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      const layout = await summary.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        const header = document
          .querySelector(
            window.innerWidth <= 760 ? '.mobile-navigation-trigger' : '.sidebar-header',
          )!
          .getBoundingClientRect()
        return {
          top: rect.top,
          bottom: rect.bottom,
          headerTop: header.top,
          left: rect.left,
          right: rect.right,
          viewport: window.innerWidth,
        }
      })
      expect(layout.top).toBe(0)
      expect(layout.bottom).toBeLessThanOrEqual(layout.headerTop)
      expect(layout.left).toBeGreaterThanOrEqual(0)
      expect(layout.right).toBeLessThanOrEqual(layout.viewport)
    }
  } finally {
    await poomsae.restart()
  }
  expect(inventoryReads).toBe(0)
})
