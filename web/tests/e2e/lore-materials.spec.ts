import { expect, test, type APIRequestContext } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import type { LoreItem } from '../../src/lib/api'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'

const portrait = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=',
  'base64',
)
function wave() {
  const data = Buffer.alloc(44 + 32000)
  data.write('RIFF')
  data.writeUInt32LE(data.length - 8, 4)
  data.write('WAVEfmt ', 8)
  data.writeUInt32LE(16, 16)
  data.writeUInt16LE(1, 20)
  data.writeUInt16LE(1, 22)
  data.writeUInt32LE(8000, 24)
  data.writeUInt32LE(16000, 28)
  data.writeUInt16LE(2, 32)
  data.writeUInt16LE(16, 34)
  data.write('data', 36)
  data.writeUInt32LE(data.length - 44, 40)
  return data
}
async function readItems(request: APIRequestContext, project: string): Promise<LoreItem[]> {
  const response = await request.get(`/api/projects/${project}/book/lore/items`)
  expect(response.ok(), await response.text()).toBe(true)
  return (await response.json()).items
}

for (const theme of ['dark', 'light']) {
  test(`manages shared lore materials and stops audio in ${theme}`, async ({
    page,
    request,
  }, testInfo) => {
    const settings = await (await request.get('/api/settings')).json()
    const patch = await request.patch('/api/settings', {
      data: {
        layer: 'user',
        base_revision: settings.revisions.user,
        changes: { theme, language: 'zh-CN' },
      },
    })
    expect(patch.ok(), await patch.text()).toBe(true)
    const book = await createAndOpenBook(request, `Lore media ${theme}`)
    const route = `/api/projects/${book.projectId}/book/lore/items`
    const create = await request.post(route, {
      data: {
        id: 'hero',
        name: '素材测试角色',
        type: 'character',
        content: '原始正文',
        enabled: true,
      },
    })
    expect(create.ok(), await create.text()).toBe(true)
    await page.goto('/')
    await page
      .getByLabel('工作台侧边栏')
      .getByRole('button', { name: '资料库', exact: true })
      .click()
    await page.getByRole('tab', { name: '素材 (0)', exact: true }).click()
    await expect(page.getByText('还没有关联素材', { exact: true })).toBeVisible()
    await page.getByLabel('上传文件', { exact: true }).setInputFiles([
      { name: 'portrait.png', mimeType: 'image/png', buffer: portrait },
      { name: 'environment.wav', mimeType: 'audio/wav', buffer: wave() },
    ])
    await expect(page.getByRole('tab', { name: '素材 (2)', exact: true })).toBeVisible()
    await expect
      .poll(async () => (await readItems(request, book.projectId))[0]?.resolved_materials?.length)
      .toBe(2)
    expect((await readItems(request, book.projectId))[0].image).toBeUndefined()
    await page.getByRole('button', { name: '查看素材：portrait.png', exact: true }).click()
    let dialog = page.getByRole('dialog', { name: 'portrait.png', exact: true })
    await dialog
      .getByLabel('素材名称', { exact: true })
      .fill('正面参考与服装 ' + 'Long reference '.repeat(8))
    await dialog
      .getByLabel('素材说明（可选）', { exact: true })
      .fill('保持脸型；当前镜头可以换装。'.repeat(20))
    await dialog.getByRole('button', { name: '保存', exact: true }).click()
    await expect(dialog).toBeHidden()
    await page.getByRole('button', { name: /^素材操作：正面参考/ }).click()
    await page.getByRole('menuitem', { name: '设为封面', exact: true }).click()
    await expect
      .poll(async () => (await readItems(request, book.projectId))[0].image?.image_path)
      .toBeTruthy()
    await page.screenshot({
      path: testInfo.outputPath(`materials-${theme}-wide.png`),
      animations: 'disabled',
    })
    const audio = page.getByRole('tabpanel').locator('audio').first()
    await audio.evaluate((element: HTMLAudioElement) => element.play())
    await expect
      .poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused))
      .toBe(false)
    const playing = await audio.elementHandle()
    await page.getByRole('tab', { name: '设定', exact: true }).click()
    expect(await playing?.evaluate((element: HTMLAudioElement) => element.paused)).toBe(true)
    await page.getByRole('tab', { name: '素材 (2)', exact: true }).click()
    await page.setViewportSize({ width: 390, height: 844 })
    await page.getByRole('button', { name: /^查看素材：正面参考/ }).click()
    dialog = page
      .getByRole('dialog')
      .filter({ has: page.getByLabel('素材说明（可选）', { exact: true }) })
    await expect(dialog.getByLabel('素材说明（可选）', { exact: true })).toBeVisible()
    await page.screenshot({
      path: testInfo.outputPath(`materials-${theme}-390.png`),
      animations: 'disabled',
    })
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true)
    await dialog.getByRole('button', { name: '清除封面', exact: true }).click()
    await expect
      .poll(async () => (await readItems(request, book.projectId))[0].image)
      .toBeUndefined()
    await dialog.getByRole('button', { name: '移除关联', exact: true }).click()
    await expect(page.getByRole('tab', { name: '素材 (1)', exact: true })).toBeVisible()
    await page.getByRole('button', { name: '添加素材', exact: true }).click()
    await page.getByRole('menuitem', { name: '从作品素材选择', exact: true }).click()
    await page
      .getByRole('dialog', { name: '从作品素材选择', exact: true })
      .getByRole('button', { name: '查看素材：portrait.png', exact: true })
      .click()
    await expect(page.getByRole('tab', { name: '素材 (2)', exact: true })).toBeVisible()
    const persisted = (await readItems(request, book.projectId))[0]
    expect(persisted.content).toBe('原始正文')
    expect(persisted.type).toBe('character')
    expect(persisted.image).toBeUndefined()
    // The actual asset boundary serves audio and byte ranges, not only an URL.
    const sound = persisted.resolved_materials!.find(
      (material) => material.mime_type === 'audio/wav',
    )!
    const response = await request.get(
      `/api/projects/${book.projectId}/files/asset?path=${encodeURIComponent(sound.path)}`,
      { headers: { Range: 'bytes=0-43' } },
    )
    expect(response.status()).toBe(206)
    expect((await response.body()).length).toBe(44)
    expect(response.headers()['content-type']).toContain('audio/wav')
  })
}

