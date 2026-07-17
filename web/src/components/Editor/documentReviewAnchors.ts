import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { EditorState } from '@tiptap/pm/state'
import type { Editor } from '@tiptap/react'
import { diffChars } from 'diff'
import type { DocumentReviewAnchor, DocumentReviewAnchorKind } from '@/features/document-review/types'

export interface EditorReviewRange {
  from: number
  to: number
  widgetPos: number
  kind: DocumentReviewAnchorKind
  displayQuote: string
}

export interface DocumentReviewSnapshot {
  content: string
  revision: string
}

/** Freezes the exact editor document that owns a review selection before saving starts. */
export interface DocumentReviewSelectionSnapshot {
  document: ProseMirrorNode
  range: EditorReviewRange
}

export function captureDocumentReviewSelection(editor: Editor, range: EditorReviewRange): DocumentReviewSelectionSnapshot {
  return { document: editor.state.doc, range }
}

/** Maps a frozen TipTap selection to exact bytes in the revision-bound Markdown source. */
export function createDocumentReviewAnchor(
  editor: Editor,
  snapshot: DocumentReviewSnapshot,
  selection: DocumentReviewSelectionSnapshot,
): DocumentReviewAnchor {
  const canonical = snapshot.content
  const { document, range } = selection
  if (!snapshot.revision.trim()) throw new Error('Document revision is unavailable')
  if (!editor.markdown) throw new Error('Markdown serialization is unavailable')

  const canonicalDocument = editor.schema.nodeFromJSON(editor.markdown.parse(canonical))
  if (!reviewDocumentsEquivalent(canonicalDocument, document)) {
    throw new Error('The editor and workspace snapshots differ')
  }

  const startMarker = uniqueMarker(canonical, '\uE000nova-review-start\uE001')
  const endMarker = uniqueMarker(canonical + startMarker, '\uE000nova-review-end\uE001')
  const transaction = EditorState.create({ doc: document }).tr
    .insertText(endMarker, range.to)
    .insertText(startMarker, range.from)
  const marked = editor.markdown.serialize(transaction.doc.toJSON())
  const startMarkerOffset = marked.indexOf(startMarker)
  const endMarkerOffset = marked.indexOf(endMarker)
  if (startMarkerOffset < 0 || endMarkerOffset <= startMarkerOffset) {
    throw new Error('The selected text could not be serialized with stable boundaries')
  }

  const serialized = marked.slice(0, startMarkerOffset)
    + marked.slice(startMarkerOffset + startMarker.length, endMarkerOffset)
    + marked.slice(endMarkerOffset + endMarker.length)
  const serializedStart = startMarkerOffset
  const serializedEnd = endMarkerOffset - startMarker.length
  const mapped = mapAlignedRange(serialized, canonical, serializedStart, serializedEnd)
  if (!mapped || !canonicalRangeMatchesEditor(editor, canonical, mapped.start, mapped.end, startMarker, endMarker, range)) {
    throw new Error('The selected text cannot be mapped safely to canonical Markdown')
  }

  const { start, end } = mapped
  const quote = canonical.slice(start, end)
  if (!quote) throw new Error('The selected text maps to an empty Markdown range')

  const byteStart = utf8Bytes(canonical.slice(0, start))
  const byteEnd = byteStart + utf8Bytes(quote)
  return {
    kind: range.kind,
    encoding: 'utf8-bytes-v1',
    revision: snapshot.revision,
    start: byteStart,
    end: byteEnd,
    quote,
    prefix: boundedSuffix(canonical.slice(0, start)),
    suffix: boundedPrefix(canonical.slice(end)),
    display_quote: range.displayQuote,
    editor_from: range.from,
    editor_to: range.to,
  }
}

interface AlignmentSegment {
  sourceStart: number
  sourceEnd: number
  targetStart: number
  targetEnd: number
  equal: boolean
}

function reviewDocumentsEquivalent(left: ProseMirrorNode, right: ProseMirrorNode): boolean {
  if (!left.sameMarkup(right)) return false
  const leftChildren = reviewContentChildCount(left)
  const rightChildren = reviewContentChildCount(right)
  if (leftChildren !== rightChildren) return false
  for (let index = 0; index < leftChildren; index += 1) {
    if (!left.child(index).eq(right.child(index))) return false
  }
  return true
}

function reviewContentChildCount(document: ProseMirrorNode): number {
  let count = document.childCount
  while (count > 0) {
    const node = document.child(count - 1)
    if (node.type.name !== 'paragraph' || node.content.size > 0) break
    count -= 1
  }
  return count
}

function mapAlignedRange(source: string, target: string, start: number, end: number): { start: number; end: number } | null {
  if (start < 0 || end < start || end > source.length) return null
  const segments: AlignmentSegment[] = []
  const changes = diffChars(source, target)
  let sourceOffset = 0
  let targetOffset = 0
  for (let index = 0; index < changes.length;) {
    const change = changes[index]
    if (!change.added && !change.removed) {
      segments.push({
        sourceStart: sourceOffset,
        sourceEnd: sourceOffset + change.value.length,
        targetStart: targetOffset,
        targetEnd: targetOffset + change.value.length,
        equal: true,
      })
      sourceOffset += change.value.length
      targetOffset += change.value.length
      index += 1
      continue
    }

    const sourceStart = sourceOffset
    const targetStart = targetOffset
    while (index < changes.length && (changes[index].added || changes[index].removed)) {
      const edited = changes[index]
      if (edited.removed) sourceOffset += edited.value.length
      if (edited.added) targetOffset += edited.value.length
      index += 1
    }
    segments.push({ sourceStart, sourceEnd: sourceOffset, targetStart, targetEnd: targetOffset, equal: false })
  }

  const mappedStart = mapAlignedBoundary(segments, start, 'start', source.length, target.length)
  const mappedEnd = mapAlignedBoundary(segments, end, 'end', source.length, target.length)
  return mappedStart !== null && mappedEnd !== null && mappedEnd >= mappedStart
    ? { start: mappedStart, end: mappedEnd }
    : null
}

