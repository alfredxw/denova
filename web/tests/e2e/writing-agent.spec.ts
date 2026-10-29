import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, readProjectFile } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { getModelStatus, modelControlURL, releaseDelayedRequest } from '../support/model'

const delayedReplyMarker = 'E2E_DELAYED_AGENT_REPLY'

test('applies an Agent edit and supports review, undo, and redo', { tag: '@smoke' }, async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Writing Agent E2E Book')
  await request.patch('/api/settings', { data: { layer: 'user', changes: { motion_intensity: 'full' } } })
  const chapterPath = 'chapters/e2e-agent-chapter.md'
  const original = '# Agent 测试\n\nAgent 修改前。'
  await createProjectFile(request, book.projectId, chapterPath, original)

  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '写作', exact: true }).click()
  await page.getByRole('button', { name: /^e2e agent chapter\b/ }).click()
  const composer = await openWritingAgent(page)
  // Observe real row measurements: opacity alone would still insert all of the
  // review's height in one frame and move the reading position abruptly.
  await page.evaluate(() => {
    const samples: number[] = []
    Object.assign(window, { reviewHeights: samples })
    const sample = () => {
      const card = document.querySelector('[data-nova-timeline-attachment]')
      if (card) samples.push(card.getBoundingClientRect().height)
      if (samples.length < 60) requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  await submitAgentChatMessage(page, composer, 'Update the chapter for the deterministic scenario. E2E_EDIT_CHAPTER')

  const summary = page.getByRole('region', { name: '已编辑 1 个文件' })
  await expect(summary).toBeVisible()
  await expect.poll(() => page.evaluate(() => {
    const heights = (window as unknown as { reviewHeights: number[] }).reviewHeights
    return heights.some(height => height > 1 && height < Math.max(...heights) - 1)
  })).toBe(true)
  await expect.poll(() => page.locator('[data-nova-timeline-attachment]').evaluate(element => (element as HTMLElement).style.height)).toBe('auto')
  await expect(page.locator('[data-agent-execution-process]').getByRole('button').last()).toHaveAttribute('aria-expanded', 'true')
  await page.screenshot({ path: test.info().outputPath('completed-agent-review.png') })
  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content)
    .toContain('Agent 已通过工具完成修改。')

  await submitAgentChatMessage(page, composer, 'Keep the previous process open. E2E_DELAYED_AGENT_REPLY')
  try {
    await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[delayedReplyMarker] ?? 0).toBe(1)
    await expect(page.locator('[data-agent-execution-process]').getByRole('button').first()).toHaveAttribute('aria-expanded', 'true')
  } finally {
    await releaseDelayedRequest(request, delayedReplyMarker)
  }
  await expect(page.getByText('Recovered response completed exactly once.', { exact: true })).toBeVisible()

  await summary.getByRole('button', { name: '审阅', exact: true }).click()
  const review = page.getByRole('region', { name: '变更审阅' })
  await expect(review).toBeVisible()
  await review.getByRole('button', { name: '接受本轮', exact: true }).click()

  const undo = review.getByRole('button', { name: '撤销整组', exact: true })
  await expect(undo).toBeEnabled()
  await undo.click()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content).toBe(original)

  const redo = review.getByRole('button', { name: '重做整组', exact: true })
  await expect(redo).toBeEnabled()
  await redo.click()
  await expect.poll(async () => (await readProjectFile(request, book.projectId, chapterPath)).content)
    .toContain('Agent 已通过工具完成修改。')

  // Reopening history must render the existing review at its full height.
  await page.addInitScript(() => {
    const entries: string[] = []
    Object.assign(window, { historicalReviewHeights: entries })
    const observer = new MutationObserver(() => {
      const card = document.querySelector<HTMLElement>('[data-nova-timeline-attachment]')
      if (card) entries.push(card.style.height)
    })
    observer.observe(document, { childList: true, subtree: true, attributes: true, attributeFilter: ['style'] })
  })
  await page.reload()
  await openWritingAgent(page)
  await expect(summary).toBeVisible()
  expect(await page.evaluate(() => {
    const heights = (window as unknown as { historicalReviewHeights: string[] }).historicalReviewHeights
    return heights.length > 0 && heights.every(value => value === '' || value === 'auto')
  })).toBe(true)
  await expect(page.locator('[data-agent-execution-process]').getByRole('button').last()).toHaveAttribute('aria-expanded', 'false')
})

