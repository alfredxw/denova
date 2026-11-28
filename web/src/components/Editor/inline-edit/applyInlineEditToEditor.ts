import type { JSONContent } from '@tiptap/core'
import type { Editor } from '@tiptap/react'

export type InlineEditApplyAction = 'replace' | 'insert_below'

export interface InlineEditTargetRange {
  from: number
  to: number
}

function hasBlockMarkdownStructure(text: string): boolean {
  if (/\n\s*\n/.test(text)) return true
  const lines = text.split('\n')
  if (lines.length > 1) return true
  const trimmed = text.trimStart()
  return /^(?:#{1,6}\s|>\s|[-*+]\s|\d+\.\s|```|---+$|\*\*\*+$)/.test(trimmed)
}

function extractInlineContentFromParsedDoc(parsed: JSONContent): JSONContent[] | null {
  if (parsed.type !== 'doc' || !Array.isArray(parsed.content) || parsed.content.length !== 1) {
    return null
  }
  const onlyBlock = parsed.content[0]
  if (onlyBlock?.type !== 'paragraph') return null
  return Array.isArray(onlyBlock.content) ? onlyBlock.content : []
}

function resolveContainingBlockEnd(editor: Editor, pos: number): number {
  const { doc } = editor.state
  const clamped = Math.max(0, Math.min(pos, doc.content.size))
  const resolved = doc.resolve(clamped)
  if (resolved.depth >= 1) {
    return resolved.after(1)
  }
  return clamped
}

function buildBlockNodes(editor: Editor, text: string, isMarkdown: boolean): JSONContent[] {
  const normalized = text.replace(/\r\n/g, '\n')
  if (isMarkdown && editor.markdown) {
    const parsed = editor.markdown.parse(normalized)
    if (parsed?.type === 'doc' && Array.isArray(parsed.content) && parsed.content.length > 0) {
      return parsed.content
    }
  }
  return normalized.split(/\n{2,}/).map((para) => {
    const lines = para.split('\n')
    const content: JSONContent[] = []
    lines.forEach((line, index) => {
      if (index > 0) content.push({ type: 'hardBreak' })
      if (line) content.push({ type: 'text', text: line })
    })
    return content.length > 0 ? { type: 'paragraph', content } : { type: 'paragraph' }
  })
}

/**
 * Applies an accepted inline AI edit to TipTap as one atomic transaction so
 * native Cmd/Ctrl+Z reverts the entire edit in a single undo step.
 */
export function applyInlineEditToTipTap(
  editor: Editor,
  range: InlineEditTargetRange,
  text: string,
  action: InlineEditApplyAction,
  isMarkdown: boolean,
): boolean {
  if (editor.isDestroyed) return false
  const normalized = text.replace(/\r\n/g, '\n')
  if (!normalized.trim()) return false

  const maxPos = editor.state.doc.content.size
  const from = Math.max(0, Math.min(range.from, maxPos))
  const to = Math.max(from, Math.min(range.to, maxPos))

  if (action === 'insert_below') {
    const insertPos = resolveContainingBlockEnd(editor, to)
    const nodes = buildBlockNodes(editor, normalized, isMarkdown)
    return editor
      .chain()
      .focus()
      .insertContentAt(insertPos, nodes, { updateSelection: true })
      .run()
  }

  const sameParentBlock = editor.state.doc.resolve(from).sameParent(editor.state.doc.resolve(to))
  if (sameParentBlock && !hasBlockMarkdownStructure(normalized)) {
    if (isMarkdown && editor.markdown) {
      const parsed = editor.markdown.parse(normalized)
      const inlineNodes = extractInlineContentFromParsedDoc(parsed)
      if (inlineNodes && inlineNodes.length > 0) {
        return editor
          .chain()
          .focus()
          .insertContentAt({ from, to }, inlineNodes, { updateSelection: true })
          .run()
      }
    }
    return editor
      .chain()
      .focus()
      .insertContentAt({ from, to }, { type: 'text', text: normalized }, { updateSelection: true })
      .run()
  }

  const nodes = buildBlockNodes(editor, normalized, isMarkdown)
  return editor
    .chain()
    .focus()
    .insertContentAt({ from, to }, nodes, { updateSelection: true })
    .run()
}

/**
 * Extracts bounded surrounding context (`before`, `selection`, `after`) around
 * a ProseMirror range without serializing the full document on every trigger.
 */
export function extractTipTapInlineEditContext(
  editor: Editor,
  range: InlineEditTargetRange,
  beforeBudget: number,
  afterBudget: number,
): { before: string; selection: string; after: string } {
  const { doc } = editor.state
  const maxPos = doc.content.size
  const from = Math.max(0, Math.min(range.from, maxPos))
  const to = Math.max(from, Math.min(range.to, maxPos))

  const beforeStart = Math.max(0, from - beforeBudget * 2)
  const afterEnd = Math.min(maxPos, to + afterBudget * 2)

  const beforeRaw = doc.textBetween(beforeStart, from, '\n\n')
  const selection = from < to ? doc.textBetween(from, to, '\n\n') : ''
  const afterRaw = doc.textBetween(to, afterEnd, '\n\n')

  return {
    before: beforeRaw.slice(-beforeBudget),
    selection,
    after: afterRaw.slice(0, afterBudget),
  }
}
