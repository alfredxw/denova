import { Node, mergeAttributes, type Content, type JSONContent } from '@tiptap/core'
import Image from '@tiptap/extension-image'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { EditorState, Selection } from '@tiptap/pm/state'
import type { EditorView } from '@tiptap/pm/view'
import type { Editor } from '@tiptap/react'

interface ParsedMarkdownDocumentEntry {
  source: string
  document: JSONContent
}

/**
 * Small per-editor LRU for Markdown parser output. Source equality is intentional: revisions may
 * change after a save without changing the document, while watcher/Agent updates with new content
 * must never reuse a stale parsed tree.
 */
export class ParsedMarkdownDocumentCache {
  private readonly entries = new Map<string, ParsedMarkdownDocumentEntry>()
  private readonly maxEntries: number

  constructor(maxEntries = 12) {
    if (!Number.isInteger(maxEntries) || maxEntries < 1) {
      throw new RangeError('Parsed Markdown cache size must be a positive integer')
    }
    this.maxEntries = maxEntries
  }

  get(key: string, source: string): JSONContent | undefined {
    const entry = this.entries.get(key)
    if (!entry || entry.source !== source) return undefined
    this.entries.delete(key)
    this.entries.set(key, entry)
    return entry.document
  }

  set(key: string, source: string, document: JSONContent) {
    this.entries.delete(key)
    this.entries.set(key, { source, document })
    while (this.entries.size > this.maxEntries) {
      const oldestKey = this.entries.keys().next().value
      if (oldestKey === undefined) break
      this.entries.delete(oldestKey)
    }
  }
}

/** 检测文本是否已自带缩进（首个非空行以全角/半角空格开头）。 */
export function hasNativeIndent(text: string): boolean {
  const lines = text.split('\n')
  for (const line of lines) {
    if (!line.trim()) continue
    return /^[\s\u3000]{2,}/.test(line)
  }
  return false
}

/** 判断文件是否为纯文本（.txt）格式。 */
export function isTxtFile(name: string | null): boolean {
  return !!name && name.toLowerCase().endsWith('.txt')
}

export function isMarkdownFile(name: string | null): boolean {
  return !!name && /\.(md|markdown)$/i.test(name)
}

export function createWorkspaceImageExtension(resolveAsset: (path: string) => string) {
  return Image.extend({
    renderHTML({ HTMLAttributes }) {
      const src = resolveWorkspaceImageSrc(HTMLAttributes.src, resolveAsset)
      return ['img', mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, { src })]
    },
  }).configure({
    inline: false,
    allowBase64: true,
  })
}

/**
 * Treat a copied workspace image reference as Markdown even when the clipboard
 * also contains a rich-text <pre> fragment from a chat code block.
 */
export function insertPastedWorkspaceMarkdownImage(view: EditorView, event: ClipboardEvent): boolean {
  let markdown = event.clipboardData?.getData('text/plain').trim() || ''
  const fenced = markdown.match(/^```(?:markdown|md)?\s*\r?\n([\s\S]*?)\r?\n```$/i)
  if (fenced) markdown = fenced[1].trim()

  const match = markdown.match(/^!\[((?:\\.|[^\]])*)\]\((assets\/[^\s)]*)\)$/)
  if (!match) return false
  const imageType = view.state.schema.nodes.image
  if (!imageType) return false

  const alt = match[1].replace(/\\([\\\]])/g, '$1')
  const image = imageType.create({ src: match[2], alt, title: alt || undefined })
  event.preventDefault()
  view.dispatch(view.state.tr.replaceSelectionWith(image).scrollIntoView())
  return true
}

/** 与 StarterKit 中禁用的 hardBreak 对应，保留创作模式的段首缩进渲染。 */
export function createIndentedHardBreakExtension() {
  return Node.create({
    name: 'hardBreak',
    inline: true,
    group: 'inline',
    selectable: false,
    linebreakReplacement: true,
    parseHTML() {
      return [{ tag: 'br' }]
    },
    renderHTML() {
      return ['span', { class: 'nova-hard-break' }, ['br']]
    },
    addKeyboardShortcuts() {
      return {
        'Shift-Enter': () => {
          if (!this.editor || this.editor.isDestroyed) return false
          return this.editor.commands.setHardBreak()
        },
      }
    },
    addCommands() {
      return {
        setHardBreak: () => ({ commands }) => {
          return commands.first([
            () => commands.exitCode(),
            () => commands.insertContent({ type: this.name }),
          ])
        },
      }
    },
  })
}