function mapAlignedBoundary(
  segments: AlignmentSegment[],
  offset: number,
  side: 'start' | 'end',
  sourceLength: number,
  targetLength: number,
): number | null {
  for (const segment of segments) {
    if (!segment.equal) continue
    const inside = side === 'start'
      ? offset >= segment.sourceStart && (offset < segment.sourceEnd || offset === sourceLength)
      : offset <= segment.sourceEnd && (offset > segment.sourceStart || offset === 0)
    if (inside) return segment.targetStart + offset - segment.sourceStart
  }

  const edited = segments.find((segment) => !segment.equal && offset >= segment.sourceStart && offset <= segment.sourceEnd)
  if (!edited) return offset === sourceLength ? targetLength : null
  const sourceSpan = edited.sourceEnd - edited.sourceStart
  const targetSpan = edited.targetEnd - edited.targetStart
  if (sourceSpan === 0) return side === 'start' ? edited.targetEnd : edited.targetStart
  if (offset === edited.sourceStart) return edited.targetStart
  if (offset === edited.sourceEnd) return edited.targetEnd
  return edited.targetStart + Math.round(((offset - edited.sourceStart) / sourceSpan) * targetSpan)
}

function canonicalRangeMatchesEditor(
  editor: Editor,
  canonical: string,
  start: number,
  end: number,
  startMarker: string,
  endMarker: string,
  range: EditorReviewRange,
): boolean {
  const markdown = editor.markdown
  if (!markdown) return false
  const markedCanonical = canonical.slice(0, start)
    + startMarker
    + canonical.slice(start, end)
    + endMarker
    + canonical.slice(end)
  const markedDocument = editor.schema.nodeFromJSON(markdown.parse(markedCanonical))
  const startPosition = textPosition(markedDocument, startMarker)
  const endPosition = textPosition(markedDocument, endMarker)
  return startPosition === range.from && endPosition === range.to + startMarker.length
}

function textPosition(document: ProseMirrorNode, text: string): number {
  let result = -1
  document.descendants((node, position) => {
    if (result >= 0 || !node.isText || !node.text) return
    const offset = node.text.indexOf(text)
    if (offset >= 0) result = position + offset
  })
  return result
}

/** Places a comment after its text block while keeping block widgets out of inline DOM. */
export function commentWidgetPosition(doc: ProseMirrorNode, position: number): number {
  const nestedTextBlockPosition = nestedCommentWidgetPosition(doc, position)
  if (nestedTextBlockPosition !== null) return nestedTextBlockPosition

  let result = doc.content.size
  let found = false
  doc.forEach((node, offset) => {
    if (!found && position <= offset + node.nodeSize) {
      result = offset + node.nodeSize
      found = true
    }
  })
  return Math.max(0, Math.min(doc.content.size, result))
}

export function textBlockRangeAtPosition(doc: ProseMirrorNode, position: number): EditorReviewRange | null {
  const safePosition = Math.max(0, Math.min(doc.content.size, position))
  const resolved = doc.resolve(safePosition)
  for (let depth = resolved.depth; depth > 0; depth -= 1) {
    const node = resolved.node(depth)
    if (!node.isTextblock) continue
    const from = resolved.start(depth)
    const to = resolved.end(depth)
    const displayQuote = node.textBetween(0, node.content.size, '\n').trim()
    if (!displayQuote || to <= from) return null
    return { from, to, widgetPos: commentWidgetPosition(doc, to), kind: 'text-block', displayQuote }
  }
  return null
}

function nestedCommentWidgetPosition(doc: ProseMirrorNode, position: number): number | null {
  const safePosition = Math.max(0, Math.min(doc.content.size, position))
  const candidates = safePosition > 0 ? [safePosition, safePosition - 1] : [safePosition]
  for (const candidate of candidates) {
    const resolved = doc.resolve(candidate)
    for (let depth = resolved.depth; depth > 0; depth -= 1) {
      if (!resolved.node(depth).isTextblock || depth < 2) continue
      const parentName = resolved.node(depth - 1).type.name
      if (parentName !== 'listItem' && parentName !== 'taskItem' && parentName !== 'tableCell' && parentName !== 'tableHeader') return null
      return Math.max(0, Math.min(doc.content.size, resolved.after(depth)))
    }
  }
  return null
}

function uniqueMarker(content: string, base: string): string {
  let marker = base
  while (content.includes(marker)) marker += '\uE002'
  return marker
}

function utf8Bytes(value: string): number {
  return new TextEncoder().encode(value).length
}

function boundedPrefix(value: string): string {
  return Array.from(value).slice(0, 128).join('')
}

function boundedSuffix(value: string): string {
  return Array.from(value).slice(-128).join('')
}
