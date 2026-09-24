import { createRequire } from 'node:module'
import { expect, test, type APIRequestContext, type Page } from '../support/fixtures'
import { createAndOpenBook, createStartedStory } from '../support/api'
import { openWritingAgent, submitAgentChatMessage } from '../support/agent-chat'
import { modelControlURL } from '../support/model'

type WireRequest = { messages: { role: string; content: unknown }[]; stream?: boolean }

for (const mode of ['Writing', 'Game'] as const) {
  test(`${mode} compacts native images and continues after reload`, async ({ page, request }, testInfo) => {
    const marker = `E2E_IMAGE_COMPACTION_${mode.toUpperCase()}`
    const original = await (await request.get('/api/settings')).json() as { user: { model_profiles?: unknown[] } }
    const configured = await request.patch('/api/settings', {
      data: { layer: 'user', changes: { model_profiles: [{ id: 'e2e', name: 'Image compaction model', endpoint_id: 'e2e', model: 'claude-sonnet-4-7', context_window_tokens: 64_000 }] } },
    })
    expect(configured.ok(), await configured.text()).toBe(true)
    try {
      await createAndOpenBook(request, `${mode} image compaction`)
      if (mode === 'Game') await createStartedStory(request, 'Image compaction story')
      const sharp = createRequire(import.meta.url)('sharp') as typeof import('sharp').default
      const image = await sharp({ create: { width: 2048, height: 2048, channels: 3, background: '#165cbd' } }).png().toBuffer()
      await page.goto('/')
      let composer = await openComposer(page, mode)
      const surface = page.getByTestId(mode === 'Writing' ? 'right' : 'story-stage')
      for (let turn = 0; turn < 6; turn++) {
        await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
        await surface.getByLabel('添加文件', { exact: true }).setInputFiles([0, 1].map(index => ({
          name: `reference-${turn}-${index}.png`, mimeType: 'image/png', buffer: image,
        })))
        await submitAgentChatMessage(page, composer, `${marker} TURN_${turn} Inspect the references.`)
        await expect(page.getByText(`${marker} ${turn} accepted.`, { exact: true })).toBeVisible({ timeout: 30_000 })
      }
      await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)

      const calls = await captured(request, marker)
      const summaries = calls.filter(call => JSON.stringify(call.messages).includes('[Runtime context compaction request]'))
      expect(summaries.length, 'Visual pressure must generate a checkpoint').toBeGreaterThan(0)
      expect(summaries.some(call => nativeImages(call) > 0), 'The summary model must receive actual image parts').toBe(true)
      const last = calls.at(-1)!
      expect(JSON.stringify(last.messages)).toContain(`${marker} checkpoint`)
      expect(nativeImages(last)).toBeGreaterThanOrEqual(2)
      expect(nativeImages(last)).toBeLessThan(12)

      await page.reload()
      composer = await openComposer(page, mode)
      await submitAgentChatMessage(page, composer, `${marker} Continue from the retained references.`)
      await expect(page.getByText(`${marker} continue accepted.`, { exact: true })).toBeVisible({ timeout: 30_000 })
      await expect(page.locator('[data-action="stop"]').filter({ visible: true })).toHaveCount(0)
      const reopened = (await captured(request, marker)).at(-1)!
      expect(JSON.stringify(reopened.messages)).toContain(`${marker} checkpoint`)
      expect(nativeImages(reopened)).toBeGreaterThan(0)
      await page.screenshot({ path: testInfo.outputPath(`${mode.toLowerCase()}-image-compaction.png`) })
    } finally {
      const restored = await request.patch('/api/settings', { data: { layer: 'user', changes: { model_profiles: original.user.model_profiles ?? null } } })
      expect(restored.ok(), await restored.text()).toBe(true)
    }
  })
}

async function openComposer(page: Page, mode: 'Writing' | 'Game') {
  if (mode === 'Writing') return openWritingAgent(page)
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '游戏', exact: true }).click()
  const composer = page.getByPlaceholder(/你要做什么/)
  await expect(composer).toBeVisible()
  return composer
}

function nativeImages(request: WireRequest): number {
  return request.messages.flatMap(message => Array.isArray(message.content) ? message.content : []).filter(part => part.type === 'image_url').length
}

async function captured(request: APIRequestContext, marker: string): Promise<WireRequest[]> {
  const response = await request.get(`${modelControlURL}/control/compaction-requests?marker=${marker}`)
  expect(response.ok()).toBe(true)
  return response.json()
}
