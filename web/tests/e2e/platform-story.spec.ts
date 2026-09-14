import { createExampleSource } from '../support/platform'
import { expect, test, type APIRequestContext } from '../support/fixtures'
import { createAndOpenBook, createStartedStory, getStorySnapshot } from '../support/api'

test.use({ actionTimeout: 15_000 })

async function installExample(request: APIRequestContext, id: string) {
  const source = await createExampleSource(request, 'galgame', 'game', id)
  const checkedResponse = await request.get(`/api/platform/manage/development/${source.developmentId}/check`)
  expect(checkedResponse.ok(), await checkedResponse.text()).toBe(true)
  const checked = await checkedResponse.json()
  const installed = await request.post('/api/platform/manage/packages/install', { data: { candidateId: checked.candidateId, grants: checked.manifest.permissions.required } })
  expect(installed.ok(), await installed.text()).toBe(true)
  return installed.json()
}

test('composes the existing Story, library and image service without a presentation model pass', async ({ page, request }) => {
  test.slow()
  const book = await createAndOpenBook(request, 'Visual Novel Composition')
  const original = await createStartedStory(request, 'Original station story')
  const before = await getStorySnapshot(request, original.id)
  const lore = await request.post(`/api/projects/${book.projectId}/book/lore/items`, { data: {
    type: 'character', name: 'Mira from the library', enabled: true,
    content: 'An adult archivist with a red scarf, keeper of forgotten station letters.',
  } })
  expect(lore.ok(), await lore.text()).toBe(true)
  await installExample(request, 'test.visual-novel')
  await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme: 'dark' } } })
  await page.addInitScript(() => {
    if (!localStorage.getItem('nova:mode')) localStorage.setItem('nova:mode', 'interactive')
    if (!localStorage.getItem('nova.locale.configured')) localStorage.setItem('nova.locale.configured', 'en-US')
    if (!localStorage.getItem('theme')) localStorage.setItem('theme', 'dark')
  })
  await page.goto('/')
  await page.getByLabel('Workbench sidebar').getByRole('button', { name: 'Game', exact: true }).click()
  await page.getByRole('button', { name: 'New', exact: true }).filter({ visible: true }).click()
  await page.getByLabel('Game type').click()
  await page.getByRole('option', { name: 'test.visual-novel', exact: true }).click()
  await page.getByLabel('Storyline name (optional)').fill('A different view of the station')
  await page.getByLabel('Story', { exact: true }).click()
  await page.getByRole('option', { name: 'Original station story', exact: true }).click()
  const modelSelects = page.getByRole('combobox').filter({ hasText: 'Choose a model profile' })
  await modelSelects.first().click()
  await page.getByRole('option', { name: 'E2E deterministic model', exact: true }).click()
  await modelSelects.first().click()
  await page.getByRole('option', { name: 'E2E image model', exact: true }).click()
  await page.getByRole('button', { name: 'Create storyline', exact: true }).click()
  const frame = page.frameLocator('iframe[title="Game"]')
  await expect(frame.locator('#begin')).toBeEnabled()
  await frame.locator('#historyButton').click()
  await expect(frame.locator('#historyList')).toContainText(before.turns[0].narrative!)
  await frame.locator('[data-close="historyDialog"]').click()
  await frame.locator('#castButton').click()
  await frame.locator('#castDialog summary').click()
  await expect(frame.locator('#libraryList')).toContainText('Mira from the library')
  await frame.locator('#libraryList article').filter({ hasText: 'Mira from the library' }).getByRole('button', { name: 'Add character', exact: true }).click()
  await frame.locator('#saveCast').click()
  await expect(frame.locator('#castDialog')).not.toBeVisible()
  await frame.locator('#begin').click()
  await expect(frame.locator('#line')).toHaveText(before.turns[0].narrative!, { timeout: 30_000 })
  expect((await getStorySnapshot(request, original.id)).turns).toEqual(before.turns)
  await frame.locator('#artButton').click()
  await frame.locator('#artList article').filter({ hasText: 'Background' }).getByRole('button', { name: 'Generate', exact: true }).click()
  await expect(frame.locator('#artStatus')).toHaveText('Illustration saved', { timeout: 30_000 })
  await frame.locator('[data-close="artDialog"]').click()
  await expect(frame.locator('#background')).toBeVisible()
  await expect.poll(() => frame.locator('#background').evaluate((image: HTMLImageElement) => image.naturalWidth)).toBeGreaterThan(0)
  await page.screenshot({ path: 'test-results/galgame-dark-wide.png', fullPage: true })
  const counts = await (await request.get('http://127.0.0.1:18081/control/status')).json()
  await page.reload()
  await expect(frame.locator('#line')).toHaveText(before.turns[0].narrative!)
  await expect(frame.locator('#background')).toBeVisible()
  expect((await (await request.get('http://127.0.0.1:18081/control/status')).json()).request_counts).toEqual(counts.request_counts)
  await frame.locator('#input').fill('E2E_GALGAME_STREAM Read the letter together.')
  await frame.locator('#send').click()
  try {
    await expect(frame.locator('#line')).toHaveText('The lamp')
    await expect(frame.locator('#speaker')).toHaveText('Lin')
    expect((await getStorySnapshot(request, original.id)).turns).toHaveLength(1)
  } finally {
    await request.post('http://127.0.0.1:18081/control/release', { data: { marker: 'E2E_GALGAME_STREAM' } })
  }
  await expect.poll(async () => (await getStorySnapshot(request, original.id)).turns.length).toBe(2)
  await expect(frame.locator('#line')).toHaveText('The lamp is still warm.', { timeout: 30_000 })
  for (const language of ['en-US', 'zh-CN']) {
    const theme = language === 'en-US' ? 'light' : 'dark'
    await request.patch('/api/settings', { data: { layer: 'user', changes: { language, theme } } })
    await page.evaluate(({ language, theme }) => { localStorage.setItem('theme', theme); localStorage.setItem('nova.locale.configured', language) }, { language, theme })
    await page.reload()
    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: 900 })
      const gameFrame = page.frameLocator('iframe[title="' + (language === 'zh-CN' ? '游戏画面' : 'Game') + '"]')
      await expect(gameFrame.locator('#line')).toBeVisible()
      expect(await gameFrame.locator('html').evaluate(element => element.scrollWidth <= window.innerWidth)).toBe(true)
      await page.screenshot({ path: `test-results/galgame-${language}-${theme}-${width}.png`, fullPage: true })
    }
  }
  const instances = await (await request.get('/api/platform/manage/instances')).json()
  const instance = instances.find((item: { gameId: string }) => item.gameId === 'test.visual-novel')
  expect(instance).toMatchObject({ storyId: original.id, projectId: book.projectId })
  const removed = await request.delete(`/api/platform/manage/instances/${instance.instanceId}`)
  expect(removed.ok(), await removed.text()).toBe(true)
  expect((await getStorySnapshot(request, original.id)).turns).toHaveLength(2)
})

