import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  type KeyboardEventHandler,
  type PointerEventHandler,
} from 'react'
import type { Layout } from 'react-resizable-panels'

interface PersistedPanelLayoutOptions {
  storageKey?: string
  panelIds: readonly string[]
}

interface ResizeHandleIntentProps {
  onPointerDownCapture: PointerEventHandler<HTMLElement>
  onKeyDownCapture: KeyboardEventHandler<HTMLElement>
}

interface PersistedPanelLayout {
  defaultLayout: Layout | undefined
  resizeHandleIntentProps: ResizeHandleIntentProps
  isUserResizeActive: () => boolean
  persistUserLayout: (layout: Layout) => boolean
}

const PANEL_IDS_SEPARATOR = '\u0000'
const VALID_LAYOUT_TOTAL_MIN = 99
const VALID_LAYOUT_TOTAL_MAX = 101

/**
 * Restores a panel layout on mount, but only lets an explicit pointer or keyboard resize
 * replace it. Panel groups also emit layout callbacks during initialization, constraint
 * recalculation, and programmatic collapse/expand; treating those as user preferences can
 * silently reset a carefully adjusted sidebar during reloads or development hot updates,
 * even though the user did not touch a resize handle.
 */
export function usePersistedPanelLayout({ storageKey, panelIds }: PersistedPanelLayoutOptions): PersistedPanelLayout {
  const panelIdsSignature = panelIds.join(PANEL_IDS_SEPARATOR)
  const resizeIntentRef = useRef(false)
  const pointerIntentCleanupRef = useRef<(() => void) | null>(null)
  const pointerIntentFinishFrameRef = useRef<number | null>(null)
  const keyboardIntentTimerRef = useRef<number | null>(null)

  const clearResizeIntent = useCallback(() => {
    resizeIntentRef.current = false
    pointerIntentCleanupRef.current?.()
    pointerIntentCleanupRef.current = null
    if (pointerIntentFinishFrameRef.current !== null) {
      window.cancelAnimationFrame(pointerIntentFinishFrameRef.current)
      pointerIntentFinishFrameRef.current = null
    }
    if (keyboardIntentTimerRef.current !== null) {
      window.clearTimeout(keyboardIntentTimerRef.current)
      keyboardIntentTimerRef.current = null
    }
  }, [])

  useEffect(() => {
    clearResizeIntent()
    return clearResizeIntent
  }, [clearResizeIntent, panelIdsSignature, storageKey])

  const defaultLayout = useMemo(
    () => readPersistedPanelLayout(storageKey, panelIdsSignature.split(PANEL_IDS_SEPARATOR)),
    [panelIdsSignature, storageKey],
  )

  const armPointerResize = useCallback<PointerEventHandler<HTMLElement>>(() => {
    clearResizeIntent()
    if (!storageKey) return
    resizeIntentRef.current = true

    // The panel store finalizes on pointerup, but React can deliver its onLayoutChanged callback
    // after the native event has finished. Keep this interaction armed through that final commit.
    const clearAfterPointer = () => {
      pointerIntentCleanupRef.current?.()
      pointerIntentCleanupRef.current = null
      if (pointerIntentFinishFrameRef.current !== null) window.cancelAnimationFrame(pointerIntentFinishFrameRef.current)
      // onLayoutChanged is delivered from a passive effect after the committed panel size paints.
      // A second frame closes the grace period after that effect without using an arbitrary delay.
      pointerIntentFinishFrameRef.current = window.requestAnimationFrame(() => {
        pointerIntentFinishFrameRef.current = window.requestAnimationFrame(() => {
          pointerIntentFinishFrameRef.current = null
          clearResizeIntent()
        })
      })
    }
    window.addEventListener('pointerup', clearAfterPointer, { once: true })
    window.addEventListener('pointercancel', clearAfterPointer, { once: true })
    pointerIntentCleanupRef.current = () => {
      window.removeEventListener('pointerup', clearAfterPointer)
      window.removeEventListener('pointercancel', clearAfterPointer)
    }
  }, [clearResizeIntent, storageKey])

  const armKeyboardResize = useCallback<KeyboardEventHandler<HTMLElement>>((event) => {
    if (!storageKey || !isPanelResizeKey(event.key)) return
    clearResizeIntent()
    resizeIntentRef.current = true
    // The panel library applies keyboard resizing synchronously during this key event.
    keyboardIntentTimerRef.current = window.setTimeout(clearResizeIntent, 0)
  }, [clearResizeIntent, storageKey])

  const persistUserLayout = useCallback((layout: Layout) => {
    if (!storageKey || !resizeIntentRef.current) return false
    // A pointer drag emits many layouts. Pointerup (or the keyboard timer) owns cleanup so the
    // final layout keeps replacing the earlier intermediate widths throughout the interaction.

    const normalized = normalizePanelLayout(layout, panelIdsSignature.split(PANEL_IDS_SEPARATOR))
    if (!normalized) {
      console.warn('[panel-layout] Ignored invalid user resize layout', { storageKey, layout })
      return false
    }

    try {
      window.localStorage.setItem(storageKey, JSON.stringify(normalized))
      return true
    } catch (error) {
      console.warn('[panel-layout] Unable to persist user resize layout', { storageKey, error })
      return false
    }
  }, [panelIdsSignature, storageKey])

  const resizeHandleIntentProps = useMemo<ResizeHandleIntentProps>(() => ({
    onPointerDownCapture: armPointerResize,
    onKeyDownCapture: armKeyboardResize,
  }), [armKeyboardResize, armPointerResize])
  const isUserResizeActive = useCallback(() => resizeIntentRef.current, [])

  return { defaultLayout, resizeHandleIntentProps, isUserResizeActive, persistUserLayout }
}

