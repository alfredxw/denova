import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { server } from '@/test/msw/server'
import { abortLoreImagesGenerate, previewLoreClassification, uploadLoreItemImage } from './lore'

describe('lore API', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('binds classification and image task controls to the originating Project', async () => {
    const projectId = 'project-中文作品'
    let previewBody: unknown
    server.use(
      http.post('/api/projects/:projectId/book/lore/classification/preview', async ({ params, request }) => {
        expect(params.projectId).toBe(projectId)
        previewBody = await request.json()
        return HttpResponse.json({ revision: 'r1', suggestions: [] })
      }),
      http.post('/api/projects/:projectId/book/lore/images/generate/abort', ({ params }) => {
        expect(params.projectId).toBe(projectId)
        return HttpResponse.json({ status: 'ok' })
      }),
    )

    await previewLoreClassification(projectId, { mode: 'heuristic' })
    await abortLoreImagesGenerate(projectId)
    expect(previewBody).toEqual({ mode: 'heuristic' })
  })

  it('uploads a lore image within the originating Project', async () => {
    const projectId = 'project-中文作品'
    const itemId = 'hero/林川'
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      expect(path).toBe(`/api/projects/${encodeURIComponent(projectId)}/book/lore/items/${encodeURIComponent(itemId)}/image/upload`)
      const form = init?.body as FormData
      const file = form.get('file')
      expect(file).not.toBeNull()
      expect((file as File).type).toBe('image/png')
      expect((file as File).size).toBeGreaterThan(0)
      return new Response(JSON.stringify({ id: itemId, name: '林川' }), {
        headers: { 'Content-Type': 'application/json' },
      })
    })

    await uploadLoreItemImage(projectId, itemId, new File(['image'], 'portrait.png', { type: 'image/png' }))
  })
})
