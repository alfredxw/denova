import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey, type Transaction } from '@tiptap/pm/state'
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view'

import {
  LORE_LENS_ATTR,
  LORE_LENS_CLASS,
  type LoreEntityMatcher,
} from '@/features/lore/lore-entity-matcher'

interface LoreLensPluginState {
  decorations: DecorationSet
  complete: boolean
  signature: string
}

interface LoreLensMeta {
  rebuild?: boolean
  completed?: {
    document: ProseMirrorNode
    signature: string
    decorations: DecorationSet
  }
}

interface DocumentRange {
  from: number
  to: number
}

const DEFERRED_REBUILD_MIN_SIZE = 32 * 1024
const FALLBACK_BLOCKS_PER_FRAME = 32
const EXCLUDED_BLOCK_TYPES = new Set(['codeBlock'])
const EXCLUDED_MARK_TYPES = new Set(['code', 'link'])

export const loreLensPluginKey = new PluginKey<LoreLensPluginState>('nova-editor-lore-lens')

/**
 * Non-destructive incremental Lore entity overlay for TipTap manuscripts.
 * Matches enabled Lore names/keywords purely in the decoration layer without
 * mutating the ProseMirror document or serialized Markdown, and defers
 * rebuilds while a CJK IME composition is active.
 */
export function createLoreLensPlugin(matcherRef: { current: LoreEntityMatcher | null }) {
  let composing = false

  return new Plugin<LoreLensPluginState>({
    key: loreLensPluginKey,
    state: {
      init: () => ({
        decorations: DecorationSet.empty,
        complete: !(matcherRef.current?.hasTerms),
        signature: matcherRef.current?.signature ?? '',
      }),
      apply: (transaction, previous, _oldState, nextState) => {
        const meta = transaction.getMeta(loreLensPluginKey) as LoreLensMeta | undefined
        const matcher = matcherRef.current

        if (meta?.completed) {
          if (
            meta.completed.document === nextState.doc
            && meta.completed.signature === (matcher?.signature ?? '')
          ) {
            return {
              decorations: meta.completed.decorations,
              complete: true,
              signature: meta.completed.signature,
            }
          }
          return previous
        }

        if (meta?.rebuild) {
          if (!matcher || !matcher.hasTerms) {
            return { decorations: DecorationSet.empty, complete: true, signature: '' }
          }
          return {
            decorations: DecorationSet.empty,
            complete: false,
            signature: matcher.signature,
          }
        }

        if (!matcher || !matcher.hasTerms) {
          return previous.decorations === DecorationSet.empty && previous.complete && previous.signature === ''
            ? previous
            : { decorations: DecorationSet.empty, complete: true, signature: '' }
        }

        if (previous.signature !== matcher.signature) {
          return {
            decorations: DecorationSet.empty,
            complete: false,
            signature: matcher.signature,
          }
        }

        if (!transaction.docChanged) return previous

        const mapped = previous.decorations.map(transaction.mapping, transaction.doc)
        // Avoid mutating inline spans mid-IME composition; rebuild once composition ends.
        if (composing || !previous.complete) {
          return { decorations: mapped, complete: false, signature: matcher.signature }
        }

        const changedRanges = changedTextblockRanges(transaction, nextState.doc)
        let decorations = mapped
        for (const range of changedRanges) {
          decorations = decorations.remove(decorations.find(range.from, range.to))
        }
        if (shouldDeferChangedRanges(changedRanges, nextState.doc)) {
          return { decorations, complete: false, signature: matcher.signature }
        }

        for (const range of changedRanges) {
          const additions = createLoreDecorationsInRange(nextState.doc, range, matcher)
          if (additions.length > 0) decorations = decorations.add(nextState.doc, additions)
        }
        return { decorations, complete: true, signature: matcher.signature }
      },
    },
    props: {
      decorations: (state) => loreLensPluginKey.getState(state)?.decorations ?? DecorationSet.empty,
      handleDOMEvents: {
        compositionstart: () => {
          composing = true
          return false
        },
        compositionend: (view) => {
          composing = false
          const pluginState = loreLensPluginKey.getState(view.state)
          if (pluginState && !pluginState.complete && matcherRef.current?.hasTerms) {
            view.dispatch(
              view.state.tr
                .setMeta(loreLensPluginKey, { rebuild: true } satisfies LoreLensMeta)
                .setMeta('addToHistory', false),
            )
          }
          return false
        },
      },
    },
    view: (view) => createLoreDecorationView(view, matcherRef),
  })
}

