import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createAgentChatSession, registerAgentChatProject } from '../support/api'
import { revealAgentChatProject } from '../support/agent-chat'

test('revalidates Project Agent choices when creating a conversation', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Fresh Agent choices')
  const settingsPath = '/api/settings'
  const setAgents = async (agents: unknown[]) => {
    const response = await request.patch(settingsPath, { data: { layer: 'user', changes: { custom_agents: agents } } })
    expect(response.ok(), await response.text()).toBe(true)
  }
  await setAgents([])
  await page.addInitScript(() => localStorage.setItem('nova:mode', 'agentchat'))
  await page.goto('/')
  const create = page.getByRole('button', { name: `在 ${book.title} 中新建对话`, exact: true })
  await create.click()
  await expect(page.getByRole('tab', { name: /新会话/ }).first()).toBeVisible()
  await expect(page.getByRole('menu')).toHaveCount(0)
  await setAgents([{ id: 'fresh-editor', name: 'Fresh Editor', enabled: true,
    contract: 'writing.primary.v1', instructions: 'Keep the saved role.' }])
  await create.click()
  const created = page.waitForResponse(response => response.request().method() === 'POST'
    && response.url().endsWith(`/api/projects/${book.projectId}/agent-chat/sessions`))
  await page.getByRole('menuitem', { name: 'Fresh Editor', exact: true }).click()
  const session = await (await created).json()
  const config = await (await request.get(`/api/projects/${book.projectId}/conversation-config?mode=agent_chat&session_id=${session.id}`)).json()
  expect(config.custom_agent_id).toBe('fresh-editor')
  await setAgents([])
  await create.click()
  await expect(page.getByRole('menu')).toHaveCount(0)
})

for (const [theme, language] of [['dark', 'zh-CN'], ['light', 'en-US']] as const) {
  test(`opens ephemeral workbench drafts and preserves restored tabs in ${theme}`, async ({ page, request }) => {
    const chinese = language === 'zh-CN'
    const labels = chinese ? {
      draft: '新会话', closeDraft: '关闭 新会话', empty: '还没有打开任何标签页',
      secondary: '显示右侧工作区', hideSecondary: '隐藏右侧工作区', files: '文件',
    } : {
      draft: 'New Session', closeDraft: 'Close New Session', empty: 'Nothing open yet',
      secondary: 'Show secondary workspace', hideSecondary: 'Hide secondary workspace', files: 'Files',
    }
    const book = await createAndOpenBook(request, 'Workbench entry with a long project name for responsive navigation')
    // Exercise deferred rows even when this file runs without the full catalog.
    const catalog = await (await request.get('/api/agent-chat/projects')).json()
    for (let index = catalog.projects.length; index < 35; index++) {
      await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'workbench-neighbor-')))
    }
    const project = await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'workbench-entry-')))
    const snapshot = await (await request.get(`/api/agent-chat/projects?project_id=${project.id}`)).json()
    const settings = await request.patch('/api/settings', { data: { layer: 'user', changes: { language, theme } } })
    expect(settings.ok()).toBe(true)
    await page.addInitScript(language => {
      localStorage.setItem('nova:mode', 'agentchat')
      localStorage.setItem('nova.locale.configured', language)
    }, language)
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/')
    const primary = page.locator('[data-agent-chat-group="primary"]').filter({ visible: true })
    const composer = page.locator('[contenteditable="true"]').filter({ visible: true })
    await expect(primary.getByRole('tab')).toHaveCount(1)
    await expect(primary.getByRole('tab')).toContainText(labels.draft)
    await expect(composer).toBeFocused()
    // Unvisited Projects must not mount hidden tab bars and empty workspaces.
    await expect(page.locator('[data-agent-chat-group]')).toHaveCount(1)
    await page.screenshot({ path: test.info().outputPath('desktop-draft.png'), animations: 'disabled' })

    await primary.getByRole('button', { name: labels.closeDraft, exact: true }).click()
    await expect(primary.getByText(labels.empty, { exact: true })).toBeVisible()
    const refresh = page.waitForResponse(response => response.url().endsWith('/api/agent-chat/projects'))
    await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    await refresh
    await expect(primary.getByRole('tab')).toHaveCount(0)

    await revealAgentChatProject(page, project.name)
    await page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${project.id}"]`).click()
    await expect(page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${project.id}"]`)).toHaveAttribute('aria-current', 'location')
    await expect(primary.getByRole('tab')).toHaveCount(1)
    await expect(composer).toBeFocused()
    const afterEntry = await (await request.get(`/api/agent-chat/projects?project_id=${project.id}`)).json()
    expect(afterEntry.projects[0].sessions).toEqual(snapshot.projects[0].sessions)
    await page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${book.projectId}"]`).click()
    await expect(page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${book.projectId}"]`)).toHaveAttribute('aria-current', 'location')
    await expect(primary.getByRole('tab')).toHaveCount(1)
    await expect(primary.getByRole('tab')).toContainText(labels.draft)
    await page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${project.id}"]`).click()
    await expect(page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${project.id}"]`)).toHaveAttribute('aria-current', 'location')
    await expect(primary.getByRole('tab')).toHaveCount(1)

    // A durable tab must survive reload without a second default conversation.
    await primary.getByRole('button', { name: labels.closeDraft, exact: true }).click()
    await expect(primary.getByRole('tab')).toHaveCount(0)
    const session = await createAgentChatSession(request, project.id, `${theme} restored conversation`)
    const refreshSession = page.waitForResponse(response => response.url().endsWith('/api/agent-chat/projects'))
    await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    await refreshSession
    const projectRow = page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${project.id}"]`)
    if (await projectRow.getAttribute('aria-expanded') !== 'true') await projectRow.click()
    const projectContainer = projectRow.locator('xpath=ancestor::div[./div[@data-slot="agent-chat-project-content"]][1]')
    await projectContainer.locator('[data-slot="agent-chat-conversation-row"]').filter({ hasText: session.title }).click()
    await expect(primary.getByRole('tab')).toHaveCount(1)
    await page.getByRole('button', { name: labels.secondary, exact: true }).click()
    await page.getByRole('menuitem', { name: labels.files, exact: true }).click()
    await expect(page.locator('[data-agent-chat-group="secondary"]').getByRole('tab', { name: new RegExp(labels.files) })).toBeVisible()
    await page.getByRole('button', { name: labels.hideSecondary, exact: true }).click()
    await page.reload()
    await expect(primary.getByRole('tab')).toHaveCount(1)
    await expect(primary.getByRole('tab')).toContainText(session.title)
    await expect(page.getByRole('button', { name: labels.secondary, exact: true })).toBeVisible()

    await page.setViewportSize({ width: 390, height: 844 })
    await primary.getByRole('button', { name: chinese ? `关闭 ${session.title}` : `Close ${session.title}`, exact: true }).click()
    // Close the remaining Files tab so re-entry starts from a genuinely empty workbench.
    await page.getByRole('button', { name: labels.secondary, exact: true }).filter({ visible: true }).click()
    await page.getByRole('button', { name: chinese ? '关闭 文件' : 'Close Files', exact: true }).filter({ visible: true }).click()
    await page.reload()
    await expect(primary.getByRole('tab')).toHaveCount(1)
    await expect(primary.getByRole('tab')).toContainText(labels.draft)
    await expect(composer).toBeVisible()
    await expect(composer).not.toBeFocused()
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    await page.screenshot({ path: test.info().outputPath('mobile-draft.png'), animations: 'disabled' })
  })
}
