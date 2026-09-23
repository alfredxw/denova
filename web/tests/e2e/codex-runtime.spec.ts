import { execFileSync } from 'node:child_process'
import { expect, test, type APIRequestContext, type Page } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, createStartedStory, getStorySnapshot } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { modelControlURL } from '../support/model'

// Run with DENOVA_TEST_CODEX_EXE. No product API or runtime event is mocked.
// The ordinary core journey specs use the same installed runtime in this mode.
test.skip(!process.env.DENOVA_TEST_CODEX_EXE, 'Installed Codex acceptance is opt-in')
test.beforeAll(async ({}, info) => {
  if (process.env.DENOVA_TEST_CODEX_EXE) await info.attach('runtime-version', {
    body: execFileSync(process.env.DENOVA_TEST_CODEX_EXE, ['--version']), contentType: 'text/plain',
  })
})

type WireRequest = { prompt_cache_key?: string; input?: unknown[]; messages?: unknown[]; tools?: Array<{ name: string }> }

async function wire(request: APIRequestContext, marker: string): Promise<WireRequest[]> {
  const response = await request.get(`${modelControlURL}/control/runtime-requests?marker=${encodeURIComponent(marker)}`)
  expect(response.ok()).toBe(true)
  return response.json()
}

async function openProduct(page: Page, product: 'writing' | 'game') {
  if (product === 'writing') return openWritingAgent(page)
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await expect(composer).toBeVisible()
  return composer
}

async function settled(page: Page) {
  await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
}

for (const product of ['writing', 'game'] as const) {
  test(`${product} uses Codex automatic compaction and resumes the compacted thread`, async ({ page, request }) => {
    const book = await createAndOpenBook(request, `Codex automatic ${product}`)
    await createProjectFile(request, book.projectId, 'runtime-evidence.txt', 'Runtime evidence is verified.')
    if (product === 'game') await createStartedStory(request, 'Automatic runtime context')
    const marker = `E2E_RUNTIME_AUTO_${product.toUpperCase()}`
    await page.goto('/')
    const composer = await openProduct(page, product)
    await submitAgentChatMessage(page, composer, marker)
    await expect(page.getByText(`${marker} completed after compaction.`, { exact: true })).toBeVisible()
    await settled(page)
    await expect(page.getByText('自动压缩', { exact: true })).toBeVisible()
    await expect(page.getByText('运行时已压缩工作上下文，会话历史完整保留。', { exact: true })).toBeVisible()
    const before = await wire(request, marker)
    const compactIndex = before.findIndex(call => JSON.stringify(call.input?.at(-1)).includes('CONTEXT CHECKPOINT COMPACTION'))
    expect(compactIndex).toBeGreaterThan(0)
    expect(JSON.stringify(before[compactIndex]?.input)).toContain('Runtime evidence is verified.')
    expect(before.at(-1)?.prompt_cache_key).toBe(before[0]?.prompt_cache_key)
    expect(JSON.stringify(before.at(-1)?.input)).toContain('read evidence verified')
    await page.reload()
    await submitAgentChatMessage(page, await openProduct(page, product), `${marker} CONTINUE_AFTER_COMPACT`)
    await expect(page.getByText(`${marker} continued.`, { exact: true })).toBeVisible()
    await settled(page)
    const after = await wire(request, marker)
    expect(after.at(-1)?.prompt_cache_key).toBe(before[0]?.prompt_cache_key)
    expect(JSON.stringify(after.at(-1)?.input)).toContain('read evidence verified')
    await expect(page.getByText('自动压缩', { exact: true })).toBeVisible()
    if (product === 'game') await expect(page.getByText(/^正在执行/)).toHaveCount(0)
    await page.getByText('自动压缩', { exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath(`${product}-codex-compaction.png`) })
  })

  test(`${product} restores the native Codex plan after reload`, async ({ page, request }) => {
    await createAndOpenBook(request, `Codex plan ${product}`)
    if (product === 'game') await createStartedStory(request, 'Native plan')
    const marker = `E2E_RUNTIME_PLAN_${product.toUpperCase()}`
    await page.goto('/')
    await submitAgentChatMessage(page, await openProduct(page, product), marker)
    await expect(page.getByText(`${marker} planned.`, { exact: true })).toBeVisible()
    await settled(page)
    const calls = await wire(request, marker)
    expect(calls[0]?.tools?.some(tool => tool.name === 'update_plan')).toBe(true)
    expect(calls[0]?.tools?.some(tool => tool.name === 'todo')).toBe(false)
    await page.reload()
    await openProduct(page, product)
    await expect(page.locator('li').getByText('Verify the station evidence', { exact: true })).toBeVisible()
    await expect(page.getByText('1/1', { exact: true })).toBeVisible()
  })

  test(`${product} preserves canonical history through Native, Codex, and Native`, async ({ page, request }) => {
    const book = await createAndOpenBook(request, `Runtime history ${product}`)
    const story = product === 'game' ? await createStartedStory(request, 'Runtime switching') : undefined
    const binding = story ? { mode: 'interactive', story_id: story.id, branch_id: 'main' } : { mode: 'agent_chat', session_id: 'default' }
    const configURL = `/api/projects/${book.projectId}/conversation-config`
    const query = new URLSearchParams(binding)
    const marker = `E2E_RUNTIME_SWITCH_${product.toUpperCase()}`
    for (const [index, kind] of ['native', 'codex', 'native'].entries()) {
      const snapshot = await (await request.get(`${configURL}?${query}`)).json()
      const changed = await request.patch(configURL, { data: { binding, base_revision: snapshot.revision, changes: {
        runtime: kind === 'codex' ? { kind, codex: { profile_id: 'e2e-codex' } } : { kind },
      } } })
      expect(changed.ok(), await changed.text()).toBe(true)
      expect((await changed.json()).runtime_capabilities.goal).toBe(product === 'writing')
      await page.goto('/')
      await submitAgentChatMessage(page, await openProduct(page, product), `${marker} STEP_${index + 1}`)
      await expect(page.getByText(`${marker} reply ${index + 1}.`, { exact: true })).toBeVisible()
      await settled(page)
    }
    const calls = await wire(request, marker)
    expect(calls.some(call => Array.isArray(call.input))).toBe(true)
    expect(calls.some(call => Array.isArray(call.messages))).toBe(true)
    await page.reload()
    await openProduct(page, product)
    for (const step of [1, 2, 3]) await expect(page.getByText(`${marker} reply ${step}.`, { exact: true })).toHaveCount(1)
    if (story) expect((await getStorySnapshot(request, story.id)).turns).toHaveLength(4)
  })
}

