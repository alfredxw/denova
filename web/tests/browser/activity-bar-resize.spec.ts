import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

test('keeps the primary menu and workspace aligned when reversing a drag across the snap boundary', async ({ page, request }) => {
  await createAndOpenBook(request, 'Navigation resize verification with a long project title')
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.addInitScript(() => {
    localStorage.setItem('nova.layout.activityBarExpanded', 'true')
    localStorage.setItem('nova.layout.activityBarWidth', '160')
  })
  await page.goto('/')

  const menu = page.getByRole('navigation', { name: '工作台侧边栏' })
  const handle = menu.getByRole('separator')
  const edges = () => menu.evaluate(element => ({
    menu: element.getBoundingClientRect().right,
    workspace: document.querySelector('[data-nova-workspace-frame]')!.getBoundingClientRect().left,
    brand: element.querySelector('img[alt="Denova"]')!.getBoundingClientRect().left,
  }))
  const expectEdges = (edge: number) => expect.poll(edges).toEqual({ menu: edge, workspace: edge, brand: 12 })
  await expectEdges(160)
  const bounds = (await handle.boundingBox())!
  const y = bounds.y + bounds.height / 2
  await page.mouse.move(160, y)
  await page.mouse.down()
  for (const x of [130, 101, 91, 107, 113, 139]) {
    await page.mouse.move(x, y)
    await expectEdges(x)
  }
  await page.mouse.up()
  await expectEdges(139)
  await expect(handle).toHaveAttribute('aria-valuenow', '139')

  await menu.getByRole('button', { name: '游戏', exact: true }).click()
  await page.mouse.move(139, y)
  await page.mouse.down()
  await page.mouse.move(90, y)
  await expectEdges(90)
  await page.mouse.up()
  await expectEdges(48)

  await handle.press('ArrowRight')
  await expectEdges(139)
})
