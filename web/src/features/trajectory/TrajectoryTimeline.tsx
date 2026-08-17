import { useEffect, useMemo, useRef, useState } from 'react'
import { Maximize2, ZoomIn } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import type { TimelineProjection, TimelineSpan } from './trajectory-analysis'
import { formatTrajectoryDuration } from './trajectory-analysis'

export interface TrajectoryRange {
  start: number
  end: number
}

interface TrajectoryTimelineProps {
  projection: TimelineProjection
  selectedSpanID: string
  range: TrajectoryRange | null
  onRangeChange: (range: TrajectoryRange | null) => void
  onSpanSelect: (spanID: string) => void
}

type PointerGesture =
  | { kind: 'select'; pointerID: number; anchor: number }
  | { kind: 'pan'; pointerID: number; anchorX: number; viewport: TrajectoryRange }

const MIN_VIEWPORT_SHARE = 0.02

/** Three-lane overview with duration/TTFT segments, range focus, wheel zoom, and right-drag pan. */
export function TrajectoryTimeline({
  projection,
  selectedSpanID,
  range,
  onRangeChange,
  onSpanSelect,
}: TrajectoryTimelineProps) {
  const { t } = useTranslation()
  const trackRef = useRef<HTMLDivElement | null>(null)
  const gestureRef = useRef<PointerGesture | null>(null)
  const [draftRange, setDraftRange] = useState<TrajectoryRange | null>(null)
  const [viewport, setViewport] = useState<TrajectoryRange | null>(null)
  const [hoverTime, setHoverTime] = useState<number | null>(null)
  const domain = Math.max(1, projection.end - projection.start)
  const visible = normalizeViewport(viewport, projection.start, projection.end)
  const visibleDuration = Math.max(1, visible.end - visible.start)

  useEffect(() => {
    setViewport(null)
    setDraftRange(null)
  }, [projection.start, projection.end])

  const visibleSpans = useMemo(() => projection.spans.filter((span) => (
    span.start <= visible.end && span.end >= visible.start
  )), [projection.spans, visible.end, visible.start])
  const selection = draftRange ?? range
  const formatAxisValue = (value: number) => projection.unit === 'step'
    ? t('trajectory.timeline.steps', { count: Math.round(value) })
    : formatTrajectoryDuration(value)

  const timeAtClientX = (clientX: number) => {
    const rect = trackRef.current?.getBoundingClientRect()
    if (!rect || rect.width <= 0) return visible.start
    const ratio = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width))
    return visible.start + ratio * visibleDuration
  }

  const finishGesture = (event: React.PointerEvent<HTMLDivElement>) => {
    const gesture = gestureRef.current
    if (!gesture || gesture.pointerID !== event.pointerId) return
    if (gesture.kind === 'select') {
      const end = timeAtClientX(event.clientX)
      const next = { start: Math.min(gesture.anchor, end), end: Math.max(gesture.anchor, end) }
      onRangeChange(next.end - next.start < visibleDuration * 0.004 ? null : next)
      setDraftRange(null)
    }
    gestureRef.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
  }

  return (
    <section className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)]" aria-label={t('trajectory.timeline.title')}>
      <div className="flex h-9 items-center gap-2 border-b border-[var(--nova-border-soft)] px-3">
        <div className="min-w-0 flex-1">
          <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">{t('trajectory.timeline.title')}</div>
        </div>
        {range && (
          <Button type="button" size="xs" variant="ghost" onClick={() => onRangeChange(null)}>
            {formatAxisValue(Math.abs(range.end - range.start))} · {t('trajectory.timeline.clearRange')}
          </Button>
        )}
        <div className="hidden items-center gap-2 text-[9px] text-[var(--nova-text-faint)] md:flex">
          <span className="inline-flex items-center gap-1"><span className="size-1.5 rounded-full bg-[var(--nova-warning)]" />TTFT</span>
          <span className="inline-flex items-center gap-1"><span className="size-1.5 rounded-full bg-[var(--nova-text-muted)]" />{t('trajectory.field.generation')}</span>
        </div>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button type="button" size="icon-xs" variant="ghost" onClick={() => setViewport(null)} aria-label={t('trajectory.timeline.resetZoom')}>
              <Maximize2 />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('trajectory.timeline.resetZoom')}</TooltipContent>
        </Tooltip>
        <div className="hidden items-center gap-1 text-[9px] text-[var(--nova-text-faint)] sm:flex">
          <ZoomIn className="size-3" />{t('trajectory.timeline.zoomHint')}
        </div>
      </div>
      <div className="grid grid-cols-[54px_minmax(0,1fr)] px-3 py-2 sm:grid-cols-[70px_minmax(0,1fr)]">
        <div className="grid h-[78px] grid-rows-3 pr-2 text-right text-[9px] uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">
          <span className="self-center">{t('trajectory.lane.input')}</span>
          <span className="self-center">{t('trajectory.lane.model')}</span>
          <span className="self-center">{t('trajectory.lane.tools')}</span>
        </div>
        <div
          ref={trackRef}
          className="relative h-[78px] touch-none overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] select-none"
          onContextMenu={(event) => {
            event.preventDefault()
            if (!gestureRef.current) onRangeChange(null)
          }}
          onWheel={(event) => {
            event.preventDefault()
            const rect = event.currentTarget.getBoundingClientRect()
            const pointerRatio = rect.width <= 0 ? 0.5 : Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width))
            const currentDuration = visible.end - visible.start
            const minDuration = domain * MIN_VIEWPORT_SHARE
            const scale = Math.exp(event.deltaY * 0.0015)
            const nextDuration = Math.min(domain, Math.max(minDuration, currentDuration * scale))
            const pivot = visible.start + pointerRatio * currentDuration
            setViewport(clampViewport({
              start: pivot - pointerRatio * nextDuration,
              end: pivot + (1 - pointerRatio) * nextDuration,
            }, projection.start, projection.end))
          }}
          onPointerDown={(event) => {
            if (event.button !== 0 && event.button !== 2) return
            event.currentTarget.setPointerCapture(event.pointerId)
            if (event.button === 2) {
              gestureRef.current = { kind: 'pan', pointerID: event.pointerId, anchorX: event.clientX, viewport: visible }
              return
            }
            const anchor = timeAtClientX(event.clientX)
            gestureRef.current = { kind: 'select', pointerID: event.pointerId, anchor }
            setDraftRange({ start: anchor, end: anchor })
          }}
          onPointerMove={(event) => {
            setHoverTime(timeAtClientX(event.clientX))
            const gesture = gestureRef.current
            if (!gesture || gesture.pointerID !== event.pointerId) return
            if (gesture.kind === 'select') {
              const current = timeAtClientX(event.clientX)
              setDraftRange({ start: Math.min(gesture.anchor, current), end: Math.max(gesture.anchor, current) })
              return
            }
            const rect = event.currentTarget.getBoundingClientRect()
            if (rect.width <= 0) return
            const delta = (event.clientX - gesture.anchorX) / rect.width * (gesture.viewport.end - gesture.viewport.start)
            setViewport(clampViewport({ start: gesture.viewport.start - delta, end: gesture.viewport.end - delta }, projection.start, projection.end))
          }}
          onPointerUp={finishGesture}
          onPointerCancel={finishGesture}
          onPointerLeave={() => {
            if (!gestureRef.current) setHoverTime(null)
          }}
        >
          <div className="pointer-events-none absolute inset-0 grid grid-rows-3">
            <div className="border-b border-[var(--nova-border-soft)]" />
            <div className="border-b border-[var(--nova-border-soft)]" />
            <div />
          </div>
          {visibleSpans.map((span) => (
            <TimelineOperation
              key={span.id}
              span={span}
              visible={visible}
              selected={span.id === selectedSpanID}
              offset={formatAxisValue(Math.max(0, span.start - projection.start))}
              onSelect={() => onSpanSelect(span.id)}
            />
          ))}
          {selection && (
            <div
              className="pointer-events-none absolute inset-y-0 z-20 border-x border-[var(--nova-text-muted)] bg-[var(--nova-selection-bg)]"
              style={rangeStyle(selection, visible)}
            />
          )}
          {hoverTime !== null && !gestureRef.current && (
            <div
              className="pointer-events-none absolute inset-y-0 z-40 w-px bg-[var(--nova-text-muted)] opacity-60"
              style={{ left: `${(hoverTime - visible.start) / visibleDuration * 100}%` }}
            >
              <span className="absolute -top-1 left-1 whitespace-nowrap rounded-[3px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1 py-0.5 font-mono text-[8px] text-[var(--nova-text-muted)] shadow-sm">
                +{formatAxisValue(Math.max(0, hoverTime - projection.start))}
              </span>
            </div>
          )}
          {visibleSpans.length === 0 && (
            <div className="absolute inset-0 grid place-items-center text-[10px] text-[var(--nova-text-faint)]">{t('trajectory.timeline.empty')}</div>
          )}
        </div>
        <div />
        <div className="mt-1 flex justify-between font-mono text-[9px] text-[var(--nova-text-faint)]">
          <span>+{formatAxisValue(Math.max(0, visible.start - projection.start))}</span>
          <span>{formatAxisValue(visible.end - visible.start)}</span>
          <span>+{formatAxisValue(Math.max(0, visible.end - projection.start))}</span>
        </div>
      </div>
    </section>
  )
}

