import type { Monaco } from '@monaco-editor/react'
import type { IDisposable, editor } from 'monaco-editor'
import type { ReviewThreadFile, WorkspaceChangeCommentAnchor } from '../../types'
import { sameReviewZonePortalTargets, trackReviewZoneContentHeight, type ReviewZoneDescriptor, type ReviewZonePortalTarget } from './review-editor-adapter'
import { ReviewCommentGutter } from './review-comment-gutter'
import { installReviewEditorPointerFocus } from './review-editor-dom'
import type { UnifiedReviewLine, UnifiedReviewProjection } from './unified-review-projection'
import { Utf8OffsetIndex } from './utf8-offset-index'

interface UnifiedAdapterCallbacks {
  commentLabel: string
  beforeLabel: string
  afterLabel: string
  onCommentRequest: (anchor: WorkspaceChangeCommentAnchor) => void
  onPortalTargetsChange: (targets: ReviewZonePortalTarget[]) => void
  onExpandRegion: (collapseID: string) => void
}

interface ZoneRecord {
  key: string
  id: string
  zone: editor.IViewZone
  heightTracker?: { dispose: () => void }
}

const MAX_QUOTE_BYTES = 16 * 1024
const CONTEXT_BYTES = 256

/** Owns the single-gutter unified Monaco model and maps its rows back to source snapshots. */
export class UnifiedReviewEditorAdapter {
  private readonly editor: editor.IStandaloneCodeEditor
  private readonly monaco: Monaco
  private readonly callbacks: UnifiedAdapterCallbacks
  private readonly disposables: IDisposable[] = []
  private readonly decorations: editor.IEditorDecorationsCollection
  private readonly gutter: ReviewCommentGutter
  private zones: ZoneRecord[] = []
  private portalTargets: ReviewZonePortalTarget[] = []
  private file: ReviewThreadFile | null = null
  private projection: UnifiedReviewProjection = { value: '', lines: [] }
  private zoneDescriptors: ReviewZoneDescriptor[] = []
  private beforeIndex = new Utf8OffsetIndex('')
  private afterIndex = new Utf8OffsetIndex('')
  private disposed = false

