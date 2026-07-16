import type { Monaco, MonacoDiffEditor } from '@monaco-editor/react'
import type { IDisposable, editor } from 'monaco-editor'
import type { ReviewThreadFile, WorkspaceChangeCommentAnchor } from '../../types'
import { ReviewCommentGutter } from './review-comment-gutter'
import { installReviewEditorPointerFocus } from './review-editor-dom'
import { Utf8OffsetIndex } from './utf8-offset-index'

export type ReviewDiffLayout = 'unified' | 'split'

export interface ReviewZoneDescriptor {
  key: string
  side: 'before' | 'after'
  start: number
  end: number
}

export interface ReviewZonePortalTarget extends ReviewZoneDescriptor {
  domNode: HTMLElement
}

/** Compares portal targets without relying on refetch-created object identity. */
export function sameReviewZonePortalTargets(before: ReviewZonePortalTarget[], after: ReviewZonePortalTarget[]): boolean {
  return before.length === after.length && before.every((target, index) => {
    const next = after[index]
    return target.key === next.key
      && target.side === next.side
      && target.start === next.start
      && target.end === next.end
      && target.domNode === next.domNode
  })
}

interface AdapterCallbacks {
  commentLabel: string
  beforeLabel: string
  afterLabel: string
  onCommentRequest: (anchor: WorkspaceChangeCommentAnchor) => void
  onPortalTargetsChange: (targets: ReviewZonePortalTarget[]) => void
}

interface ZoneRecord {
  key: string
  editor: editor.ICodeEditor
  id: string
  zone: editor.IViewZone
  heightTracker?: ReviewZoneHeightTracker
}

interface ReviewZoneHeightTracker {
  dispose: () => void
}

const MAX_QUOTE_BYTES = 16 * 1024
const CONTEXT_BYTES = 256

/** Owns every imperative Monaco resource used by the review surface. */
export class ReviewEditorAdapter {
  private readonly editor: MonacoDiffEditor
  private readonly monaco: Monaco
  private readonly callbacks: AdapterCallbacks
  private readonly disposables: IDisposable[] = []
  private readonly originalDecorations: editor.IEditorDecorationsCollection
  private readonly modifiedDecorations: editor.IEditorDecorationsCollection
  private readonly originalGutter: ReviewCommentGutter
  private readonly modifiedGutter: ReviewCommentGutter
  private zones: ZoneRecord[] = []
  private portalTargets: ReviewZonePortalTarget[] = []
  private file: ReviewThreadFile | null = null
  private layout: ReviewDiffLayout = 'unified'
  private zoneDescriptors: ReviewZoneDescriptor[] = []
  private commentingDisabled = false
  private beforeIndex = new Utf8OffsetIndex('')
  private afterIndex = new Utf8OffsetIndex('')
  private disposed = false

  constructor(editorInstance: MonacoDiffEditor, monaco: Monaco, callbacks: AdapterCallbacks) {
    this.editor = editorInstance
    this.monaco = monaco
    this.callbacks = callbacks
    this.originalDecorations = editorInstance.getOriginalEditor().createDecorationsCollection()
    this.modifiedDecorations = editorInstance.getModifiedEditor().createDecorationsCollection()
    this.originalGutter = new ReviewCommentGutter(editorInstance.getOriginalEditor(), monaco, {
      id: 'split-before',
      labelForLine: (lineNumber) => `${callbacks.commentLabel} · ${callbacks.beforeLabel} · L${lineNumber}`,
      isCommentable: () => this.layout === 'split',
      onRequest: (lineNumber) => this.requestCommentForLine(editorInstance.getOriginalEditor(), 'before', lineNumber),
    })
    this.modifiedGutter = new ReviewCommentGutter(editorInstance.getModifiedEditor(), monaco, {
      id: 'split-after',
      labelForLine: (lineNumber) => `${callbacks.commentLabel} · ${callbacks.afterLabel} · L${lineNumber}`,
      isCommentable: () => this.layout === 'split',
      onRequest: (lineNumber) => this.requestCommentForLine(editorInstance.getModifiedEditor(), 'after', lineNumber),
    })
    this.disposables.push(
      installReviewEditorPointerFocus(editorInstance.getOriginalEditor()),
      installReviewEditorPointerFocus(editorInstance.getModifiedEditor()),
      editorInstance.onDidUpdateDiff(() => this.rebuild()),
      editorInstance.onDidChangeModel(() => this.rebuild()),
      this.installCommentAction(editorInstance.getOriginalEditor(), 'before'),
      this.installCommentAction(editorInstance.getModifiedEditor(), 'after'),
    )
  }