function createLoreDecorationView(
  view: EditorView,
  matcherRef: { current: LoreEntityMatcher | null },
) {
  let buildDocument: ProseMirrorNode | null = null
  let buildSignature = ''
  let cancelBuild: (() => void) | null = null

  const cancelPending = () => {
    cancelBuild?.()
    cancelBuild = null
    buildDocument = null
    buildSignature = ''
  }

  const ensureComplete = (currentView: EditorView) => {
    const matcher = matcherRef.current
    const pluginState = loreLensPluginKey.getState(currentView.state)
    if (!matcher || !matcher.hasTerms || pluginState?.complete) {
      cancelPending()
      return
    }
    if (currentView.composing) {
      cancelPending()
      return
    }

    const document = currentView.state.doc
    const signature = matcher.signature
    if (buildDocument === document && buildSignature === signature) return

    cancelPending()
    buildDocument = document
    buildSignature = signature
    cancelBuild = buildLoreDecorations(document, matcher, (decorations) => {
      if (
        buildDocument !== document
        || buildSignature !== signature
        || currentView.state.doc !== document
        || matcherRef.current?.signature !== signature
      ) {
        return
      }
      buildDocument = null
      buildSignature = ''
      cancelBuild = null
      currentView.dispatch(
        currentView.state.tr
          .setMeta(loreLensPluginKey, {
            completed: { document, signature, decorations },
          } satisfies LoreLensMeta)
          .setMeta('addToHistory', false),
      )
    })
  }

  ensureComplete(view)
  return {
    update: ensureComplete,
    destroy: cancelPending,
  }
}

function buildLoreDecorations(
  document: ProseMirrorNode,
  matcher: LoreEntityMatcher,
  onComplete: (decorations: DecorationSet) => void,
) {
  let childIndex = 0
  let childOffset = 0
  let decorations = DecorationSet.empty
  let cancelSlice: (() => void) | null = null
  let cancelled = false

  const processSlice = (deadline?: IdleDeadline) => {
    cancelSlice = null
    const additions: Decoration[] = []
    let processedBlocks = 0
    while (
      childIndex < document.childCount
      && processedBlocks < (deadline ? Number.POSITIVE_INFINITY : FALLBACK_BLOCKS_PER_FRAME)
      && (processedBlocks === 0 || !deadline || deadline.timeRemaining() > 1)
    ) {
      const child = document.child(childIndex)
      appendNodeLoreDecorations(child, childOffset, matcher, additions)
      childOffset += child.nodeSize
      childIndex += 1
      processedBlocks += 1
    }

    if (additions.length > 0) decorations = decorations.add(document, additions)
    if (cancelled) return
    if (childIndex >= document.childCount) {
      onComplete(decorations)
      return
    }
    cancelSlice = scheduleWorkSlice(processSlice)
  }

  cancelSlice = scheduleWorkSlice(processSlice)
  return () => {
    cancelled = true
    cancelSlice?.()
    cancelSlice = null
  }
}

function scheduleWorkSlice(callback: (deadline?: IdleDeadline) => void) {
  if (typeof window.requestIdleCallback === 'function') {
    const requestID = window.requestIdleCallback(callback)
    return () => window.cancelIdleCallback(requestID)
  }
  const frameID = window.requestAnimationFrame(() => callback())
  return () => window.cancelAnimationFrame(frameID)
}

function createLoreDecorationsInRange(
  document: ProseMirrorNode,
  range: DocumentRange,
  matcher: LoreEntityMatcher,
) {
  const decorations: Decoration[] = []
  document.nodesBetween(range.from, range.to, (node, position) => {
    if (EXCLUDED_BLOCK_TYPES.has(node.type.name)) return false
    if (node.isTextblock) {
      appendTextblockLoreDecorations(node, position, matcher, decorations)
      return false
    }
    return true
  })
  return decorations
}

