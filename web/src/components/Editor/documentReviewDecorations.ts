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
  showWidget?: boolean
}

export interface DocumentReviewDecorationState {
  enabled: boolean
  decorations: DocumentReviewDecoration[]
  onHighlightClick?: (key: string) => void
  expandLabel?: string
  collapseLabel?: string
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
        state: {
          init: (_, state) => createDecorations(state.doc, stateRef.current),
          apply: (transaction, previous, _oldState, nextState) => {
            if (transaction.getMeta(documentReviewPluginKey)) {
              return createDecorations(nextState.doc, stateRef.current)
            }
            return transaction.docChanged
              ? previous.map(transaction.mapping, transaction.doc)
              : previous
          },
        },
        props: {
          decorations: (state) => documentReviewPluginKey.getState(state) ?? DecorationSet.empty,
          handleClick: (_view, _position, event) => {
            if (event.button !== 0 || event.detail > 1) return false
            const target = event.target instanceof Element
              ? event.target.closest<HTMLElement>('[data-document-review-key]')
              : null
            const key = target?.dataset.documentReviewKey
            if (!key) return false
            stateRef.current.onHighlightClick?.(key)
            return false
          },
          handleKeyDown: (_view, event) => {
            if (event.key !== 'Enter' && event.key !== ' ') return false
            const target = event.target instanceof Element
              ? event.target.closest<HTMLElement>('[data-document-review-key]')
              : null
            const key = target?.dataset.documentReviewKey
            if (!key) return false
            event.preventDefault()
            stateRef.current.onHighlightClick?.(key)
            return true
          },
        },
        view: (view) => {
          let frame = 0
          const publish = () => {
            normalizeHighlightDisclosures(view.dom, stateRef.current)
            if (frame) cancelAnimationFrame(frame)
            frame = requestAnimationFrame(() => {
              normalizeHighlightDisclosures(view.dom, stateRef.current)
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
      const expanded = Boolean(item.showWidget)
      decorations.push(Decoration.inline(from, to, {
        class: 'nova-document-review-highlight',
        'data-document-review-key': item.key,
        ...documentReviewDisclosureAttributes(item, state, expanded),
      }, { documentReviewKey: item.key, kind: 'highlight' }))
    }
    if (!item.showWidget) continue
    const widgetPos = Math.max(0, Math.min(doc.content.size, item.widgetPos))
    decorations.push(Decoration.widget(widgetPos, () => {
      const element = document.createElement('div')
      element.className = `nova-document-review-widget${item.outdated ? ' is-outdated' : ''}`
      element.id = documentReviewTargetID(item.key)
      element.dataset.documentReviewTarget = item.key
      element.contentEditable = 'false'
      return element
    }, {
      key: item.key,
      side: 1,
      stopEvent: () => true,
      ignoreSelection: true,
    }))
  }
  return DecorationSet.create(doc, decorations)
}

function documentReviewTargetID(key: string): string {
  return `nova-document-review-${encodeURIComponent(key)}`
}

function documentReviewDisclosureAttributes(
  item: DocumentReviewDecoration,
  state: DocumentReviewDecorationState,
  expanded = Boolean(item.showWidget),
): Record<string, string> {
  return {
    role: 'button',
    tabindex: '0',
    'aria-expanded': String(expanded),
    'aria-controls': documentReviewTargetID(item.key),
    'aria-label': expanded
      ? state.collapseLabel || 'Collapse comment'
      : state.expandLabel || 'Expand comment',
  }
}

// ProseMirror splits one inline decoration at block boundaries. Only the first
// fragment represents the keyboard disclosure; every fragment remains a mouse
// target through data-document-review-key.
function normalizeHighlightDisclosures(root: HTMLElement, state: DocumentReviewDecorationState): void {
  const seen = new Set<string>()
  const disclosureAttributes = ['role', 'tabindex', 'aria-expanded', 'aria-controls', 'aria-label']
  const decorations = new Map(state.decorations.map((item) => [item.key, item]))
  for (const fragment of root.querySelectorAll<HTMLElement>('[data-document-review-key]')) {
    const key = fragment.dataset.documentReviewKey
    if (!key || !seen.has(key)) {
      if (key) {
        seen.add(key)
        const item = decorations.get(key)
        if (item) {
          for (const [attribute, value] of Object.entries(documentReviewDisclosureAttributes(item, state))) {
            if (fragment.getAttribute(attribute) !== value) fragment.setAttribute(attribute, value)
          }
        }
      }
      continue
    }
    for (const attribute of disclosureAttributes) {
      if (fragment.hasAttribute(attribute)) fragment.removeAttribute(attribute)
    }
  }
}