  update(file: ReviewThreadFile, layout: ReviewDiffLayout, zones: ReviewZoneDescriptor[], commentingDisabled = false) {
    if (this.disposed) return
    const contentChanged = this.file?.path !== file.path
      || this.file.base_revision !== file.base_revision
      || this.file.revision !== file.revision
    this.file = file
    this.layout = layout
    this.zoneDescriptors = zones
    this.commentingDisabled = commentingDisabled
    const gutterDisabled = commentingDisabled || layout !== 'split'
    this.originalGutter.updateDisabled(gutterDisabled)
    this.modifiedGutter.updateDisabled(gutterDisabled)
    if (contentChanged) {
      this.beforeIndex = new Utf8OffsetIndex(file.before_content)
      this.afterIndex = new Utf8OffsetIndex(file.after_content)
    }
    this.editor.updateOptions({
      renderSideBySide: layout === 'split',
      useInlineViewWhenSpaceIsLimited: layout !== 'split',
    })
    this.rebuild()
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.originalGutter.dispose()
    this.modifiedGutter.dispose()
    this.clearZones(true)
    this.originalDecorations.clear()
    this.modifiedDecorations.clear()
    for (const disposable of this.disposables.splice(0)) disposable.dispose()
  }

  private installCommentAction(codeEditor: editor.IStandaloneCodeEditor, side: 'before' | 'after'): IDisposable {
    return codeEditor.addAction({
      id: `denova.change-review.comment.${side}`,
      label: this.callbacks.commentLabel,
      keybindings: [this.monaco.KeyMod.CtrlCmd | this.monaco.KeyMod.Shift | this.monaco.KeyCode.KeyM],
      contextMenuGroupId: 'navigation',
      run: () => this.requestCommentForSelection(codeEditor, side),
    })
  }

  private rebuild() {
    if (this.disposed || !this.file) return
    this.rebuildZones()
    this.rebuildDecorations()
  }

  private requestCommentForSelection(codeEditor: editor.IStandaloneCodeEditor, side: 'before' | 'after') {
    if (this.commentingDisabled) return
    const model = codeEditor.getModel()
    if (!model) return
    const selection = codeEditor.getSelection()
    if (!selection || selection.isEmpty()) {
      this.requestCommentForLine(codeEditor, side, codeEditor.getPosition()?.lineNumber ?? 1)
      return
    }
    this.emitAnchor(side, selection.getStartPosition(), selection.getEndPosition())
  }

  private requestCommentForLine(codeEditor: editor.ICodeEditor, side: 'before' | 'after', lineNumber: number) {
    const model = codeEditor.getModel()
    if (!model) return
    const line = clampLine(lineNumber, codeEditor)
    this.emitAnchor(
      side,
      { lineNumber: line, column: 1 },
      { lineNumber: line, column: model.getLineMaxColumn(line) },
    )
  }

  private emitAnchor(side: 'before' | 'after', startPosition: { lineNumber: number; column: number }, endPosition: { lineNumber: number; column: number }) {
    const file = this.file
    if (!file) return
    const index = side === 'before' ? this.beforeIndex : this.afterIndex
    const revision = side === 'before' ? file.base_revision : file.revision
    let start = index.byteOffsetAtPosition(startPosition)
    let end = index.byteOffsetAtPosition(endPosition)
    if (end < start) [start, end] = [end, start]
    const cappedEndOffset = Math.min(end, start + MAX_QUOTE_BYTES)
    const safeEnd = index.byteOffsetAtUtf16Offset(index.utf16OffsetAtByteOffset(cappedEndOffset))
    const prefixStart = index.byteOffsetAtUtf16Offset(index.utf16OffsetAtByteOffset(Math.max(0, start - CONTEXT_BYTES)))
    const suffixEnd = index.byteOffsetAtUtf16Offset(index.utf16OffsetAtByteOffset(Math.min(index.byteLength, safeEnd + CONTEXT_BYTES)))
    this.callbacks.onCommentRequest({
      kind: 'text-range',
      side,
      encoding: 'utf8-bytes-v1',
      revision,
      start,
      end: safeEnd,
      quote: index.sliceBytes(start, safeEnd),
      prefix: index.sliceBytes(prefixStart, start),
      suffix: index.sliceBytes(safeEnd, suffixEnd),
    })
  }