test('an upload completing after leaving the material tab preserves unsaved text', async ({
  page,
  request,
  browserDiagnostics,
}) => {
  browserDiagnostics.allow(
    /console\.error: Failed to load resource:.*409 \(Conflict\).*\/book\/lore\/items\/hero:/,
  )
  const book = await createAndOpenBook(request, 'Lore upload concurrency')
  const route = `/api/projects/${book.projectId}/book/lore/items`
  await request.post(route, {
    data: { id: 'hero', name: '并发角色', type: 'character', content: 'Original', enabled: true },
  })
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '资料库', exact: true }).click()
  await page.getByRole('tab', { name: '素材 (0)', exact: true }).click()
  let release!: () => void
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  let committed!: () => void
  const committedPromise = new Promise<void>((resolve) => {
    committed = resolve
  })
  await page.route('**/materials/upload', async (route) => {
    const response = await route.fetch()
    committed()
    await held
    await route.fulfill({ response })
  })
  await page
    .getByLabel('上传文件', { exact: true })
    .setInputFiles({ name: 'portrait.png', mimeType: 'image/png', buffer: portrait })
  await committedPromise
  await page.getByRole('tab', { name: '设定', exact: true }).click()
  const editor = page.getByRole('textbox', { name: '正文', exact: true })
  await editor.fill('Text written while the upload response was pending.')
  release()
  await editor.press(process.platform === 'darwin' ? 'Meta+S' : 'Control+S')
  await expect
    .poll(async () => (await readItems(request, book.projectId))[0]?.content)
    .toContain('Text written while the upload response was pending.')
  await expect
    .poll(async () => (await readItems(request, book.projectId))[0]?.resolved_materials?.length)
    .toBe(1)
  await expect(page.getByRole('status', { name: '所有更改均已保存' })).toBeVisible()
  await expect(page.getByText(/内容已被 Agent 或其他操作更新/)).toHaveCount(0)
})

