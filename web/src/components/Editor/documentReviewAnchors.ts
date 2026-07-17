import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/react'
import type { DocumentReviewAnchor, DocumentReviewAnchorKind } from '@/features/document-review/types'
import { normalizeEditorText } from './editorDocument'

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

/** Maps a TipTap selection to canonical Markdown bytes without mutating editor state. */
export function createDocumentReviewAnchor(editor: Editor, snapshot: DocumentReviewSnapshot, range: EditorReviewRange): DocumentReviewAnchor {
  const canonical = normalizeEditorText(snapshot.content)
  if (!snapshot.revision.trim()) throw new Error('Document revision is unavailable')

  const startMarker = uniqueMarker(canonical, '\uE000nova-review-start\uE001')
  const endMarker = uniqueMarker(canonical + startMarker, '\uE000nova-review-end\uE001')
  const transaction = editor.state.tr.insertText(endMarker, range.to).insertText(startMarker, range.from)
  if (!editor.markdown) throw new Error('Markdown serialization is unavailable')
  const marked = normalizeEditorText(editor.markdown.serialize(transaction.doc.toJSON()))
  const startMarkerOffset = marked.indexOf(startMarker)
  const endMarkerOffset = marked.indexOf(endMarker)

  let start = -1
  let end = -1
  let quote = ''
  if (startMarkerOffset >= 0 && endMarkerOffset > startMarkerOffset) {
    const withoutMarkers = marked.replace(startMarker, '').replace(endMarker, '')
    if (withoutMarkers === canonical) {
      start = startMarkerOffset
      end = endMarkerOffset - startMarker.length
      quote = withoutMarkers.slice(start, end)
    }
  }

  // Markdown serializers can normalize syntax that was authored differently.
  // A unique visible quote remains safe; ambiguous text is deliberately rejected.
  if (start < 0 || end < start || !quote) {
    const visibleQuote = range.displayQuote
    const first = canonical.indexOf(visibleQuote)
    if (!visibleQuote || first < 0 || canonical.lastIndexOf(visibleQuote) !== first) {
      throw new Error('The selected text cannot be mapped uniquely to Markdown')
    }
    start = first
    end = first + visibleQuote.length
    quote = canonical.slice(start, end)
  }

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

export function topLevelWidgetPosition(doc: ProseMirrorNode, position: number): number {
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
    return { from, to, widgetPos: topLevelWidgetPosition(doc, to), kind: 'text-block', displayQuote }
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
