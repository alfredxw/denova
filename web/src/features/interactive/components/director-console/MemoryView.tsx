import { Edit3, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import type { StoryMemoryRecord, StoryMemoryStructure } from '../../types'
import { MemoryChip } from './shared'
import { allStructuresId } from './types'
import { formatShortDate, recordFieldValue, storyMemoryEnabled, storyMemoryRecordTitle } from './utils'

export function MemoryView({
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
    <div className="flex min-h-full flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex h-8 min-w-[8rem] flex-1 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-xs text-[var(--nova-text-muted)]">
          <Search className="h-3.5 w-3.5 shrink-0" />
          <input value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder={t('memoryPanel.search')} className="min-w-0 flex-1 bg-transparent text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]" />
        </label>
        <TooltipIconButton label={t('memoryPanel.openManager')} className="border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45" variant="ghost" size="icon-sm" onClick={onOpenMemoryManager} disabled={!onOpenMemoryManager}>
          <Edit3 className="h-4 w-4" />
        </TooltipIconButton>
      </div>
      <div className="-mx-1 overflow-x-auto px-1" aria-label={t('memoryPanel.structureTabs')} data-testid="memory-panel-structure-tabs">
        <div className="flex w-max min-w-full gap-1">
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
        <div className="flex min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.loading')}</div>
      ) : filteredRecords.length === 0 ? (
        <div className="flex min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{query.trim() ? t('memoryPanel.noMatches') : t('memoryPanel.empty')}</div>
      ) : (
        <div className="space-y-4">
          {visibleStructures.map((structure) => {
            const records = filteredRecords.filter((record) => record.structure_id === structure.id)
            if (records.length === 0) {
              if (selectedStructureId === allStructuresId) return null
              return (
                <section key={structure.id} className="space-y-2">
                  <MemoryStructureHeader structure={structure} count={0} />
                  <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-3 py-6 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.tableEmpty')}</div>
                </section>
              )
            }
            return (
              <section key={structure.id} className="space-y-2">
                <MemoryStructureHeader structure={structure} count={records.length} />
                <div className="space-y-2">
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
function StructureTab({ active, label, count, onClick }: { active: boolean; label: string; count: number; onClick: () => void }) {
  return (
    <button
      type="button"
      className={`inline-flex h-7 max-w-[168px] shrink-0 items-center gap-1 rounded-[var(--nova-radius)] border px-2 text-[11px] transition-colors ${active ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
      aria-label={`${label} ${count}`}
      aria-pressed={active}
      onClick={onClick}
    >
      <span className="min-w-0 truncate">{label}</span>
      <span className="shrink-0 text-[10px] opacity-70">{count}</span>
    </button>
  )
}

function MemoryStructureHeader({ structure, count }: { structure: StoryMemoryStructure; count: number }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-w-0 items-center justify-between gap-2">
      <div className="min-w-0">
        <h3 className="truncate text-xs font-semibold text-[var(--nova-text)]">{structure.name || structure.id}</h3>
        {structure.description && <p className="mt-0.5 line-clamp-1 break-words text-[11px] text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{structure.description}</p>}
      </div>
      <span className="shrink-0 rounded-full border border-[var(--nova-border)] px-2 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('memoryPanel.recordCount', { count })}</span>
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
    <article className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 ${record.archived ? 'opacity-55' : ''}`}>
      <div className="min-w-0">
        <h4 className="break-words text-sm font-medium text-[var(--nova-text)] [overflow-wrap:anywhere]">{storyMemoryRecordTitle(record, structure, t('storyMemory.untitled'))}</h4>
        <div className="mt-1 flex flex-wrap gap-1.5">
          {record.manual && <MemoryChip>{t('storyMemory.manual')}</MemoryChip>}
          {record.inherited_from && <MemoryChip>{t('storyMemory.inherited')}</MemoryChip>}
          {record.archived && <MemoryChip>{t('memoryPanel.archived')}</MemoryChip>}
          {record.updated_at && <MemoryChip>{`${t('storyMemory.updated')} ${formatShortDate(record.updated_at)}`}</MemoryChip>}
        </div>
      </div>
      <div className="mt-2 space-y-2">
        {visibleFields.map((field) => (
          <section key={field.id} className="min-w-0">
            <div className="mb-0.5 truncate text-[11px] font-medium text-[var(--nova-text-muted)]">{field.name || field.id}</div>
            <p className="line-clamp-4 whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{recordFieldValue(record, field.id) || t('storyMemory.noValue')}</p>
          </section>
        ))}
      </div>
    </article>
  )
}
