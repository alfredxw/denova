import type { Monaco, MonacoDiffEditor } from '@monaco-editor/react'
import type { IDisposable, editor } from 'monaco-editor'
import type { ReviewThreadFile, WorkspaceChangeCommentAnchor } from '../../types'
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
  observer?: ResizeObserver
}

interface GlyphRecord {
  editor: editor.ICodeEditor
  widget: editor.IGlyphMarginWidget
  domNode: HTMLButtonElement
  click: (event: MouseEvent) => void
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
  private glyphs: GlyphRecord[] = []
  private zones: ZoneRecord[] = []
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
    this.disposables.push(
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
    this.clearGlyphs()
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
    this.rebuildGlyphs()
    this.rebuildZones()
    this.rebuildDecorations()
  }

  private rebuildGlyphs() {
    this.clearGlyphs()
    const changes = this.editor.getLineChanges() ?? []
    const originalEditor = this.editor.getOriginalEditor()
    const modifiedEditor = this.editor.getModifiedEditor()
    const seen = new Set<string>()

    for (const [index, change] of changes.entries()) {
      const hasOriginalLines = change.originalStartLineNumber > 0 && change.originalEndLineNumber >= change.originalStartLineNumber
      const hasModifiedLines = change.modifiedStartLineNumber > 0 && change.modifiedEndLineNumber >= change.modifiedStartLineNumber
      if (this.layout === 'split' && hasOriginalLines) {
        this.addGlyph(originalEditor, originalEditor, 'before', change.originalStartLineNumber, change.originalStartLineNumber, `before:${index}`, seen)
      }
      const modifiedLine = clampLine(
        change.modifiedStartLineNumber || change.modifiedEndLineNumber || change.originalStartLineNumber,
        modifiedEditor,
      )
      if (this.layout === 'unified' && hasOriginalLines) {
        // Unified Monaco paints deleted source lines into the modified inline
        // surface. The widget lives there, but its authoritative anchor must
        // still be calculated against the original snapshot.
        this.addGlyph(modifiedEditor, originalEditor, 'before', modifiedLine, change.originalStartLineNumber, `before-inline:${index}`, seen)
      }
      if (hasModifiedLines) {
        this.addGlyph(modifiedEditor, modifiedEditor, 'after', modifiedLine, change.modifiedStartLineNumber || modifiedLine, `after:${index}`, seen)
      }
    }
  }

  private addGlyph(
    displayEditor: editor.ICodeEditor,
    anchorEditor: editor.ICodeEditor,
    side: 'before' | 'after',
    displayLineNumber: number,
    anchorLineNumber: number,
    id: string,
    seen: Set<string>,
  ) {
    const displayLine = clampLine(displayLineNumber, displayEditor)
    const anchorLine = clampLine(anchorLineNumber, anchorEditor)
    const dedupeKey = `${side}:${anchorLine}`
    if (seen.has(dedupeKey)) return
    seen.add(dedupeKey)
    const domNode = document.createElement('button')
    domNode.type = 'button'
    domNode.tabIndex = 0
    domNode.className = 'nova-review-glyph-button'
    domNode.dataset.reviewCommentSide = side
    domNode.disabled = this.commentingDisabled
    domNode.setAttribute('aria-disabled', String(this.commentingDisabled))
    const sideLabel = side === 'before' ? this.callbacks.beforeLabel : this.callbacks.afterLabel
    const label = `${this.callbacks.commentLabel} · ${sideLabel}`
    domNode.setAttribute('aria-label', label)
    domNode.title = label
    const icon = document.createElement('span')
    icon.className = 'codicon codicon-add'
    icon.setAttribute('aria-hidden', 'true')
    domNode.append(icon)
    const range = new this.monaco.Range(displayLine, 1, displayLine, 1)
    const widget: editor.IGlyphMarginWidget = {
      getId: () => `denova.change-review.${id}`,
      getDomNode: () => domNode,
      getPosition: () => ({
        lane: side === 'before' ? this.monaco.editor.GlyphMarginLane.Left : this.monaco.editor.GlyphMarginLane.Right,
        range,
        zIndex: 50,
      }),
    }
    const click = (event: MouseEvent) => {
      event.preventDefault()
      event.stopPropagation()
      if (this.commentingDisabled) return
      this.requestCommentForLine(anchorEditor, side, anchorLine)
    }
    domNode.addEventListener('click', click)
    displayEditor.addGlyphMarginWidget(widget)
    this.glyphs.push({ editor: displayEditor, widget, domNode, click })
  }

  private clearGlyphs() {
    for (const glyph of this.glyphs.splice(0)) {
      glyph.domNode.removeEventListener('click', glyph.click)
      glyph.editor.removeGlyphMarginWidget(glyph.widget)
    }
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
    const reusableHosts = new Map(this.zones.map((record) => [record.key, record.zone.domNode as HTMLElement]))
    this.clearZones(false)
    const targets: ReviewZonePortalTarget[] = []
    for (const descriptor of this.zoneDescriptors) {
      const location = this.zoneLocation(descriptor)
      if (!location) continue
      const domNode = reusableHosts.get(descriptor.key) ?? document.createElement('div')
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
      if (typeof ResizeObserver !== 'undefined') {
        record.observer = new ResizeObserver(() => {
          const nextHeight = Math.max(64, Math.ceil(domNode.scrollHeight))
          if (zone.heightInPx === nextHeight) return
          zone.heightInPx = nextHeight
          location.editor.changeViewZones((accessor) => accessor.layoutZone(zoneID))
        })
        record.observer.observe(domNode)
      }
      this.zones.push(record)
      targets.push({ ...descriptor, domNode })
    }
    this.callbacks.onPortalTargetsChange(targets)
  }

  private clearZones(notify: boolean) {
    for (const record of this.zones.splice(0)) {
      record.observer?.disconnect()
      record.editor.changeViewZones((accessor) => accessor.removeZone(record.id))
    }
    if (notify) this.callbacks.onPortalTargetsChange([])
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
