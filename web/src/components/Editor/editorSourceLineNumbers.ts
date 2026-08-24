import { Extension, type JSONContent } from '@tiptap/core'
import type { MarkdownManager } from '@tiptap/markdown'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

export const sourceLineNumbersPluginKey = new PluginKey<DecorationSet>('nova-source-line-numbers')

/**
 * Marks each rendered top-level block with its start line in the canonical Markdown file.
 * TipTap joins those blocks with two newlines, so counting the renderer output preserves
 * blank source lines instead of exposing ProseMirror's internal block index.
 */
export function createSourceLineNumberDecorations(doc: ProseMirrorNode, markdown: MarkdownManager) {
  const decorations: Decoration[] = []
  const documentJSON = doc.toJSON()
  let sourceLine = 1

  doc.forEach((node, offset, index) => {
    decorations.push(Decoration.node(offset, offset + node.nodeSize, {
      'data-source-line': String(sourceLine),
    }, { sourceLine }))

    const rendered = markdown.renderNodeToMarkdown(node.toJSON(), documentJSON as JSONContent, index)
    sourceLine += countLineBreaks(rendered) + 2
  })

  return DecorationSet.create(doc, decorations)
}

/** Keeps source-backed line metadata synchronized without changing the Markdown document. */
export function createSourceLineNumberExtension() {
  return Extension.create({
    name: 'novaSourceLineNumbers',

    addProseMirrorPlugins() {
      const markdown = this.editor.markdown
      if (!markdown) throw new Error('Source line numbers require the Markdown extension')

      return [
        new Plugin<DecorationSet>({
          key: sourceLineNumbersPluginKey,
          state: {
            init: (_, state) => createSourceLineNumberDecorations(state.doc, markdown),
            apply: (transaction, decorations, _oldState, newState) => (
              transaction.docChanged
                ? createSourceLineNumberDecorations(newState.doc, markdown)
                : decorations.map(transaction.mapping, transaction.doc)
            ),
          },
          props: {
            decorations: (state) => sourceLineNumbersPluginKey.getState(state) ?? DecorationSet.empty,
          },
        }),
      ]
    },
  })
}

function countLineBreaks(value: string): number {
  return value.match(/\n/g)?.length ?? 0
}
