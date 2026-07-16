import { describe, expect, it, vi } from 'vitest'
import type { Monaco, MonacoDiffEditor } from '@monaco-editor/react'
import type { editor } from 'monaco-editor'
import type { ReviewThreadFile } from '../../types'
import { mapOriginalLineToModified, ReviewEditorAdapter } from './review-editor-adapter'

describe('ReviewEditorAdapter', () => {
  it('keeps a unified pure-deletion glyph anchored to the before snapshot and disposes resources', () => {
    const original = fakeCodeEditor(['kept', '删除 😀', 'tail'])
    const modified = fakeCodeEditor(['kept', 'tail'])
    const lineChange = {
      originalStartLineNumber: 2,
      originalEndLineNumber: 2,
      modifiedStartLineNumber: 2,
      modifiedEndLineNumber: 0,
      charChanges: undefined,
    } satisfies editor.ILineChange
    const diff = fakeDiffEditor(original, modified, [lineChange])
    const onCommentRequest = vi.fn()
    const onPortalTargetsChange = vi.fn()
    const adapter = new ReviewEditorAdapter(diff.value, fakeMonaco(), {
      commentLabel: '评论',
      beforeLabel: '修改前',
      afterLabel: '修改后',
      onCommentRequest,
      onPortalTargetsChange,
    })

    adapter.update(reviewFile({ before_content: 'kept\n删除 😀\ntail', after_content: 'kept\ntail' }), 'unified', [{ key: 'zone', side: 'before', start: 5, end: 16 }])

    expect(original.widgets).toHaveLength(0)
    expect(modified.widgets).toHaveLength(1)
    modified.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onCommentRequest).toHaveBeenCalledWith(expect.objectContaining({
      side: 'before',
      encoding: 'utf8-bytes-v1',
      revision: 'before-revision',
      quote: '删除 😀',
    }))

    adapter.dispose()
    expect(modified.widgets).toHaveLength(0)
    // Unified before-side threads render their zone in the modified inline
    // surface while the persisted anchor remains on the original snapshot.
    expect(modified.removedZoneIDs).toHaveLength(1)
    expect(onPortalTargetsChange).toHaveBeenLastCalledWith([])
    expect(diff.disposedSubscriptions).toBe(2)
    expect(original.disposedActions).toBe(1)
    expect(modified.disposedActions).toBe(1)
  })

  it('puts a pure-deletion glyph only on the original side in split layout', () => {
    const original = fakeCodeEditor(['one', 'deleted'])
    const modified = fakeCodeEditor(['one'])
    const diff = fakeDiffEditor(original, modified, [{
      originalStartLineNumber: 2,
      originalEndLineNumber: 2,
      modifiedStartLineNumber: 2,
      modifiedEndLineNumber: 0,
      charChanges: undefined,
    }])
    const adapter = new ReviewEditorAdapter(diff.value, fakeMonaco(), {
      commentLabel: 'Comment',
      beforeLabel: 'Before',
      afterLabel: 'After',
      onCommentRequest: vi.fn(),
      onPortalTargetsChange: vi.fn(),
    })

    adapter.update(reviewFile({ before_content: 'one\ndeleted', after_content: 'one' }), 'split', [])
    expect(diff.value.updateOptions).toHaveBeenLastCalledWith({
      renderSideBySide: true,
      useInlineViewWhenSpaceIsLimited: false,
    })
    expect(original.widgets).toHaveLength(1)
    expect(modified.widgets).toHaveLength(0)
    adapter.dispose()
  })

  it('offers distinct before and after anchors for replacements in unified layout', () => {
    const original = fakeCodeEditor(['old line'])
    const modified = fakeCodeEditor(['new line'])
    const diff = fakeDiffEditor(original, modified, [{
      originalStartLineNumber: 1,
      originalEndLineNumber: 1,
      modifiedStartLineNumber: 1,
      modifiedEndLineNumber: 1,
      charChanges: undefined,
    }])
    const onCommentRequest = vi.fn()
    const adapter = new ReviewEditorAdapter(diff.value, fakeMonaco(), {
      commentLabel: 'Comment',
      beforeLabel: 'Before',
      afterLabel: 'After',
      onCommentRequest,
      onPortalTargetsChange: vi.fn(),
    })

    adapter.update(reviewFile({ before_content: 'old line', after_content: 'new line' }), 'unified', [])

    expect(modified.widgets).toHaveLength(2)
    const before = modified.widgets.find((widget) => widget.getDomNode().dataset.reviewCommentSide === 'before')
    const after = modified.widgets.find((widget) => widget.getDomNode().dataset.reviewCommentSide === 'after')
    before?.getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    after?.getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onCommentRequest).toHaveBeenNthCalledWith(1, expect.objectContaining({ side: 'before', quote: 'old line' }))
    expect(onCommentRequest).toHaveBeenNthCalledWith(2, expect.objectContaining({ side: 'after', quote: 'new line' }))
    adapter.dispose()
  })

  it('reuses a comment zone host across projection refreshes', () => {
    const original = fakeCodeEditor(['old'])
    const modified = fakeCodeEditor(['new'])
    const diff = fakeDiffEditor(original, modified, [])
    const onPortalTargetsChange = vi.fn()
    const adapter = new ReviewEditorAdapter(diff.value, fakeMonaco(), {
      commentLabel: 'Comment',
      beforeLabel: 'Before',
      afterLabel: 'After',
      onCommentRequest: vi.fn(),
      onPortalTargetsChange,
    })
    const descriptor = { key: 'comment:after:0:3', side: 'after' as const, start: 0, end: 3 }

    adapter.update(reviewFile(), 'unified', [descriptor])
    const firstHost = onPortalTargetsChange.mock.calls.at(-1)?.[0][0].domNode
    adapter.update({ ...reviewFile() }, 'unified', [{ ...descriptor }])
    const refreshedHost = onPortalTargetsChange.mock.calls.at(-1)?.[0][0].domNode

    expect(refreshedHost).toBe(firstHost)
    adapter.dispose()
  })

  it('disables glyph and keyboard comment creation while mutations are locked', () => {
    const original = fakeCodeEditor(['old'])
    const modified = fakeCodeEditor(['new'])
    const diff = fakeDiffEditor(original, modified, [{
      originalStartLineNumber: 1,
      originalEndLineNumber: 1,
      modifiedStartLineNumber: 1,
      modifiedEndLineNumber: 1,
      charChanges: undefined,
    }])
    const onCommentRequest = vi.fn()
    const adapter = new ReviewEditorAdapter(diff.value, fakeMonaco(), {
      commentLabel: 'Comment',
      beforeLabel: 'Before',
      afterLabel: 'After',
      onCommentRequest,
      onPortalTargetsChange: vi.fn(),
    })

    adapter.update(reviewFile({ before_content: 'old', after_content: 'new' }), 'unified', [], true)
    expect(modified.widgets.every((widget) => (widget.getDomNode() as HTMLButtonElement).disabled)).toBe(true)
    modified.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onCommentRequest).not.toHaveBeenCalled()
    adapter.dispose()
  })
})

