import { expect, test } from './fixtures'

test.use({ browserName: 'firefox', trace: 'off' })

test(
  'direct loopback tab loads the workspace snapshot in Firefox',
  { tag: '@firefox' },
  async ({ page, poomsae }) => {
    await page.goto(`${poomsae.origin}/poomsae?view=all-open`)
    await expect(page.getByRole('button', { name: 'New task' })).toBeVisible()
  },
)
