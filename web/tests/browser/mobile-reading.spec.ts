import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createProjectFile } from '../support/api'
import type { ProductMessage } from '../../src/features/messages/types'

for (const theme of ['dark', 'light']) {
  test(`mobile inbox separates list, detail and read state in ${theme}`, async ({ page, request }) => {
    await createAndOpenBook(request, `Inbox ${theme} ${test.info().project.name}`)
    await page.setViewportSize({ width: 320, height: 844 })
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
      const response = await route.fetch()
      const settings = await response.json()
      await route.fulfill({ response, json: { ...settings, effective: { ...settings.effective, theme } } })
    })
    let items: ProductMessage[] = [
      { id: 'action', type: 'automation_action', title: '需要确认：一条很长的自动化任务通知，请检查整理结果后继续', summary: '作品资料已经整理完成，等待你确认下一步。', body: '请确认这份资料再继续后续流程。', action_required: true, task_id: 'task-1' },
      { id: 'release', type: 'changelog', title: 'v0.5.0', summary: '更新内容应当先于支持项目的提示展示。', body: '# 更新正文\n\n本次更新的详细内容。' },
      ...Array.from({ length: 12 }, (_, i) => ({ id: `run-${i}`, type: 'automation', title: `章节检查 ${i}`, summary: '自动化检查已经完成。', body: `检查 ${i} 的详细结果。`, read_at: '2026-09-01T00:00:00Z' })),
    ].map((item) => ({ ...item, published_at: '2026-09-07T00:00:00Z' }))
    const readIDs: string[] = []
    await page.route(/\/api\/messages(?:\/.*)?$/, async (route) => {
      const url = new URL(route.request().url())
      const id = url.pathname.match(/\/messages\/([^/]+)\/read$/)?.[1]
      if (id) {
        readIDs.push(id)
        items = items.map((item) => item.id === id ? { ...item, read_at: new Date().toISOString() } : item)
        await route.fulfill({ json: items.find((item) => item.id === id) })
      } else {
        if (url.pathname.endsWith('/read-all')) items = items.map((item) => ({ ...item, read_at: new Date().toISOString() }))
        await route.fulfill({ json: { items, unread_count: items.filter((item) => !item.read_at).length } })
      }
    })
    await page.goto('/')
    await page.getByRole('button', { name: '导航菜单', exact: true }).click()
    await page.getByRole('button', { name: '打开消息中心', exact: true }).click()
    const inbox = page.getByRole('dialog', { name: '消息中心', exact: true })
    await expect(inbox.getByRole('button', { name: /需要确认/ })).toBeVisible()
    await expect(inbox.locator('article')).toBeHidden()
    expect(readIDs).toEqual([])
    const panel = inbox.getByRole('tabpanel')
    expect((await panel.boundingBox())!.height).toBeGreaterThan(650)
    expect((await inbox.boundingBox())!.width).toBeCloseTo(320, 2)
    await inbox.getByRole('tab', { name: '待处理', exact: true }).click()
    await expect(inbox.locator('[data-message-id]')).toHaveCount(1)
    await inbox.getByRole('button', { name: /需要确认/ }).click()
    await expect(inbox.locator('article')).toContainText('请确认这份资料再继续后续流程。')
    await expect(inbox.getByRole('tablist')).toBeHidden()
    await expect.poll(() => readIDs).toEqual(['action'])
    await inbox.getByRole('button', { name: '返回消息列表', exact: true }).click()
    await expect(inbox.getByRole('tab', { name: '待处理', exact: true })).toHaveAttribute('aria-selected', 'true')
    await expect(inbox.getByRole('button', { name: /需要确认/ })).toBeFocused()
    await inbox.getByRole('tab', { name: '全部', exact: true }).click()
    const last = inbox.getByRole('button', { name: /章节检查 11/ })
    await last.scrollIntoViewIfNeeded()
    const scrollTop = await panel.evaluate((element) => element.scrollTop)
    await last.click()
    await inbox.getByRole('button', { name: '返回消息列表', exact: true }).click()
    expect(await panel.evaluate((element) => element.scrollTop)).toBeCloseTo(scrollTop, 0)
    await inbox.getByRole('tab', { name: '产品', exact: true }).click()
    await inbox.getByRole('button', { name: /Denova v0.5.0/ }).click()
    await expect(inbox.getByRole('heading', { name: '更新正文', exact: true })).toBeVisible()
    const contentTop = (await inbox.getByRole('heading', { name: '更新正文', exact: true }).boundingBox())!.y
    const supportTop = (await inbox.getByRole('region', { name: '给 Denova 点个 Star', exact: true }).boundingBox())!.y
    expect(contentTop).toBeLessThan(supportTop)
    await page.screenshot({ path: `test-results/mobile-ux/inbox-detail-${theme}-${test.info().project.name}.png` })
    // Desktop keeps side-by-side reading; the navigation shell changes at this breakpoint.
    await inbox.getByRole('button', { name: '关闭', exact: true }).click()
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.getByRole('button', { name: '打开消息中心', exact: true }).click()
    await expect(inbox.getByRole('tablist')).toBeVisible()
    await expect(inbox.locator('article')).toBeVisible()
    await expect(inbox.getByRole('button', { name: '返回消息列表', exact: true })).toBeHidden()
    await inbox.getByRole('button', { name: '关闭', exact: true }).click()
  })
}

