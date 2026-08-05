import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { abortLoreImagesGenerate, previewLoreClassification } from './lore'

describe('lore API', () => {
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
})
