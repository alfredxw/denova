import { expect, test } from '../support/fixtures'
import { createAndOpenBook, createProjectFile } from '../support/api'

for (const theme of ['dark', 'light']) {
  test(`source hover stays next to its symbol in ${theme}`, async ({ page, request }) => {
    const book = await createAndOpenBook(request, `Source hover ${theme}`)
    await createProjectFile(request, book.projectId, 'client.mjs',
      'export function receive(event) {\n  return event;\n}\nreceive("hello");\n')
    await page.route(/\/api\/(?:projects\/[^/]+\/)?settings$/, async (route) => {
      const response = await route.fetch()
      const settings = await response.json()
      await route.fulfill({ response, json: { ...settings, effective: { ...settings.effective, theme } } })
    })
    await page.goto('/')
    await page.getByRole('button', { name: '项目文件', exact: true }).click()
    await page.getByRole('treeitem', { name: 'client.mjs', exact: true }).click()
    const symbol = page.locator('.monaco-editor:visible .view-line').first().getByText('receive', { exact: true })
    const hover = page.locator('.monaco-hover:visible')
    for (const width of [1440, 640]) {
      await page.setViewportSize({ width, height: 900 })
      await symbol.hover()
      await expect(hover).toContainText('receive')
      const anchor = (await symbol.boundingBox())!
      const popup = (await hover.boundingBox())!
      expect(Math.abs(popup.x - anchor.x)).toBeLessThan(30)
      expect(Math.min(Math.abs(popup.y + popup.height - anchor.y), Math.abs(popup.y - anchor.y - anchor.height))).toBeLessThan(30)
      expect(await hover.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        return element.contains(document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2))
      })).toBe(true)
      await page.screenshot({ path: `test-results/source-hover-${theme}-${width}.png` })
      await page.keyboard.press('Escape')
    }
  })
}
