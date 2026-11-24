import { expect, test } from './fixtures'

test.use({ trace: 'off' })

test('direct loopback tabs create independent browser sessions', async ({ browser, poomsae }) => {
  const firstContext = await browser.newContext()
  const firstPage = await firstContext.newPage()
  await firstPage.goto(`${poomsae.origin}/poomsae?view=all-open`)
  await expect(firstPage.getByRole('button', { name: 'New task' })).toBeVisible()

  const secondContext = await browser.newContext()
  const secondPage = await secondContext.newPage()
  await secondPage.goto(`${poomsae.origin}/poomsae?view=all-open`)
  await expect(secondPage.getByRole('button', { name: 'New task' })).toBeVisible()

  await firstContext.close()
  await secondContext.close()
})

test('loopback launch stores tab credentials and reaches writable authority', async ({
  page,
  poomsae,
}) => {
  const credentials = await poomsae.launch(page)
  expect(credentials.session).not.toEqual(credentials.csrf)
  const snapshot = await poomsae.snapshot(page, credentials)
  expect(snapshot).toEqual(expect.objectContaining({ capabilities: expect.any(Object) }))
})
