import type { ScrollElementBottomIntoViewOptions } from './useVirtuosoBottomLock'

type BottomAnchor = (element: HTMLElement, options?: ScrollElementBottomIntoViewOptions) => void

/**
 * Anchors a virtualized chat row after React and Virtuoso finish the current
 * layout frame. Resolving the container lazily avoids retaining a row that was
 * remounted while an approval or plan card expanded.
 */
export function scheduleResolvedChatRowBottomAnchor(
  resolveContainer: () => HTMLElement | null,
  rowKey: string,
  bottomInsetPx: number,
  anchor: BottomAnchor,
) {
  let cancelled = false
  const frameID = requestAnimationFrame(() => {
    if (cancelled) return
    const container = resolveContainer()
    const row = findChatRowElement(container, rowKey)
    if (!row) return
    anchor(row, {
      bottomInsetPx,
      lockAfterScroll: true,
      visibleBottomPx: resolveChatVisibleBottomPx(container, bottomInsetPx),
    })
  })
  return () => {
    cancelled = true
    cancelAnimationFrame(frameID)
  }
}

export function scheduleChatRowBottomAnchor(
  container: HTMLElement | null,
  rowKey: string,
  bottomInsetPx: number,
  anchor: BottomAnchor,
) {
  return scheduleResolvedChatRowBottomAnchor(() => container, rowKey, bottomInsetPx, anchor)
}

function findChatRowElement(container: HTMLElement | null, rowKey: string) {
  if (!container) return null
  const rows = container.querySelectorAll<HTMLElement>('[data-nova-chat-row-key]')
  for (const row of rows) {
    if (row.dataset.novaChatRowKey === rowKey) return row
  }
  return null
}

function resolveChatVisibleBottomPx(container: HTMLElement | null, bottomInsetPx: number) {
  const scroller = container?.querySelector<HTMLElement>('.nova-chat-canvas') || null
  if (!scroller) return undefined
  const scrollerRect = scroller.getBoundingClientRect()
  const composerTop = findChatComposerTop(container, scrollerRect)
  if (composerTop !== null) return composerTop
  return scrollerRect.bottom - Math.max(0, bottomInsetPx)
}

function findChatComposerTop(container: HTMLElement | null, scrollerRect: DOMRect) {
  const parent = container?.parentElement
  if (!parent) return null
  const composers = parent.querySelectorAll<HTMLElement>('.nova-chat-input-area .nova-agent-composer')
  let visibleTop: number | null = null
  for (const composer of composers) {
    if (container?.contains(composer)) continue
    const rect = composer.getBoundingClientRect()
    if (
      rect.width <= 0
      || rect.height <= 0
      || !Number.isFinite(rect.top)
      || rect.top <= scrollerRect.top
      || rect.top > scrollerRect.bottom
    ) {
      continue
    }
    visibleTop = visibleTop === null ? rect.top : Math.max(visibleTop, rect.top)
  }
  return visibleTop
}
