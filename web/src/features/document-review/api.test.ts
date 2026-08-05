import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import { createDocumentComment, deleteDocumentComment, getDocumentReview, updateDocumentComment } from './api'
import type { DocumentReviewAnchor } from './types'

describe('document review API', () => {
  it('uses the stable Project resource and normalizes comment envelopes', async () => {
    const projectId = 'book-中文作品'
    const projectPath = encodeURIComponent(projectId)
    const workspace = '/books/中文作品'
    const anchor: DocumentReviewAnchor = {
      kind: 'text-range', encoding: 'utf8-bytes-v1', revision: 'sha256:body', start: 3, end: 9,
      quote: '正文', display_quote: '正文', editor_from: 2, editor_to: 4,
    }
    const requests: Array<{ method: string; body?: unknown }> = []
    const target = { kind: 'workspace_file' as const, id: 'chapters/a.md' }
    const thread = (body: string) => ({
      id: 'review-1',
      comments: [{ id: 'comment-1', thread_id: 'review-1', target, body, anchor, created_at: '', updated_at: '' }],
    })
    server.use(
      http.get(`/api/projects/${projectPath}/book/document-review`, () => {
        return HttpResponse.json({ project_id: projectId, workspace, review_thread: { id: '', comments: null } })
      }),
      http.post(`/api/projects/${projectPath}/book/document-comments`, async ({ request }) => {
        requests.push({ method: request.method, body: await request.json() })
        return HttpResponse.json({ project_id: projectId, workspace, review_thread: thread('修改这里'), comment: thread('修改这里').comments[0] }, { status: 201 })
      }),
      http.patch(`/api/projects/${projectPath}/book/document-comments/comment-1`, async ({ request }) => {
        requests.push({ method: request.method, body: await request.json() })
        return HttpResponse.json({ project_id: projectId, workspace, review_thread: thread('更新意见'), comment: thread('更新意见').comments[0] })
      }),
      http.delete(`/api/projects/${projectPath}/book/document-comments/comment-1`, ({ request }) => {
        requests.push({ method: request.method })
        return HttpResponse.json({ project_id: projectId, workspace, review_thread: { id: '', comments: [] }, comment: { ...thread('更新意见').comments[0], deleted: true } })
      }),
    )

    await expect(getDocumentReview(projectId)).resolves.toEqual(expect.objectContaining({ id: '', comments: [] }))
    await expect(createDocumentComment(projectId, { target, body: '修改这里', anchor })).resolves.toMatchObject({ reviewThread: { id: 'review-1' }, comment: { id: 'comment-1' } })
    await updateDocumentComment(projectId, 'comment-1', '更新意见')
    await deleteDocumentComment(projectId, 'comment-1')
    expect(requests).toEqual([
      { method: 'POST', body: { target, body: '修改这里', anchor } },
      { method: 'PATCH', body: { body: '更新意见' } },
      { method: 'DELETE' },
    ])
  })

  it('rejects review state returned for another Project', async () => {
    server.use(
      http.get('/api/projects/book-a/book/document-review', () => HttpResponse.json({
        project_id: 'book-b',
        workspace: '/books/b',
        review_thread: { id: '', comments: [] },
      })),
    )

    await expect(getDocumentReview('book-a')).rejects.toThrow('Document review scope mismatch')
  })
})