describe('mapOriginalLineToModified', () => {
  it('applies prior line deltas without mapping through an unrelated hunk', () => {
    const changes = [{
      originalStartLineNumber: 2,
      originalEndLineNumber: 2,
      modifiedStartLineNumber: 2,
      modifiedEndLineNumber: 4,
      charChanges: undefined,
    }] satisfies editor.ILineChange[]
    expect(mapOriginalLineToModified(1, changes)).toBe(1)
    expect(mapOriginalLineToModified(2, changes)).toBe(2)
    expect(mapOriginalLineToModified(8, changes)).toBe(10)
  })
})

function reviewFile(overrides: Partial<ReviewThreadFile> = {}): ReviewThreadFile {
  return {
    path: 'chapters/ch01.md',
    before_content: 'before',
    after_content: 'after',
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
    ...overrides,
  }
}

function fakeMonaco(): Monaco {
  class Range {
    startLineNumber: number
    startColumn: number
    endLineNumber: number
    endColumn: number

    constructor(
      startLineNumber: number,
      startColumn: number,
      endLineNumber: number,
      endColumn: number,
    ) {
      this.startLineNumber = startLineNumber
      this.startColumn = startColumn
      this.endLineNumber = endLineNumber
      this.endColumn = endColumn
    }
  }
  return {
    Range,
    KeyMod: { CtrlCmd: 1, Shift: 2 },
    KeyCode: { KeyM: 4 },
    editor: {
      GlyphMarginLane: { Left: 1, Center: 2, Right: 3 },
      TrackedRangeStickiness: { NeverGrowsWhenTypingAtEdges: 1 },
    },
  } as unknown as Monaco
}

type FakeCodeEditor = ReturnType<typeof fakeCodeEditor>

function fakeCodeEditor(lines: string[]) {
  const widgets: editor.IGlyphMarginWidget[] = []
  const zones = new Map<string, editor.IViewZone>()
  const removedZoneIDs: string[] = []
  let disposedActions = 0
  let zoneSequence = 0
  const value = {
    addAction: () => ({ dispose: () => { disposedActions += 1 } }),
    addGlyphMarginWidget: (widget: editor.IGlyphMarginWidget) => { widgets.push(widget) },
    removeGlyphMarginWidget: (widget: editor.IGlyphMarginWidget) => {
      const index = widgets.indexOf(widget)
      if (index >= 0) widgets.splice(index, 1)
    },
    createDecorationsCollection: () => ({ set: vi.fn(), clear: vi.fn(), length: 0, getRanges: () => [] }),
    changeViewZones: (callback: (accessor: editor.IViewZoneChangeAccessor) => void) => callback({
      addZone: (zone) => {
        const id = `zone-${++zoneSequence}`
        zones.set(id, zone)
        return id
      },
      removeZone: (id) => { zones.delete(id); removedZoneIDs.push(id) },
      layoutZone: vi.fn(),
    }),
    getModel: () => ({
      getLineCount: () => lines.length,
      getLineMaxColumn: (lineNumber: number) => (lines[lineNumber - 1] ?? '').length + 1,
    }),
    getSelection: () => null,
    getPosition: () => ({ lineNumber: 1, column: 1 }),
  } as unknown as editor.IStandaloneCodeEditor
  return {
    value,
    widgets,
    zones,
    removedZoneIDs,
    get disposedActions() { return disposedActions },
  }
}

function fakeDiffEditor(original: FakeCodeEditor, modified: FakeCodeEditor, lineChanges: editor.ILineChange[]) {
  let disposedSubscriptions = 0
  const subscription = () => ({ dispose: () => { disposedSubscriptions += 1 } })
  const value = {
    getOriginalEditor: () => original.value,
    getModifiedEditor: () => modified.value,
    getLineChanges: () => lineChanges,
    onDidUpdateDiff: subscription,
    onDidChangeModel: subscription,
    updateOptions: vi.fn(),
  } as unknown as MonacoDiffEditor
  return { value, get disposedSubscriptions() { return disposedSubscriptions } }
}
