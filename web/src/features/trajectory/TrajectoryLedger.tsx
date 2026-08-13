import { useEffect, useMemo, useState } from 'react'
import { Activity, Bot, Braces, ChevronRight, Database, Hammer, Search, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import type { TrajectoryAnalysis, TrajectoryCategory, TrajectoryEventRecord, TrajectorySpan } from './trajectory-analysis'
import { formatTrajectoryDuration, visibleTreeSpanIDs } from './trajectory-analysis'

export type TrajectoryFilter = 'all' | 'model' | 'tool' | 'context' | 'errors'
export type TrajectoryLedgerView = 'tree' | 'events'

interface TrajectoryLedgerProps {
  analysis: TrajectoryAnalysis
  selectedSpanID: string
  selectedEventID: string
  rangeSpanIDs: ReadonlySet<string> | null
  onSpanSelect: (spanID: string) => void
  onEventSelect: (eventID: string) => void
}

/** Searchable semantic tree and full auxiliary event ledger for one trace. */
export function TrajectoryLedger({
  analysis,
  selectedSpanID,
  selectedEventID,
  rangeSpanIDs,
  onSpanSelect,
  onEventSelect,
}: TrajectoryLedgerProps) {
  const { t } = useTranslation()
  const [view, setView] = useState<TrajectoryLedgerView>('tree')
  const [filter, setFilter] = useState<TrajectoryFilter>('all')
  const [query, setQuery] = useState('')
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set())

  useEffect(() => {
    setExpanded(new Set(analysis.spans.filter((span) => span.children.length > 0).map((span) => span.id)))
    setQuery('')
    setFilter('all')
    setView('tree')
  }, [analysis])

  const normalizedQuery = query.trim().toLowerCase()
  const matchedSpanIDs = useMemo(() => {
    const directMatches = new Set(analysis.spans.filter((span) => (
      matchesSpanFilter(span, filter)
      && (!rangeSpanIDs || rangeSpanIDs.has(span.id) || span.category === 'run')
      && (!normalizedQuery || spanSearchText(span).includes(normalizedQuery))
    )).map((span) => span.id))
    return visibleTreeSpanIDs(analysis.roots, directMatches)
  }, [analysis.roots, analysis.spans, filter, normalizedQuery, rangeSpanIDs])
  const matchedEvents = useMemo(() => analysis.events.filter((event) => (
    matchesEventFilter(event, filter)
    && (!normalizedQuery || eventSearchText(event).includes(normalizedQuery))
  )), [analysis.events, filter, normalizedQuery])

  const filterItems: Array<{ id: TrajectoryFilter; label: string; count: number }> = [
    { id: 'all', label: t('trajectory.filter.all'), count: analysis.spans.length },
    { id: 'model', label: t('trajectory.filter.model'), count: analysis.spans.filter((span) => span.category === 'model').length },
    { id: 'tool', label: t('trajectory.filter.tools'), count: analysis.spans.filter((span) => span.category === 'tool').length },
    { id: 'context', label: t('trajectory.filter.context'), count: analysis.spans.filter((span) => span.category === 'context').length },
    { id: 'errors', label: t('trajectory.filter.errors'), count: analysis.spans.filter((span) => isErrorStatus(span.status)).length },
  ]

  const toggleExpanded = (id: string) => {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col border-r border-[var(--nova-border)]" aria-label={t('trajectory.ledger.title')}>
      <div className="flex min-h-10 shrink-0 flex-wrap items-center gap-1.5 border-b border-[var(--nova-border)] px-2 py-1.5">
        <div className="flex rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5">
          <Button type="button" size="xs" variant="ghost" className={cn('h-6 px-2', view === 'tree' && 'bg-[var(--nova-active)]')} onClick={() => setView('tree')}>
            {t('trajectory.ledger.tree')}
          </Button>
          <Button type="button" size="xs" variant="ghost" className={cn('h-6 px-2', view === 'events' && 'bg-[var(--nova-active)]')} onClick={() => setView('events')}>
            {t('trajectory.ledger.events', { count: analysis.events.length })}
          </Button>
        </div>
        {view === 'tree' && (
          <>
            <Button type="button" size="xs" variant="ghost" onClick={() => setExpanded(new Set(analysis.spans.map((span) => span.id)))}>
              {t('trajectory.ledger.expandAll')}
            </Button>
            <Button type="button" size="xs" variant="ghost" onClick={() => setExpanded(new Set())}>
              {t('trajectory.ledger.collapseAll')}
            </Button>
          </>
        )}
        <label className="relative ml-auto min-w-36 flex-1 sm:max-w-56">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-[var(--nova-text-faint)]" />
          <Input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
            placeholder={t('trajectory.search.placeholder')}
            aria-label={t('trajectory.search.label')}
            className="h-7 pl-7 text-[11px]"
          />
        </label>
      </div>
      <div className="flex shrink-0 gap-1 overflow-x-auto border-b border-[var(--nova-border-soft)] px-2 py-1.5">
        {filterItems.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => setFilter(item.id)}
            aria-pressed={filter === item.id}
            className={cn(
              'inline-flex h-6 shrink-0 items-center gap-1 rounded-[var(--nova-radius)] border px-2 text-[10px] transition-colors',
              filter === item.id
                ? 'border-[var(--nova-border-strong)] bg-[var(--nova-active)] text-[var(--nova-text)]'
                : 'border-[var(--nova-border-soft)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]',
            )}
          >
            {item.label}<span className="font-mono text-[var(--nova-text-faint)]">{item.count}</span>
          </button>
        ))}
      </div>
      {rangeSpanIDs && view === 'tree' && (
        <div className="shrink-0 border-b border-[var(--nova-border-soft)] bg-[var(--nova-selection-bg)] px-3 py-1 text-[10px] text-[var(--nova-text-muted)]">
          {t('trajectory.ledger.rangeFocus', { count: rangeSpanIDs.size })}
        </div>
      )}
      <div className="min-h-0 flex-1 overflow-auto bg-[var(--nova-surface)] p-2">
        {view === 'tree' ? (
          analysis.roots.some((root) => matchedSpanIDs.has(root.id)) ? (
            <div role="tree" aria-label={t('trajectory.ledger.tree')}>
              {analysis.roots.map((root) => matchedSpanIDs.has(root.id) ? (
                <TrajectoryTreeNode
                  key={root.id}
                  span={root}
                  depth={0}
                  expanded={expanded}
                  forceExpanded={Boolean(normalizedQuery || rangeSpanIDs)}
                  visibleIDs={matchedSpanIDs}
                  selectedSpanID={selectedSpanID}
                  onToggle={toggleExpanded}
                  onSelect={onSpanSelect}
                />
              ) : null)}
            </div>
          ) : <EmptyLedger />
        ) : matchedEvents.length > 0 ? (
          <div className="space-y-1">
            {matchedEvents.map((event) => (
              <EventRow key={event.id} event={event} selected={selectedEventID === event.id} onSelect={() => onEventSelect(event.id)} />
            ))}
          </div>
        ) : <EmptyLedger />}
      </div>
    </section>
  )
}

