import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { abortLoreImagesGenerate, previewLoreClassification, uploadLoreItemImage } from './lore'

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

  it('uploads a lore image within the originating Project', async () => {
		const projectId = 'project-中文作品'
		const itemId = 'hero/林川'
		server.use(
			http.post('/api/projects/:projectId/book/lore/items/:itemId/image/upload', async ({ params, request }) => {
				expect(params).toMatchObject({ projectId, itemId })
				const form = await request.formData()
				const file = form.get('file')
				expect(file).not.toBeNull()
				expect((file as File).type).toBe('image/png')
				expect((file as File).size).toBeGreaterThan(0)
				return HttpResponse.json({ id: itemId, name: '林川' })
			}),
		)

		await uploadLoreItemImage(projectId, itemId, new File(['image'], 'portrait.png', { type: 'image/png' }))
	})
})
