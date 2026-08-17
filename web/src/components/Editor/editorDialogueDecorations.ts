import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey, type Transaction } from '@tiptap/pm/state'
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view'

import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'

interface DialogueDecorationState {
  decorations: DecorationSet
  complete: boolean
}

interface DialogueDecorationResult {
  document: ProseMirrorNode
  decorations: DecorationSet
}

interface DocumentRange {
  from: number
  to: number
}

const DIALOGUE_HIGHLIGHT_CLASS = 'nova-editor-dialogue-highlight'
const DEFERRED_REBUILD_MIN_SIZE = 32 * 1024
const FALLBACK_BLOCKS_PER_FRAME = 32

export const dialogueHighlightPluginKey = new PluginKey<DialogueDecorationState>('nova-editor-dialogue-highlight')

/**
 * Keeps typing incremental and moves whole-document parsing outside the input/navigation task.
 * A scheduled build is tied to the exact ProseMirror document and is discarded after navigation.
 */
export function createDialogueHighlightPlugin() {
  return new Plugin<DialogueDecorationState>({
    key: dialogueHighlightPluginKey,
    state: {
      init: () => ({ decorations: DecorationSet.empty, complete: false }),
      apply: (transaction, previous, _oldState, nextState) => {
        const completedBuild = transaction.getMeta(dialogueHighlightPluginKey) as DialogueDecorationResult | undefined
        if (completedBuild?.document === nextState.doc) {
          return { decorations: completedBuild.decorations, complete: true }
        }
        if (!transaction.docChanged) return previous

        const mapped = previous.decorations.map(transaction.mapping, transaction.doc)
        if (!previous.complete) return { decorations: mapped, complete: false }

        const changedRanges = changedTextblockRanges(transaction, nextState.doc)
        let decorations = mapped
        for (const range of changedRanges) {
          decorations = decorations.remove(decorations.find(range.from, range.to))
        }
        if (shouldDeferChangedRanges(changedRanges, nextState.doc)) {
          return { decorations, complete: false }
        }

        for (const range of changedRanges) {
          const additions = createDialogueDecorationsInRange(nextState.doc, range)
          if (additions.length > 0) decorations = decorations.add(nextState.doc, additions)
        }
        return { decorations, complete: true }
      },
    },
    props: {
      decorations: (state) => dialogueHighlightPluginKey.getState(state)?.decorations ?? DecorationSet.empty,
    },
    view: (view) => createDialogueDecorationView(view),
  })
}

function createDialogueDecorationView(view: EditorView) {
  let buildDocument: ProseMirrorNode | null = null
  let cancelBuild: (() => void) | null = null

  const ensureComplete = (currentView: EditorView) => {
    const pluginState = dialogueHighlightPluginKey.getState(currentView.state)
    if (pluginState?.complete) {
      cancelBuild?.()
      cancelBuild = null
      buildDocument = null
      return
    }

    const document = currentView.state.doc
    if (buildDocument === document) return
    cancelBuild?.()
    buildDocument = document
    cancelBuild = buildDialogueDecorations(document, (decorations) => {
      if (buildDocument !== document || currentView.state.doc !== document) return
      buildDocument = null
      cancelBuild = null
      currentView.dispatch(currentView.state.tr
        .setMeta(dialogueHighlightPluginKey, { document, decorations } satisfies DialogueDecorationResult)
        .setMeta('addToHistory', false))
    })
  }

  ensureComplete(view)
  return {
    update: ensureComplete,
    destroy: () => {
      cancelBuild?.()
      cancelBuild = null
      buildDocument = null
    },
  }
}

function buildDialogueDecorations(document: ProseMirrorNode, onComplete: (decorations: DecorationSet) => void) {
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
      appendDialogueDecorations(child, childOffset, additions)
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

function createDialogueDecorationsInRange(document: ProseMirrorNode, range: DocumentRange) {
  const decorations: Decoration[] = []
  document.nodesBetween(range.from, range.to, (node, position) => {
    if (node.isText && node.text) appendTextDecorations(node.text, position, decorations)
  })
  return decorations
}

function appendDialogueDecorations(node: ProseMirrorNode, position: number, decorations: Decoration[]) {
  if (node.isText && node.text) {
    appendTextDecorations(node.text, position, decorations)
    return
  }
  node.descendants((descendant, relativePosition) => {
    if (descendant.isText && descendant.text) {
      appendTextDecorations(descendant.text, position + 1 + relativePosition, decorations)
    }
  })
}

function appendTextDecorations(text: string, position: number, decorations: Decoration[]) {
  for (const range of findDialogueHighlightRanges(text)) {
    decorations.push(Decoration.inline(position + range.from, position + range.to, {
      class: DIALOGUE_HIGHLIGHT_CLASS,
    }))
  }
}

function clampPosition(document: ProseMirrorNode, position: number) {
  return Math.max(0, Math.min(document.content.size, position))
}
