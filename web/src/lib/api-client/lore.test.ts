import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { abortLoreImagesGenerate, previewLoreClassification } from './lore'

describe('lore API', () => {
  it('binds classification and image task controls to the originating workspace', async () => {
    const workspace = '/books/中文作品'
    const encodedWorkspace = encodeURIComponent(workspace)
    let previewBody: unknown
    server.use(
      http.post('/api/lore/classification/preview', async ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodedWorkspace)
        previewBody = await request.json()
        return HttpResponse.json({ revision: 'r1', suggestions: [] })
      }),
      http.post('/api/lore/images/generate/abort', ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodedWorkspace)
        return HttpResponse.json({ status: 'ok' })
      }),
    )

    await previewLoreClassification(workspace, { mode: 'heuristic' })
    await abortLoreImagesGenerate(workspace)
    expect(previewBody).toEqual({ mode: 'heuristic' })
  })
})