test('retries only failed uploads without changing the current selection', async ({
  page,
  request,
  browserDiagnostics,
}) => {
  const book = await createAndOpenBook(request, 'Lore upload retry')
  const route = `/api/projects/${book.projectId}/book/lore/items`
  for (const [id, name] of [
    ['a', 'A 角色'],
    ['b', 'B 角色'],
  ]) {
    const result = await request.post(route, {
      data: { id, name, type: 'character', content: name, enabled: true },
    })
    expect(result.ok()).toBe(true)
  }
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '资料库', exact: true }).click()
  await page.getByRole('tab', { name: '素材 (0)', exact: true }).click()
  let attempts = 0
  browserDiagnostics.allow(/console\.error: Failed to load resource:.*422/)
  await page.route('**/materials/upload', async (route) => {
    if (route.request().postDataBuffer()?.includes(Buffer.from('retry.wav')) && ++attempts === 1) {
      await route.fulfill({ status: 422, json: { error: 'Temporary upload failure.' } })
    } else {
      await route.continue()
    }
  })
  await page.getByLabel('上传文件', { exact: true }).setInputFiles([
    { name: 'portrait.png', mimeType: 'image/png', buffer: portrait },
    { name: 'retry.wav', mimeType: 'audio/wav', buffer: wave() },
  ])
  await expect(page.getByRole('tab', { name: '素材 (1)', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '重试失败项', exact: true }).click()
  await expect(page.getByRole('tab', { name: '素材 (2)', exact: true })).toBeVisible()
  expect(attempts).toBe(2)

  let release!: () => void
  const pending = new Promise<void>((resolve) => {
    release = resolve
  })
  let committed!: () => void
  const committedPromise = new Promise<void>((resolve) => {
    committed = resolve
  })
  await page.unroute('**/materials/upload')
  await page.route('**/materials/upload', async (route) => {
    const response = await route.fetch()
    committed()
    await pending
    await route.fulfill({ response })
  })
  try {
    await page
      .getByLabel('上传文件', { exact: true })
      .setInputFiles({ name: 'another.png', mimeType: 'image/png', buffer: portrait })
    await committedPromise
    await page.getByRole('button', { name: /^B 角色/ }).click()
    release()
    await expect(page.getByLabel('名称', { exact: true })).toHaveValue('B 角色')
    await expect
      .poll(
        async () =>
          (await readItems(request, book.projectId)).find((item) => item.id === 'a')
            ?.resolved_materials?.length,
      )
      .toBe(3)
    await expect(page.getByLabel('名称', { exact: true })).toHaveValue('B 角色')
    expect(
      (await readItems(request, book.projectId)).find((item) => item.id === 'b')
        ?.resolved_materials ?? [],
    ).toHaveLength(0)
  } finally {
    release()
  }
})

test('reads released single-image data without rewriting and appends generated images', async ({
  page,
  request,
}) => {
  const book = await createAndOpenBook(request, 'Lore legacy images')
  const imagePath = 'assets/lore/images/hero/old.png'
  const collectionPath = path.join(book.workspace, 'setting/lore/items.json')
  await mkdir(path.dirname(path.join(book.workspace, imagePath)), { recursive: true })
  await mkdir(path.dirname(collectionPath), { recursive: true })
  await writeFile(path.join(book.workspace, imagePath), portrait)
  const legacy = JSON.stringify({
    version: 2,
    items: [
      {
        id: 'hero',
        name: '旧版角色',
        type: 'character',
        type_source: 'manual',
        enabled: true,
        content: '保留正文',
        image: { image_path: imagePath, mime_type: 'image/png', alt_text: '旧图说明' },
      },
    ],
  })
  await writeFile(collectionPath, legacy)
  await page.goto('/')
  await page.getByLabel('工作台侧边栏').getByRole('button', { name: '资料库', exact: true }).click()
  await page.getByRole('tab', { name: '素材 (1)', exact: true }).click()
  await expect(page.getByText('旧图说明', { exact: true })).toBeVisible()
  expect(await readFile(collectionPath, 'utf8')).toBe(legacy)
  for (const count of [2, 3]) {
    await page.getByRole('button', { name: '添加素材', exact: true }).click()
    await page.getByRole('menuitem', { name: '生成图片', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: '生成图片', exact: true })
    await dialog.getByLabel('提示词编写方式', { exact: true }).click()
    await page.getByRole('option', { name: '自定义最终提示词', exact: true }).click()
    await dialog.getByLabel('最终提示词', { exact: true }).fill(`Draw a portrait ${count}`)
    await dialog.getByRole('button', { name: '生成图片', exact: true }).click()
    await expect(page.getByRole('tab', { name: `素材 (${count})`, exact: true })).toBeVisible()
  }
  const item = (await readItems(request, book.projectId))[0]
  expect(item.image?.image_path).toBe(imagePath)
  expect(item.content).toBe('保留正文')
  expect(new Set(item.resolved_materials!.map((material) => material.path)).size).toBe(3)
  expect(
    item.resolved_materials!.slice(1).every((material) => material.source.kind === 'generated'),
  ).toBe(true)
  expect(await readFile(path.join(book.workspace, imagePath))).toEqual(portrait)
})