  constructor(editorInstance: editor.IStandaloneCodeEditor, monaco: Monaco, callbacks: UnifiedAdapterCallbacks) {
    this.editor = editorInstance
    this.monaco = monaco
    this.callbacks = callbacks
    this.decorations = editorInstance.createDecorationsCollection()
    this.gutter = new ReviewCommentGutter(editorInstance, monaco, {
      id: 'unified',
      labelForLine: (lineNumber) => this.commentLabelForLine(lineNumber),
      isCommentable: (lineNumber) => Boolean(this.projection.lines[lineNumber - 1]?.commentSide),
      onRequest: (lineNumber) => this.requestCommentForLine(lineNumber),
    })
    this.disposables.push(
      installReviewEditorPointerFocus(editorInstance),
      editorInstance.onDidChangeModel(() => this.rebuild()),
      editorInstance.onDidChangeModelContent(() => this.rebuild()),
      editorInstance.onMouseDown((event) => {
        const line = event.target.position?.lineNumber
        const collapseID = line ? this.projection.lines[line - 1]?.collapseID : undefined
        if (collapseID) callbacks.onExpandRegion(collapseID)
      }),
      editorInstance.addAction({
        id: 'denova.change-review.comment.unified',
        label: callbacks.commentLabel,
        keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyM],
        contextMenuGroupId: 'navigation',
        run: () => this.requestCommentForLine(editorInstance.getPosition()?.lineNumber ?? 1),
      }),
    )
  }

  update(file: ReviewThreadFile, projection: UnifiedReviewProjection, zones: ReviewZoneDescriptor[], commentingDisabled = false) {
    if (this.disposed) return
    const contentChanged = this.file?.path !== file.path
      || this.file.base_revision !== file.base_revision
      || this.file.revision !== file.revision
    this.file = file
    this.projection = projection
    this.zoneDescriptors = zones
    if (contentChanged) {
      this.beforeIndex = new Utf8OffsetIndex(file.before_content)
      this.afterIndex = new Utf8OffsetIndex(file.after_content)
    }
    this.gutter.updateDisabled(commentingDisabled)
    this.rebuild()
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.gutter.dispose()
    this.clearZones(true)
    this.decorations.clear()
    for (const disposable of this.disposables.splice(0)) disposable.dispose()
  }

  private rebuild() {
    if (this.disposed || !this.file) return
    this.rebuildZones()
    this.rebuildDecorations()
  }

  private requestCommentForLine(displayLineNumber: number) {
    const row = this.projection.lines[displayLineNumber - 1]
    const side = row?.commentSide
    const sourceLineNumber = row?.sourceLineNumber
    const model = this.editor.getModel()
    if (!side || !sourceLineNumber || !model) return
    this.emitAnchor(
      side,
      { lineNumber: sourceLineNumber, column: 1 },
      { lineNumber: sourceLineNumber, column: model.getLineMaxColumn(displayLineNumber) },
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
      const afterLineNumber = this.displayLineForDescriptor(descriptor)
      if (!afterLineNumber) continue
      const existing = existingByKey.get(descriptor.key)
      existingByKey.delete(descriptor.key)
      if (existing && existing.zone.afterLineNumber === afterLineNumber) {
        nextZones.push(existing)
        targets.push({ ...descriptor, domNode: existing.zone.domNode as HTMLElement })
        continue
      }
      if (existing) this.removeZone(existing)
      const domNode = (existing?.zone.domNode as HTMLElement | undefined) ?? document.createElement('div')
      domNode.className = 'nova-review-zone-host'
      const zone: editor.IViewZone = {
        afterLineNumber,
        heightInPx: 112,
        domNode,
        suppressMouseDown: false,
        showInHiddenAreas: true,
      }
      let zoneID = ''
      this.editor.changeViewZones((accessor) => { zoneID = accessor.addZone(zone) })
      const record: ZoneRecord = { key: descriptor.key, id: zoneID, zone }
      record.heightTracker = trackReviewZoneContentHeight(this.editor, zoneID, zone, domNode)
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
    this.editor.changeViewZones((accessor) => accessor.removeZone(record.id))
  }

  private rebuildDecorations() {
    const decorations: editor.IModelDeltaDecoration[] = []
    this.projection.lines.forEach((line, index) => {
      const lineNumber = index + 1
      decorations.push({
        range: new this.monaco.Range(lineNumber, 1, lineNumber, 1),
        options: decorationForLine(line),
      })
      const wordClassName = line.kind === 'removed'
        ? 'nova-review-diff-word-removed'
        : line.kind === 'added'
          ? 'nova-review-diff-word-added'
          : null
      if (!wordClassName) return
      for (const range of line.wordDiffs ?? []) {
        decorations.push({
          range: new this.monaco.Range(lineNumber, range.startColumn, lineNumber, range.endColumn),
          options: {
            className: wordClassName,
            stickiness: this.monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
            zIndex: 10,
          },
        })
      }
    })
    for (const descriptor of this.zoneDescriptors) {
      const index = descriptor.side === 'before' ? this.beforeIndex : this.afterIndex
      const start = index.positionAtByteOffset(descriptor.start)
      const end = index.positionAtByteOffset(descriptor.end)
      const displayStart = this.displayLine(descriptor.side, start.lineNumber)
      const displayEnd = this.displayLine(descriptor.side, end.lineNumber)
      if (!displayStart || !displayEnd) continue
      decorations.push({
        range: new this.monaco.Range(displayStart, start.column, displayEnd, end.column),
        options: {
          className: 'nova-review-comment-range',
          isWholeLine: descriptor.start === descriptor.end,
          stickiness: this.monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
          zIndex: 20,
        },
      })
    }
    this.decorations.set(decorations)
  }

  private displayLineForDescriptor(descriptor: ReviewZoneDescriptor): number | undefined {
    const index = descriptor.side === 'before' ? this.beforeIndex : this.afterIndex
    const sourceLine = index.positionAtByteOffset(descriptor.end || descriptor.start).lineNumber
    return this.displayLine(descriptor.side, sourceLine)
  }

  private displayLine(side: 'before' | 'after', sourceLine: number): number | undefined {
    const index = this.projection.lines.findIndex((line) => (
      side === 'before' ? line.beforeLineNumber === sourceLine : line.afterLineNumber === sourceLine
    ))
    return index >= 0 ? index + 1 : undefined
  }

  private commentLabelForLine(displayLineNumber: number): string {
    const row = this.projection.lines[displayLineNumber - 1]
    const sideLabel = row?.commentSide === 'before' ? this.callbacks.beforeLabel : this.callbacks.afterLabel
    return `${this.callbacks.commentLabel} · ${sideLabel} · L${row?.sourceLineNumber ?? displayLineNumber}`
  }
}

function decorationForLine(line: UnifiedReviewLine): editor.IModelDecorationOptions {
  switch (line.kind) {
    case 'added':
      return {
        isWholeLine: true,
        className: 'nova-review-diff-line-added',
        lineNumberClassName: 'nova-review-diff-number-added',
        marginClassName: 'nova-review-diff-margin-added',
      }
    case 'removed':
      return {
        isWholeLine: true,
        className: 'nova-review-diff-line-removed',
        lineNumberClassName: 'nova-review-diff-number-removed',
        marginClassName: 'nova-review-diff-margin-removed',
      }
    case 'collapsed':
      return { isWholeLine: true, className: 'nova-review-diff-line-collapsed', lineNumberClassName: 'nova-review-diff-number-collapsed' }
    case 'unchanged':
      return { isWholeLine: true }
  }
}
