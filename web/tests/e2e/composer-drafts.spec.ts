import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import { expect, test, type Locator, type Page } from '../support/fixtures'
import { createAgentChatSession, createAndOpenBook, createStartedStory, getStoryBranches, getStorySnapshot, registerAgentChatProject } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench, openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'

const original = '原来的草稿 🌙\n保留第二行。'
const other = '另一处的草稿 🧭\n留在这里。'

async function expectDraft(composer: Locator, text: string) {
  await expect.poll(async () => (await composer.innerText()).trimEnd().replaceAll('\n\n', '\n')).toBe(text)
}

async function sendWithDelayedReceipt(page: Page, composer: Locator, endpoint: string) {
  let release = () => {}
  const gate = new Promise<void>(resolve => { release = resolve })
  let intercepted = () => {}
  const request = new Promise<void>(resolve => { intercepted = resolve })
  await page.route(`**${endpoint}`, async route => {
    expect(route.request().postDataJSON().message).toBe(original)
    intercepted()
    await gate
    await route.continue()
  }, { times: 1 })
  try {
    await page.locator('[data-action="send"]').filter({ visible: true }).click()
    await request
    await expectDraft(composer, original)
    await composer.fill(other)
    const response = page.waitForResponse(response => new URL(response.url()).pathname === endpoint && response.request().method() === 'POST')
    release()
    expect((await response).ok()).toBe(true)
    await expectDraft(composer, other)
  } finally {
    release()
  }
}

for (const surface of ['Writing', 'General'] as const) {
  test(`${surface} restores unsent text only in its conversation`, async ({ page, request }) => {
    const writing = surface === 'Writing'
    const projectId = writing
      ? (await createAndOpenBook(request, 'Writing composer drafts')).projectId
      : (await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'composer-drafts-')))).id
    const first = await createAgentChatSession(request, projectId, 'First draft destination')
    const second = await createAgentChatSession(request, projectId, 'Second draft destination')
    const endpoint = `/api/projects/${encodeURIComponent(projectId)}/agent-chat/chat`
    let submissions = 0
    page.on('request', request => {
      if (new URL(request.url()).pathname === endpoint && request.method() === 'POST') submissions++
    })
    const select = async (title: string) => {
      if (!writing) return openAgentChatSession(page, projectId, title)
      const composer = await openWritingAgent(page)
      await page.getByRole('button', { name: '会话历史', exact: true }).filter({ visible: true }).click()
      await page.getByRole('option', { name: new RegExp(title) }).click()
      return composer
    }

    await page.goto('/')
    if (!writing) await openAgentChatWorkbench(page)
    await (await select(first.title)).fill(original)
    const secondComposer = await select(second.title)
    await expectDraft(secondComposer, '')
    await secondComposer.fill(other)
    await expectDraft(await select(first.title), original)
    await page.reload()
    if (!writing) await openAgentChatWorkbench(page)
    await expectDraft(await select(second.title), other)
    const composer = await select(first.title)
    await expectDraft(composer, original)
    expect(submissions).toBe(0)

    await sendWithDelayedReceipt(page, composer, endpoint)
    await expect(page.getByText('Deterministic E2E response completed.', { exact: true }).filter({ visible: true })).toBeVisible()
    await page.reload()
    if (!writing) await openAgentChatWorkbench(page)
    const reopened = await select(first.title)
    await expectDraft(reopened, other)
    await submitAgentChatMessage(page, reopened, other)
    await expectDraft(reopened, '')
    await page.reload()
    if (!writing) await openAgentChatWorkbench(page)
    await expectDraft(await select(first.title), '')
  })
}

test('Game restores each route and preserves the next action while editing history', async ({ page, request, browserDiagnostics }) => {
  await createAndOpenBook(request, 'Game composer drafts')
  const story = await createStartedStory(request, 'Game composer drafts')
  const openGame = () => page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await page.goto('/')
  await openGame()
  await submitAgentChatMessage(page, composer, '走进车站')
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns.length).toBe(2)
  await expectDraft(composer, '')
  await composer.fill(original)

  await page.getByRole('button', { name: '编辑这轮输入' }).last().click()
  await expectDraft(composer, '走进车站')
  await composer.fill('历史回合的临时编辑')
  await page.getByRole('button', { name: '取消编辑' }).click()
  await expectDraft(composer, original)

  await page.getByRole('button', { name: '从此处创建分支' }).last().click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('剧情线名称').fill('草稿支线')
  await dialog.getByRole('button', { name: '创建并切换', exact: true }).click()
  await expect.poll(() => getStoryBranches(request, story.id)).toContainEqual(expect.objectContaining({ title: '草稿支线', current: true }))
  await expectDraft(composer, '')
  await composer.fill(other)
  await page.reload()
  await openGame()
  await expectDraft(composer, other)
  await page.getByRole('tab', { name: '路线', exact: true }).click()
  await page.getByRole('button', { name: '切换到剧情线：主线', exact: true }).click()
  await expectDraft(composer, original)

  browserDiagnostics.allow(/^console\.error: Failed to load resource:.*409.*\/api\/interactive\/chat/)
  await page.route('**/api/interactive/chat', route => route.fulfill({ status: 409, json: { error: 'Draft fixture rejection' } }), { times: 1 })
  await page.locator('[data-action="send"]').filter({ visible: true }).click()
  await expect(page.getByText('Draft fixture rejection', { exact: false })).toBeVisible()
  await page.reload()
  await openGame()
  await expectDraft(composer, original)
  await sendWithDelayedReceipt(page, composer, '/api/interactive/chat')
  await expect.poll(async () => (await getStorySnapshot(request, story.id)).turns.length).toBe(3)
  await page.reload()
  await openGame()
  await expectDraft(composer, other)
  await expect(page.getByRole('status').filter({ hasText: '已恢复草稿' })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(composer).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
