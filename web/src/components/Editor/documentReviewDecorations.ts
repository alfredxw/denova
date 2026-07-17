import { Extension } from '@tiptap/core'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

export interface DocumentReviewDecoration {
  key: string
  from?: number
  to?: number
  widgetPos: number
  outdated?: boolean
}

export interface DocumentReviewDecorationState {
  enabled: boolean
  decorations: DocumentReviewDecoration[]
}

export type DocumentReviewPortalTarget = { key: string; element: HTMLElement }

export const documentReviewPluginKey = new PluginKey<DecorationSet>('nova-document-review')

export function createDocumentReviewExtension(
  stateRef: { current: DocumentReviewDecorationState },
  onTargetsChange: (targets: DocumentReviewPortalTarget[]) => void,
) {
  return Extension.create({
    name: 'novaDocumentReview',
    addProseMirrorPlugins() {
      return [new Plugin<DecorationSet>({
        key: documentReviewPluginKey,
        // Keep ProseMirror selectable in Review mode while rejecting every
        // transaction that could change the manuscript (typing, paste, drop,
        // commands, or IME input).
        filterTransaction: (transaction) => !stateRef.current.enabled || !transaction.docChanged,
        state: {
          init: (_, state) => createDecorations(state.doc, stateRef.current),
          apply: (transaction, previous, _oldState, nextState) => {
            if (transaction.docChanged || transaction.getMeta(documentReviewPluginKey)) {
              return createDecorations(nextState.doc, stateRef.current)
            }
            return previous.map(transaction.mapping, transaction.doc)
          },
        },
        props: {
          decorations: (state) => documentReviewPluginKey.getState(state) ?? DecorationSet.empty,
        },
        view: (view) => {
          let frame = 0
          const publish = () => {
            if (frame) cancelAnimationFrame(frame)
            frame = requestAnimationFrame(() => {
              const targets = Array.from(view.dom.querySelectorAll<HTMLElement>('[data-document-review-target]'))
                .map((element) => ({ key: element.dataset.documentReviewTarget || '', element }))
                .filter((target) => target.key)
              onTargetsChange(targets)
            })
          }
          publish()
          return {
            update: publish,
            destroy: () => {
              if (frame) cancelAnimationFrame(frame)
              onTargetsChange([])
            },
          }
        },
      })]
    },
  })
}

function createDecorations(doc: ProseMirrorNode, state: DocumentReviewDecorationState) {
  if (!state.enabled || !state.decorations.length) return DecorationSet.empty
  const decorations: Decoration[] = []
  for (const item of state.decorations) {
    const from = item.from ?? 0
    const to = item.to ?? 0
    if (!item.outdated && from >= 0 && to > from && to <= doc.content.size) {
      decorations.push(Decoration.inline(from, to, { class: 'nova-document-review-highlight' }))
    }
    const widgetPos = Math.max(0, Math.min(doc.content.size, item.widgetPos))
    decorations.push(Decoration.widget(widgetPos, () => {
      const element = document.createElement('div')
      element.className = `nova-document-review-widget${item.outdated ? ' is-outdated' : ''}`
      element.dataset.documentReviewTarget = item.key
      element.contentEditable = 'false'
      return element
    }, { key: item.key, side: 1 }))
  }
  return DecorationSet.create(doc, decorations)
}
