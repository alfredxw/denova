import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { createDocumentComment, deleteDocumentComment, getDocumentReview, updateDocumentComment } from './api'
import type { DocumentReviewAnchor } from './types'

describe('document review API', () => {
  it('uses the canonical workspace lease and normalizes comment envelopes', async () => {
    const workspace = '/books/中文作品'
    const anchor: DocumentReviewAnchor = {
      kind: 'text-range', encoding: 'utf8-bytes-v1', revision: 'sha256:body', start: 3, end: 9,
      quote: '正文', display_quote: '正文', editor_from: 2, editor_to: 4,
    }
    const requests: Array<{ method: string; body?: unknown }> = []
    const thread = (body: string) => ({
      id: 'review-1',
      comments: [{ id: 'comment-1', thread_id: 'review-1', path: 'chapters/a.md', body, anchor, created_at: '', updated_at: '' }],
    })
    server.use(
      http.get('/api/workspace/document-review', ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodeURIComponent(workspace))
        return HttpResponse.json({ workspace, review_thread: { id: '', comments: null } })
      }),
      http.post('/api/workspace/document-comments', async ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodeURIComponent(workspace))
        requests.push({ method: request.method, body: await request.json() })
        return HttpResponse.json({ workspace, review_thread: thread('修改这里'), comment: thread('修改这里').comments[0] }, { status: 201 })
      }),
      http.patch('/api/workspace/document-comments/comment-1', async ({ request }) => {
        requests.push({ method: request.method, body: await request.json() })
        return HttpResponse.json({ workspace, review_thread: thread('更新意见'), comment: thread('更新意见').comments[0] })
      }),
      http.delete('/api/workspace/document-comments/comment-1', ({ request }) => {
        requests.push({ method: request.method })
        return HttpResponse.json({ workspace, review_thread: { id: '', comments: [] }, comment: { ...thread('更新意见').comments[0], deleted: true } })
      }),
    )

    await expect(getDocumentReview(workspace)).resolves.toEqual(expect.objectContaining({ id: '', comments: [] }))
    await expect(createDocumentComment(workspace, { path: 'chapters/a.md', body: '修改这里', anchor })).resolves.toMatchObject({ reviewThread: { id: 'review-1' }, comment: { id: 'comment-1' } })
    await updateDocumentComment(workspace, 'comment-1', '更新意见')
    await deleteDocumentComment(workspace, 'comment-1')
    expect(requests).toEqual([
      { method: 'POST', body: { path: 'chapters/a.md', body: '修改这里', anchor } },
      { method: 'PATCH', body: { body: '更新意见' } },
      { method: 'DELETE' },
    ])
  })
})