function TimelineOperation({
  span,
  visible,
  selected,
  offset,
  onSelect,
}: {
  span: TimelineSpan
  visible: TrajectoryRange
  selected: boolean
  offset: string
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const lane = laneIndex(span.category)
  const start = Math.max(visible.start, span.start)
  const end = Math.min(visible.end, span.end)
  const width = Math.max(0.2, (end - start) / Math.max(1, visible.end - visible.start) * 100)
  const left = (start - visible.start) / Math.max(1, visible.end - visible.start) * 100
  const hasError = ['error', 'failed', 'blocked', 'aborted'].includes(span.status.toLowerCase())
  return (
    <Tooltip delayDuration={500}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className={cn(
            'absolute z-10 h-[17px] min-w-px overflow-hidden rounded-[3px] border transition-[filter,opacity] hover:brightness-110 focus-visible:outline-none focus-visible:brightness-125',
            categoryTone(span.category),
            selected && 'z-30 brightness-125 saturate-125',
            hasError && '!border-[var(--nova-danger)]',
          )}
          style={{ left: `${left}%`, top: `${5 + lane * 26}px`, width: `${width}%` }}
          onPointerDown={(event) => event.stopPropagation()}
          onClick={onSelect}
          aria-label={`${span.label}, ${formatTrajectoryDuration(span.durationMs)}`}
        >
          {span.ttftEnd !== null && span.category === 'model' && (
            <span
              className="absolute inset-y-0 left-0 bg-[var(--nova-warning)] opacity-70"
              style={{ width: `${Math.min(100, Math.max(0, (span.ttftEnd - span.start) / Math.max(1, span.end - span.start) * 100))}%` }}
            />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" className="block">
        <div className="font-medium">{span.label}</div>
        <div className="mt-0.5 font-mono text-[10px] text-[var(--nova-text-muted)]">
          +{offset} · {t('trajectory.timeline.duration', { duration: formatTrajectoryDuration(span.durationMs) })}
          {span.ttftMs === null ? '' : ` · TTFT ${formatTrajectoryDuration(span.ttftMs)}`}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

function laneIndex(category: TimelineSpan['category']) {
  if (category === 'model') return 1
  if (category === 'tool') return 2
  return 0
}

function categoryTone(category: TimelineSpan['category']) {
  if (category === 'model') return 'border-transparent bg-[var(--nova-text-muted)]'
  if (category === 'tool') return 'border-transparent bg-[var(--nova-success-muted)]'
  return 'border-transparent bg-[var(--nova-warning-bg)]'
}

function normalizeViewport(viewport: TrajectoryRange | null, start: number, end: number) {
  return viewport ? clampViewport(viewport, start, end) : { start, end: Math.max(start + 1, end) }
}

function clampViewport(viewport: TrajectoryRange, domainStart: number, domainEnd: number) {
  const domain = Math.max(1, domainEnd - domainStart)
  const duration = Math.min(domain, Math.max(1, viewport.end - viewport.start))
  const start = Math.min(Math.max(viewport.start, domainStart), domainEnd - duration)
  return { start, end: start + duration }
}

function rangeStyle(range: TrajectoryRange, visible: TrajectoryRange) {
  const start = Math.max(visible.start, Math.min(range.start, range.end))
  const end = Math.min(visible.end, Math.max(range.start, range.end))
  const duration = Math.max(1, visible.end - visible.start)
  return {
    left: `${(start - visible.start) / duration * 100}%`,
    width: `${Math.max(0, end - start) / duration * 100}%`,
  }
}
