import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test('generates usable LAN credentials in one save and preserves existing credentials across writing and game', async ({ page, request }, testInfo) => {
  test.setTimeout(60_000)
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await createAndOpenBook(request, 'LAN Access Browser Book')
  await page.goto('/')
  const sidebar = page.getByLabel('工作台侧边栏')
  await sidebar.getByRole('button', { name: '设置', exact: true }).click()
  await page.getByRole('button', { name: '局域网访问', exact: true }).click()

  const username = page.getByLabel('远程访问用户名', { exact: true })
  const password = page.getByLabel('远程访问密码', { exact: true })
  await expect(username).toHaveValue('')
  await expect(password).toHaveValue('')
  await page.getByRole('combobox', { name: '允许局域网访问' }).click()
  const save = page.waitForResponse(response => response.url().endsWith('/api/settings') && response.request().method() === 'PATCH')
  await page.getByRole('option', { name: '开启', exact: true }).click()
  const response = await save
  expect(response.ok()).toBe(true)
  const saved = await response.json()
  expect(saved.user.allow_lan_access).toBe(true)
  expect(saved.user.remote_access_password_set).toBe(true)
  expect(saved.user.remote_access_password).toBeFalsy()
  const generatedUsername = await username.inputValue()
  const generatedPassword = await password.inputValue()
  expect(generatedUsername).toMatch(/^denova-.+/)
  expect(generatedPassword).toMatch(/^[\w-]{24}$/)
  const login = await request.post('/api/auth/login', {
    headers: { 'X-Forwarded-For': '192.168.1.8' },
    data: { username: generatedUsername, password: generatedPassword },
  })
  expect(login.ok()).toBe(true)
  expect(await login.json()).toMatchObject({ authenticated: true, local: false })

  for (const destination of ['写作', '游戏', '设置']) {
    await sidebar.getByRole('button', { name: destination, exact: true }).click()
    await expect(sidebar.getByRole('button', { name: destination, exact: true })).toHaveAttribute('aria-current', 'page')
    if (destination === '游戏') await expect(page.getByRole('heading', { name: '开始这条故事线', exact: true })).toBeVisible()
  }
  await page.getByRole('button', { name: '局域网访问', exact: true }).click()
  await expect(username).toHaveValue(generatedUsername)
  await expect(password).toHaveValue('')
  await expect(page.getByRole('button', { name: '复制密码', exact: true })).toBeDisabled()

  for (const scenario of [
    { width: 1440, height: 960, theme: 'dark', language: 'zh-CN' },
    { width: 1440, height: 960, theme: 'light', language: 'en-US' },
    { width: 390, height: 844, theme: 'dark', language: 'en-US' },
    { width: 390, height: 844, theme: 'light', language: 'zh-CN' },
  ]) {
    const labels = scenario.language === 'en-US'
      ? { settings: 'Settings', categories: 'Categories', access: 'LAN Access', password: 'Remote Access Password' }
      : { settings: '设置', categories: '设置分类', access: '局域网访问', password: '远程访问密码' }
    await page.setViewportSize(scenario)
    const update = await request.patch('/api/settings', { data: { layer: 'user', changes: {
      theme: scenario.theme, language: scenario.language, remote_access_username: `${generatedUsername}-${'long-username-'.repeat(10)}`,
    } } })
    expect(update.ok()).toBe(true)
    await page.reload()
    // Settings is restored after reload; clicking its primary entry again would
    // close it before the category controls can be used.
    await expect(page.getByRole('heading', { name: labels.settings, exact: true })).toBeVisible()
    if (scenario.width < 768) {
      await page.locator('.nova-mobile-topbar').getByRole('button', { name: labels.categories, exact: true }).click()
      await page.getByRole('dialog', { name: labels.categories, exact: true }).getByRole('button', { name: labels.access, exact: true }).click()
      await expect(page.getByRole('dialog', { name: labels.categories, exact: true })).toBeHidden()
    } else {
      await page.getByRole('button', { name: labels.access, exact: true }).click()
    }
    await expect(page.getByLabel(labels.password, { exact: true })).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath(`lan-access-${scenario.width}-${scenario.theme}.png`), animations: 'disabled' })
  }
})
