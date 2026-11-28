import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

export interface InlineEditDecorationRange {
  from: number
  to: number
}

export const inlineEditPluginKey = new PluginKey<InlineEditDecorationRange | null>('nova-inline-edit-range')

/**
 * Non-destructive highlight for the locked inline-edit target range or insertion
 * caret. Never mutates document content or undo history while previewing.
 */
export function createInlineEditDecorationExtension() {
  return Extension.create({
    name: 'novaInlineEditDecoration',

    addProseMirrorPlugins() {
      return [
        new Plugin<InlineEditDecorationRange | null>({
          key: inlineEditPluginKey,
          state: {
            init: () => null,
            apply: (tr, previous) => {
              const meta = tr.getMeta(inlineEditPluginKey) as InlineEditDecorationRange | null | undefined
              if (meta !== undefined) return meta
              if (!previous) return null
              if (tr.docChanged) {
                return {
                  from: tr.mapping.map(previous.from),
                  to: tr.mapping.map(previous.to),
                }
              }
              return previous
            },
          },
          props: {
            decorations: (state) => {
              const range = inlineEditPluginKey.getState(state)
              if (!range) return DecorationSet.empty
              const maxPos = state.doc.content.size
              const from = Math.max(0, Math.min(range.from, maxPos))
              const to = Math.max(from, Math.min(range.to, maxPos))
              if (from < to) {
                return DecorationSet.create(state.doc, [
                  Decoration.inline(from, to, {
                    class: 'nova-inline-edit-target',
                    'data-inline-edit-target': 'true',
                  }),
                ])
              }
              return DecorationSet.create(state.doc, [
                Decoration.widget(from, () => {
                  const marker = document.createElement('span')
                  marker.className = 'nova-inline-edit-caret'
                  marker.setAttribute('aria-hidden', 'true')
                  return marker
                }, { side: 1 }),
              ])
            },
          },
        }),
      ]
    },
  })
}