test('reattaches to an Agent run after the page reloads mid-response', async ({ page, request }) => {
  await createAndOpenBook(request, 'Agent Recovery E2E Book')
  await page.goto('/')
  const composer = await openWritingAgent(page)
  await submitAgentChatMessage(page, composer, 'Wait until the browser reconnects. E2E_DELAYED_AGENT_REPLY')

  await expect.poll(async () => (await getModelStatus(request)).delayed_waiting_by_marker[delayedReplyMarker] ?? 0)
    .toBe(1)

  let released = false
  try {
    await page.reload()
    await releaseDelayedRequest(request, delayedReplyMarker)
    released = true
  } finally {
    if (!released) await releaseDelayedRequest(request, delayedReplyMarker)
  }

  await openWritingAgent(page)
  await expect(page.getByText('Recovered response completed exactly once.', { exact: true })).toHaveCount(1)
})

test('opts into Native Todo using the existing switch and keeps history after disabling', async ({ page, request }) => {
  await createAndOpenBook(request, 'Writing Todo preference')
  const marker = 'E2E_RUNTIME_TODO_WRITING'
  const settings = await (await request.get('/api/settings')).json()
  const selected = await request.patch('/api/settings', { data: {
    layer: 'user', base_revision: settings.revisions.user, changes: { agent_runtimes: { ide: { selected: 'native' } } },
  } })
  expect(selected.ok(), await selected.text()).toBe(true)
  const wire = async (): Promise<Array<{ tools: Array<{ function: { name: string } }> }>> =>
    (await request.get(`${modelControlURL}/control/runtime-requests?marker=${marker}`)).json()
  const expectSavedList = async () => {
    const execution = page.getByRole('button', { name: /^执行过程 · 1 / })
    if (await execution.getAttribute('aria-expanded') === 'false') await execution.click()
    await expect(page.locator('li').getByText('Verify the saved writing draft', { exact: true })).toBeVisible()
  }

  await page.goto('/')
  for (const stage of ['DEFAULT', 'ENABLED', 'DISABLED']) {
    const enabled = stage === 'ENABLED'
    if (stage !== 'DEFAULT') {
      await page.getByRole('button', { name: 'Agents', exact: true }).click()
      await page.getByRole('tab', { name: '能力', exact: true }).click()
      const capabilities = page.locator('[data-agent-configuration-section="capabilities"]')
      const toggle = capabilities.getByRole('switch', { name: '任务清单', exact: true })
      if (!await toggle.isVisible()) await capabilities.getByRole('button', { name: '能力', exact: true }).click()
      await expect(toggle).toBeChecked({ checked: !enabled })
      await toggle.setChecked(enabled)
      await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.agent_tools?.ide?.todo).toBe(enabled)
      await page.reload()
    }
    const before = (await wire()).length
    await submitAgentChatMessage(page, await openWritingAgent(page), `${marker} STAGE_${stage}`)
    await expect(page.getByText(`${marker} ${stage} complete.`, { exact: true })).toBeVisible()
    const calls = (await wire()).slice(before)
    expect(calls.length).toBeGreaterThan(0)
    for (const call of calls) expect(call.tools.some(tool => tool.function.name === 'todo')).toBe(enabled)
    if (stage !== 'DEFAULT') await expectSavedList()
  }

  await page.reload()
  await openWritingAgent(page)
  await expectSavedList()
})
