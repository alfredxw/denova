import { useMemo, useState } from 'react'
import { Bot, Braces, Database, LockKeyhole, Search, UserRound, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'
import type { TrajectoryContentAnalysis, TrajectoryContentEntry, TrajectoryContentKind } from './trajectory-content'

type ContentFilter = 'all' | TrajectoryContentKind

interface TrajectoryContentLedgerProps {
  content: TrajectoryContentAnalysis
  selectedEntryID: string
  rangeSpanIDs: ReadonlySet<string> | null
  onSelect: (entryID: string) => void
}

/** Searchable model-visible message ledger grouped by model request. */
export function TrajectoryContentLedger({ content, selectedEntryID, rangeSpanIDs, onSelect }: TrajectoryContentLedgerProps) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState<ContentFilter>('all')
  const [query, setQuery] = useState('')
  const normalizedQuery = query.trim().toLowerCase()
  const visibleEntries = useMemo(() => content.entries.filter((entry) => (
    (filter === 'all' || entry.kind === filter)
    && (!rangeSpanIDs || !entry.span || rangeSpanIDs.has(entry.span.id))
    && (!normalizedQuery || contentSearchText(entry).includes(normalizedQuery))
  )), [content.entries, filter, normalizedQuery, rangeSpanIDs])
  const visibleIDs = new Set(visibleEntries.map((entry) => entry.id))
  const filterItems: Array<{ id: ContentFilter; count: number }> = (['all', 'system', 'user', 'context', 'assistant', 'tool'] as const).map((id) => ({
    id,
    count: id === 'all' ? content.entries.length : content.entries.filter((entry) => entry.kind === id).length,
  }))

  if (!content.available) {
    return (
      <section className="flex min-h-0 min-w-0 flex-1 items-center justify-center border-r border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 text-center" aria-label={t('trajectory.records.title')}>
        <div className="max-w-md">
          <span className="mx-auto grid size-9 place-items-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)]"><LockKeyhole className="size-4 text-[var(--nova-text-faint)]" /></span>
          <div className="mt-3 text-xs font-medium text-[var(--nova-text)]">{t('trajectory.records.unavailable')}</div>
          <p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('trajectory.records.unavailableDescription')}</p>
        </div>
      </section>
    )
  }

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col border-r border-[var(--nova-border)]" aria-label={t('trajectory.records.title')}>
      <div className="flex min-h-10 shrink-0 flex-wrap items-center gap-1.5 border-b border-[var(--nova-border)] px-2 py-1.5">
        <div className="flex min-w-0 flex-1 gap-1 overflow-x-auto">
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
                  : 'border-transparent text-[var(--nova-text-muted)] hover:border-[var(--nova-border-soft)] hover:bg-[var(--nova-hover)]',
              )}
            >
              {t(`trajectory.records.filter.${item.id}`)}<span className="font-mono text-[var(--nova-text-faint)]">{item.count}</span>
            </button>
          ))}
        </div>
        <label className="relative min-w-36 flex-1 sm:max-w-56">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-[var(--nova-text-faint)]" />
          <Input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
            placeholder={t('trajectory.records.searchPlaceholder')}
            aria-label={t('trajectory.records.search')}
            className="h-7 pl-7 text-[11px]"
          />
        </label>
      </div>
      {rangeSpanIDs && (
        <div className="shrink-0 border-b border-[var(--nova-border-soft)] bg-[var(--nova-selection-bg)] px-3 py-1 text-[10px] text-[var(--nova-text-muted)]">
          {t('trajectory.records.rangeFocus', { count: visibleEntries.length })}
        </div>
      )}
      <div className="min-h-0 flex-1 overflow-auto bg-[var(--nova-surface)]">
        {content.requests.map((request) => {
          const entries = request.entries.filter((entry) => visibleIDs.has(entry.id))
          if (entries.length === 0) return null
          return (
            <div key={request.id}>
              <div className="sticky top-0 z-10 flex h-7 items-center border-y border-[var(--nova-border-soft)] bg-[color-mix(in_srgb,var(--nova-surface-2)_92%,transparent)] px-3 backdrop-blur-sm">
                <span className="font-mono text-[9px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">{t('trajectory.records.request', { index: request.index })}</span>
                <span className="ml-auto truncate font-mono text-[9px] text-[var(--nova-text-faint)]">{request.id}</span>
              </div>
              <div role="list" aria-label={t('trajectory.records.request', { index: request.index })}>
                {entries.map((entry) => (
                  <ContentRow key={entry.id} entry={entry} selected={selectedEntryID === entry.id} onSelect={() => onSelect(entry.id)} />
                ))}
              </div>
            </div>
          )
        })}
        {visibleEntries.length === 0 && <div className="grid h-32 place-items-center text-[11px] text-[var(--nova-text-faint)]">{t('trajectory.ledger.noMatches')}</div>}
      </div>
    </section>
  )
}

