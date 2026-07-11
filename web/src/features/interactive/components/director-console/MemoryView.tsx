import { BookMarked, Edit3, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { StoryMemoryRecord, StoryMemoryStructure } from '../../types'
import { MemoryChip, SyncBadge } from './shared'
import { allStructuresId } from './types'
import { formatShortDate, recordFieldValue, storyMemoryEnabled, storyMemoryRecordTitle } from './utils'

export function MemoryView({
  loadError,
  memoryStatus,
  memorySyncError,
  memoryLoading,
  structures,
  filteredRecords,
  visibleStructures,
  structureRecordCounts,
  selectedStructureId,
  onSelectStructure,
  query,
  onQueryChange,
  onOpenMemoryManager,
}: {
  loadError?: string
  memoryStatus?: string
  memorySyncError?: string
  memoryLoading: boolean
  structures: StoryMemoryStructure[]
  filteredRecords: StoryMemoryRecord[]
  visibleStructures: StoryMemoryStructure[]
  structureRecordCounts: Map<string, number>
  selectedStructureId: string
  onSelectStructure: (structureId: string) => void
  query: string
  onQueryChange: (value: string) => void
  onOpenMemoryManager?: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-full flex-col gap-4">
      <section className="overflow-hidden rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)]">
        <div className="flex min-w-0 items-start justify-between gap-3 px-3 py-3.5">
          <div className="flex min-w-0 items-start gap-2.5">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--director-brass)]">
              <BookMarked className="h-3.5 w-3.5" />
            </span>
            <div className="min-w-0">
              <h3 className="director-console__display truncate text-base font-semibold leading-5 text-[var(--nova-text)]">{t('memoryPanel.memoryTitle')}</h3>
              <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('memoryPanel.memoryHint')}</p>
            </div>
          </div>
          <SyncBadge status={memoryStatus} error={memorySyncError} loading={memoryStatus === 'pending' || memoryStatus === 'running'} />
        </div>
        <div className="grid grid-cols-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <MemoryMetric label={t('memoryPanel.memoryRecords')} value={filteredRecords.length} />
          <MemoryMetric label={t('memoryPanel.memoryStructures')} value={structures.length} />
        </div>
      </section>

      {loadError || memorySyncError ? (
        <div className="rounded-[10px] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs leading-5 text-[var(--nova-danger)]">{loadError || memorySyncError}</div>
      ) : null}

      <div className="flex items-center gap-2">
        <label className="flex h-9 min-w-[8rem] flex-1 items-center gap-2 rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-2.5 text-xs text-[var(--nova-text-muted)] focus-within:border-[var(--director-brass)] focus-within:ring-2 focus-within:ring-[color-mix(in_srgb,var(--director-brass)_16%,transparent)]">
          <Search className="h-3.5 w-3.5 shrink-0" />
          <input value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder={t('memoryPanel.search')} className="min-w-0 flex-1 bg-transparent text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]" />
        </label>
        <button
          type="button"
          aria-label={t('memoryPanel.openManager')}
          title={t('memoryPanel.openManager')}
          className="inline-flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-[10px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-2.5 text-xs font-medium text-[var(--nova-text-muted)] transition-colors hover:border-[var(--director-brass)] hover:text-[var(--nova-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--director-brass)] disabled:opacity-45"
          onClick={onOpenMemoryManager}
          disabled={!onOpenMemoryManager}
        >
          <Edit3 className="h-3.5 w-3.5" />
          <span>{t('memoryPanel.manage')}</span>
        </button>
      </div>

      <div className="-mx-1 overflow-x-auto px-1 pb-0.5" aria-label={t('memoryPanel.structureTabs')} data-testid="memory-panel-structure-tabs">
        <div className="flex w-max min-w-full gap-1.5">
          <StructureTab
            active={selectedStructureId === allStructuresId}
            label={t('memoryPanel.allStructures')}
            count={filteredRecords.length}
            onClick={() => onSelectStructure(allStructuresId)}
          />
          {structures.map((structure) => (
            <StructureTab
              key={structure.id}
              active={selectedStructureId === structure.id}
              label={structure.name || structure.id}
              count={structureRecordCounts.get(structure.id) || 0}
              onClick={() => onSelectStructure(structure.id)}
            />
          ))}
        </div>
      </div>

      {memoryLoading ? (
        <MemoryEmpty text={t('memoryPanel.loading')} />
      ) : filteredRecords.length === 0 ? (
        <MemoryEmpty text={query.trim() ? t('memoryPanel.noMatches') : t('memoryPanel.empty')} />
      ) : (
        <div className="space-y-6">
          {visibleStructures.map((structure) => {
            const records = filteredRecords.filter((record) => record.structure_id === structure.id)
            if (records.length === 0) {
              if (selectedStructureId === allStructuresId) return null
              return (
                <section key={structure.id} className="space-y-3">
                  <MemoryStructureHeader structure={structure} count={0} />
                  <MemoryEmpty text={t('memoryPanel.tableEmpty')} compact />
                </section>
              )
            }
            return (
              <section key={structure.id} className="space-y-3">
                <MemoryStructureHeader structure={structure} count={records.length} />
                <div className="space-y-2.5">
                  {records.map((record) => (
                    <MemoryRecordCard key={record.id} record={record} structure={structure} />
                  ))}
                </div>
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}

function MemoryMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="border-r border-[var(--nova-border)] px-3 py-2 last:border-r-0">
      <div className="font-mono text-sm tabular-nums text-[var(--nova-text)]">{value}</div>
      <div className="mt-0.5 text-[9px] uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{label}</div>
    </div>
  )
}

function StructureTab({ active, label, count, onClick }: { active: boolean; label: string; count: number; onClick: () => void }) {
  return (
    <button
      type="button"
      className={`inline-flex h-8 max-w-[180px] shrink-0 items-center gap-1.5 rounded-full border px-2.5 text-[10px] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--director-brass)] ${active ? 'border-[var(--director-brass)] bg-[color-mix(in_srgb,var(--director-brass)_10%,var(--nova-surface))] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--nova-text-muted)] hover:border-[color-mix(in_srgb,var(--director-brass)_45%,var(--nova-border))] hover:text-[var(--nova-text)]'}`}
      aria-label={`${label} ${count}`}
      aria-pressed={active}
      onClick={onClick}
    >
      <span className="min-w-0 truncate">{label}</span>
      <span className="shrink-0 font-mono text-[9px] opacity-65">{count}</span>
    </button>
  )
}

function MemoryStructureHeader({ structure, count }: { structure: StoryMemoryStructure; count: number }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-w-0 items-end justify-between gap-3 border-b border-[var(--nova-border)] px-0.5 pb-2">
      <div className="min-w-0">
        <h3 className="director-console__display truncate text-base font-semibold text-[var(--nova-text)]">{structure.name || structure.id}</h3>
        {structure.description ? <p className="mt-0.5 line-clamp-1 break-words text-[10px] leading-4 text-[var(--nova-text-faint)] [overflow-wrap:anywhere]">{structure.description}</p> : null}
      </div>
      <span className="shrink-0 font-mono text-[9px] text-[var(--nova-text-faint)]">{t('memoryPanel.recordCount', { count })}</span>
    </div>
  )
}

function MemoryRecordCard({ record, structure }: { record: StoryMemoryRecord; structure: StoryMemoryStructure }) {
  const { t } = useTranslation()
  const enabledFields = structure.fields.filter((field) => storyMemoryEnabled(field.enabled))
  const fields = enabledFields.length ? enabledFields : [{ id: 'value', name: t('storyMemory.value'), order: 10 }]
  const displayFields = fields.filter((field) => recordFieldValue(record, field.id).trim()).slice(0, 4)
  const visibleFields = displayFields.length > 0 ? displayFields : fields.slice(0, 1)
  return (
    <article className={`memory-ledger-card relative overflow-hidden rounded-[11px] border border-[var(--nova-border)] bg-[var(--director-panel)] px-3 pb-3 pt-3.5 ${record.archived ? 'opacity-55' : ''}`}>
      <div className="min-w-0">
        <div className="flex min-w-0 items-start justify-between gap-2">
          <h4 className="director-console__display min-w-0 break-words text-[15px] font-semibold leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{storyMemoryRecordTitle(record, structure, t('storyMemory.untitled'))}</h4>
          {record.updated_at ? <span className="shrink-0 font-mono text-[8px] text-[var(--nova-text-faint)]">{formatShortDate(record.updated_at)}</span> : null}
        </div>
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {record.manual ? <MemoryChip>{t('storyMemory.manual')}</MemoryChip> : null}
          {record.inherited_from ? <MemoryChip>{t('storyMemory.inherited')}</MemoryChip> : null}
          {record.archived ? <MemoryChip>{t('memoryPanel.archived')}</MemoryChip> : null}
        </div>
      </div>
      <div className="mt-3 divide-y divide-[var(--nova-border-soft)] border-t border-[var(--nova-border-soft)]">
        {visibleFields.map((field) => (
          <section key={field.id} className="grid grid-cols-[minmax(68px,0.72fr)_minmax(0,1.6fr)] gap-2 py-2 first:pt-2.5 last:pb-0">
            <div className="truncate text-[9px] font-medium uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{field.name || field.id}</div>
            <p className="line-clamp-5 whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{recordFieldValue(record, field.id) || t('storyMemory.noValue')}</p>
          </section>
        ))}
      </div>
    </article>
  )
}

function MemoryEmpty({ text, compact = false }: { text: string; compact?: boolean }) {
  return (
    <div className={`flex items-center justify-center rounded-[11px] border border-dashed border-[var(--nova-border)] bg-[var(--director-panel)] px-4 text-center text-xs leading-5 text-[var(--nova-text-faint)] ${compact ? 'min-h-[104px]' : 'min-h-[220px]'}`}>
      {text}
    </div>
  )
}
