import { expect, test } from './fixtures'

test.use({ trace: 'off' })

test('token-configured direct loopback tab loads the workspace snapshot', async ({
  page,
  poomsae,
}) => {
  await page.goto(`${poomsae.origin}/poomsae?view=all-open`)
  await expect(page.getByRole('button', { name: 'New task' })).toBeVisible()
})
