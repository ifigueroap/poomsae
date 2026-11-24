import { expect, test } from './fixtures'

test.use({ trace: 'off' })

test('the root route visibly normalizes to Inbox', async ({ page, poomsae }) => {
  await poomsae.launch(page, '/')
  await expect(page).toHaveURL(`${poomsae.origin}/poomsae`)
  await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible()
})

test('canonical routes survive reload and UID issue deep links', async ({ page, poomsae }) => {
  const credentials = await poomsae.launch(page, '/poomsae?view=all-open')
  const issue = await poomsae.seedIssue(page, credentials, { title: 'Deep link task' })

  await page.goto(`${poomsae.origin}/poomsae?issue=${issue.uid}`)
  await expect(page.getByRole('region', { name: 'Task detail' })).toContainText('Deep link task')
  await page.reload()
  await expect(page).toHaveURL(`${poomsae.origin}/poomsae?issue=${issue.uid}`)
  await expect(page.getByRole('region', { name: 'Task detail' })).toContainText('Deep link task')
})

test('opens the graph from a qualified short link with a different project filter', async ({
  page,
  poomsae,
}) => {
  const credentials = await poomsae.launch(page, '/poomsae?view=all-open')
  const issue = await poomsae.seedIssue(page, credentials, { title: 'Other project detail' })
  const url = `${poomsae.origin}/poomsae?scope=example-inbox&issue=example-project%23${issue.short_id}`
  await page.goto(url)
  await expect(page.getByRole('heading', { name: 'example-inbox', exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Task detail' })).toContainText(
    'Other project detail',
  )
  await page
    .getByRole('region', { name: 'Task detail' })
    .getByRole('button', { name: 'Graph' })
    .click()
  await expect(page).toHaveURL(`${url}&graph=1`)
  await expect(page.getByRole('region', { name: 'Reachable task graph' })).toBeVisible()
})

test('issue selection preserves the active Forge project scope', async ({ page, poomsae }) => {
  const credentials = await poomsae.launch(page, '/poomsae?view=all-open')
  const issue = await poomsae.seedIssue(page, credentials, { title: 'Scoped route task' })

  await page.getByRole('button', { name: /^example-project \d+$/ }).click()
  await expect(page).toHaveURL(`${poomsae.origin}/poomsae?scope=${poomsae.projectUID}`)
  await page.getByRole('button', { name: /Scoped route task/ }).click()

  await expect(page).toHaveURL(
    `${poomsae.origin}/poomsae?scope=${poomsae.projectUID}&issue=${issue.uid}`,
  )
  await expect(page.getByRole('heading', { name: 'example-project', exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Task detail' })).toContainText('Scoped route task')
})

test('short issue references remain invalid routes with an explicit search recovery', async ({
  page,
  poomsae,
}) => {
  await poomsae.launch(page)
  const issue = await poomsae.seedIssue(page, await storedCredentials(page), {
    title: 'Short reference search task',
  })
  await page.goto(`${poomsae.origin}/poomsae?issue=${issue.short_id}`)
  await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible()
  await page.getByRole('button', { name: `Search for ${issue.short_id}` }).click()
  await expect(page).toHaveURL(
    new RegExp(`/poomsae\\?view=all-open&status=all&text=${encodeURIComponent(issue.short_id)}`),
  )
})

async function storedCredentials(page: import('@playwright/test').Page) {
  return page.evaluate(() =>
    JSON.parse(sessionStorage.getItem('poomsae.web.session.v1')!),
  ) as Promise<{
    session: string
    csrf: string
  }>
}
