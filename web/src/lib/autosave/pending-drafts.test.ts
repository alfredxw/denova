import { beforeEach, describe, expect, it } from 'vitest'
import {
  listPendingDrafts,
  pendingDraftsForWorkspace,
  queuePendingDraft,
  removePendingDraft,
  removePendingDraftsForFile,
} from './pending-drafts'

describe('pending drafts', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('queues a draft bound to workspace, file, and base revision', () => {
    const list = queuePendingDraft({
      workspace: '/projects/lost-garden',
      path: 'chapters/ch01.md',
      content: '断线时写下的内容',
      baseContent: '基线内容',
      baseRevision: 'sha256:base',
      mode: 'auto',
    })

    expect(list).toHaveLength(1)
    const draft = list[0]
    expect(draft.workspace).toBe('/projects/lost-garden')
    expect(draft.path).toBe('chapters/ch01.md')
    expect(draft.content).toBe('断线时写下的内容')
    expect(draft.baseContent).toBe('基线内容')
    expect(draft.baseRevision).toBe('sha256:base')
    expect(draft.id).toBeTruthy()
    expect(draft.queuedAt).toBeTruthy()
    expect(listPendingDrafts()).toEqual([draft])
  })

  it('replaces an older pending draft for the same workspace and file', () => {
    queuePendingDraft({
      workspace: '/projects/lost-garden',
      path: 'chapters/ch01.md',
      content: 'first',
      baseContent: 'base',
      baseRevision: 'sha256:base',
      mode: 'auto',
    })
    const list = queuePendingDraft({
      workspace: '/projects/lost-garden',
      path: 'chapters/ch01.md',
      content: 'second',
      baseContent: 'base',
      baseRevision: 'sha256:newer',
      mode: 'manual',
    })

    expect(list).toHaveLength(1)
    expect(list[0].content).toBe('second')
    expect(list[0].baseRevision).toBe('sha256:newer')
    expect(list[0].mode).toBe('manual')
  })

  it('drops drafts whose content exceeds the per-draft byte limit', () => {
    const list = queuePendingDraft({
      workspace: '/projects/lost-garden',
      path: 'chapters/ch01.md',
      content: '界'.repeat(300_000),
      baseContent: 'base',
      baseRevision: 'sha256:base',
      mode: 'auto',
    })

    expect(list).toEqual([])
    expect(listPendingDrafts()).toEqual([])
  })

  it('ignores corrupted or incomplete entries when reading storage', () => {
    localStorage.setItem('nova:pending-drafts:v1', JSON.stringify([
      { workspace: '/projects/lost-garden', path: 'chapters/ch01.md', content: 'ok', baseContent: 'base', baseRevision: 'sha256:a', mode: 'auto', id: 'd-1', queuedAt: '2026-08-04T00:00:00Z' },
      { workspace: '/projects/other', path: 'missing-base.md', content: 'no base', baseRevision: 'sha256:b', mode: 'auto', id: 'd-2', queuedAt: '2026-08-04T00:00:01Z' },
      { workspace: '/projects/other', path: 'missing-content.md', baseContent: 'base', baseRevision: 'sha256:b', mode: 'auto', id: 'd-3', queuedAt: '2026-08-04T00:00:01Z' },
      'not-an-object',
    ]))

    const drafts = listPendingDrafts()
    expect(drafts).toHaveLength(1)
    expect(drafts[0].id).toBe('d-1')
  })

  it('removes a pending draft by id', () => {
    queuePendingDraft({
      workspace: '/projects/lost-garden',
      path: 'chapters/ch01.md',
      content: 'first',
      baseContent: 'base',
      baseRevision: 'sha256:base',
      mode: 'auto',
    })
    const [draft] = listPendingDrafts()

    const remaining = removePendingDraft(draft.id)
    expect(remaining).toEqual([])
    expect(pendingDraftsForWorkspace('/projects/lost-garden')).toEqual([])
  })

  it('removes all pending drafts for one workspace file', () => {
    queuePendingDraft({
      workspace: '/projects/lost-garden',
      path: 'chapters/ch01.md',
      content: 'first',
      baseContent: 'base',
      baseRevision: 'sha256:base',
      mode: 'auto',
    })
    queuePendingDraft({
      workspace: '/projects/lost-garden',
      path: 'chapters/ch02.md',
      content: 'second',
      baseContent: 'base',
      baseRevision: 'sha256:base',
      mode: 'auto',
    })

    const remaining = removePendingDraftsForFile('/projects/lost-garden', 'chapters/ch01.md')
    expect(remaining.map((draft) => draft.path)).toEqual(['chapters/ch02.md'])
    expect(pendingDraftsForWorkspace('/projects/lost-garden').map((draft) => draft.path)).toEqual(['chapters/ch02.md'])
  })
})