test('a preview generates its own Story while the normal story remains selected', async ({ request }) => {
  const book = await createAndOpenBook(request, 'Isolated Visual Novel Preview')
  const original = await createStartedStory(request, 'Keep this story selected')
  await installExample(request, 'test.visual-novel-preview')
  const sources = await (await request.get('/api/platform/manage/development')).json()
  const source = sources.find((item: { manifest?: { id: string } }) => item.manifest?.id === 'test.visual-novel-preview')
  expect(source).toBeDefined()
  const checked = await (await request.get(`/api/platform/manage/development/${source.developmentId}/check`)).json()
  const prepared = await request.post(`/api/platform/manage/candidates/${checked.candidateId}/prepare-preview`, { data: { grants: checked.manifest.permissions.required } })
  expect(prepared.ok(), await prepared.text()).toBe(true)
  const release = await prepared.json()
  const created = await request.post('/api/platform/manage/instances', { data: {
    gameId: release.manifest.id, releaseId: release.ref.releaseId, projectId: book.projectId,
    title: 'Independent test story', preview: true, models: { 'local:writer': 'e2e' },
  } })
  expect(created.ok(), await created.text()).toBe(true)
  const instance = await created.json()
  const opened = await request.post(`/api/platform/manage/instances/${instance.instanceId}/open`, { data: { locale: 'en-US', theme: 'light' } })
  expect(opened.ok(), await opened.text()).toBe(true)
  const runtime = await opened.json()
  const headers = { Authorization: `Bearer ${runtime.connection.token}` }
  const started = await request.post(runtime.connection.baseUrl + '/story/commands', { headers, data: {
    kind: 'advance', commandId: 'preview-opening', message: 'E2E_EXTENSION_OPENING: Start a story in the old train station.', locale: 'en-US',
  } })
  expect(started.ok(), await started.text()).toBe(true)
  try {
    await expect.poll(async () => (await (await request.get(runtime.connection.baseUrl + '/story', { headers })).json()).turns.length).toBe(1)
    const visible = await (await request.get('/api/interactive/stories')).json()
    expect(visible.stories.map((item: { id: string }) => item.id)).toContain(original.id)
    expect(visible.stories.map((item: { id: string }) => item.id)).not.toContain(instance.storyId)
    expect(visible.current_story_id).toBe(original.id)
  } finally {
    const stopped = await request.post(`/api/platform/manage/runtimes/${runtime.id}/stop`, { data: {} })
    expect(stopped.ok(), await stopped.text()).toBe(true)
  }
})
