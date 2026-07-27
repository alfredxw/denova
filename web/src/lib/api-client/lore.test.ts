import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { abortLoreImagesGenerate, getLoreItems, updateLoreItem } from './lore'

describe('lore API', () => {
  it('binds reads, edits, and task controls to the originating workspace', async () => {
    const workspace = '/books/中文作品'
    const encodedWorkspace = encodeURIComponent(workspace)
    const item = {
      id: 'hero',
      enabled: true,
      type: 'character',
      type_source: 'manual',
      name: '林川',
      importance: 'major',
      tags: [],
      brief_description: '',
      keywords: [],
      load_mode: 'auto',
      content: '正文',
      created_at: 'r0',
      updated_at: 'r1',
    }
    let updateBody: unknown
    server.use(
      http.get('/api/lore/items', ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodedWorkspace)
        return HttpResponse.json({ items: [item] })
      }),
      http.patch('/api/lore/items/hero', async ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodedWorkspace)
        updateBody = await request.json()
        return HttpResponse.json({
          ...item,
          content: '新正文',
          updated_at: 'r2',
        })
      }),
      http.post('/api/lore/images/generate/abort', ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodedWorkspace)
        return HttpResponse.json({ status: 'ok' })
      }),
    )

    await expect(getLoreItems(workspace)).resolves.toEqual([item])
    await expect(
      updateLoreItem(
        workspace,
        item.id,
        { content: '新正文' },
        item.updated_at,
      ),
    ).resolves.toMatchObject({ updated_at: 'r2' })
    await abortLoreImagesGenerate(workspace)
    expect(updateBody).toEqual({ content: '新正文', base_revision: 'r1' })
  })
})
