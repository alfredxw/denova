import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test('creates independent Projects when a scenario reuses its readable title', async ({ request }) => {
  const first = await createAndOpenBook(request, 'Repeated scenario')
  const second = await createAndOpenBook(request, 'Repeated scenario')
  expect(first.projectId).not.toBe(second.projectId)
  expect(first.workspace).not.toBe(second.workspace)
  expect(first.title).not.toBe(second.title)
})

test('blocks unmocked public requests before they reach the network', async ({ page, browserDiagnostics }) => {
  const url = 'https://example.invalid/unmocked-test-dependency'
  browserDiagnostics.allow(/^network\.external: GET https:\/\/example\.invalid\/unmocked-test-dependency$/)
  browserDiagnostics.allow(/console\.error: Failed to load resource: net::ERR_BLOCKED_BY_CLIENT.*example\.invalid/)
  const failed = page.waitForEvent('requestfailed', request => request.url() === url)
  await page.evaluate(url => fetch(url).catch(() => undefined), url)
  expect((await failed).failure()?.errorText).toMatch(/^net::ERR_BLOCKED_BY_CLIENT(?:\.Inspector)?$/)
})

test('allows explicitly mocked public responses without external network access', async ({ page }) => {
  const url = 'https://example.invalid/mocked-test-dependency'
  await page.route(url, route => route.fulfill({
    headers: { 'Access-Control-Allow-Origin': '*' }, json: { source: 'fixture' },
  }))
  expect(await page.evaluate(url => fetch(url).then(response => response.json()), url)).toEqual({ source: 'fixture' })
})