function appendNodeLoreDecorations(
  node: ProseMirrorNode,
  position: number,
  matcher: LoreEntityMatcher,
  decorations: Decoration[],
) {
  if (EXCLUDED_BLOCK_TYPES.has(node.type.name)) return
  if (node.isTextblock) {
    appendTextblockLoreDecorations(node, position, matcher, decorations)
    return
  }
  node.descendants((descendant, relativePosition) => {
    if (EXCLUDED_BLOCK_TYPES.has(descendant.type.name)) return false
    if (descendant.isTextblock) {
      appendTextblockLoreDecorations(descendant, position + 1 + relativePosition, matcher, decorations)
      return false
    }
    return true
  })
}

function appendTextblockLoreDecorations(
  textblock: ProseMirrorNode,
  blockStart: number,
  matcher: LoreEntityMatcher,
  decorations: Decoration[],
) {
  // Deduplicate each Lore entry to its first occurrence within this paragraph/block.
  const seenItemIds = new Set<string>()
  textblock.forEach((child, offset) => {
    if (!child.isText || !child.text) return
    if (child.marks.some((mark) => EXCLUDED_MARK_TYPES.has(mark.type.name))) return
    const textPos = blockStart + 1 + offset
    for (const match of matcher.findMatches(child.text, seenItemIds)) {
      decorations.push(
        Decoration.inline(textPos + match.from, textPos + match.to, {
          class: LORE_LENS_CLASS,
          [LORE_LENS_ATTR]: match.itemId,
        }),
      )
    }
  })
}

function changedTextblockRanges(transaction: Transaction, document: ProseMirrorNode) {
  const ranges: DocumentRange[] = []
  transaction.mapping.maps.forEach((stepMap, stepIndex) => {
    stepMap.forEach((_oldFrom, _oldTo, newFrom, newTo) => {
      const remainingMapping = transaction.mapping.slice(stepIndex + 1)
      ranges.push(expandToTextblocks(
        document,
        remainingMapping.map(newFrom, -1),
        remainingMapping.map(newTo, 1),
      ))
    })
  })
  return mergeRanges(ranges)
}

function expandToTextblocks(document: ProseMirrorNode, rawFrom: number, rawTo: number): DocumentRange {
  const from = clampPosition(document, Math.min(rawFrom, rawTo))
  const to = clampPosition(document, Math.max(rawFrom, rawTo))
  const start = textblockBoundsNear(document, from, 1)
  const end = textblockBoundsNear(document, to, -1)
  return {
    from: start?.from ?? from,
    to: end?.to ?? Math.max(from, to),
  }
}

function textblockBoundsNear(document: ProseMirrorNode, position: number, direction: -1 | 1): DocumentRange | null {
  const candidates = [position, position + direction, position - direction]
  for (const candidate of candidates) {
    if (candidate < 0 || candidate > document.content.size) continue
    const resolved = document.resolve(candidate)
    for (let depth = resolved.depth; depth > 0; depth -= 1) {
      if (resolved.node(depth).isTextblock) {
        return { from: resolved.start(depth), to: resolved.end(depth) }
      }
    }
  }
  return null
}

function mergeRanges(ranges: DocumentRange[]) {
  const sorted = ranges
    .filter((range) => range.to >= range.from)
    .sort((left, right) => left.from - right.from)
  const merged: DocumentRange[] = []
  for (const range of sorted) {
    const previous = merged[merged.length - 1]
    if (!previous || range.from > previous.to) {
      merged.push({ ...range })
    } else {
      previous.to = Math.max(previous.to, range.to)
    }
  }
  return merged
}

function shouldDeferChangedRanges(ranges: DocumentRange[], document: ProseMirrorNode) {
  const changedSize = ranges.reduce((total, range) => total + range.to - range.from, 0)
  return changedSize >= DEFERRED_REBUILD_MIN_SIZE && changedSize >= document.content.size / 4
}

function clampPosition(document: ProseMirrorNode, position: number) {
  return Math.max(0, Math.min(document.content.size, position))
}