function TrajectoryTreeNode({
  span,
  depth,
  expanded,
  forceExpanded,
  visibleIDs,
  selectedSpanID,
  onToggle,
  onSelect,
}: {
  span: TrajectorySpan
  depth: number
  expanded: ReadonlySet<string>
  forceExpanded: boolean
  visibleIDs: ReadonlySet<string>
  selectedSpanID: string
  onToggle: (id: string) => void
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation()
  const hasChildren = span.children.some((child) => visibleIDs.has(child.id))
  const isExpanded = forceExpanded || expanded.has(span.id)
  const Icon = categoryIcon(span.category)
  const selected = selectedSpanID === span.id
  const secondary = span.category === 'model'
    ? t('trajectory.row.modelMeta', { tokens: span.inputTokens + span.outputTokens, ttft: formatTrajectoryDuration(span.ttftMs) })
    : span.category === 'tool'
      ? t('trajectory.row.toolMeta', { gap: formatTrajectoryDuration(span.gapBeforeMs) })
      : span.type
  const activate = () => {
    onSelect(span.id)
    if (hasChildren) onToggle(span.id)
  }
  return (
    <div role="treeitem" aria-level={depth + 1} aria-expanded={hasChildren ? isExpanded : undefined}>
      <button
        type="button"
        onClick={activate}
        aria-current={selected ? 'true' : undefined}
        className={cn(
          'group mb-1 grid w-full grid-cols-[20px_22px_minmax(0,1fr)_auto] items-center gap-1 rounded-[var(--nova-radius)] border px-2 py-1.5 text-left transition-colors',
          selected
            ? 'border-[var(--nova-border-strong)] bg-[var(--nova-active)]'
            : 'border-transparent hover:border-[var(--nova-border-soft)] hover:bg-[var(--nova-hover)]',
        )}
        style={{ paddingLeft: `${8 + depth * 16}px` }}
      >
        <ChevronRight className={cn('size-3 text-[var(--nova-tree-chevron)] transition-transform', !hasChildren && 'opacity-0', isExpanded && 'rotate-90')} />
        <span className={cn('grid size-5 place-items-center rounded-[5px] border', iconTone(span.category, span.status))}>
          <Icon className="size-3" />
        </span>
        <span className="min-w-0">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="truncate text-[11px] font-medium text-[var(--nova-text)]">{span.label}</span>
            <span className="shrink-0 font-mono text-[9px] uppercase text-[var(--nova-text-faint)]">{span.status}</span>
          </span>
          <span className="block truncate text-[9px] text-[var(--nova-text-faint)]">{secondary}</span>
        </span>
        <span className="flex items-center gap-1 font-mono text-[9px] text-[var(--nova-text-muted)]">
          {span.gapBeforeMs > 0 && <span className="text-[var(--nova-warning)]">+{formatTrajectoryDuration(span.gapBeforeMs)}</span>}
          <span>{formatTrajectoryDuration(span.durationMs)}</span>
        </span>
      </button>
      {hasChildren && isExpanded && (
        <div role="group">
          {span.children.map((child) => visibleIDs.has(child.id) ? (
            <TrajectoryTreeNode
              key={child.id}
              span={child}
              depth={depth + 1}
              expanded={expanded}
              forceExpanded={forceExpanded}
              visibleIDs={visibleIDs}
              selectedSpanID={selectedSpanID}
              onToggle={onToggle}
              onSelect={onSelect}
            />
          ) : null)}
        </div>
      )}
    </div>
  )
}

function EventRow({ event, selected, onSelect }: { event: TrajectoryEventRecord; selected: boolean; onSelect: () => void }) {
  const Icon = categoryIcon(event.category)
  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        'grid w-full grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-2 rounded-[var(--nova-radius)] border px-2 py-1.5 text-left',
        selected ? 'border-[var(--nova-border-strong)] bg-[var(--nova-active)]' : 'border-transparent hover:border-[var(--nova-border-soft)] hover:bg-[var(--nova-hover)]',
      )}
    >
      <span className={cn('grid size-5 place-items-center rounded-[5px] border', iconTone(event.category, event.status))}><Icon className="size-3" /></span>
      <span className="min-w-0">
        <span className="block truncate text-[11px] text-[var(--nova-text)]">{event.label}</span>
        <span className="block truncate font-mono text-[9px] text-[var(--nova-text-faint)]">{event.record.type}</span>
      </span>
      <span className="font-mono text-[9px] text-[var(--nova-text-faint)]">{formatClock(event.timestamp)}</span>
    </button>
  )
}