export function readPersistedPanelLayout(storageKey?: string, panelIds?: readonly string[]): Layout | undefined {
  if (!storageKey || typeof window === 'undefined') return undefined
  try {
    const stored = window.localStorage.getItem(storageKey)
    if (!stored) return undefined
    const parsed = JSON.parse(stored) as unknown
    return normalizePanelLayout(parsed, panelIds)
  } catch (error) {
    console.warn('[panel-layout] Unable to restore persisted layout', { storageKey, error })
    return undefined
  }
}

/**
 * Converts a persisted percentage into an explicit imperative resize target.
 *
 * A panel that mounts collapsed cannot rely on the panel library's internal expanded-size cache:
 * constraint re-registration may discard that cache before the first visible layout. Supplying
 * the persisted percentage explicitly keeps reloads and dynamic layout contexts deterministic.
 */
export function resolvePanelInitialSize(
  layout: Layout | undefined,
  panelId: string,
  fallbackSize: number | string,
): number | string {
  const persistedSize = layout?.[panelId]
  return typeof persistedSize === 'number' && Number.isFinite(persistedSize) && persistedSize > 0
    ? `${persistedSize}%`
    : fallbackSize
}

function normalizePanelLayout(value: unknown, panelIds?: readonly string[]): Layout | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const candidate = value as Record<string, unknown>
  const ids = panelIds?.length ? panelIds : Object.keys(candidate)
  if (ids.length === 0) return undefined

  const normalized: Layout = {}
  let total = 0
  for (const panelId of ids) {
    const size = candidate[panelId]
    if (typeof size !== 'number' || !Number.isFinite(size) || size < 0 || size > 100) return undefined
    normalized[panelId] = size
    total += size
  }
  if (total < VALID_LAYOUT_TOTAL_MIN || total > VALID_LAYOUT_TOTAL_MAX) return undefined
  return normalized
}

function isPanelResizeKey(key: string) {
  return key === 'ArrowLeft'
    || key === 'ArrowRight'
    || key === 'ArrowUp'
    || key === 'ArrowDown'
    || key === 'Home'
    || key === 'End'
}