function ContentRow({ entry, selected, onSelect }: { entry: TrajectoryContentEntry; selected: boolean; onSelect: () => void }) {
  const { t } = useTranslation()
  const Icon = contentIcon(entry.kind)
  const span = entry.span
  const label = localizedEntryLabel(entry, t)
  const meta = entry.kind === 'system'
    ? t('trajectory.records.toolsCount', { count: entry.tools.length })
    : entry.kind === 'assistant' && span
      ? t('trajectory.records.assistantMeta', { tokens: span.outputTokens, duration: formatTrajectoryDuration(span.durationMs) })
      : entry.kind === 'tool'
        ? entry.toolCallID || entry.toolName
        : t(`trajectory.records.kind.${entry.kind}`)
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-label={`${t(`trajectory.records.kind.${entry.kind}`)} · ${label}`}
      aria-current={selected ? 'true' : undefined}
      className={cn(
        'grid w-full grid-cols-[76px_minmax(0,1fr)_auto] items-start gap-3 border-b border-[var(--nova-border-soft)] px-3 py-2.5 text-left transition-colors',
        selected ? 'bg-[var(--nova-active)]' : 'hover:bg-[var(--nova-hover)]',
      )}
    >
      <span className={cn('mt-0.5 inline-flex items-center gap-1.5 text-[9px] font-semibold uppercase tracking-[0.08em]', contentTone(entry.kind))}>
        <Icon className="size-3" />{t(`trajectory.records.kind.${entry.kind}`)}
      </span>
      <span className="min-w-0">
        <span className="block truncate text-[11px] font-medium text-[var(--nova-text)]">{label}</span>
        <span className="mt-0.5 block truncate text-[10px] leading-4 text-[var(--nova-text-faint)]">{entryPreview(entry) || t('trajectory.records.noText')}</span>
      </span>
      <span className="max-w-36 truncate pt-0.5 font-mono text-[9px] text-[var(--nova-text-faint)]">{meta}</span>
    </button>
  )
}

function contentIcon(kind: TrajectoryContentKind) {
  if (kind === 'system') return Braces
  if (kind === 'user') return UserRound
  if (kind === 'context') return Database
  if (kind === 'tool') return Wrench
  return Bot
}

function contentTone(kind: TrajectoryContentKind) {
  if (kind === 'tool') return 'text-[var(--nova-success)]'
  if (kind === 'context') return 'text-[var(--nova-warning)]'
  return 'text-[var(--nova-text-muted)]'
}

function entryPreview(entry: TrajectoryContentEntry) {
  if (entry.content) return entry.content.replaceAll(/\s+/g, ' ').trim()
  if (entry.reasoning) return entry.reasoning.replaceAll(/\s+/g, ' ').trim()
  if (entry.toolCalls.length > 0) return entry.toolCalls.map((call) => call.name).join(', ')
  if (entry.kind === 'system') return entry.tools.map((tool) => tool.name).join(', ')
  return ''
}

function contentSearchText(entry: TrajectoryContentEntry) {
  return `${entry.kind} ${entry.label} ${entry.content} ${entry.reasoning} ${entry.toolName} ${entry.tools.map((tool) => tool.name).join(' ')}`.toLowerCase()
}

function localizedEntryLabel(entry: TrajectoryContentEntry, t: (key: string, options?: Record<string, unknown>) => string) {
  if (entry.kind === 'system') return t(entry.previousContent || entry.previousTools.length > 0 ? 'trajectory.records.label.systemUpdate' : 'trajectory.records.label.initialSystem')
  if (entry.kind === 'assistant') return entry.label === 'Assistant History' ? t('trajectory.records.label.assistantHistory') : t('trajectory.records.request', { index: entry.requestIndex })
  if (entry.kind === 'user') return t('trajectory.records.label.user')
  if (entry.kind === 'tool') return entry.toolName || t('trajectory.records.label.toolResult')
  return t(entry.label === 'Input Snapshot Changed' ? 'trajectory.records.label.snapshotChanged' : 'trajectory.records.label.context')
}