function EmptyLedger() {
  const { t } = useTranslation()
  return <div className="grid h-32 place-items-center text-[11px] text-[var(--nova-text-faint)]">{t('trajectory.ledger.noMatches')}</div>
}

function matchesSpanFilter(span: TrajectorySpan, filter: TrajectoryFilter) {
  if (filter === 'all') return true
  if (filter === 'errors') return isErrorStatus(span.status)
  return span.category === filter
}

function matchesEventFilter(event: TrajectoryEventRecord, filter: TrajectoryFilter) {
  if (filter === 'all') return true
  if (filter === 'errors') return isErrorStatus(event.status)
  return event.category === filter
}

function spanSearchText(span: TrajectorySpan) {
  return `${span.label} ${span.type} ${span.status} ${JSON.stringify(span.attrs)}`.toLowerCase()
}

function eventSearchText(event: TrajectoryEventRecord) {
  return `${event.label} ${event.record.type} ${event.status} ${JSON.stringify(event.data)}`.toLowerCase()
}

function categoryIcon(category: TrajectoryCategory) {
  if (category === 'run') return Bot
  if (category === 'model') return Activity
  if (category === 'tool') return Hammer
  if (category === 'context') return Database
  if (category === 'verification') return ShieldCheck
  return Braces
}

function iconTone(category: TrajectoryCategory, status: string) {
  if (isErrorStatus(status)) return 'border-[var(--nova-danger)] text-[var(--nova-danger)]'
  if (category === 'tool') return 'border-[var(--nova-success)] text-[var(--nova-success)]'
  if (category === 'context' || category === 'verification') return 'border-[var(--nova-warning)] text-[var(--nova-warning)]'
  return 'border-[var(--nova-border-soft)] text-[var(--nova-text-muted)]'
}

function isErrorStatus(status: string) {
  return ['error', 'failed', 'blocked', 'aborted'].includes(status.toLowerCase())
}

function formatClock(timestamp: number) {
  if (!timestamp) return '—'
  return new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
