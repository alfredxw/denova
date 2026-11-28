import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'
import type { Editor } from '@tiptap/react'
import { createLoreEntityMatcher } from '@/features/lore/lore-entity-matcher'
import type { LoreItem } from '@/lib/api'
import {
  applyInlineEditToTipTap,
  extractTipTapInlineEditContext,
  type InlineEditApplyAction,
  type InlineEditTargetRange,
} from './applyInlineEditToEditor'
import { inlineEditPluginKey } from './editorInlineEditDecorations'
import type { InlineEditAnchor } from './InlineEditPopover'
import {
  INLINE_EDIT_AFTER_BUDGET,
  INLINE_EDIT_BEFORE_BUDGET,
  matchInlineEditLoreIds,
} from './inlineEditPresets'

export interface ActiveTipTapInlineEdit {
  range: InlineEditTargetRange
  selection: string
  before: string
  after: string
  matchedLoreIds: string[]
  anchor: InlineEditAnchor | null
}

interface UseTipTapInlineEditOptions {
  editor: Editor | null
  containerRef: RefObject<HTMLElement | null>
  enabled: boolean
  isMarkdown: boolean
  resetKey: string
  loreItems: readonly LoreItem[]
}

/**
 * Coordinates inline AI edit state, ProseMirror target highlighting, editor
 * lock during preview, and single-transaction acceptance for TipTap editors.
 * Remains completely inert (zero dispatches, zero setEditable calls) until activated.
 */
export function useTipTapInlineEdit({
  editor,
  containerRef,
  enabled,
  isMarkdown,
  resetKey,
  loreItems,
}: UseTipTapInlineEditOptions) {
  const [session, setSession] = useState<ActiveTipTapInlineEdit | null>(null)
  const loreItemsRef = useRef(loreItems)
  loreItemsRef.current = loreItems
  const lockedRef = useRef(false)

  const releaseLock = useCallback((instance: Editor | null, restoreEditable: boolean) => {
    if (!lockedRef.current) return
    lockedRef.current = false
    if (!instance || instance.isDestroyed) return
    instance.view.dispatch(instance.state.tr.setMeta(inlineEditPluginKey, null))
    if (restoreEditable) {
      instance.setEditable(true)
    }
  }, [])

  const close = useCallback(() => {
    setSession(null)
    releaseLock(editor, enabled)
    if (editor && !editor.isDestroyed && enabled) {
      editor.commands.focus()
    }
  }, [editor, enabled, releaseLock])

  const cancelSilently = useCallback(() => {
    if (!lockedRef.current) return
    setSession(null)
    releaseLock(editor, enabled)
  }, [editor, enabled, releaseLock])

  // Abort/reset only if a session is currently active when the file/mode changes.
  useEffect(() => {
    if (!lockedRef.current) return
    setSession(null)
    releaseLock(editor, enabled)
  }, [editor, enabled, releaseLock, resetKey])

  // Lock document edits only while an inline edit preview is active so accidental
  // keystrokes cannot shift the locked target range.
  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    if (!enabled || !session) {
      releaseLock(editor, enabled)
      return
    }
    lockedRef.current = true
    editor.setEditable(false)
    editor.view.dispatch(editor.state.tr.setMeta(inlineEditPluginKey, session.range))
  }, [editor, enabled, releaseLock, session])

  const openFromCurrentSelection = useCallback(() => {
    if (!enabled || !editor || editor.isDestroyed) return false
    const { from, to } = editor.state.selection
    const range: InlineEditTargetRange = { from, to }
    const { before, selection, after } = extractTipTapInlineEditContext(
      editor,
      range,
      INLINE_EDIT_BEFORE_BUDGET,
      INLINE_EDIT_AFTER_BUDGET,
    )
    const matcher = loreItemsRef.current.length > 0
      ? createLoreEntityMatcher(loreItemsRef.current, { enabled: true, matchKeywords: true })
      : null
    const matchedLoreIds = matchInlineEditLoreIds(matcher, selection, before, after)

    let anchor: InlineEditAnchor | null = null
    const containerEl = containerRef.current
    if (containerEl) {
      try {
        const startCoords = editor.view.coordsAtPos(from)
        const endCoords = editor.view.coordsAtPos(to)
        const rect = containerEl.getBoundingClientRect()
        const scrollTop = containerEl.scrollTop
        anchor = {
          top: Math.min(startCoords.top, endCoords.top) - rect.top + scrollTop,
          bottom: Math.max(startCoords.bottom, endCoords.bottom) - rect.top + scrollTop,
          left: startCoords.left - rect.left,
        }
      } catch {
        anchor = null
      }
    }

    setSession({
      range,
      selection,
      before,
      after,
      matchedLoreIds,
      anchor,
    })
    return true
  }, [containerRef, editor, enabled])

  const apply = useCallback((text: string, action: InlineEditApplyAction) => {
    if (!editor || editor.isDestroyed || !session) return
    const targetRange = session.range
    setSession(null)
    releaseLock(editor, true)
    applyInlineEditToTipTap(editor, targetRange, text, action, isMarkdown)
  }, [editor, isMarkdown, releaseLock, session])

  return {
    session,
    isOpen: session !== null,
    openFromCurrentSelection,
    close,
    cancelSilently,
    apply,
  }
}
