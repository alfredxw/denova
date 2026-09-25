import { readFile } from 'node:fs/promises'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'

// The independent index owns example bytes. Opt in when validating that checkout.
const archive = process.env.DENOVA_MARKET_GAME_ZIP

test('market example game saves choices and resumes in both locales and themes', async ({
  page,
  request,
}) => {
  test.skip(
    !archive,
    'Set DENOVA_MARKET_GAME_ZIP to the lantern-crossing example archive',
  )
  await request.patch('/api/settings', {
    data: { layer: 'user', changes: { language: 'zh-CN', theme: 'dark' } },
  })
  await createAndOpenBook(request, 'Market example game')
  const previewResponse = await request.post(
    '/api/resource-exchange/previews',
    {
      multipart: {
        file: {
          name: 'lantern-crossing.zip',
          mimeType: 'application/zip',
          buffer: await readFile(archive!),
        },
      },
    },
  )
  expect(previewResponse.ok(), await previewResponse.text()).toBe(true)
  const preview = await previewResponse.json()
  const candidate = preview.candidates[0]
  const resource = candidate.resources[0]
  const planResponse = await request.post('/api/resource-exchange/plans', {
    data: {
      preview_id: preview.preview_id,
      candidate_id: candidate.candidate_id,
      resources: [resource.id],
      grants: { [resource.id]: ['gameData'] },
    },
  })
  expect(planResponse.ok(), await planResponse.text()).toBe(true)
  const plan = await planResponse.json()
  const installed = await request.post(
    `/api/resource-exchange/plans/${plan.plan_id}/apply`,
  )
  expect(installed.ok(), await installed.text()).toBe(true)
  await page.addInitScript(() =>
    localStorage.setItem('nova:mode', 'interactive'),
  )
  await page.goto('/')
  await page
    .getByRole('button', { name: '新建', exact: true })
    .filter({ visible: true })
    .click()
  await page.getByLabel('游戏类型').click()
  await page.getByRole('option', { name: '雾中灯渡', exact: true }).click()
  await page.getByLabel('故事线名称（可选）').fill('Lantern validation')
  await page.getByRole('button', { name: '创建故事线', exact: true }).click()
  const frame = page.frameLocator('iframe[title="游戏画面"]')
  await expect(
    frame.getByRole('heading', { name: '迟到的灯', exact: true }),
  ).toBeVisible()
  await frame
    .getByRole('button', { name: '查看仓单背面的字迹', exact: true })
    .click()
  await expect(
    frame.getByRole('heading', { name: '被改过的日期', exact: true }),
  ).toBeVisible()
  await expect(frame.getByRole('status')).toHaveText('进度已保存。')
  await page.reload()
  await expect(
    frame.getByRole('heading', { name: '被改过的日期', exact: true }),
  ).toBeVisible()
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await page.screenshot({
      path: `test-results/market-lantern-dark-${width}.png`,
    })
    expect(
      await frame
        .locator('body')
        .evaluate((body) => body.scrollWidth <= body.clientWidth),
    ).toBe(true)
  }
  await request.patch('/api/settings', {
    data: { layer: 'user', changes: { language: 'en-US', theme: 'light' } },
  })
  await page.reload()
  const english = page.frameLocator('iframe[title="Game"]')
  await expect(
    english.getByRole('heading', { name: 'The altered date', exact: true }),
  ).toBeVisible()
  await english
    .getByRole('button', { name: 'Ask the keeper at the tower', exact: true })
    .click()
  await english
    .getByRole('button', {
      name: 'Light the maintenance hatch for her',
      exact: true,
    })
    .click()
  await expect(
    english.getByRole('heading', { name: 'After the tide', exact: true }),
  ).toBeVisible()
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    await page.screenshot({
      path: `test-results/market-lantern-light-${width}.png`,
    })
    expect(
      await english
        .locator('body')
        .evaluate((body) => body.scrollWidth <= body.clientWidth),
    ).toBe(true)
  }
  await english
    .getByRole('button', { name: 'Start a new journey', exact: true })
    .click()
  await expect(
    english.getByRole('heading', { name: 'The late lantern', exact: true }),
  ).toBeVisible()
  await page.reload()
  await expect(
    english.getByRole('heading', { name: 'The late lantern', exact: true }),
  ).toBeVisible()
})
