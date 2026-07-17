import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import {
  createWorkspaceChangeComment,
  deleteWorkspaceChangeComment,
  getWorkspaceChangeGroup,
  getWorkspaceChangeReviewThread,
  listWorkspaceChangeGroups,
  redoWorkspaceChangeGroup,
  resolveWorkspaceChangeComment,
  reviewWorkspaceChangeGroup,
  undoWorkspaceChangeGroup,
  updateWorkspaceChangeComment,
} from './api'

describe('workspace change API', () => {
  it('lists and reads durable change groups', async () => {
    const workspace = '/books/中文作品'
    server.use(
      http.get('/api/workspace/change-groups', ({ request }) => {
        const params = new URL(request.url).searchParams
        expect(params.get('status')).toBe('pending')
        expect(params.get('run_id')).toBe('run-1')
        expect(params.get('session_id')).toBe('session-1')
        expect(params.get('review_thread_id')).toBe('thread-1')
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodeURIComponent(workspace))
        return HttpResponse.json({ groups: [{ id: 'group-1', review_status: 'pending', apply_state: 'applied', created_at: '2026-07-16T00:00:00Z', change_set_count: 1, paths: ['chapters/ch01.md'] }] })
      }),
      http.get('/api/workspace/change-groups/group-1', ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodeURIComponent(workspace))
        return HttpResponse.json({
          group: { id: 'group-1', review_status: 'pending', apply_state: 'applied', created_at: '2026-07-16T00:00:00Z', change_sets: [], comments: [] },
        })
      }),
    )

    await expect(listWorkspaceChangeGroups(workspace, {
      status: 'pending',
      runID: 'run-1',
      sessionID: 'session-1',
      reviewThreadID: 'thread-1',
    })).resolves.toEqual([
      expect.objectContaining({ id: 'group-1', paths: ['chapters/ch01.md'] }),
    ])
    await expect(getWorkspaceChangeGroup(workspace, 'group-1')).resolves.toMatchObject({ id: 'group-1', comments: [] })
  })

  it('reads the cumulative review projection from the canonical review_thread envelope', async () => {
    const workspace = '/books/中文作品'
    server.use(
      http.get('/api/workspace/change-review-threads/thread-1', ({ request }) => {
        expect(request.headers.get('X-Denova-Workspace')).toBe(encodeURIComponent(workspace))
        return HttpResponse.json({
          workspace,
          review_thread: {
            id: 'thread-1',
            latest_group_id: 'group-2',
            groups: [],
            comments: [],
            files: [{
              path: 'chapters/ch01.md',
              before_content: '旧',
              after_content: '新',
              base_revision: 'before',
              revision: 'after',
              base_group_id: 'group-1',
              base_change_set_id: 'set-1',
              latest_group_id: 'group-2',
              latest_change_set_id: 'set-2',
              group_ids: ['group-1', 'group-2'],
              change_set_ids: ['set-1', 'set-2'],
              pending_edit_ids: ['edit-2'],
              review_status: 'pending',
              apply_state: 'applied',
              continuity: 'continuous',
            }],
          },
        })
      }),
    )

    await expect(getWorkspaceChangeReviewThread(workspace, 'thread-1')).resolves.toMatchObject({
      id: 'thread-1',
      files: [{ before_content: '旧', after_content: '新' }],
    })
  })

  it('normalizes null Go slices at the review-thread API boundary', async () => {
    server.use(
      http.get('/api/workspace/change-review-threads/thread-empty', () => HttpResponse.json({
        review_thread: {
          id: 'thread-empty',
          latest_group_id: 'group-empty',
          groups: [{ id: 'group-empty', review_status: 'pending', apply_state: 'applied', created_at: '2026-07-16T00:00:00Z' }],
          comments: null,
          files: [{
            path: 'chapters/empty.md',
            before_content: '',
            after_content: '新章节',
            group_ids: null,
            change_set_ids: null,
            pending_edit_ids: null,
            review_status: 'pending',
            apply_state: 'applied',
            continuity: 'continuous',
          }],
        },
      })),
    )

    await expect(getWorkspaceChangeReviewThread('/books/demo', 'thread-empty')).resolves.toMatchObject({
      comments: [],
      files: [{ group_ids: [], change_set_ids: [], pending_edit_ids: [] }],
    })
  })

  it('rejects a successful non-API response instead of presenting an empty review', async () => {
    server.use(
      http.get('/api/workspace/change-review-threads/thread-invalid', () => HttpResponse.text('<!doctype html><title>Denova</title>')),
    )

    await expect(getWorkspaceChangeReviewThread('/books/demo', 'thread-invalid')).rejects.toThrow('Invalid workspace change review thread response')
  })

  it('sends the workspace lease header for every mutation and preserves request bodies', async () => {
    const workspace = '/books/中文作品'
    const requests: Array<{ path: string; body: unknown; workspace: string | null }> = []
    const record = async (path: string, request: Request) => {
      const body = request.method === 'DELETE' ? undefined : await request.json().catch(() => undefined)
      requests.push({ path, body, workspace: request.headers.get('X-Denova-Workspace') })
    }
    server.use(
      http.post('/api/workspace/change-groups/group-1/review', async ({ request }) => {
        await record('/review', request)
        return HttpResponse.json({ workspace, group: { id: 'group-1', change_sets: [] }, affected_paths: ['chapters/ch01.md'] })
      }),
      http.post('/api/workspace/change-groups/group-1/undo', async ({ request }) => {
        await record('/undo', request)
        return HttpResponse.json({ workspace, group: { id: 'group-1', change_sets: [] } })
      }),
      http.post('/api/workspace/change-groups/group-1/redo', async ({ request }) => {
        await record('/redo', request)
        return HttpResponse.json({ workspace, group: { id: 'group-1', change_sets: [] } })
      }),
      http.post('/api/workspace/change-comments', async ({ request }) => {
        await record('/comments', request)
        return HttpResponse.json({ workspace, comment: { id: 'comment-1', group_id: 'group-1', body: '调整人称' } }, { status: 201 })
      }),
      http.patch('/api/workspace/change-comments/comment-1', async ({ request }) => {
        await record('/comment-update', request)
        return HttpResponse.json({ workspace, comment: { id: 'comment-1', group_id: 'group-1', body: '更新评论' } })
      }),
      http.post('/api/workspace/change-comments/comment-1/resolve', async ({ request }) => {
        await record('/resolve', request)
        return HttpResponse.json({ workspace, comment: { id: 'comment-1', group_id: 'group-1', body: '调整人称', resolved: true } })
      }),
      http.delete('/api/workspace/change-comments/comment-1', async ({ request }) => {
        await record('/comment-delete', request)
        return HttpResponse.json({ workspace, comment: { id: 'comment-1', group_id: 'group-1', body: '调整人称', deleted: true } })
      }),
    )

    await reviewWorkspaceChangeGroup(workspace, 'group-1', { decision: 'reject', change_set_id: 'set-1', edit_ids: ['edit-1'], base_revision: 'sha256:current-after' })
    await undoWorkspaceChangeGroup(workspace, 'group-1')
    await redoWorkspaceChangeGroup(workspace, 'group-1')
    await createWorkspaceChangeComment(workspace, {
      group_id: 'group-1',
      change_set_id: 'set-1',
      edit_id: 'edit-1',
      body: '调整人称',
      anchor: { side: 'after', encoding: 'utf8-bytes-v1', revision: 'after', start: 3, end: 7, quote: '😀' },
    })
    await updateWorkspaceChangeComment(workspace, 'comment-1', '更新评论')
    await resolveWorkspaceChangeComment(workspace, 'comment-1', true)
    await deleteWorkspaceChangeComment(workspace, 'comment-1')

    expect(requests).toEqual([
      { path: '/review', body: { decision: 'reject', change_set_id: 'set-1', edit_ids: ['edit-1'], base_revision: 'sha256:current-after' }, workspace: encodeURIComponent(workspace) },
      { path: '/undo', body: undefined, workspace: encodeURIComponent(workspace) },
      { path: '/redo', body: undefined, workspace: encodeURIComponent(workspace) },
      { path: '/comments', body: { group_id: 'group-1', change_set_id: 'set-1', edit_id: 'edit-1', body: '调整人称', anchor: { side: 'after', encoding: 'utf8-bytes-v1', revision: 'after', start: 3, end: 7, quote: '😀' } }, workspace: encodeURIComponent(workspace) },
      { path: '/comment-update', body: { body: '更新评论' }, workspace: encodeURIComponent(workspace) },
      { path: '/resolve', body: { resolved: true }, workspace: encodeURIComponent(workspace) },
      { path: '/comment-delete', body: undefined, workspace: encodeURIComponent(workspace) },
    ])
  })
})