  private rebuildZones() {
    const existingByKey = new Map(this.zones.map((record) => [record.key, record]))
    const nextZones: ZoneRecord[] = []
    const targets: ReviewZonePortalTarget[] = []
    for (const descriptor of this.zoneDescriptors) {
      const location = this.zoneLocation(descriptor)
      if (!location) continue
      const existing = existingByKey.get(descriptor.key)
      existingByKey.delete(descriptor.key)
      if (existing && existing.editor === location.editor && existing.zone.afterLineNumber === location.afterLineNumber) {
        nextZones.push(existing)
        targets.push({ ...descriptor, domNode: existing.zone.domNode as HTMLElement })
        continue
      }
      if (existing) this.removeZone(existing)
      const domNode = (existing?.zone.domNode as HTMLElement | undefined) ?? document.createElement('div')
      domNode.className = 'nova-review-zone-host'
      const zone: editor.IViewZone = {
        afterLineNumber: location.afterLineNumber,
        heightInPx: 112,
        domNode,
        suppressMouseDown: false,
        showInHiddenAreas: true,
      }
      let zoneID = ''
      location.editor.changeViewZones((accessor) => {
        zoneID = accessor.addZone(zone)
      })
      const record: ZoneRecord = { key: descriptor.key, editor: location.editor, id: zoneID, zone }
      record.heightTracker = trackReviewZoneContentHeight(location.editor, zoneID, zone, domNode)
      nextZones.push(record)
      targets.push({ ...descriptor, domNode })
    }
    for (const stale of existingByKey.values()) this.removeZone(stale)
    this.zones = nextZones
    if (!sameReviewZonePortalTargets(this.portalTargets, targets)) {
      this.portalTargets = targets
      this.callbacks.onPortalTargetsChange(targets)
    }
  }

  private clearZones(notify: boolean) {
    for (const record of this.zones.splice(0)) this.removeZone(record)
    if (!notify) return
    this.portalTargets = []
    this.callbacks.onPortalTargetsChange([])
  }

  private removeZone(record: ZoneRecord) {
    record.heightTracker?.dispose()
    record.editor.changeViewZones((accessor) => accessor.removeZone(record.id))
  }

  private zoneLocation(descriptor: ReviewZoneDescriptor): { editor: editor.ICodeEditor; afterLineNumber: number } | null {
    const sourceIndex = descriptor.side === 'before' ? this.beforeIndex : this.afterIndex
    const sourcePosition = sourceIndex.positionAtByteOffset(descriptor.end || descriptor.start)
    if (descriptor.side === 'before' && this.layout === 'split') {
      return {
        editor: this.editor.getOriginalEditor(),
        afterLineNumber: clampLine(sourcePosition.lineNumber, this.editor.getOriginalEditor()),
      }
    }
    const modifiedEditor = this.editor.getModifiedEditor()
    const line = descriptor.side === 'before'
      ? mapOriginalLineToModified(sourcePosition.lineNumber, this.editor.getLineChanges() ?? [])
      : sourcePosition.lineNumber
    return { editor: modifiedEditor, afterLineNumber: clampLine(line, modifiedEditor) }
  }

