import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { abortLoreImagesGenerate, getLoreItems, updateLoreItem } from './lore'
import type { LoreItem, LoreItemInput } from './types'

describe('lore API', () => {
  it('binds reads, edits, and task controls to the originating workspace', async () => {
    const workspace = '/books/中文作品'
    const encodedWorkspace = encodeURIComponent(workspace)
    const item: LoreItem = {
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
      http.put('/api/lore/items/hero', async ({ request }) => {
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
    const update = {
      id: item.id,
      enabled: item.enabled,
      type: item.type,
      type_source: item.type_source,
      name: item.name,
      importance: item.importance,
      tags: item.tags,
      brief_description: item.brief_description,
      keywords: item.keywords,
      load_mode: item.load_mode,
      content: '新正文',
    } satisfies LoreItemInput
    await expect(updateLoreItem(workspace, item.id, update, item.updated_at)).resolves.toMatchObject({ updated_at: 'r2' })
    await abortLoreImagesGenerate(workspace)
    expect(updateBody).toEqual({ ...update, base_revision: 'r1' })
  })
})
