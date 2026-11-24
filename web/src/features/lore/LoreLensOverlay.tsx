import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { ArrowUpRight, Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { loreImageURL, type LoreItem } from '@/lib/api'
import { LORE_LENS_ATTR, loreLensExcerpt } from './lore-entity-matcher'
import { MaterialImage } from './MaterialImage'
import { loreImportanceLabel, loreTypeLabel } from './options'
import { useOptionalLoreCategories } from './lore-lens-preferences'

interface ActiveLorePreview {
  itemId: string
  anchorRect: DOMRect
  pinned: boolean
}

export interface LoreLensOverlayProps {
  containerRef: RefObject<HTMLElement | null>
  projectId: string
  items: readonly LoreItem[]
  enabled: boolean
  /** Editable surfaces avoid intercepting plain taps on touch screens so typing stays uninterrupted. */
  editable?: boolean
  onOpenItem?: (id: string) => void
  /** Optional Game-mode summary line reflecting live actor state for this Lore item. */
  getActorSummary?: (item: LoreItem) => string | null
}

const HOVER_OPEN_DELAY_MS = 220
const HOVER_CLOSE_DELAY_MS = 180
const CARD_WIDTH = 312
const CARD_ESTIMATED_HEIGHT = 200
const VIEWPORT_MARGIN = 12
const ANCHOR_GAP = 8

/**
 * Delegated singleton hover/click preview card for Lore Lens mentions.
 * Keeps ProseMirror and StoryStage free of per-mention React mounts.
 */
export function LoreLensOverlay({
  containerRef,
  projectId,
  items,
  enabled,
  editable = false,
  onOpenItem,
  getActorSummary,
}: LoreLensOverlayProps) {
  const { t } = useTranslation()
  const categories = useOptionalLoreCategories(projectId)
  const [preview, setPreview] = useState<ActiveLorePreview | null>(null)
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const cardRef = useRef<HTMLDivElement | null>(null)
  const openTimerRef = useRef<number | null>(null)
  const closeTimerRef = useRef<number | null>(null)
  const hoveredElementRef = useRef<HTMLElement | null>(null)
  const previewRef = useRef<ActiveLorePreview | null>(null)
  previewRef.current = preview

  const itemById = useMemo(
    () => new Map(items.filter((item) => item.enabled).map((item) => [item.id, item])),
    [items],
  )

  const clearTimers = useCallback(() => {
    if (openTimerRef.current !== null) {
      window.clearTimeout(openTimerRef.current)
      openTimerRef.current = null
    }
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current)
      closeTimerRef.current = null
    }
  }, [])

  const closePreview = useCallback(() => {
    clearTimers()
    hoveredElementRef.current = null
    setPreview(null)
  }, [clearTimers])

  const scheduleClose = useCallback(() => {
    if (previewRef.current?.pinned) return
    if (openTimerRef.current !== null) {
      window.clearTimeout(openTimerRef.current)
      openTimerRef.current = null
    }
    if (closeTimerRef.current !== null) window.clearTimeout(closeTimerRef.current)
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null
      hoveredElementRef.current = null
      setPreview((current) => (current?.pinned ? current : null))
    }, HOVER_CLOSE_DELAY_MS)
  }, [])

  useEffect(() => {
    if (!enabled) closePreview()
  }, [closePreview, enabled])

  useEffect(() => {
    if (preview && !itemById.has(preview.itemId)) closePreview()
  }, [closePreview, itemById, preview])

  useEffect(() => {
    if (!enabled) return
    const container = containerRef.current
    if (!container) return

    const findMention = (target: EventTarget | null): HTMLElement | null => {
      if (!(target instanceof Element)) return null
      const match = target.closest<HTMLElement>(`[${LORE_LENS_ATTR}]`)
      return match && container.contains(match) ? match : null
    }

    const handleMouseOver = (event: MouseEvent) => {
      const mention = findMention(event.target)
      if (!mention) return
      const itemId = mention.getAttribute(LORE_LENS_ATTR) || ''
      if (!itemId || !itemById.has(itemId)) return

      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current)
        closeTimerRef.current = null
      }
      if (hoveredElementRef.current === mention && previewRef.current?.itemId === itemId) return
      hoveredElementRef.current = mention

      if (openTimerRef.current !== null) window.clearTimeout(openTimerRef.current)
      openTimerRef.current = window.setTimeout(() => {
        openTimerRef.current = null
        if (!mention.isConnected) return
        setPreview({
          itemId,
          anchorRect: mention.getBoundingClientRect(),
          pinned: false,
        })
      }, HOVER_OPEN_DELAY_MS)
    }

    const handleMouseOut = (event: MouseEvent) => {
      const mention = findMention(event.target)
      if (!mention) return
      const related = event.relatedTarget instanceof Node ? event.relatedTarget : null
      if (related && (mention.contains(related) || cardRef.current?.contains(related))) return
      scheduleClose()
    }

    const handleClick = (event: MouseEvent) => {
      const mention = findMention(event.target)
      if (!mention) {
        if (previewRef.current?.pinned) closePreview()
        return
      }
      const itemId = mention.getAttribute(LORE_LENS_ATTR) || ''
      if (!itemId || !itemById.has(itemId)) return

      if ((event.metaKey || event.ctrlKey) && onOpenItem) {
        event.preventDefault()
        closePreview()
        console.info('[lore-lens] navigating to lore item via modifier click', { projectId, itemId })
        onOpenItem(itemId)
        return
      }

      // In read-only prose (Game), clicking/tapping a mention opens the preview card.
      if (!editable) {
        clearTimers()
        setPreview({
          itemId,
          anchorRect: mention.getBoundingClientRect(),
          pinned: true,
        })
      }
    }

    const handleScroll = () => {
      if (previewRef.current) closePreview()
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && previewRef.current) closePreview()
    }

    const handleOutsidePointerDown = (event: PointerEvent) => {
      if (!previewRef.current) return
      const target = event.target instanceof Node ? event.target : null
      if (!target) return
      if (cardRef.current?.contains(target)) return
      if (findMention(target)) return
      closePreview()
    }

    container.addEventListener('mouseover', handleMouseOver)
    container.addEventListener('mouseout', handleMouseOut)
    container.addEventListener('click', handleClick)
    container.addEventListener('scroll', handleScroll, true)
    document.addEventListener('keydown', handleKeyDown)
    document.addEventListener('pointerdown', handleOutsidePointerDown)
    return () => {
      clearTimers()
      container.removeEventListener('mouseover', handleMouseOver)
      container.removeEventListener('mouseout', handleMouseOut)
      container.removeEventListener('click', handleClick)
      container.removeEventListener('scroll', handleScroll, true)
      document.removeEventListener('keydown', handleKeyDown)
      document.removeEventListener('pointerdown', handleOutsidePointerDown)
    }
  }, [clearTimers, closePreview, containerRef, editable, enabled, itemById, onOpenItem, projectId, scheduleClose])

  if (!enabled || !preview) return null
  const item = itemById.get(preview.itemId)
  if (!item) return null

  const position = computePopoverPosition(preview.anchorRect)
  const image = loreImageURL(projectId, item)
  const categoryLabel = categories.find((category) => category.id === item.type)?.name || loreTypeLabel(item.type, t)
  const excerpt = loreLensExcerpt(item)
  const actorSummary = getActorSummary?.(item) || null

  const handleCopyName = async () => {
    try {
      await navigator.clipboard.writeText(item.name)
      setCopiedId(item.id)
      window.setTimeout(() => setCopiedId((current) => (current === item.id ? null : current)), 1500)
    } catch (error) {
      console.warn('[lore-lens] failed to copy lore name', error)
    }
  }

  return createPortal(
    <div
      ref={cardRef}
      role="dialog"
      aria-label={t('lore.lens.previewTitle', { name: item.name })}
      data-testid="lore-lens-card"
      data-lore-lens-card-id={item.id}
      className="fixed z-50 w-[min(312px,calc(100vw-24px))] rounded-lg border border-[var(--nova-border)] bg-[var(--nova-menu-bg)] p-3 text-[var(--nova-text)] shadow-xl backdrop-blur"
      style={{ top: position.top, left: position.left }}
      onMouseEnter={() => {
        if (closeTimerRef.current !== null) {
          window.clearTimeout(closeTimerRef.current)
          closeTimerRef.current = null
        }
      }}
      onMouseLeave={scheduleClose}
      onPointerDown={(event) => {
        // Keep writing editor focus intact when interacting with the preview card.
        if (editable) event.preventDefault()
      }}
    >
      <div className="flex items-start gap-2.5">
        {image ? (
          <div className="h-14 w-11 shrink-0 overflow-hidden rounded-md border border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)]">
            <MaterialImage src={image} alt={item.image?.alt_text || item.name} className="h-full w-full object-cover" />
          </div>
        ) : null}
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="min-w-0 truncate text-sm font-medium text-[var(--nova-text)]">{item.name}</span>
            <Badge variant="secondary" className="h-5 px-1.5 text-[10px] font-normal">
              {categoryLabel}
            </Badge>
            {item.importance === 'major' ? (
              <Badge variant="outline" className="h-5 px-1.5 text-[10px] font-normal">
                {loreImportanceLabel(item.importance, t)}
              </Badge>
            ) : null}
          </div>
          {excerpt ? (
            <p className="line-clamp-3 whitespace-pre-wrap text-xs leading-relaxed text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">
              {excerpt}
            </p>
          ) : (
            <p className="text-xs text-[var(--nova-text-faint)]">{t('lore.lens.emptyBrief')}</p>
          )}
        </div>
      </div>

      {actorSummary ? (
        <div className="mt-2 truncate rounded-md bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]" title={actorSummary}>
          {actorSummary}
        </div>
      ) : null}

      <div className="mt-2.5 flex items-center justify-end gap-1.5 border-t border-[var(--nova-border-soft)] pt-2">
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={() => { void handleCopyName() }}
        >
          {copiedId === item.id ? <Check data-icon="inline-start" /> : <Copy data-icon="inline-start" />}
          {t(copiedId === item.id ? 'lore.lens.copied' : 'lore.lens.copyName')}
        </Button>
        {onOpenItem ? (
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() => {
              closePreview()
              console.info('[lore-lens] opening lore item from preview card', { projectId, itemId: item.id })
              onOpenItem(item.id)
            }}
          >
            {t('lore.lens.openItem')}
            <ArrowUpRight data-icon="inline-end" />
          </Button>
        ) : null}
      </div>
    </div>,
    document.body,
  )
}

function computePopoverPosition(rect: DOMRect) {
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  const width = Math.min(CARD_WIDTH, viewportWidth - VIEWPORT_MARGIN * 2)
  const left = Math.max(
    VIEWPORT_MARGIN,
    Math.min(rect.left, viewportWidth - width - VIEWPORT_MARGIN),
  )
  const spaceBelow = viewportHeight - rect.bottom - VIEWPORT_MARGIN
  const top = spaceBelow >= CARD_ESTIMATED_HEIGHT || spaceBelow >= rect.top
    ? Math.min(rect.bottom + ANCHOR_GAP, viewportHeight - VIEWPORT_MARGIN - 80)
    : Math.max(VIEWPORT_MARGIN, rect.top - CARD_ESTIMATED_HEIGHT - ANCHOR_GAP)
  return { top, left }
}
