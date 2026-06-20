import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Archive, Brain, Edit3, Loader2, RefreshCw, RotateCcw, Search, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MessageList } from '@/components/Chat/MessageList'
import { useAgentEventStream } from '@/hooks/useAgentEventStream'
import { generateStoryMemoryStream, getStoryMemory } from '../api'
import type { Snapshot, StoryMemoryRecord, StoryMemoryState, StoryMemoryStructure } from '../types'

type MemoryPanelView = 'content' | 'generation'

interface MemoryPanelProps {
  storyId?: string
  branchId?: string
  snapshot: Snapshot | null
  loading?: boolean
  refreshKey?: string | number
  onOpenMemoryManager?: () => void
}

const allStructuresId = '__all__'

export function MemoryPanel({ storyId, branchId, snapshot, loading = false, refreshKey, onOpenMemoryManager }: MemoryPanelProps) {
  const { t } = useTranslation()
  const [memory, setMemory] = useState<StoryMemoryState | null>(null)
  const [memoryLoading, setMemoryLoading] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [showArchived, setShowArchived] = useState(false)
  const [selectedStructureId, setSelectedStructureId] = useState(allStructuresId)
  const [view, setView] = useState<MemoryPanelView>('content')
  const autoGenerateTurnKeyRef = useRef('')

  const effectiveBranchId = branchId || snapshot?.branch_id || ''
  const turnSyncStatus = snapshot?.current_turn?.memory_status || snapshot?.current_turn?.state_status || ''
  const syncStatus = turnSyncStatus === 'pending' || turnSyncStatus === 'failed' ? turnSyncStatus : memory?.sync_status || turnSyncStatus
  const syncError = snapshot?.current_turn?.memory_error || snapshot?.current_turn?.state_error || memory?.sync_error || ''

  const loadMemory = useCallback(async () => {
    if (!storyId) {
      setMemory(null)
      return
    }
    setMemoryLoading(true)
    setError('')
    try {
      const next = await getStoryMemory(storyId, effectiveBranchId, showArchived)
      setMemory(next)
      setSelectedStructureId((current) => {
        if (current === allStructuresId || next.structures.some((structure) => structure.id === current)) return current
        return allStructuresId
      })
    } catch (err) {
      console.error('[interactive-memory-panel] load failed', err)
      setError(err instanceof Error ? err.message : t('memoryPanel.loadFailed'))
    } finally {
      setMemoryLoading(false)
    }
  }, [effectiveBranchId, showArchived, storyId, t])

  const { messages: generateMessages, setMessages: setGenerateMessages, isStreaming: generating, activityContent: generateActivity, consumeAgentStream, resetStreamingState, setAbortController, abortLocalStream } = useAgentEventStream({
    onEvent: (event, data) => {
      if (event.event !== 'story_memory_result') return
      setGenerateMessages(prev => [...prev, {
        role: 'system',
        content: t('memoryPanel.generateDone', {
          patches: readNumber(data.patches),
          records: readNumber(data.records),
        }),
      }])
      void loadMemory()
    },
  })

  useEffect(() => {
    void loadMemory()
  }, [loadMemory, refreshKey])

  const structures = memory?.structures || []
  const filteredRecords = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const source = memory?.records || []
    if (!needle) return source
    return source.filter((record) => {
      const structure = structures.find((item) => item.id === record.structure_id)
      return storyMemorySearchText(record, structure).toLowerCase().includes(needle)
    })
  }, [memory?.records, query, structures])
  const structureRecordCounts = useMemo(() => {
    const counts = new Map<string, number>()
    filteredRecords.forEach((record) => counts.set(record.structure_id, (counts.get(record.structure_id) || 0) + 1))
    return counts
  }, [filteredRecords])
  const visibleStructures = useMemo(() => {
    if (selectedStructureId === allStructuresId) return structures
    return structures.filter((structure) => structure.id === selectedStructureId)
  }, [selectedStructureId, structures])

  const runStoryMemoryGenerate = useCallback(async (source: 'manual' | 'auto' = 'manual') => {
    if (!storyId || generating) return
    setView('generation')
    resetStreamingState()
    setGenerateMessages([{ role: 'user', content: source === 'auto' ? t('memoryPanel.autoGenerateRequest') : t('memoryPanel.generateRequest') }])
    const controller = new AbortController()
    setAbortController(controller)
    try {
      const stream = await generateStoryMemoryStream(storyId, effectiveBranchId, controller.signal)
      await consumeAgentStream(stream)
      await loadMemory()
    } catch (err) {
      console.error('[interactive-memory-panel] generate stream failed', err)
      setGenerateMessages(prev => [...prev, { role: 'error', content: err instanceof Error ? err.message : t('memoryPanel.generateFailed') }])
      resetStreamingState()
    }
  }, [consumeAgentStream, effectiveBranchId, generating, loadMemory, resetStreamingState, setAbortController, setGenerateMessages, storyId, t])

  useEffect(() => {
    const turn = snapshot?.current_turn
    if (!storyId || !effectiveBranchId || !turn?.id || turn.memory_status !== 'pending' || generating) return
    const turnKey = `${storyId}:${effectiveBranchId}:${turn.id}`
    if (autoGenerateTurnKeyRef.current === turnKey) return
    autoGenerateTurnKeyRef.current = turnKey
    void runStoryMemoryGenerate('auto')
  }, [effectiveBranchId, generating, runStoryMemoryGenerate, snapshot?.current_turn?.id, snapshot?.current_turn?.memory_status, storyId])

  return (
    <aside className="flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="shrink-0 border-b border-[var(--nova-border)] px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div data-testid="memory-panel-icon" className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)]" aria-label={t('memoryPanel.title')} title={t('memoryPanel.title')}>
            <Brain className="h-4 w-4" />
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <div className="flex h-7 min-w-0 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('memoryPanel.panelSwitch')}>
              <button
                type="button"
                onClick={() => setView('content')}
                className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'content' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
              >
                {t('memoryPanel.view.content')}
              </button>
              <button
                type="button"
                onClick={() => setView('generation')}
                className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'generation' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
              >
                {t('memoryPanel.view.generation')}
              </button>
            </div>
            <SyncBadge status={syncStatus} error={syncError} loading={loading || memoryLoading} />
          </div>
        </div>
        {view === 'content' ? (
          <div className="mt-3 flex items-center gap-2">
            <label className="flex min-w-0 flex-1 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs text-[var(--nova-text-muted)]">
              <Search className="h-3.5 w-3.5 shrink-0" />
              <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('memoryPanel.search')} className="min-w-0 flex-1 bg-transparent text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]" />
            </label>
            <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={showArchived ? t('memoryPanel.hideArchived') : t('memoryPanel.showArchived')} onClick={() => setShowArchived((value) => !value)}>
              {showArchived ? <RotateCcw className="h-4 w-4" /> : <Archive className="h-4 w-4" />}
            </button>
            <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={t('memoryPanel.generate')} onClick={() => void runStoryMemoryGenerate()} disabled={generating || !storyId}>
              {generating ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            </button>
            <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={t('memoryPanel.openManager')} onClick={onOpenMemoryManager} disabled={!onOpenMemoryManager}>
              <Edit3 className="h-4 w-4" />
            </button>
          </div>
        ) : (
          <div className="mt-3 flex items-center gap-2">
            <div className="flex min-w-0 flex-1 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs text-[var(--nova-text-muted)]">
              <Brain className="h-3.5 w-3.5 shrink-0" />
              <span className="min-w-0 truncate">{t('memoryPanel.generateLog')}</span>
            </div>
            {generating ? (
              <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('memoryPanel.abortGenerate')} onClick={abortLocalStream}>
                <X className="h-4 w-4" />
              </button>
            ) : (
              <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-60" aria-label={t('memoryPanel.generate')} onClick={() => void runStoryMemoryGenerate()} disabled={!storyId}>
                <RefreshCw className="h-4 w-4" />
              </button>
            )}
          </div>
        )}
        {error && <p className="mt-2 text-xs text-[var(--nova-danger)]">{error}</p>}
      </div>

      {view === 'generation' ? (
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-4 py-3">
          {generateMessages.length > 0 || generating ? (
            <MessageList
              messages={generateMessages}
              isStreaming={generating}
              activityContent={generateActivity}
              scrollResetKey={`${storyId || ''}:${effectiveBranchId}`}
              bottomPaddingClassName="pb-3"
              messageStyle={{ fontSize: '12px', lineHeight: 1.55 }}
              collapseTraceBeforeAssistant
            />
          ) : (
            <div className="flex h-full min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.generateEmpty')}</div>
          )}
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          <div className="-mx-1 mb-3 overflow-x-auto px-1" aria-label={t('memoryPanel.structureTabs')} data-testid="memory-panel-structure-tabs">
            <div className="flex w-max min-w-full gap-1">
              <StructureTab
                active={selectedStructureId === allStructuresId}
                label={t('memoryPanel.allStructures')}
                count={filteredRecords.length}
                onClick={() => setSelectedStructureId(allStructuresId)}
              />
              {structures.map((structure) => (
                <StructureTab
                  key={structure.id}
                  active={selectedStructureId === structure.id}
                  label={structure.name || structure.id}
                  count={structureRecordCounts.get(structure.id) || 0}
                  onClick={() => setSelectedStructureId(structure.id)}
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
      )}
    </aside>
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
  const fields = structure.fields.length ? structure.fields : [{ id: 'value', name: t('storyMemory.value'), order: 10 }]
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

function SyncBadge({ status, error, loading }: { status?: string; error?: string; loading?: boolean }) {
  const { t } = useTranslation()
  if (loading || status === 'pending') {
    return (
      <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-[var(--nova-border)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
        <Loader2 className="h-3 w-3 animate-spin" />
        {t('memoryPanel.syncing')}
      </span>
    )
  }
  if (status === 'failed') {
    return <span className="inline-flex max-w-[120px] shrink-0 truncate rounded-full border border-[var(--nova-danger)] px-2 py-1 text-[11px] text-[var(--nova-danger)]" title={error}>{t('memoryPanel.failed')}</span>
  }
  return <span className="inline-flex shrink-0 rounded-full border border-[var(--nova-border)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">{t('memoryPanel.ready')}</span>
}

function MemoryChip({ children }: { children: string }) {
  return <span className="max-w-full truncate rounded-full border border-[var(--nova-border)] px-2 py-0.5 text-[11px] text-[var(--nova-text-muted)]">{children}</span>
}

function readNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function storyMemorySearchText(record: StoryMemoryRecord, structure?: StoryMemoryStructure) {
  return [
    structure?.name,
    structure?.description,
    record.key,
    ...Object.values(record.values || {}),
  ].filter(Boolean).join('\n')
}

function storyMemoryRecordTitle(record: StoryMemoryRecord, structure: StoryMemoryStructure, fallback: string) {
  if (record.key?.trim()) return record.key.trim()
  const keyField = structure.key_field_id ? record.values?.[structure.key_field_id]?.trim() : ''
  if (keyField) return keyField
  const firstValue = structure.fields.map((field) => record.values?.[field.id]?.trim()).find(Boolean)
  return firstValue || structure.name || fallback
}

function recordFieldValue(record: StoryMemoryRecord, fieldId: string) {
  return record.values?.[fieldId] || ''
}

function formatShortDate(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString()
}
