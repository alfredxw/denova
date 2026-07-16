import { describe, expect, it, vi } from 'vitest'
import type { Monaco, MonacoDiffEditor } from '@monaco-editor/react'
import type { editor } from 'monaco-editor'
import type { ReviewThreadFile } from '../../types'
import { mapOriginalLineToModified, ReviewEditorAdapter, trackReviewZoneContentHeight } from './review-editor-adapter'
import { UnifiedReviewEditorAdapter } from './unified-review-editor-adapter'
import { buildUnifiedReviewProjection } from './unified-review-projection'

describe('ReviewEditorAdapter', () => {
  it('offers one hover comment control for every source line in split layout', () => {
    const original = fakeCodeEditor(['kept', '删除 😀', 'tail'])
    const modified = fakeCodeEditor(['kept', '新增 😀', 'tail'])
    const diff = fakeDiffEditor(original, modified, [])
    const onCommentRequest = vi.fn()
    const onPortalTargetsChange = vi.fn()
    const adapter = new ReviewEditorAdapter(diff.value, fakeMonaco(), {
      commentLabel: '评论',
      beforeLabel: '修改前',
      afterLabel: '修改后',
      onCommentRequest,
      onPortalTargetsChange,
    })

    adapter.update(reviewFile({ before_content: 'kept\n删除 😀\ntail', after_content: 'kept\n新增 😀\ntail' }), 'split', [])

    expect(original.widgets).toHaveLength(1)
    expect(modified.widgets).toHaveLength(1)
    expect(original.widgets[0].getDomNode()).toHaveAttribute('data-visible', 'false')
    expect(original.widgets[0].getDomNode()).toHaveAttribute('aria-hidden', 'true')

    original.hover(2)
    expect(original.widgets[0].getDomNode()).toHaveAttribute('data-visible', 'true')
    expect(original.widgets[0].getDomNode()).toHaveAttribute('aria-hidden', 'false')
    original.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    modified.hover(2)
    modified.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))

    expect(onCommentRequest).toHaveBeenNthCalledWith(1, expect.objectContaining({
      side: 'before',
      encoding: 'utf8-bytes-v1',
      revision: 'before-revision',
      quote: '删除 😀',
    }))
    expect(onCommentRequest).toHaveBeenNthCalledWith(2, expect.objectContaining({
      side: 'after',
      revision: 'after-revision',
      quote: '新增 😀',
    }))

    adapter.dispose()
    expect(original.widgets).toHaveLength(0)
    expect(modified.widgets).toHaveLength(0)
    expect(onPortalTargetsChange).toHaveBeenLastCalledWith([])
    expect(diff.disposedSubscriptions).toBe(2)
    expect(original.disposedActions).toBe(1)
    expect(modified.disposedActions).toBe(1)
  })

  it('keeps deleted lines commentable on the original side in split layout', () => {
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
    original.hover(2)
    original.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(original.widgets[0].getDomNode()).toHaveAttribute('data-visible', 'true')
    expect(modified.widgets).toHaveLength(1)
    adapter.dispose()
  })

  it('offers distinct before and after anchors from the single-column unified projection', () => {
    const unified = fakeCodeEditor(['same', 'old line', 'new line', 'tail'])
    const onCommentRequest = vi.fn()
    const adapter = new UnifiedReviewEditorAdapter(unified.value, fakeMonaco(), {
      commentLabel: 'Comment',
      beforeLabel: 'Before',
      afterLabel: 'After',
      onCommentRequest,
      onPortalTargetsChange: vi.fn(),
      onExpandRegion: vi.fn(),
    })
    const file = reviewFile({ before_content: 'same\nold line\ntail', after_content: 'same\nnew line\ntail' })
    const projection = buildUnifiedReviewProjection(file.before_content, file.after_content, { collapsedLabel: (count) => `${count} unchanged` })

    adapter.update(file, projection, [])

    expect(unified.widgets).toHaveLength(1)
    unified.hover(2)
    unified.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    unified.hover(3)
    unified.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onCommentRequest).toHaveBeenNthCalledWith(1, expect.objectContaining({ side: 'before', quote: 'old line' }))
    expect(onCommentRequest).toHaveBeenNthCalledWith(2, expect.objectContaining({ side: 'after', quote: 'new line' }))

    const decorations = unified.decorationSets.at(-1) ?? []
    expect(decorations).toEqual(expect.arrayContaining([
      expect.objectContaining({ options: expect.objectContaining({ marginClassName: 'nova-review-diff-margin-removed' }) }),
      expect.objectContaining({ options: expect.objectContaining({ marginClassName: 'nova-review-diff-margin-added' }) }),
      expect.objectContaining({ options: expect.objectContaining({ className: 'nova-review-diff-word-removed' }) }),
      expect.objectContaining({ options: expect.objectContaining({ className: 'nova-review-diff-word-added' }) }),
    ]))
    expect(decorations).not.toEqual(expect.arrayContaining([
      expect.objectContaining({ options: expect.objectContaining({ inlineClassName: expect.stringContaining('nova-review-diff-word-') }) }),
    ]))
    adapter.dispose()
  })

  it('keeps a split comment zone mounted across equivalent projection refreshes', () => {
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

    adapter.update(reviewFile(), 'split', [descriptor])
    const firstHost = onPortalTargetsChange.mock.calls.at(-1)?.[0][0].domNode
    adapter.update({ ...reviewFile() }, 'split', [{ ...descriptor }])
    const refreshedHost = onPortalTargetsChange.mock.calls.at(-1)?.[0][0].domNode

    expect(refreshedHost).toBe(firstHost)
    expect(modified.removedZoneIDs).toEqual([])
    expect(onPortalTargetsChange).toHaveBeenCalledTimes(1)
    adapter.dispose()
  })

  it('keeps a unified comment zone mounted across equivalent projection refreshes', () => {
    const unified = fakeCodeEditor(['new'])
    const onPortalTargetsChange = vi.fn()
    const adapter = new UnifiedReviewEditorAdapter(unified.value, fakeMonaco(), {
      commentLabel: 'Comment',
      beforeLabel: 'Before',
      afterLabel: 'After',
      onCommentRequest: vi.fn(),
      onPortalTargetsChange,
      onExpandRegion: vi.fn(),
    })
    const file = reviewFile({ before_content: 'old', after_content: 'new' })
    const descriptor = { key: 'comment:after:0:3', side: 'after' as const, start: 0, end: 3 }
    const projection = buildUnifiedReviewProjection(file.before_content, file.after_content, { collapsedLabel: String })

    adapter.update(file, projection, [descriptor])
    const firstHost = onPortalTargetsChange.mock.calls.at(-1)?.[0][0].domNode
    adapter.update({ ...file }, { ...projection }, [{ ...descriptor }])
    const refreshedHost = onPortalTargetsChange.mock.calls.at(-1)?.[0][0].domNode

    expect(refreshedHost).toBe(firstHost)
    expect(unified.removedZoneIDs).toEqual([])
    expect(onPortalTargetsChange).toHaveBeenCalledTimes(1)
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

    adapter.update(reviewFile({ before_content: 'old', after_content: 'new' }), 'split', [], true)
    modified.hover(1)
    expect(modified.widgets.every((widget) => (widget.getDomNode() as HTMLButtonElement).disabled)).toBe(true)
    expect(modified.widgets[0].getDomNode()).toHaveAttribute('data-visible', 'false')
    modified.widgets[0].getDomNode().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onCommentRequest).not.toHaveBeenCalled()
    adapter.dispose()
  })

  it('reserves the portaled card height so an expanding editor cannot cover later diff lines', () => {
    const disconnected = vi.fn()
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      unobserve() {}
      disconnect() { disconnected() }
    })
    try {
      const host = document.createElement('div')
      host.style.paddingTop = '4px'
      host.style.paddingBottom = '4px'
      const card = document.createElement('div')
      card.style.marginTop = '4px'
      card.style.marginBottom = '4px'
      card.getBoundingClientRect = () => ({ height: 140 } as DOMRect)
      Object.defineProperty(card, 'scrollHeight', { configurable: true, value: 140 })
      host.append(card)
      const layoutZone = vi.fn()
      const codeEditor = {
        changeViewZones: (callback: (accessor: editor.IViewZoneChangeAccessor) => void) => callback({ layoutZone } as unknown as editor.IViewZoneChangeAccessor),
      } as unknown as editor.ICodeEditor
      const zone = { afterLineNumber: 1, heightInPx: 112, domNode: host } satisfies editor.IViewZone

      const tracker = trackReviewZoneContentHeight(codeEditor, 'zone-1', zone, host)

      expect(zone.heightInPx).toBe(156)
      expect(layoutZone).toHaveBeenCalledWith('zone-1')
      tracker?.dispose()
      expect(disconnected).toHaveBeenCalledTimes(1)
    } finally {
      vi.unstubAllGlobals()
    }
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
      MouseTargetType: { GUTTER_VIEW_ZONE: 5, CONTENT_TEXT: 6, CONTENT_EMPTY: 7, CONTENT_VIEW_ZONE: 8 },
      TrackedRangeStickiness: { NeverGrowsWhenTypingAtEdges: 1 },
    },
  } as unknown as Monaco
}

