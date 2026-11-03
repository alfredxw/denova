import { createRequire } from 'node:module'

const { expect } = createRequire(new URL('../../web/package.json', import.meta.url))('@playwright/test')

/** DOM clicks bypass native hit testing, so also check the window's drag regions. */
export async function verifyMessageCenter(page) {
  const dialog = page.getByRole('dialog', { name: '消息中心', exact: true })
  if (!await dialog.isVisible()) await page.getByRole('button', { name: '打开消息中心', exact: true }).click()
  const markAll = dialog.getByRole('button', { name: '全部已读', exact: true })
  const close = dialog.getByRole('button', { name: '关闭', exact: true })
  await expect(markAll).toBeVisible()
  await expect(dialog.locator('[data-message-id]').first()).toBeVisible()
  if (process.platform === 'darwin') {
    for (const button of [markAll, close]) {
      await expect.poll(() => button.evaluate(element => {
        const target = element.getBoundingClientRect()
        const x = target.x + target.width / 2, y = target.y + target.height / 2
        let drag = false, excluded = false
        for (const node of document.querySelectorAll('*')) {
          for (const pseudo of [null, '::before', '::after']) {
            const style = getComputedStyle(node, pseudo)
            const region = style.getPropertyValue('-webkit-app-region')
            if (!['drag', 'no-drag'].includes(region) || (pseudo && style.content === 'none')) continue
            if (style.visibility === 'hidden' || style.display === 'none') continue
            let rect = node.getBoundingClientRect()
            if (pseudo) {
              const left = rect.left + node.clientLeft + parseFloat(style.left)
              const top = rect.top + node.clientTop + parseFloat(style.top)
              rect = { left, top, right: left + parseFloat(style.width), bottom: top + parseFloat(style.height) }
            }
            if (![rect.left, rect.top, rect.right, rect.bottom].every(Number.isFinite)) continue
            if (x < rect.left || x >= rect.right || y < rect.top || y >= rect.bottom) continue
            if (region === 'drag') drag = true
            else excluded = true
          }
        }
        return drag && !excluded
      }), { message: 'Message actions must not be swallowed by a native window drag region', timeout: 3000 }).toBe(false)
    }
  }
  if (await markAll.isEnabled()) {
    const response = page.waitForResponse(response => response.url().endsWith('/api/messages/read-all') && response.request().method() === 'POST')
    await markAll.click()
    expect((await response).ok()).toBe(true)
  }
  await expect(markAll).toBeDisabled()
  await expect(dialog.getByText('未读', { exact: true })).toHaveCount(0)
  await expect.poll(() => page.evaluate(async () => (await (await fetch('/api/activity/summary')).json()).message_unread_count)).toBe(0)
  await close.click()
  await expect(dialog).toHaveCount(0)
  await page.reload()
  await page.getByRole('button', { name: '打开消息中心', exact: true }).click()
  await expect(markAll).toBeDisabled()
  await expect(dialog.getByText('未读', { exact: true })).toHaveCount(0)
  await close.click()
  await expect(dialog).toHaveCount(0)
  console.log('Desktop messages passed: native hit regions, mark all read, close and reload persistence')
}
