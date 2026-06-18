import { useCallback, useEffect, useMemo, useState } from 'react'
import { Eye, EyeOff, Loader2, Pencil, Plus, Save, Search, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { createInteractiveMemory, getInteractiveMemory, setInteractiveMemoryHidden, updateInteractiveMemory } from '../api'
import type { InteractiveMemoryEntry, InteractiveMemoryState, Snapshot } from '../types'

interface MemoryPanelProps {
  storyId?: string
  branchId?: string
  snapshot: Snapshot | null
  loading?: boolean
  refreshKey?: string | number
}

interface MemoryFormState {
  title: string
  summary: string
  content: string
  people: string
  places: string
  tags: string
  importance: number
}

const emptyForm: MemoryFormState = {
  title: '',
  summary: '',
  content: '',
  people: '',
  places: '',
  tags: '',
  importance: 3,
}

export function MemoryPanel({ storyId, branchId, snapshot, loading = false, refreshKey }: MemoryPanelProps) {
  const { t } = useTranslation()
  const [memory, setMemory] = useState<InteractiveMemoryState | null>(null)
  const [memoryLoading, setMemoryLoading] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [showHidden, setShowHidden] = useState(false)
  const [editing, setEditing] = useState<InteractiveMemoryEntry | null>(null)
  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState<MemoryFormState>(emptyForm)

  const effectiveBranchId = branchId || snapshot?.branch_id || ''
  const syncStatus = memory?.sync_status || snapshot?.current_turn?.memory_status || snapshot?.current_turn?.state_status || ''
  const syncError = memory?.sync_error || snapshot?.current_turn?.memory_error || snapshot?.current_turn?.state_error || ''

  const loadMemory = useCallback(async () => {
    if (!storyId) {
      setMemory(null)
      return
    }
    setMemoryLoading(true)
    setError('')
    try {
      setMemory(await getInteractiveMemory(storyId, effectiveBranchId, showHidden))
    } catch (err) {
      console.error('[interactive-memory-panel] load failed', err)
      setError(err instanceof Error ? err.message : t('memoryPanel.loadFailed'))
    } finally {
      setMemoryLoading(false)
    }
  }, [effectiveBranchId, showHidden, storyId, t])

  useEffect(() => {
    void loadMemory()
  }, [loadMemory, refreshKey])

  const entries = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const source = memory?.entries || []
    if (!needle) return source
    return source.filter((entry) => {
      const haystack = [entry.title, entry.summary, entry.content, ...(entry.people || []), ...(entry.places || []), ...(entry.tags || [])].join('\n').toLowerCase()
      return haystack.includes(needle)
    })
  }, [memory?.entries, query])

  const startCreate = () => {
    setCreating(true)
    setEditing(null)
    setForm(emptyForm)
  }

  const startEdit = (entry: InteractiveMemoryEntry) => {
    setCreating(false)
    setEditing(entry)
    setForm({
      title: entry.title || '',
      summary: entry.summary || '',
      content: entry.content || '',
      people: (entry.people || []).join(', '),
      places: (entry.places || []).join(', '),
      tags: (entry.tags || []).join(', '),
      importance: entry.importance || 3,
    })
  }

  const cancelForm = () => {
    setCreating(false)
    setEditing(null)
    setForm(emptyForm)
  }

  const saveForm = async () => {
    if (!storyId) return
    const payload = {
      branch_id: effectiveBranchId,
      title: form.title.trim(),
      summary: form.summary.trim(),
      content: form.content.trim(),
      people: splitList(form.people),
      places: splitList(form.places),
      tags: splitList(form.tags),
      importance: form.importance,
    }
    if (!payload.title || (!payload.summary && !payload.content)) {
      setError(t('memoryPanel.validation'))
      return
    }
    setError('')
    if (editing) {
      await updateInteractiveMemory(storyId, editing.id, payload)
    } else {
      await createInteractiveMemory(storyId, payload)
    }
    cancelForm()
    await loadMemory()
  }

  const toggleHidden = async (entry: InteractiveMemoryEntry) => {
    if (!storyId) return
    await setInteractiveMemoryHidden(storyId, entry.id, !entry.hidden)
    await loadMemory()
  }

  return (
    <aside className="flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="shrink-0 border-b border-[var(--nova-border)] px-4 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-[var(--nova-text)]">{t('memoryPanel.title')}</h2>
            <p className="mt-0.5 truncate text-xs text-[var(--nova-text-muted)]">{t('memoryPanel.subtitle')}</p>
          </div>
          <SyncBadge status={syncStatus} error={syncError} loading={loading || memoryLoading} />
        </div>
        <div className="mt-3 flex items-center gap-2">
          <label className="flex min-w-0 flex-1 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs text-[var(--nova-text-muted)]">
            <Search className="h-3.5 w-3.5 shrink-0" />
            <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('memoryPanel.search')} className="min-w-0 flex-1 bg-transparent text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]" />
          </label>
          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={showHidden ? t('memoryPanel.hideHidden') : t('memoryPanel.showHidden')} onClick={() => setShowHidden((value) => !value)}>
            {showHidden ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
          </button>
          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('memoryPanel.add')} onClick={startCreate}>
            <Plus className="h-4 w-4" />
          </button>
        </div>
        {error && <p className="mt-2 text-xs text-[var(--nova-danger)]">{error}</p>}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
        {(creating || editing) && <MemoryEditor form={form} setForm={setForm} onSave={saveForm} onCancel={cancelForm} />}
        {entries.length === 0 ? (
          <div className="flex min-h-[160px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs text-[var(--nova-text-muted)]">{memoryLoading ? t('memoryPanel.loading') : t('memoryPanel.empty')}</div>
        ) : (
          <div className="space-y-2">
            {entries.map((entry) => (
              <article key={entry.id} className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3 ${entry.hidden ? 'opacity-55' : ''}`}>
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <h3 className="break-words text-sm font-medium text-[var(--nova-text)]">{entry.title || t('memoryPanel.untitled')}</h3>
                    <p className="mt-1 break-words text-xs leading-5 text-[var(--nova-text-muted)]">{entry.summary || entry.content || t('memoryPanel.noContent')}</p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <button type="button" className="nova-icon-button flex h-7 w-7 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('memoryPanel.edit')} onClick={() => startEdit(entry)}>
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    <button type="button" className="nova-icon-button flex h-7 w-7 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={entry.hidden ? t('memoryPanel.restore') : t('memoryPanel.hide')} onClick={() => void toggleHidden(entry)}>
                      {entry.hidden ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
                    </button>
                  </div>
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <MemoryChip>{t('memoryPanel.importance', { value: entry.importance || 3 })}</MemoryChip>
                  {(entry.people || []).map((value) => <MemoryChip key={`p-${entry.id}-${value}`}>{value}</MemoryChip>)}
                  {(entry.places || []).map((value) => <MemoryChip key={`l-${entry.id}-${value}`}>{value}</MemoryChip>)}
                  {(entry.tags || []).map((value) => <MemoryChip key={`t-${entry.id}-${value}`}>{value}</MemoryChip>)}
                  {entry.manual && <MemoryChip>{t('memoryPanel.manual')}</MemoryChip>}
                </div>
              </article>
            ))}
          </div>
        )}
      </div>
    </aside>
  )
}

function MemoryEditor({ form, setForm, onSave, onCancel }: { form: MemoryFormState; setForm: (form: MemoryFormState) => void; onSave: () => void; onCancel: () => void }) {
  const { t } = useTranslation()
  const update = (patch: Partial<MemoryFormState>) => setForm({ ...form, ...patch })
  return (
    <div className="mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
      <input value={form.title} onChange={(event) => update({ title: event.target.value })} placeholder={t('memoryPanel.fieldTitle')} className="mb-2 w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs text-[var(--nova-text)] outline-none" />
      <textarea value={form.summary} onChange={(event) => update({ summary: event.target.value })} placeholder={t('memoryPanel.fieldSummary')} rows={2} className="mb-2 w-full resize-none rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs text-[var(--nova-text)] outline-none" />
      <textarea value={form.content} onChange={(event) => update({ content: event.target.value })} placeholder={t('memoryPanel.fieldContent')} rows={4} className="mb-2 w-full resize-y rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs text-[var(--nova-text)] outline-none" />
      <div className="grid gap-2 sm:grid-cols-3">
        <input value={form.people} onChange={(event) => update({ people: event.target.value })} placeholder={t('memoryPanel.fieldPeople')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs text-[var(--nova-text)] outline-none" />
        <input value={form.places} onChange={(event) => update({ places: event.target.value })} placeholder={t('memoryPanel.fieldPlaces')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs text-[var(--nova-text)] outline-none" />
        <input value={form.tags} onChange={(event) => update({ tags: event.target.value })} placeholder={t('memoryPanel.fieldTags')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs text-[var(--nova-text)] outline-none" />
      </div>
      <div className="mt-2 flex items-center justify-between gap-2">
        <label className="flex items-center gap-2 text-xs text-[var(--nova-text-muted)]">
          {t('memoryPanel.fieldImportance')}
          <input type="number" min={1} max={5} value={form.importance} onChange={(event) => update({ importance: Number(event.target.value) || 3 })} className="w-14 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 text-xs text-[var(--nova-text)] outline-none" />
        </label>
        <div className="flex items-center gap-1">
          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('common.cancel')} onClick={onCancel}>
            <X className="h-4 w-4" />
          </button>
          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('common.save')} onClick={onSave}>
            <Save className="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
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

function splitList(value: string): string[] {
  return value
    .split(/[，,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}
