import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw/server'
import {
  createProjectChangeComment,
  deleteProjectChangeComment,
  getProjectChangeGroup,
  getProjectChangeReviewThread,
  listProjectChangeGroups,
  redoProjectChangeGroup,
  reviewProjectChangeGroup,
  undoProjectChangeGroup,
  updateProjectChangeComment,
} from './api'

describe('Project change API', () => {
  it('lists and reads durable change groups', async () => {
    const projectId = 'project-one'
    server.use(
      http.get('/api/projects/project-one/changes/groups', ({ request }) => {
        const params = new URL(request.url).searchParams
        expect(params.get('status')).toBe('pending')
        expect(params.get('run_id')).toBe('run-1')
        expect(params.get('session_id')).toBe('session-1')
        expect(params.get('review_thread_id')).toBe('thread-1')
        return HttpResponse.json({ project_id: projectId, workspace: '/books/中文作品', groups: [{ id: 'group-1', review_status: 'pending', apply_state: 'applied', created_at: '2026-07-16T00:00:00Z', change_set_count: 1, paths: ['chapters/ch01.md'] }] })
      }),
      http.get('/api/projects/project-one/changes/groups/group-1', () => {
        return HttpResponse.json({
          project_id: projectId,
          workspace: '/books/中文作品',
          group: { id: 'group-1', review_status: 'pending', apply_state: 'applied', created_at: '2026-07-16T00:00:00Z', change_sets: [], comments: [] },
        })
      }),
    )

    await expect(listProjectChangeGroups(projectId, {
      status: 'pending',
      runID: 'run-1',
      sessionID: 'session-1',
      reviewThreadID: 'thread-1',
    })).resolves.toEqual([
      expect.objectContaining({ id: 'group-1', paths: ['chapters/ch01.md'] }),
    ])
    await expect(getProjectChangeGroup(projectId, 'group-1')).resolves.toMatchObject({ id: 'group-1', comments: [] })
  })

  it('reads the cumulative review projection from the canonical review_thread envelope', async () => {
    const projectId = 'project-one'
    server.use(
      http.get('/api/projects/project-one/changes/review-threads/thread-1', () => {
        return HttpResponse.json({
          project_id: projectId,
          workspace: '/books/中文作品',
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

    await expect(getProjectChangeReviewThread(projectId, 'thread-1')).resolves.toMatchObject({
      id: 'thread-1',
      files: [{ before_content: '旧', after_content: '新' }],
    })
  })

  it('normalizes null Go slices at the review-thread API boundary', async () => {
    server.use(
      http.get('/api/projects/project-one/changes/review-threads/thread-empty', () => HttpResponse.json({
        project_id: 'project-one',
        workspace: '/books/demo',
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

    await expect(getProjectChangeReviewThread('project-one', 'thread-empty')).resolves.toMatchObject({
      comments: [],
      files: [{ group_ids: [], change_set_ids: [], pending_edit_ids: [] }],
    })
  })

  it('rejects a successful non-API response instead of presenting an empty review', async () => {
    server.use(
      http.get('/api/projects/project-one/changes/review-threads/thread-invalid', () => HttpResponse.text('<!doctype html><title>Denova</title>')),
    )

    await expect(getProjectChangeReviewThread('project-one', 'thread-invalid')).rejects.toThrow()
  })

  it('routes every mutation through stable Project identity and preserves request bodies', async () => {
    const projectId = 'project-one'
    const workspace = '/books/中文作品'
    const requests: Array<{ path: string; body: unknown }> = []
    const record = async (path: string, request: Request) => {
      const body = request.method === 'DELETE' ? undefined : await request.json().catch(() => undefined)
      requests.push({ path, body })
    }
    server.use(
      http.post('/api/projects/project-one/changes/groups/group-1/review', async ({ request }) => {
        await record('/review', request)
        return HttpResponse.json({ project_id: projectId, workspace, group: { id: 'group-1', change_sets: [] }, affected_paths: ['chapters/ch01.md'] })
      }),
      http.post('/api/projects/project-one/changes/groups/group-1/undo', async ({ request }) => {
        await record('/undo', request)
        return HttpResponse.json({ project_id: projectId, workspace, group: { id: 'group-1', change_sets: [] } })
      }),
      http.post('/api/projects/project-one/changes/groups/group-1/redo', async ({ request }) => {
        await record('/redo', request)
        return HttpResponse.json({ project_id: projectId, workspace, group: { id: 'group-1', change_sets: [] } })
      }),
      http.post('/api/projects/project-one/changes/comments', async ({ request }) => {
        await record('/comments', request)
        return HttpResponse.json({ project_id: projectId, workspace, comment: { id: 'comment-1', group_id: 'group-1', body: '调整人称' } }, { status: 201 })
      }),
      http.patch('/api/projects/project-one/changes/comments/comment-1', async ({ request }) => {
        await record('/comment-update', request)
        return HttpResponse.json({ project_id: projectId, workspace, comment: { id: 'comment-1', group_id: 'group-1', body: '更新评论' } })
      }),
      http.delete('/api/projects/project-one/changes/comments/comment-1', async ({ request }) => {
        await record('/comment-delete', request)
        return HttpResponse.json({ project_id: projectId, workspace, comment: { id: 'comment-1', group_id: 'group-1', body: '调整人称', deleted: true } })
      }),
    )

    await reviewProjectChangeGroup(projectId, 'group-1', { decision: 'reject', change_set_id: 'set-1', edit_ids: ['edit-1'], base_revision: 'sha256:current-after' })
    await undoProjectChangeGroup(projectId, 'group-1')
    await redoProjectChangeGroup(projectId, 'group-1')
    await createProjectChangeComment(projectId, {
      group_id: 'group-1',
      change_set_id: 'set-1',
      edit_id: 'edit-1',
      body: '调整人称',
      anchor: { side: 'after', encoding: 'utf8-bytes-v1', revision: 'after', start: 3, end: 7, quote: '😀' },
    })
    await updateProjectChangeComment(projectId, 'comment-1', '更新评论')
    await deleteProjectChangeComment(projectId, 'comment-1')

    expect(requests).toEqual([
      { path: '/review', body: { decision: 'reject', change_set_id: 'set-1', edit_ids: ['edit-1'], base_revision: 'sha256:current-after' } },
      { path: '/undo', body: undefined },
      { path: '/redo', body: undefined },
      { path: '/comments', body: { group_id: 'group-1', change_set_id: 'set-1', edit_id: 'edit-1', body: '调整人称', anchor: { side: 'after', encoding: 'utf8-bytes-v1', revision: 'after', start: 3, end: 7, quote: '😀' } } },
      { path: '/comment-update', body: { body: '更新评论' } },
      { path: '/comment-delete', body: undefined },
    ])
  })
})
