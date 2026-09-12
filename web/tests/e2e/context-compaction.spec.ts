import { expect, test, type APIRequestContext, type Locator, type Page } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, createStartedStory } from '../support/api'
import { openWritingAgent } from '../support/agent-chat'
import { modelControlURL } from '../support/model'

type WireMessage = { role: string; content?: unknown; tool_call_id?: string; tool_calls?: unknown[] }
type WireRequest = { model: string; tools: unknown[]; messages: WireMessage[]; [key: string]: unknown }

for (const mode of ['Writing', 'Game'] as const) {
  test(`${mode} preserves the live tool tail through automatic compaction and reload`, async ({ page, request }) => {
    const marker = `E2E_COMPACTION_${mode.toUpperCase()}`
    const evidence = `LIVE_EVIDENCE_${mode.toUpperCase()}_91731`
    const book = await createAndOpenBook(request, `${mode} Compaction E2E`)
    await createProjectFile(request, book.projectId, 'compaction-evidence.txt', `${evidence} ${'Current station evidence. '.repeat(4)}\n`.repeat(200))
    const settings = await request.patch(`/api/projects/${encodeURIComponent(book.projectId)}/settings`, {
      data: { layer: 'workspace', changes: { agent_context: {
        [mode === 'Writing' ? 'ide' : 'interactive_story']: {
          compaction_enabled: true, tool_result_context_enabled: true,
          compaction_threshold: 0.85, max_provider_input_bytes: 450_000,
        },
      } } },
    })
    expect(settings.ok(), await settings.text()).toBe(true)
    const story = mode === 'Game' ? await createStartedStory(request, `${mode} Compaction Story`) : null
    await page.goto('/')
    const composer = await openComposer(page, mode)
    await send(composer, `${marker}_SEED`)
    await expect(page.getByText(`${marker} seed complete.`, { exact: true })).toBeVisible({ timeout: 30_000 })
    await send(composer, `${marker}_PAD`)
    await expect(page.getByText(`${marker} recent turn complete.`, { exact: true })).toBeVisible({ timeout: 30_000 })
    await send(composer, `${marker}_READ`)
    await expect(page.getByText(`${marker} live evidence accepted.`, { exact: true })).toBeVisible({ timeout: 30_000 })

    const calls = await captured(request, marker)
    const forkIndex = calls.findIndex(call => JSON.stringify(call.messages.at(-1)).includes('[Runtime context compaction request]'))
    expect(forkIndex, `No automatic fork among ${calls.length} calls`).toBeGreaterThan(0)
    const fork = calls[forkIndex]!
    const read = calls[forkIndex - 1]!
    const continued = calls[forkIndex + 1]!
    expect(read.messages.some(message => message.tool_call_id === `live-${marker}`)).toBe(false)
    expect(fork.messages.slice(0, read.messages.length)).toEqual(read.messages)
    const live = fork.messages.filter(message => message.role === 'tool' && message.tool_call_id === `live-${marker}`)
    expect(live).toHaveLength(1)
    expect(JSON.stringify(live[0])).toContain(evidence)
    expect(continued.messages.filter(message => message.role === 'tool' && message.tool_call_id === `live-${marker}`)).toEqual(live)
    expect(JSON.stringify(continued.messages)).toContain(`${marker} checkpoint`)
    expect(JSON.stringify(continued.messages)).not.toContain('Historical archive detail about the station.')
    for (const call of [fork, continued]) {
      expect(call.model).toBe(read.model)
      expect(call.tools).toEqual(read.tools)
      expect(call.temperature).toEqual(read.temperature)
      expect(call.prompt_cache_key).toEqual(read.prompt_cache_key)
    }

    await page.reload()
    const reopened = await openComposer(page, mode)
    await send(reopened, `${marker}_CONTINUE`)
    await expect(page.getByText(`${marker} continued after reload.`, { exact: true })).toBeVisible({ timeout: 30_000 })
    const final = (await captured(request, marker)).at(-1)!
    expect(JSON.stringify(final.messages)).toContain(evidence)
    expect(JSON.stringify(final.messages)).toContain(`${marker} checkpoint`)

    if (story) {
      await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
      for (const theme of ['dark', 'light']) {
        const settings = await request.patch('/api/settings', { data: { layer: 'user', changes: { theme } } })
        expect(settings.ok(), await settings.text()).toBe(true)
        await page.reload()
        await openComposer(page, mode)
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
        const dialog = await openAnalysis(page)
        for (const width of [390, 1280]) {
          await page.setViewportSize({ width, height: 844 })
          await expect(dialog.getByRole('button', { name: '移除压缩', exact: true })).toBeVisible()
          await expect(dialog.getByText(/^第 \d+ 版摘要$/)).toBeVisible()
          await page.screenshot({ path: test.info().outputPath(`game-compaction-${theme}-${width}.png`) })
          expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
        }
        await page.keyboard.press('Escape')
      }
      const dialog = await openAnalysis(page)
      const remove = dialog.getByRole('button', { name: '移除压缩', exact: true })
      const restored = page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith('/api/interactive/chat/context-analysis'))
      await remove.click()
      const analysis = await (await restored).json() as { compaction_active?: boolean; context_messages: unknown[] }
      expect(analysis.compaction_active ?? false).toBe(false)
      expect(JSON.stringify(analysis.context_messages)).toContain('Historical archive detail about the station.')
      expect(JSON.stringify(analysis.context_messages)).toContain(evidence)
    } else {
      const dialog = await openAnalysis(page)
      await expect(dialog.getByText(/^第 \d+ 版摘要$/)).toBeVisible()
      await page.screenshot({ path: test.info().outputPath('writing-compaction.png') })
      // Scoped AgentChat has no removal endpoint; it must not offer an action
      // that silently returns false or targets the foreground Writing session.
      await expect(dialog.getByRole('button', { name: '移除压缩', exact: true })).toHaveCount(0)
    }
  })
}

async function openAnalysis(page: Page): Promise<Locator> {
  await page.getByRole('button', { name: '输入动作' }).filter({ visible: true }).click()
  await page.getByRole('menuitem', { name: '上下文分析', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '上下文分析' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText('预计上下文 Token', { exact: true })).toBeVisible()
  return dialog
}

async function openComposer(page: Page, mode: 'Writing' | 'Game'): Promise<Locator> {
  if (mode === 'Writing') return openWritingAgent(page)
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await expect(composer).toBeVisible()
  return composer
}

async function send(composer: Locator, text: string): Promise<void> {
  const page = composer.page()
  await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0, { timeout: 30_000 })
  await composer.fill(text)
  const send = page.locator('[data-action="send"]').filter({ visible: true })
  await expect(send).toBeEnabled()
  await send.click()
}

async function captured(request: APIRequestContext, marker: string): Promise<WireRequest[]> {
  const response = await request.get(`${modelControlURL}/control/compaction-requests?marker=${marker}`)
  expect(response.ok()).toBe(true)
  return response.json()
}