  private rebuildDecorations() {
    const before: editor.IModelDeltaDecoration[] = []
    const after: editor.IModelDeltaDecoration[] = []
    for (const descriptor of this.zoneDescriptors) {
      const index = descriptor.side === 'before' ? this.beforeIndex : this.afterIndex
      const start = index.positionAtByteOffset(descriptor.start)
      const end = index.positionAtByteOffset(descriptor.end)
      const decoration: editor.IModelDeltaDecoration = {
        range: new this.monaco.Range(
          start.lineNumber,
          start.column,
          end.lineNumber,
          end.lineNumber === start.lineNumber ? Math.max(start.column, end.column) : end.column,
        ),
        options: {
          className: 'nova-review-comment-range',
          isWholeLine: descriptor.start === descriptor.end,
          stickiness: this.monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
        },
      }
      if (descriptor.side === 'before') before.push(decoration)
      else after.push(decoration)
    }
    this.originalDecorations.set(before)
    this.modifiedDecorations.set(after)
  }
}

/** Keeps Monaco's reserved view-zone height equal to the portaled comment card. */
export function trackReviewZoneContentHeight(codeEditor: editor.ICodeEditor, zoneID: string, zone: editor.IViewZone, domNode: HTMLElement): ReviewZoneHeightTracker | undefined {
  if (typeof ResizeObserver === 'undefined') return undefined
  let observedChild: Element | null = null
  const view = domNode.ownerDocument.defaultView
  const syncHeight = () => {
    const child = domNode.firstElementChild as HTMLElement | null
    const hostStyle = view?.getComputedStyle(domNode)
    const childStyle = child ? view?.getComputedStyle(child) : undefined
    const childHeight = child ? Math.max(child.getBoundingClientRect().height, child.scrollHeight) : 0
    const contentHeight = childHeight > 0
      ? childHeight
        + cssPixels(childStyle?.marginTop)
        + cssPixels(childStyle?.marginBottom)
        + cssPixels(hostStyle?.paddingTop)
        + cssPixels(hostStyle?.paddingBottom)
      : domNode.scrollHeight
    const nextHeight = Math.max(64, Math.ceil(contentHeight))
    if (zone.heightInPx === nextHeight) return
    zone.heightInPx = nextHeight
    codeEditor.changeViewZones((accessor) => accessor.layoutZone(zoneID))
  }
  const resizeObserver = new ResizeObserver(syncHeight)
  const observeCurrentChild = () => {
    const child = domNode.firstElementChild
    if (child !== observedChild) {
      if (observedChild) resizeObserver.unobserve(observedChild)
      observedChild = child
      if (child) resizeObserver.observe(child)
    }
    syncHeight()
  }
  resizeObserver.observe(domNode)
  const mutationObserver = typeof MutationObserver === 'undefined' ? undefined : new MutationObserver(observeCurrentChild)
  mutationObserver?.observe(domNode, { childList: true })
  observeCurrentChild()
  return {
    dispose: () => {
      mutationObserver?.disconnect()
      resizeObserver.disconnect()
    },
  }
}

function cssPixels(value: string | undefined): number {
  const parsed = Number.parseFloat(value ?? '')
  return Number.isFinite(parsed) ? parsed : 0
}

export function mapOriginalLineToModified(lineNumber: number, changes: readonly editor.ILineChange[]): number {
  let delta = 0
  for (const change of changes) {
    if (lineNumber < change.originalStartLineNumber) break
    const originalCount = lineSpan(change.originalStartLineNumber, change.originalEndLineNumber)
    const modifiedCount = lineSpan(change.modifiedStartLineNumber, change.modifiedEndLineNumber)
    if (lineNumber <= Math.max(change.originalStartLineNumber, change.originalEndLineNumber)) {
      return Math.max(1, change.modifiedStartLineNumber || change.modifiedEndLineNumber || change.originalStartLineNumber + delta)
    }
    delta += modifiedCount - originalCount
  }
  return Math.max(1, lineNumber + delta)
}

function lineSpan(start: number, end: number): number {
  if (start <= 0 || end <= 0 || end < start) return 0
  return end - start + 1
}

function clampLine(lineNumber: number, codeEditor: editor.ICodeEditor): number {
  const lineCount = codeEditor.getModel()?.getLineCount() ?? 1
  return Math.max(1, Math.min(lineCount, Math.trunc(lineNumber) || 1))
}