test('mobile Writing exposes touch tab actions and compact editing tools without losing drafts', async ({ page, request }) => {
  const book = await createAndOpenBook(request, `Touch Writing ${test.info().project.name}`)
  for (const title of ['first', 'second-with-a-very-long-document-title-for-a-phone']) {
    await createProjectFile(request, book.projectId, `chapters/${title}.md`, `# ${title}\n\nThe ${title} chapter is editable.\n`)
  }
  await page.setViewportSize({ width: 320, height: 844 })
  await page.goto('/')
  const files = page.getByRole('dialog', { name: '项目', exact: true })
  for (const title of [/^first/, /^second with/]) {
    await page.getByRole('button', { name: '文件', exact: true }).click()
    await files.getByRole('button', { name: title }).click()
    await expect(files).toBeHidden()
  }
  const strip = page.locator('[data-writing-content-layer] [data-slot="workbench-tab-strip"]')
  await expect(strip.getByRole('tab')).toHaveCount(2)
  await expect(strip.getByRole('button')).toHaveCount(1)
  expect((await strip.boundingBox())!.height).toBeLessThanOrEqual(48.1)
  expect((await page.locator('.nova-mobile-editor-toolbar').boundingBox())!.height).toBeLessThanOrEqual(44.1)
  for (const name of ['固定标签页', '取消固定', '向右移动']) {
    await strip.getByRole('button', { name: '当前标签页操作', exact: true }).click()
    await page.getByRole('menuitem', { name, exact: true }).click()
  }
  await expect(strip.getByRole('tab').last()).toHaveAttribute('aria-selected', 'true')
  await strip.getByRole('button', { name: '当前标签页操作', exact: true }).click()
  await page.getByRole('menuitem', { name: '关闭', exact: true }).click()
  await expect(strip.getByRole('tab')).toHaveCount(1)
  await page.getByRole('button', { name: '编辑工具', exact: true }).click()
  await page.getByRole('menuitemcheckbox', { name: '源码', exact: true }).click()
  await expect(page.locator('.monaco-editor:visible')).toBeVisible()
  await page.getByRole('button', { name: '编辑工具', exact: true }).click()
  await expect(page.getByRole('menuitemcheckbox', { name: '源码', exact: true })).toHaveAttribute('aria-checked', 'true')
  await page.getByRole('menuitemcheckbox', { name: '源码', exact: true }).click()
  await expect(page.locator('.tiptap:visible')).toBeVisible()
  await page.getByRole('button', { name: '编辑器设置', exact: true }).click()
  const settings = page.getByRole('dialog', { name: '编辑器设置', exact: true })
  await expect(settings).toBeVisible()
  await settings.getByRole('button', { name: '关闭', exact: true }).click()
  await page.screenshot({ path: `test-results/mobile-ux/writing-editor-320-${test.info().project.name}.png` })
  await page.getByRole('tab', { name: 'Agent', exact: true }).click()
  const composer = page.getByPlaceholder(/输入消息/)
  await composer.fill('Keep this Agent draft')
  expect((await page.locator('.nova-writing-agent-toolbar:visible').boundingBox())!.height).toBeGreaterThanOrEqual(43.9)
  await expect(page.getByRole('button', { name: '显示会话侧栏', exact: true })).toBeHidden()
  await page.getByRole('button', { name: '会话历史', exact: true }).click()
  await expect(page.getByRole('combobox', { name: '搜索会话', exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(composer).toContainText('Keep this Agent draft')
  await page.getByRole('button', { name: '文件', exact: true }).click()
  await files.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.getByRole('tab', { name: 'Agent', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(composer).toContainText('Keep this Agent draft')
  await page.getByRole('button', { name: '文件', exact: true }).click()
  await files.getByRole('button', { name: /^first/ }).click()
  await expect(page.getByRole('tab', { name: '正文', exact: true })).toHaveAttribute('aria-selected', 'true')
  await page.setViewportSize({ width: 1440, height: 900 })
  await expect(page.locator('.nova-editor-toolbar')).toBeVisible()
  await expect(page.locator('.nova-mobile-editor-toolbar')).toBeHidden()
  await expect(page.getByText('The first chapter is editable.', { exact: true })).toBeVisible()
})

test('mobile inbox has a clear empty state and English controls', async ({ page, request }) => {
  await createAndOpenBook(request, `Empty Inbox ${test.info().project.name}`)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
    const response = await route.fetch()
    const settings = await response.json()
    await route.fulfill({ response, json: { ...settings, effective: { ...settings.effective, language: 'en-US' } } })
  })
  await page.route(/\/api\/messages$/, (route) => route.fulfill({ json: { items: [], unread_count: 0 } }))
  await page.goto('/')
  await page.getByRole('button', { name: 'Navigation', exact: true }).click()
  await page.getByRole('button', { name: 'Open message center', exact: true }).click()
  const inbox = page.getByRole('dialog', { name: 'Message Center', exact: true })
  await expect(inbox.getByText('No messages', { exact: true })).toBeVisible()
  await expect(inbox.getByRole('button', { name: 'Mark all read', exact: true })).toBeDisabled()
  await expect(inbox.locator('article')).toBeHidden()
  await inbox.getByRole('tab', { name: 'Product', exact: true }).click()
  await expect(inbox.getByText('No messages', { exact: true })).toBeVisible()
  await inbox.getByRole('button', { name: 'Close', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Open message center', exact: true })).toBeFocused()
})
