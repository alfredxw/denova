import { act, createElement } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReviewThreadFile, WorkspaceChangeCommentAnchor } from '../types'

const adapterHarness = vi.hoisted(() => ({
  callbacks: null as null | { onCommentRequest: (anchor: WorkspaceChangeCommentAnchor) => void },
  hosts: new Map<string, HTMLElement>(),
  updates: [] as Array<{ commentingDisabled: boolean; zoneKeys: string[] }>,
}))

vi.mock('@monaco-editor/react', async () => {
  const React = await import('react')
  const editor = {
    getContentHeight: () => 200,
    onDidContentSizeChange: () => ({ dispose: vi.fn() }),
  }
  const monaco = { editor: {} }
  const Editor = ({ onMount }: { onMount?: (editor: unknown, monaco: unknown) => void }) => {
    React.useEffect(() => { onMount?.(editor, monaco) }, [onMount])
    return React.createElement('div', { 'data-testid': 'mock-monaco-editor' })
  }
  return { Editor, DiffEditor: Editor }
})

vi.mock('./monaco/review-model-lifecycle', () => ({ scheduleDetachedReviewModelDisposal: vi.fn() }))
vi.mock('./monaco/review-monaco-theme', () => ({
  installReviewMonacoThemes: vi.fn(),
  REVIEW_MONACO_THEME_DARK: 'dark',
  REVIEW_MONACO_THEME_LIGHT: 'light',
}))
vi.mock('./monaco/review-editor-adapter', () => ({
  ReviewEditorAdapter: class {
    update() {}
    dispose() {}
  },
}))
vi.mock('./monaco/unified-review-editor-adapter', () => ({
  UnifiedReviewEditorAdapter: class {
    private callbacks: { onCommentRequest: (anchor: WorkspaceChangeCommentAnchor) => void; onPortalTargetsChange: (targets: unknown[]) => void }

    constructor(_editor: unknown, _monaco: unknown, callbacks: { onCommentRequest: (anchor: WorkspaceChangeCommentAnchor) => void; onPortalTargetsChange: (targets: unknown[]) => void }) {
      this.callbacks = callbacks
      adapterHarness.callbacks = callbacks
    }

    update(_file: unknown, _projection: unknown, zones: Array<{ key: string; side: 'before' | 'after'; start: number; end: number }>, commentingDisabled: boolean) {
      adapterHarness.updates.push({ commentingDisabled, zoneKeys: zones.map((zone) => zone.key) })
      this.callbacks.onPortalTargetsChange(zones.map((zone) => {
        let domNode = adapterHarness.hosts.get(zone.key)
        if (!domNode) {
          domNode = document.createElement('div')
          adapterHarness.hosts.set(zone.key, domNode)
          document.body.append(domNode)
        }
        return { ...zone, domNode }
      }))
    }

    dispose() {}
  },
}))

import { ReviewDiffEditor, reviewCommentTarget } from './ReviewDiffEditor'

afterEach(() => {
  adapterHarness.callbacks = null
  adapterHarness.updates = []
  for (const host of adapterHarness.hosts.values()) host.remove()
  adapterHarness.hosts.clear()
})

describe('reviewCommentTarget', () => {
  it('binds cumulative before and after comments to their owning change sets', () => {
    const file = {
      base_group_id: 'group-1',
      base_change_set_id: 'set-1',
      latest_group_id: 'group-2',
      latest_change_set_id: 'set-2',
    } as ReviewThreadFile

    expect(reviewCommentTarget(file, 'before')).toEqual({ group_id: 'group-1', change_set_id: 'set-1' })
    expect(reviewCommentTarget(file, 'after')).toEqual({ group_id: 'group-2', change_set_id: 'set-2' })
  })
})

describe('ReviewDiffEditor comment drafts', () => {
  it('keeps drafts on different lines independent and leaves submitted comments editable', async () => {
    const file = reviewFile()
    render(createElement(ReviewDiffEditor, {
      threadID: 'thread-1',
      file,
      comments: [{
        id: 'comment-1',
        group_id: 'group-1',
        change_set_id: 'set-1',
        body: '已提交评论',
        anchor: anchor(file, 12, 17, 'third'),
      }],
      layout: 'unified',
      onCreateComment: vi.fn().mockResolvedValue(undefined),
      onUpdateComment: vi.fn().mockResolvedValue(undefined),
      onResolveComment: vi.fn().mockResolvedValue(undefined),
      onDeleteComment: vi.fn().mockResolvedValue(undefined),
    }))

    await waitFor(() => expect(adapterHarness.callbacks).not.toBeNull())
    act(() => adapterHarness.callbacks?.onCommentRequest(anchor(file, 0, 5, 'first')))
    const firstDraft = await screen.findByRole('textbox')
    fireEvent.change(firstDraft, { target: { value: '第一条草稿' } })

    act(() => adapterHarness.callbacks?.onCommentRequest(anchor(file, 6, 12, 'second')))
    await waitFor(() => expect(screen.getAllByRole('textbox')).toHaveLength(2))
    expect(screen.getAllByRole('textbox')[0]).toHaveValue('第一条草稿')
    expect(adapterHarness.updates.at(-1)).toMatchObject({ commentingDisabled: false })

    fireEvent.click(screen.getByRole('button', { name: '修改评论' }))
    expect(screen.getAllByRole('textbox')).toHaveLength(3)
  })
})

function reviewFile(): ReviewThreadFile {
  return {
    path: 'chapters/ch01.md',
    before_content: 'old\n',
    after_content: 'first\nsecond\nthird\n',
    base_revision: 'before-revision',
    revision: 'after-revision',
    base_group_id: 'group-1',
    base_change_set_id: 'set-1',
    latest_group_id: 'group-1',
    latest_change_set_id: 'set-1',
    group_ids: ['group-1'],
    change_set_ids: ['set-1'],
    pending_edit_ids: ['edit-1'],
    review_status: 'pending',
    apply_state: 'applied',
    continuity: 'continuous',
  }
}

function anchor(file: ReviewThreadFile, start: number, end: number, quote: string): WorkspaceChangeCommentAnchor {
  return {
    kind: 'text-range',
    side: 'after',
    encoding: 'utf8-bytes-v1',
    revision: file.revision,
    start,
    end,
    quote,
  }
}
