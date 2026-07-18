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
  onHighlightClick?: (keys: readonly string[]) => void
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
              ? event.target.closest<HTMLElement>('[data-document-review-keys]')
              : null
            const keys = documentReviewKeysFromElement(target)
            if (!keys.length) return false
            stateRef.current.onHighlightClick?.(keys)
            return false
          },
          handleKeyDown: (_view, event) => {
            if (event.key !== 'Enter' && event.key !== ' ') return false
            const target = event.target instanceof Element
              ? event.target.closest<HTMLElement>('[data-document-review-keys]')
              : null
            const keys = documentReviewKeysFromElement(target)
            if (!keys.length) return false
            event.preventDefault()
            stateRef.current.onHighlightClick?.(keys)
            return true
          },
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
  const decorations = createHighlightDecorations(doc, state)
  for (const item of state.decorations) {
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

interface HighlightSegment {
  from: number
  to: number
  items: DocumentReviewDecoration[]
}

// ProseMirror repeats inline-decoration attributes whenever ranges cross block or
// overlap boundaries. Build disjoint spans up front so accessibility attributes
// remain owned by ProseMirror and never need post-render DOM mutation.
function createHighlightDecorations(doc: ProseMirrorNode, state: DocumentReviewDecorationState): Decoration[] {
  const decorations: Decoration[] = []
  const disclosedKeys = new Set<string>()
  for (const segment of buildHighlightSegments(doc, state.decorations)) {
    const undisclosedItems = segment.items.filter((item) => !disclosedKeys.has(item.key))
    const interactiveItems = undisclosedItems.length ? undisclosedItems : [segment.items[0]]
    for (const item of undisclosedItems) disclosedKeys.add(item.key)
    const keys = interactiveItems.map((item) => item.key)
    decorations.push(Decoration.inline(segment.from, segment.to, {
      class: 'nova-document-review-highlight',
      'data-document-review-keys': JSON.stringify(keys),
      ...(undisclosedItems.length ? documentReviewDisclosureAttributes(interactiveItems, state) : {}),
    }, {
      documentReviewKeys: segment.items.map((item) => item.key),
      kind: 'highlight',
    }))
  }
  return decorations
}

function buildHighlightSegments(doc: ProseMirrorNode, decorations: DocumentReviewDecoration[]): HighlightSegment[] {
  const valid = decorations.filter((item) => {
    const from = item.from ?? 0
    const to = item.to ?? 0
    return !item.outdated && from >= 0 && to > from && to <= doc.content.size
  })
  const segments: HighlightSegment[] = []
  doc.descendants((node, position) => {
    if (!node.isTextblock) return
    const blockFrom = position + 1
    const blockTo = blockFrom + node.content.size
    const intersecting = valid.filter((item) => (item.from ?? 0) < blockTo && (item.to ?? 0) > blockFrom)
    if (!intersecting.length) return false

    const boundaries = new Set<number>()
    for (const item of intersecting) {
      boundaries.add(Math.max(blockFrom, item.from ?? blockFrom))
      boundaries.add(Math.min(blockTo, item.to ?? blockTo))
    }
    const ordered = [...boundaries].sort((left, right) => left - right)
    for (let index = 0; index < ordered.length - 1; index += 1) {
      const from = ordered[index]
      const to = ordered[index + 1]
      if (to <= from) continue
      const items = intersecting
        .filter((item) => (item.from ?? 0) < to && (item.to ?? 0) > from)
        .sort(compareHighlightPriority)
      if (items.length) segments.push({ from, to, items })
    }
    return false
  })
  return segments
}

function compareHighlightPriority(left: DocumentReviewDecoration, right: DocumentReviewDecoration): number {
  const leftLength = (left.to ?? 0) - (left.from ?? 0)
  const rightLength = (right.to ?? 0) - (right.from ?? 0)
  return leftLength - rightLength || left.key.localeCompare(right.key)
}

function documentReviewTargetID(key: string): string {
  return `nova-document-review-${encodeURIComponent(key)}`
}

function documentReviewDisclosureAttributes(
  items: DocumentReviewDecoration[],
  state: DocumentReviewDecorationState,
): Record<string, string> {
  const expanded = items.every((item) => item.showWidget)
  return {
    role: 'button',
    tabindex: '0',
    'aria-expanded': String(expanded),
    'aria-controls': items.map((item) => documentReviewTargetID(item.key)).join(' '),
    'aria-label': expanded
      ? state.collapseLabel || 'Collapse comment'
      : state.expandLabel || 'Expand comment',
  }
}

export function documentReviewKeysFromElement(element: HTMLElement | null): string[] {
  if (!element) return []
  try {
    const parsed = JSON.parse(element.dataset.documentReviewKeys || '[]')
    return Array.isArray(parsed) ? parsed.filter((key): key is string => typeof key === 'string' && Boolean(key)) : []
  } catch {
    return []
  }
}
