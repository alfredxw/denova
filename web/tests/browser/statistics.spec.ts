import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, saveProjectFile, createStartedStory } from '../support/api'
import type { StatisticsReport, UsageTotals } from '../../src/features/statistics/api'

test('statistics records saved manuscript changes and stays a shared destination', async ({ page, request, baseURL }) => {
  const book = await createAndOpenBook(request, 'Statistics observation')
  await createProjectFile(request, book.projectId, 'chapters/ch00001-Test.md', '文'.repeat(100))
  const read = async () => {
    const response = await request.get(`/api/statistics?days=1&timezone=UTC&project_id=${book.projectId}`)
    expect(response.ok()).toBe(true)
    return response.json() as Promise<StatisticsReport>
  }
  // Keep a real observation subscription open while API saves are verified.
  // SharedWorker requests do not belong to page.waitForResponse.
  const controller = new AbortController()
  const stream = await fetch(new URL(`/api/projects/${book.projectId}/events`, baseURL), { headers: { Accept: 'text/event-stream', 'Accept-Encoding': 'identity' }, signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10_000)]) })
  expect(stream.ok).toBe(true)
  const reader = stream.body!.getReader()
  await reader.read()
  try {
    await expect.poll(async () => (await read()).writing.projects[0]?.words).toBe(100)
    await saveProjectFile(request, book.projectId, 'chapters/ch00001-Test.md', '文'.repeat(130))
    await expect.poll(async () => (await read()).writing.days[0]?.delta).toBe(30)
    await saveProjectFile(request, book.projectId, 'chapters/ch00001-Test.md', '文'.repeat(80))
    await expect.poll(async () => (await read()).writing.days[0]?.delta).toBe(-20)
  } finally {
    controller.abort()
    await reader.cancel().catch(() => {})
  }
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await expect(sidebar).toBeVisible()

  for (const origin of ['写作', '游戏']) {
    await sidebar.getByRole('button', { name: origin, exact: true }).click()
    await sidebar.getByRole('button', { name: '统计', exact: true }).click()
    await expect(page.getByTestId('statistics-page')).toBeVisible()
    await expect(sidebar.locator('[aria-current="page"]')).toHaveCount(1)
    await expect(sidebar.getByRole('button', { name: '统计', exact: true })).toHaveAttribute('aria-current', 'page')
  }
  await page.getByRole('combobox', { name: '作品或项目' }).click()
  await page.getByRole('option', { name: book.title, exact: true }).click()
  await expect(page.getByTestId('statistics-page').getByRole('definition').filter({ hasText: /^-20$/ })).toBeVisible()
  const fresh = await read()
  expect(fresh.writing.projects).toHaveLength(1)
  expect(fresh.writing.projects[0].words).toBe(80)
  const session = await request.post('/api/sessions', { data: { title: 'Statistics model capture' } })
  expect(session.ok()).toBe(true)
  const { id } = await session.json()
  const generated = await request.post('/api/chat', { data: {
    session_id: id, command_id: `statistics-${id}`, message: 'Reply with a short acknowledgement.',
  } })
  expect(generated.ok(), await generated.text()).toBe(true)
  await expect.poll(async () => (await read()).usage.totals.requests).toBeGreaterThan(0)
  expect((await read()).usage.totals.tokens).toBeGreaterThan(0)
  const story = await createStartedStory(request, 'Game activity')
  await expect.poll(async () => (await read()).game.stories.find(item => item.id === story.id)?.turns).toBe(1)
  await page.getByRole('tab', { name: '游戏', exact: true }).click()
  await page.getByRole('button', { name: '刷新统计', exact: true }).click()
  await expect(page.getByRole('tabpanel').getByText('游戏回合', { exact: true })).toBeVisible()
  await expect(page.getByRole('tabpanel').getByText(story.title, { exact: true })).toBeVisible()

})

const totals: UsageTotals = { requests: 140, external_turns: 8, failed_requests: 2, unknown_usage: 3, prompt: 1_200_000, cached: 900_000, output: 300_000, tokens: 1_500_000, duration_ms: 840_000, ttft_ms: 16_000, ttft_requests: 100 }
const emptyTotals: UsageTotals = { requests: 0, external_turns: 0, failed_requests: 0, unknown_usage: 0, prompt: 0, cached: 0, output: 0, tokens: 0, duration_ms: 0, ttft_ms: 0, ttft_requests: 0 }

