import { mkdtemp } from 'node:fs/promises'
import path from 'node:path'
import { runtimeRoot } from '../../scripts/e2e-paths.mjs'
import { expect, test, type Locator, type Page } from '../support/fixtures'
import { createAndOpenBook, createProjectFile, createStartedStory, readProjectFile, registerAgentChatProject } from '../support/api'

async function openRootMenu(tree: Locator) {
  const bounds = await tree.boundingBox()
  expect(bounds).not.toBeNull()
  await tree.click({ button: 'right', position: { x: bounds!.width / 2, y: bounds!.height - 20 } })
}

test('creates from an empty project and keeps its menu usable on a narrow screen', async ({ page, request }, testInfo) => {
  const project = await registerAgentChatProject(request, await mkdtemp(path.join(runtimeRoot, 'empty-menu-')))
  const settings = await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme: 'light' } } })
  expect(settings.ok()).toBe(true)
  await page.addInitScript(() => localStorage.setItem('nova:mode', 'agentchat'))
  await page.goto('/')
  await page.locator(`[data-slot="agent-chat-project-toggle"][data-project-id="${project.id}"]`).click()
  await page.getByRole('button', { name: 'New tab', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Files', exact: true }).click()
  const tree = page.locator('.nova-file-tree').filter({ visible: true })
  await expect(page.getByText('This project directory is empty.', { exact: true })).toBeVisible()
  await openRootMenu(tree)
  await page.getByRole('menuitem', { name: 'New Folder', exact: true }).click()
  await nameNewItem(page, 'A folder with a long name for notes and reference material')
  await expect(tree.getByRole('treeitem')).toHaveCount(1)
  await page.reload()
  await expect(tree.getByRole('treeitem')).toHaveCount(1)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: 'Show file tree', exact: true }).click()
  await expect(tree).toBeVisible()
  // The padding beside rows also belongs to the project context menu.
  await tree.locator('..').click({ button: 'right', position: { x: 1, y: 150 } })
  const menu = page.getByRole('menu')
  await expect(menu.getByRole('menuitem', { name: 'New File', exact: true })).toBeVisible()
  const bounds = await menu.boundingBox()
  expect(bounds!.x).toBeGreaterThanOrEqual(0)
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390)
  await page.screenshot({ path: testInfo.outputPath('root-menu-light-390.png') })
  await menu.getByRole('menuitem', { name: 'New File', exact: true }).click()
  const input = page.getByRole('textbox', { name: /^Rename / })
  await expect(input).toBeFocused()
  await input.press('Escape')
  await expect(tree.getByRole('treeitem')).toHaveCount(1)
})

async function nameNewItem(page: Page, name: string) {
  const input = page.getByRole('textbox', { name: /^Rename / })
  await expect(input).toBeFocused()
  await input.fill(name)
  await input.press('Enter')
  await expect(input).not.toBeVisible()
}

for (const [product, theme] of [['Writing', 'dark'], ['Workspace', 'light']] as const) {
  test(`creates and pastes at the project root from blank space in ${product}`, async ({ page, request }, testInfo) => {
    const book = await createAndOpenBook(request, `${product} root menu`)
    await createProjectFile(request, book.projectId, 'notes/source.md', 'Preserve this copied text.')
    if (product === 'Workspace') await createStartedStory(request, 'File menu story')
    const settings = await request.patch('/api/settings', { data: { layer: 'user', changes: { language: 'en-US', theme } } })
    expect(settings.ok()).toBe(true)
    await page.goto('/')
    if (product === 'Workspace') {
      await page.getByLabel('Workbench sidebar').getByRole('button', { name: 'Game', exact: true }).click()
      await expect(page.getByTestId('story-stage')).toBeVisible()
    }
    await page.getByLabel('Workbench sidebar').getByRole('button', { name: product, exact: true }).click()
    if (product === 'Writing') {
      await page.locator('[data-slot="writing-sidebar-view-switcher"]').getByRole('button', { name: 'Files', exact: true }).click()
    } else {
      await page.getByRole('button', { name: 'New tab', exact: true }).click()
      await page.getByRole('menuitem', { name: 'Files', exact: true }).click()
    }
    const tree = page.locator('.nova-file-tree').filter({ visible: true })
    const notes = tree.locator('[data-item-path="notes/"][data-type="item"]')
    await expect(notes).toBeVisible()
    await notes.click()
    await expect(tree.locator('[data-item-path="notes/source.md"][data-type="item"]')).toBeVisible()

    await openRootMenu(tree)
    const menu = page.getByRole('menu')
    await expect(menu.getByRole('menuitem', { name: 'New File', exact: true })).toBeVisible({ timeout: 5000 })
    await expect(menu.getByRole('menuitem', { name: 'Paste', exact: true })).toBeDisabled()
    await expect(menu.getByRole('menuitem', { name: 'Delete', exact: true })).toHaveCount(0)
    await page.screenshot({ path: testInfo.outputPath(`root-menu-${theme}.png`) })
    await menu.getByRole('menuitem', { name: 'New File', exact: true }).click()
    await nameNewItem(page, 'root-note.md')
    await expect(tree.locator('[data-item-path="root-note.md"][data-type="item"]')).toBeVisible()
    await expect.poll(async () => (await readProjectFile(request, book.projectId, 'root-note.md')).content ?? '').toBe('')

    await openRootMenu(tree)
    await menu.getByRole('menuitem', { name: 'New Folder', exact: true }).click()
    await nameNewItem(page, 'Root folder')
    await expect(tree.locator('[data-item-path="Root folder/"][data-type="item"]')).toBeVisible()

    // Row actions retain their own target after using the root menu.
    await notes.click({ button: 'right' })
    await expect(menu.getByRole('menuitem', { name: 'Rename', exact: true })).toBeVisible()
    await menu.getByRole('menuitem', { name: 'New File', exact: true }).click()
    await nameNewItem(page, 'nested.md')
    await expect.poll(async () => (await readProjectFile(request, book.projectId, 'notes/nested.md')).content ?? '').toBe('')
    await tree.locator('[data-item-path="notes/source.md"][data-type="item"]').click({ button: 'right' })
    await menu.getByRole('menuitem', { name: 'Copy', exact: true }).click()
    await openRootMenu(tree)
    await menu.getByRole('menuitem', { name: 'Paste', exact: true }).click()
    await expect(tree.locator('[data-item-path="source.md"][data-type="item"]')).toBeVisible()
    await expect.poll(async () => (await readProjectFile(request, book.projectId, 'source.md')).content).toBe('Preserve this copied text.')
    await openRootMenu(tree)
    await page.keyboard.press('Escape')
    await expect(menu).not.toBeVisible()
  })
}