function resolveWorkspaceImageSrc(src: unknown, resolveAsset: (path: string) => string) {
  if (typeof src !== 'string' || src.trim() === '') return src
  const value = src.trim()
  if (/^(https?:|data:|blob:|\/)/i.test(value)) return value
  if (value.startsWith('assets/')) return resolveAsset(value)
  return value
}

export function readEditorText(editor: Editor, fileName: string | null): string {
  return isTxtFile(fileName)
    ? normalizeEditorText(editor.getText({ blockSeparator: '\n' }))
    : normalizeEditorText(editor.getMarkdown())
}

/** 文件切换后重建编辑器状态，避免上一个文件的 Ctrl-Z 历史跨文件生效。 */
export function resetEditorStateHistory(editor: Editor) {
  const state = editor.state
  editor.view.updateState(EditorState.create({
    schema: state.schema,
    doc: state.doc,
    selection: state.selection,
    plugins: state.plugins,
  }))
}

/**
 * Replaces a whole file and clears every plugin's per-document state in one view update. This is
 * equivalent to setContent followed by resetEditorStateHistory, but avoids rendering the same
 * replacement document twice during chapter navigation.
 */
export function replaceEditorDocumentWithFreshState(editor: Editor, document: ProseMirrorNode) {
  const state = editor.state
  const replacement = state.tr.replaceWith(0, state.doc.content.size, document)
  editor.view.updateState(EditorState.create({
    schema: state.schema,
    doc: replacement.doc,
    selection: replacement.selection,
    plugins: state.plugins,
  }))
}

interface ReplaceEditorDocumentOptions {
  contentType: 'html' | 'markdown' | 'json'
  preserveSelection: boolean
}

/**
 * Replaces an editor document without adding an undo step. Same-document refreshes retain the
 * current caret/selection, while callers can opt out when switching to another document.
 */
export function replaceEditorDocument(
  editor: Editor,
  content: Content,
  { contentType, preserveSelection }: ReplaceEditorDocumentOptions,
) {
  const previousSelection = preserveSelection
    ? { from: editor.state.selection.from, to: editor.state.selection.to }
    : null
  const restoreFocus = preserveSelection && editor.view.hasFocus()

  editor.chain()
    .setMeta('addToHistory', false)
    .setContent(content, { emitUpdate: false, contentType })
    .run()

  if (!previousSelection) return
  const maxPosition = Math.max(1, editor.state.doc.content.size - 1)
  const from = Math.min(Math.max(1, previousSelection.from), maxPosition)
  const to = Math.min(Math.max(from, previousSelection.to), maxPosition)
  editor.commands.setTextSelection({ from, to })
  if (restoreFocus) editor.commands.focus()
}

/** Places a collapsed caret at ProseMirror's click position without disturbing range selection. */
export function placeEditorCaretAtClick(view: EditorView, position: number, event?: MouseEvent) {
  if (!view.state.selection.empty || event?.shiftKey || event?.metaKey || event?.ctrlKey || event?.altKey) return false
  const boundedPosition = Math.min(Math.max(0, position), view.state.doc.content.size)
  const selection = Selection.near(view.state.doc.resolve(boundedPosition))
  view.dispatch(view.state.tr.setSelection(selection))
  view.focus()
  return true
}

export function normalizeEditorText(text: string): string {
  return text
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((line) => line.trimEnd())
    .join('\n')
    .replace(/\n{4,}/g, '\n\n\n')
    .trimEnd()
    .concat('\n')
}

export function updateCharacterStats(editor: Editor, setSelected: (value: number) => void) {
  const { from, to, empty } = editor.state.selection
  if (empty) {
    setSelected(0)
    return
  }
  setSelected(countTextCharacters(editor.state.doc.textBetween(from, to, '\n')))
}

export function countTextCharacters(text: string) {
  return Array.from(text.replace(/\s/g, '')).length
}

/** 计算文档中某位置对应的行号（从 1 开始）。 */
export function getLineNumber(doc: ProseMirrorNode, pos: number): number {
  let line = 1
  doc.forEach((node, nodeOffset) => {
    if (nodeOffset + node.nodeSize <= pos) {
      line++
    }
  })
  return line
}
