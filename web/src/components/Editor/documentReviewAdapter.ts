import type { Editor } from '@tiptap/react'
import type { DocumentReviewAnchor } from '@/features/document-review/types'
import {
  captureDocumentReviewSelection,
  commentWidgetPosition,
  createDocumentReviewAnchor,
  createRawDocumentReviewAnchor,
  resolveRawDocumentReviewAnchor,
  type DocumentReviewSelectionSnapshot,
  type DocumentReviewSnapshot,
  type EditorReviewRange,
} from './documentReviewAnchors'
import { readMarkdownSourceDocument } from './markdownSourceDocument'

type FrozenReviewSelection =
  | { mode: 'rich'; selection: DocumentReviewSelectionSnapshot }
  | {
      mode: 'source'
      content: string
      start: number
      end: number
      range: EditorReviewRange
    }

/** Maps one TipTap document representation onto canonical Markdown review anchors. */
export interface DocumentReviewAnchorAdapter {
  coordinateSpace: 'rich-markdown-v1' | 'source-markdown-v1'
  captureSelection: (editor: Editor, range: EditorReviewRange) => FrozenReviewSelection
  createAnchor: (
    editor: Editor,
    snapshot: DocumentReviewSnapshot,
    selection: FrozenReviewSelection,
  ) => DocumentReviewAnchor
  resolveRange: (editor: Editor, anchor: DocumentReviewAnchor) => EditorReviewRange | null
}

export const richMarkdownReviewAdapter: DocumentReviewAnchorAdapter = {
  coordinateSpace: 'rich-markdown-v1',
  captureSelection: (editor, range) => ({
    mode: 'rich',
    selection: captureDocumentReviewSelection(editor, range),
  }),
  createAnchor: (editor, snapshot, frozen) => {
    if (frozen.mode !== 'rich') throw new Error('The review selection belongs to a different editor mode')
    return createDocumentReviewAnchor(editor, snapshot, frozen.selection)
  },
  resolveRange: (editor, anchor) => {
    const from = anchor.editor_from ?? 0
    const to = anchor.editor_to ?? 0
    const displayQuote = anchor.display_quote || anchor.quote
    if (from > 0 && to > from && to <= editor.state.doc.content.size
      && editor.state.doc.textBetween(from, to, '\n').trim() === displayQuote.trim()) {
      return reviewRange(editor, anchor, from, to)
    }
    return resolveUniqueRichTextRange(editor, anchor, displayQuote)
  },
}

export const sourceMarkdownReviewAdapter: DocumentReviewAnchorAdapter = {
  coordinateSpace: 'source-markdown-v1',
  captureSelection: (editor, range) => {
    const source = readMarkdownSourceDocument(editor)
    if (!source) throw new Error('Markdown source document is unavailable')
    const start = range.from - source.contentStart
    const end = range.to - source.contentStart
    if (start < 0 || end <= start || end > source.content.length) {
      throw new Error('The selected source range is invalid')
    }
    return { mode: 'source', content: source.content, start, end, range }
  },
  createAnchor: (_editor, snapshot, frozen) => {
    if (frozen.mode !== 'source') throw new Error('The review selection belongs to a different editor mode')
    const anchor = createRawDocumentReviewAnchor(snapshot, frozen)
    return {
      ...anchor,
      kind: frozen.range.kind,
      editor_from: frozen.range.from,
      editor_to: frozen.range.to,
    }
  },
  resolveRange: (editor, anchor) => {
    const source = readMarkdownSourceDocument(editor)
    if (!source) return null
    const resolved = resolveRawDocumentReviewAnchor(source.content, anchor)
    if (!resolved) return null
    const from = source.contentStart + resolved.start
    const to = source.contentStart + resolved.end
    if (from <= 0 || to <= from || to > editor.state.doc.content.size) return null
    return reviewRange(editor, anchor, from, to)
  },
}

function resolveUniqueRichTextRange(
  editor: Editor,
  anchor: DocumentReviewAnchor,
  displayQuote: string,
): EditorReviewRange | null {
  if (!editor.markdown || anchor.quote !== displayQuote || !displayQuote) return null
  const source = readMarkdownSourceDocument(editor)?.content ?? editor.getMarkdown()
  const sourceRange = resolveRawDocumentReviewAnchor(source, anchor)
  if (!sourceRange
    || source.indexOf(displayQuote) !== sourceRange.start
    || source.lastIndexOf(displayQuote) !== sourceRange.start) return null

  const matches: Array<{ from: number; to: number }> = []
  editor.state.doc.descendants((node, position) => {
    if (matches.length > 1 || !node.isText || !node.text) return
    let offset = node.text.indexOf(displayQuote)
    while (offset >= 0) {
      matches.push({ from: position + offset, to: position + offset + displayQuote.length })
      if (matches.length > 1) return
      offset = node.text.indexOf(displayQuote, offset + 1)
    }
  })
  return matches.length === 1
    ? reviewRange(editor, anchor, matches[0].from, matches[0].to)
    : null
}

function reviewRange(
  editor: Editor,
  anchor: DocumentReviewAnchor,
  from: number,
  to: number,
): EditorReviewRange {
  return {
    from,
    to,
    widgetPos: commentWidgetPosition(editor.state.doc, to),
    kind: anchor.kind,
    displayQuote: anchor.display_quote || anchor.quote,
  }
}