test('Writing Goal continues via a read-only Codex fork; Game rejects Goal', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Codex Goal')
  const marker = 'E2E_RUNTIME_GOAL_WRITING'
  const binding = { mode: 'agent_chat', session_id: 'default' }
  const goalURL = `/api/projects/${book.projectId}/conversation-goal`
  const accepted = await request.post(goalURL, { data: { binding, action: 'set', expected_revision: 0, objective: `Produce both first and final proofs. ${marker}` } })
  expect(accepted.ok(), await accepted.text()).toBe(true)
  await page.goto('/')
  await submitAgentChatMessage(page, await openWritingAgent(page), marker)
  await expect.poll(async () => (await (await request.get(`${goalURL}?${new URLSearchParams(binding)}`)).json()).goal?.status).toBe('completed')
  await settled(page)
  await expect(page.getByText(`${marker} first proof.`, { exact: true })).toBeVisible()
  await expect(page.getByText(`${marker} final proof.`, { exact: true })).toBeVisible()
  const calls = await wire(request, marker)
  const evaluations = calls.filter(call => JSON.stringify(call.input?.at(-1)).includes('[Goal evaluation request]'))
  expect(evaluations).toHaveLength(2)
  for (const call of evaluations) {
    // Fork retains host schemas, but execution is denied by the adapter's
    // empty allowlist. Built-in writes are separately tested under read-only sandbox.
    expect(call.tools?.some(tool => tool.name === 'update_plan')).toBe(false)
    expect(call.prompt_cache_key).not.toBe(calls[0]?.prompt_cache_key)
  }
  const continued = calls.find(call => !JSON.stringify(call.input?.at(-1)).includes('[Goal evaluation request]') && JSON.stringify(call.input?.at(-1)).includes('FINISH_GOAL'))
  expect(continued?.prompt_cache_key).toBe(calls[0]?.prompt_cache_key)
  expect(JSON.stringify(continued?.input)).not.toContain('[Goal evaluation request]')
  await page.reload()
  await openWritingAgent(page)
  await expect(page.getByText(`${marker} final proof.`, { exact: true })).toHaveCount(1)
  const story = await createStartedStory(request, 'No game Goal')
  const unsupported = await request.post(goalURL, { data: { binding: { mode: 'interactive', story_id: story.id, branch_id: 'main' }, action: 'set', expected_revision: 0, objective: marker } })
  expect(unsupported.status()).toBe(400)
  expect(await unsupported.json()).toEqual({ error: '当前引擎不支持此操作或参数。' })
})
