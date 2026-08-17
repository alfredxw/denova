import { Node, type JSONContent } from '@tiptap/core'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/react'

export const RAW_MARKDOWN_NODE = 'rawMarkdown'

/**
 * Literal Markdown source hosted by ProseMirror.
 *
 * It deliberately is not TipTap's normal code-block node: source mode must not
 * add Markdown fences or exit into rich paragraphs after repeated Enter presses.
 */
export const RawMarkdown = Node.create({
  name: RAW_MARKDOWN_NODE,
  group: 'block',
  content: 'text*',
  marks: '',
  code: true,
  defining: true,
  isolating: true,
  parseHTML() {
    return [{ tag: 'pre[data-nova-raw-markdown]', preserveWhitespace: 'full' }]
  },
  renderHTML() {
    return [
      'pre',
      { 'data-nova-raw-markdown': 'true', spellcheck: 'false' },
      ['code', 0],
    ]
  },
  addKeyboardShortcuts() {
    return {
      Tab: () => this.editor.commands.insertContent('\t'),
    }
  },
})

/** Creates a schema-valid source document without parsing Markdown syntax. */
export function createMarkdownSourceDocument(value: string): JSONContent {
  const source = normalizeSourceLineEndings(value)
  return {
    type: 'doc',
    content: [{
      type: RAW_MARKDOWN_NODE,
      ...(source ? { content: [{ type: 'text', text: source }] } : {}),
    }],
  }
}

/** Returns the literal source and its first ProseMirror text position. */
export function readMarkdownSourceDocument(
  value: Editor | ProseMirrorNode,
): { content: string; contentStart: number } | null {
  const document = 'state' in value ? value.state.doc : value
  let result: { content: string; contentStart: number } | null = null
  document.descendants((node, position) => {
    if (result || node.type.name !== RAW_MARKDOWN_NODE) return
    result = { content: node.textContent, contentStart: position + 1 }
    return false
  })
  return result
}

function normalizeSourceLineEndings(value: string): string {
  return value.replace(/\r\n?/g, '\n')
}
