import { expect, test } from '../support/fixtures'
import { createAgentChatSession, createAndOpenBook } from '../support/api'
import { openAgentChatSession, openAgentChatWorkbench } from '../support/agent-chat'

test('Windows terminal shell choices persist across Writing and Game', async ({ page, request }, testInfo) => {
  test.setTimeout(90_000)
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await createAndOpenBook(request, 'Terminal Shell Browser Book')
  // Host metadata, rather than the browser OS, controls the Windows choices.
  await page.route('**/api/settings', async route => {
    const response = await route.fetch()
    const body = await response.json()
    await route.fulfill({ response, json: { ...body, runtime: { ...body.runtime, goos: 'windows' } } })
  })
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  for (const [destination, shell, label] of [
    ['写作', 'wsl.exe', 'WSL（默认发行版）'],
    ['游戏', 'pwsh.exe', 'PowerShell 7（pwsh）'],
  ]) {
    await sidebar.getByRole('button', { name: destination, exact: true }).click()
    await sidebar.getByRole('button', { name: '设置', exact: true }).click()
    await page.getByRole('button', { name: '终端', exact: true }).click()
    const selector = page.getByRole('combobox', { name: '终端 Shell', exact: true })
    await selector.click()
    await page.getByRole('option', { name: label, exact: true }).click()
    await expect.poll(async () => (await (await request.get('/api/settings')).json()).user.terminal_shell).toBe(shell)
    await page.reload()
    await expect(selector).toHaveText(label)
  }
  for (const scenario of [
    { width: 1440, height: 960, theme: 'dark', language: 'zh-CN' },
    { width: 390, height: 844, theme: 'light', language: 'en-US' },
  ]) {
    const english = scenario.language === 'en-US'
    await page.setViewportSize(scenario)
    expect((await request.patch('/api/settings', { data: { layer: 'user', changes: { language: scenario.language } } })).ok()).toBe(true)
    await page.reload()
    const theme = page.locator('[data-slot="field"]').filter({ has: page.getByText(english ? 'Theme' : '主题', { exact: true }) }).getByRole('combobox')
    await theme.scrollIntoViewIfNeeded()
    await theme.click()
    await page.getByRole('option', { name: english ? 'Light' : '深色', exact: true }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', scenario.theme)
    const section = english ? 'Terminal' : '终端'
    if (scenario.width < 768) {
      await page.locator('.nova-mobile-topbar').getByRole('button', { name: 'Categories', exact: true }).click()
      await page.getByRole('dialog', { name: 'Categories', exact: true }).getByRole('button', { name: section, exact: true }).click()
    } else {
      await page.getByRole('button', { name: section, exact: true }).click()
    }
    const selector = page.getByRole('combobox', { name: english ? 'Terminal shell' : '终端 Shell', exact: true })
    await expect(selector).toBeInViewport()
    await selector.click()
    await expect(page.getByRole('option', { name: english ? 'WSL (default distribution)' : 'WSL（默认发行版）', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`terminal-shell-${scenario.width}-${scenario.theme}.png`), animations: 'disabled' })
    await page.keyboard.press('Escape')
  }
})

test('installed Windows shells accept commands in workbench terminals', async ({ page, request }) => {
  test.skip(process.env.DENOVA_TEST_WINDOWS_SHELLS !== '1', 'Requires PowerShell 7 and a configured WSL distribution')
  test.setTimeout(90_000)
  const book = await createAndOpenBook(request, 'Interactive Shell Book')
  const chat = await createAgentChatSession(request, book.projectId, 'Shell verification')
  for (const shell of ['pwsh.exe', 'wsl.exe']) {
    expect((await request.patch('/api/settings', { data: { layer: 'user', changes: { terminal_shell: shell, language: 'zh-CN' } } })).ok()).toBe(true)
    let output = ''
    page.on('websocket', socket => {
      if (socket.url().includes('/api/terminal/')) {
        socket.on('framereceived', frame => { output += frame.payload.toString() })
      }
    })
    await page.goto('/')
    await openAgentChatWorkbench(page)
    await openAgentChatSession(page, book.projectId, chat.title)
    await page.getByRole('button', { name: '新建标签页', exact: true }).first().click()
    await page.getByRole('menuitem', { name: '终端', exact: true }).click()
    const input = page.locator('.xterm-helper-textarea').filter({ visible: true })
    await expect(input).toBeVisible()
    await expect.poll(() => output).toContain('"type":"ready"')
    await input.focus()
    await page.keyboard.insertText(shell === 'pwsh.exe' ? "Write-Output ('denova-' + 'interactive-ok')" : "printf 'denova-%s\\n' 'interactive-ok'")
    await page.keyboard.press('Enter')
    await expect.poll(() => output, { timeout: 15_000 }).toContain('denova-interactive-ok')
    // Close through the API so each shell leaves no process running after this test.
    const runtime = await (await request.get('/api/terminal/sessions')).json()
    for (const session of runtime.sessions) {
      if (session.project_id === book.projectId) await request.delete(`/api/terminal/sessions/${session.id}`)
    }
  }
})