type FakeCodeEditor = ReturnType<typeof fakeCodeEditor>

function fakeCodeEditor(lines: string[]) {
  const widgets: editor.IGlyphMarginWidget[] = []
  const zones = new Map<string, editor.IViewZone>()
  const removedZoneIDs: string[] = []
  const mouseMoveListeners: Array<(event: editor.IEditorMouseEvent) => void> = []
  const mouseLeaveListeners: Array<() => void> = []
  const mouseDownListeners: Array<(event: editor.IEditorMouseEvent) => void> = []
  const decorationSets: editor.IModelDeltaDecoration[][] = []
  let disposedActions = 0
  let zoneSequence = 0
  const subscribe = <Listener,>(listeners: Listener[], listener: Listener) => {
    listeners.push(listener)
    return { dispose: () => {
      const index = listeners.indexOf(listener)
      if (index >= 0) listeners.splice(index, 1)
    } }
  }
  const value = {
    addAction: () => ({ dispose: () => { disposedActions += 1 } }),
    addGlyphMarginWidget: (widget: editor.IGlyphMarginWidget) => { widgets.push(widget) },
    removeGlyphMarginWidget: (widget: editor.IGlyphMarginWidget) => {
      const index = widgets.indexOf(widget)
      if (index >= 0) widgets.splice(index, 1)
    },
    layoutGlyphMarginWidget: vi.fn(),
    onMouseMove: (listener: (event: editor.IEditorMouseEvent) => void) => subscribe(mouseMoveListeners, listener),
    onMouseLeave: (listener: () => void) => subscribe(mouseLeaveListeners, listener),
    onMouseDown: (listener: (event: editor.IEditorMouseEvent) => void) => subscribe(mouseDownListeners, listener),
    onDidChangeModel: () => ({ dispose: vi.fn() }),
    onDidChangeModelContent: () => ({ dispose: vi.fn() }),
    createDecorationsCollection: () => ({
      set: (decorations: editor.IModelDeltaDecoration[]) => { decorationSets.push(decorations) },
      clear: vi.fn(),
      length: 0,
      getRanges: () => [],
    }),
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
    decorationSets,
    hover: (lineNumber: number) => {
      const event = {
        target: {
          type: 6,
          position: { lineNumber, column: 1 },
        },
      } as editor.IEditorMouseEvent
      for (const listener of mouseMoveListeners) listener(event)
    },
    leave: () => {
      for (const listener of mouseLeaveListeners) listener()
    },
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
