import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'

// A menu anchored at the composer grows upward. The runtime row must stay at the
// bottom of its menu, because a late engine catalog then grows only the content
// above it: a moving trigger row would drag the runtime submenu out from under
// the pointer, which Radix reads as a pointer leave and turns into a dismissal.
test('keeps the runtime submenu open while the engine catalog arrives late', async ({ page, request }) => {
  test.setTimeout(90_000)
  const current = await (await request.get('/api/settings')).json()
  const seeded = await request.patch('/api/settings', { data: { layer: 'user', base_revision: current.revisions.user,
    changes: { theme: 'dark', agent_runtimes: { ide: { selected: 'native' }, general: { selected: 'native' }, interactive_story: { selected: 'native' } } } } })
  expect(seeded.ok(), await seeded.text()).toBe(true)
  const projectId = (await createAndOpenBook(request, 'Runtime menu layout')).projectId
  const created = await request.post('/api/sessions', { data: { title: 'Runtime menu layout' } })
  const sessionId = (await created.json()).id
  expect((await request.post('/api/sessions/switch', { data: { id: sessionId } })).ok()).toBe(true)
  const params = new URLSearchParams({ mode: 'writing', session_id: sessionId, story_id: '', branch_id: 'main' })
  const configPath = `/api/projects/${projectId}/conversation-config`
  const snapshot = await (await request.get(`${configPath}?${params}`)).json()
  // A conversation already running on an external engine reads its model list
  // from the engine catalog instead of saved settings.
  const antigravity = { ...snapshot, revision: snapshot.revision + 1,
    runtime: { kind: 'antigravity', antigravity: { model: 'catalog-model' } },
    runtime_capabilities: { cancel: true, goal: false, queue: true, pause: true } }
  const switches: string[] = []
  await page.route('**/conversation-config**', async (route) => {
    if (!route.request().url().includes(configPath)) { await route.continue(); return }
    if (route.request().method() === 'PATCH') {
      const runtime = route.request().postDataJSON()?.changes?.runtime
      if (runtime) switches.push(runtime.kind)
    }
    await route.fulfill({ json: antigravity })
  })
  await page.route('**/api/agent-runtimes/*/models', async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 1_500))
    await route.fulfill({ json: { default_id: 'catalog-model', items: [{ id: 'catalog-model', display_name: 'Catalog model', efforts: ['high'] }] } })
  })
  const statuses: Record<string, string> = { native: 'ready', codex: 'ready', claude: 'ready', grok: 'ready', antigravity: 'ready' }
  await page.route('**/api/agent-runtimes', (route) => route.fulfill({ json: { items: Object.entries(statuses).map(([id, status]) => ({ id, status })) } }))
  await page.goto('/')
  await openWritingAgent(page)
  await expect(page.locator('[contenteditable="true"]').filter({ visible: true }).last()).toBeVisible()
  const trigger = page.locator('[data-model-profile-trigger]').filter({ visible: true })
  await expect(trigger).toBeEnabled()
  await trigger.click()
  const menu = page.locator('[data-slot="dropdown-menu-content"]')
  const runtimeRow = page.getByRole('menuitem', { name: '运行时：Antigravity CLI', exact: true })
  await expect(runtimeRow).toBeVisible()
  const loadingBox = await menu.boundingBox()
  const rowBox = await runtimeRow.boundingBox()
  // The runtime row is the bottom row, so content above it cannot move it.
  expect(loadingBox!.y + loadingBox!.height - (rowBox!.y + rowBox!.height)).toBeLessThan(20)
  await page.mouse.move(rowBox!.x + 20, rowBox!.y + rowBox!.height / 2)
  await page.mouse.move(rowBox!.x + 21, rowBox!.y + rowBox!.height / 2 + 1)
  const submenu = page.locator('[data-slot="dropdown-menu-sub-content"]')
  await expect(submenu).toBeVisible()
  // The catalog answers after the submenu is open and grows the model list.
  await expect.poll(async () => (await menu.boundingBox())!.height).toBeGreaterThan(loadingBox!.height)
  await expect(submenu).toBeVisible()
  await expect(page.getByRole('menuitem', { name: 'Native', exact: true })).toBeVisible()
  const settledRow = await runtimeRow.boundingBox()
  expect(Math.abs(settledRow!.y - rowBox!.y)).toBeLessThan(2)
  await page.screenshot({ path: test.info().outputPath('runtime-row-last.png'), animations: 'disabled' })
  // Switching back to Native still commits from the bottom row.
  await page.getByRole('menuitem', { name: 'Native', exact: true }).click()
  await expect.poll(() => switches).toEqual(['native'])
})