for (const theme of ['dark', 'light'] as const) {
  test(`statistics charts support ${theme}, narrow screens, filters and unknown usage`, async ({ page, request }) => {
    const book = await createAndOpenBook(request, 'Statistics charts')
    const settings = await (await request.get('/api/settings')).json()
    const updated = await request.patch('/api/settings', { data: {
      layer: 'user', base_revision: settings.revisions.user, changes: { theme },
    } })
    expect(updated.ok()).toBe(true)
    const queryScopes: string[] = []
    await page.route('**/api/statistics?*', async route => {
      const params = new URL(route.request().url()).searchParams
      queryScopes.push(params.get('project_id') || 'all')
      const days = Number(params.get('days'))
      const single = params.has('project_id')
      const weights = Array.from({ length: days }, (_, index) => 1 + index % 4)
      const sum = weights.reduce((value, weight) => value + weight, 0)
      let prefix = 0
      const series = weights.map((weight, index) => {
        const before = prefix
        prefix += weight
        const bucket = Object.fromEntries(Object.entries(totals).map(([key, value]) => [key, Math.floor(value * prefix / sum) - Math.floor(value * before / sum)])) as unknown as UsageTotals
        return { ...(single ? { ...emptyTotals, external_turns: 1, unknown_usage: 1 } : bucket), date: new Date(Date.UTC(2026, 9, 27 - days + index)).toISOString().slice(0, 10) }
      })
      const report: StatisticsReport = {
        projects: [{ id: book.projectId, name: book.title }],
        writing: {
          projects: [{ id: book.projectId, name: book.title, words: single ? 12345 : 67890, chapters: 18, confirmed: 12 }],
          days: Array.from({ length: days }, (_, index) => ({ date: series[index].date, delta: index === 0 ? null : index % 3 === 0 ? -150 : 600 })),
        },
        game: {
          stories: single ? [] : [
            { id: 'story-1', project_id: book.projectId, name: '旧车站与星光庭院：一条名字很长的故事线', turns: 24 },
            { id: 'story-2', project_id: book.projectId, name: '雾港来信', turns: 12 },
            { id: 'story-3', project_id: book.projectId, name: '最后一盏灯', turns: 6 },
          ],
          days: series.map((day, index) => ({ date: day.date, turns: single ? 0 : Math.floor(42 * weights.slice(0, index + 1).reduce((a, b) => a + b, 0) / sum) - Math.floor(42 * weights.slice(0, index).reduce((a, b) => a + b, 0) / sum) })),
          new_branches: single ? 0 : 6,
        },
        usage: {
          totals: single ? { ...emptyTotals, external_turns: 1, unknown_usage: 1 } : totals,
          days: series,
          models: single ? [] : [{ ...totals, name: 'Long-model-name-with-a-large-context-window', runtime: 'native' }],
        },
        issues: 0,
      }
      await route.fulfill({ json: report })
    })
    await page.setViewportSize({ width: 1440, height: 1100 })
    await page.goto('/')
    await page.getByLabel('工作台侧边栏').getByRole('button', { name: '统计', exact: true }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    const panel = page.getByTestId('statistics-page')
    await expect(panel.getByText('67,890', { exact: true })).toBeVisible()
    await expect(panel.locator('.recharts-surface')).toHaveCount(3)
    await page.screenshot({ animations: 'disabled', path: test.info().outputPath(`writing-${theme}.png`) })
    await page.getByRole('tab', { name: '游戏', exact: true }).click()
    await expect(panel.getByRole('definition').filter({ hasText: /^42$/ })).toBeVisible()
    await expect(panel.locator('.recharts-surface')).toHaveCount(2)
    await page.screenshot({ animations: 'disabled', path: test.info().outputPath(`game-${theme}.png`) })
    await page.setViewportSize({ width: 390, height: 844 })
    await expect.poll(async () => panel.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
    await page.screenshot({ animations: 'disabled', path: test.info().outputPath(`game-${theme}-narrow.png`) })
    await page.getByRole('combobox', { name: '作品或项目' }).click()
    await page.getByRole('option', { name: book.title, exact: true }).click()
    await expect(panel.getByText('这段时间暂无游戏回合', { exact: true })).toBeVisible()
    await expect(panel.locator('.recharts-surface')).toHaveCount(0)
    await page.getByRole('combobox', { name: '作品或项目' }).click()
    await page.getByRole('option', { name: '全部项目', exact: true }).click()
    await expect(panel.getByRole('definition').filter({ hasText: /^42$/ })).toBeVisible()
    await page.setViewportSize({ width: 1440, height: 1100 })
    await page.getByRole('tab', { name: '模型调用', exact: true }).click()
    await expect(panel.getByText('75.0%', { exact: true })).toBeVisible()
    await page.getByRole('radio', { name: '平均响应耗时', exact: true }).click()
    await expect(panel.locator('.recharts-surface')).toHaveCount(3)
    await page.screenshot({ animations: 'disabled', path: test.info().outputPath(`models-${theme}.png`) })
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(panel).toBeVisible()
    await expect.poll(async () => panel.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
    await page.screenshot({ animations: 'disabled', path: test.info().outputPath(`models-${theme}-narrow.png`) })
    await page.getByRole('combobox', { name: '作品或项目' }).click()
    await page.getByRole('option', { name: book.title, exact: true }).click()
    await expect(panel.getByText('1 次未报告用量', { exact: true })).toBeVisible()
    await expect(panel.getByText('尚无原生模型调用记录')).toBeVisible()
    expect(queryScopes).toContain(book.projectId)
    await page.getByRole('radio', { name: '30 天', exact: true }).click()
    await expect(panel.getByText('1 次未报告用量', { exact: true })).toBeVisible()
  })
}
