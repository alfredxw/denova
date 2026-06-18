import { useCallback, useEffect, useMemo, useState } from 'react'
import { Brain, Eye, EyeOff, Loader2, Plus, RefreshCw, Save, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { deleteStoryMemoryStructure, generateStoryMemory, getStoryMemory, saveStoryMemoryRecord, saveStoryMemoryStructure, setStoryMemoryRecordHidden, updateStoryMemorySettings } from '../api'
import type { StoryMemoryField, StoryMemoryRecord, StoryMemoryState, StoryMemoryStructure } from '../types'

interface StoryMemoryViewProps {
  storyId?: string
  branchId?: string
}

const emptyStructure: StoryMemoryStructure = {
  id: '',
  name: '',
  description: '',
  mode: 'append',
  key_field_id: '',
  fields: [{ id: 'event', name: '事件', description: '', required: true, order: 10 }],
  order: 100,
}

export function StoryMemoryView({ storyId, branchId }: StoryMemoryViewProps) {
  const { t } = useTranslation()
  const [state, setState] = useState<StoryMemoryState | null>(null)
  const [selectedStructureId, setSelectedStructureId] = useState('')
  const [selectedRecord, setSelectedRecord] = useState<StoryMemoryRecord | null>(null)
  const [structureDraft, setStructureDraft] = useState<StoryMemoryStructure | null>(null)
  const [recordDraft, setRecordDraft] = useState<StoryMemoryRecord | null>(null)
  const [showHidden, setShowHidden] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    if (!storyId) {
      setState(null)
      return
    }
    setLoading(true)
    setError('')
    try {
      const next = await getStoryMemory(storyId, branchId, showHidden)
      setState(next)
      setSelectedStructureId((current) => current || next.structures[0]?.id || '')
    } catch (err) {
      console.error('[story-memory-view] load failed', err)
      setError(err instanceof Error ? err.message : t('storyMemory.loadFailed'))
    } finally {
      setLoading(false)
    }
  }, [branchId, showHidden, storyId, t])

  useEffect(() => {
    void load()
  }, [load])

  const structures = state?.structures || []
  const selectedStructure = structures.find((item) => item.id === selectedStructureId) || structures[0]
  const records = useMemo(() => {
    const source = state?.records || []
    if (!selectedStructure) return source
    return source.filter((record) => record.structure_id === selectedStructure.id)
  }, [selectedStructure, state?.records])

  const startNewStructure = () => {
    setStructureDraft({ ...emptyStructure, fields: [...emptyStructure.fields] })
    setRecordDraft(null)
    setSelectedRecord(null)
  }

  const startEditStructure = (structure: StoryMemoryStructure) => {
    setStructureDraft({ ...structure, fields: structure.fields.map((field) => ({ ...field })) })
    setRecordDraft(null)
  }

  const startNewRecord = () => {
    if (!selectedStructure) return
    setSelectedRecord(null)
    setRecordDraft({
      id: '',
      structure_id: selectedStructure.id,
      branch_id: state?.branch_id || branchId || '',
      key: '',
      values: Object.fromEntries(selectedStructure.fields.map((field) => [field.id, ''])),
      manual: true,
      created_at: '',
      updated_at: '',
    })
    setStructureDraft(null)
  }

  const startEditRecord = (record: StoryMemoryRecord) => {
    setSelectedRecord(record)
    setRecordDraft({ ...record, values: { ...record.values } })
    setStructureDraft(null)
  }

  const saveSettings = async (patch: { enabled?: boolean; auto_interval_turns?: number }) => {
    if (!storyId || !state) return
    const settings = await updateStoryMemorySettings(storyId, patch)
    setState({ ...state, settings })
  }

  const saveStructure = async () => {
    if (!storyId || !structureDraft) return
    setSaving(true)
    setError('')
    try {
      const saved = await saveStoryMemoryStructure(storyId, structureDraft)
      await load()
      setSelectedStructureId(saved.id)
      setStructureDraft(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyMemory.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const removeStructure = async (structure: StoryMemoryStructure) => {
    if (!storyId || structure.built_in) return
    await deleteStoryMemoryStructure(storyId, structure.id)
    if (selectedStructureId === structure.id) setSelectedStructureId('')
    await load()
  }

  const saveRecord = async () => {
    if (!storyId || !recordDraft) return
    setSaving(true)
    setError('')
    try {
      await saveStoryMemoryRecord(storyId, recordDraft)
      await load()
      setRecordDraft(null)
      setSelectedRecord(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyMemory.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const toggleRecordHidden = async (record: StoryMemoryRecord) => {
    if (!storyId) return
    await setStoryMemoryRecordHidden(storyId, record.id, state?.branch_id || branchId, !record.hidden)
    await load()
  }

  const runGenerate = async () => {
    if (!storyId) return
    setLoading(true)
    setError('')
    try {
      setState(await generateStoryMemory(storyId, state?.branch_id || branchId))
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyMemory.generateFailed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <header className="nova-topbar flex shrink-0 items-center justify-between gap-3 border-b border-[var(--nova-border)] px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <Brain className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold">{t('storyMemory.title')}</h2>
            <p className="truncate text-xs text-[var(--nova-text-muted)]">{t('storyMemory.subtitle')}</p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <label className="flex items-center gap-2 text-xs text-[var(--nova-text-muted)]">
            <input type="checkbox" checked={state?.settings.enabled ?? true} onChange={(event) => void saveSettings({ enabled: event.target.checked })} />
            {t('storyMemory.autoEnabled')}
          </label>
          <input aria-label={t('storyMemory.interval')} type="number" min={1} max={50} value={state?.settings.auto_interval_turns || 3} onChange={(event) => void saveSettings({ auto_interval_turns: Number(event.target.value) || 3 })} className="h-8 w-16 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-xs outline-none" />
          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyMemory.generate')} onClick={() => void runGenerate()}>
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
          </button>
        </div>
      </header>
      {error && <div className="border-b border-[var(--nova-border)] px-4 py-2 text-xs text-[var(--nova-danger)]">{error}</div>}
      <div className="grid min-h-0 flex-1 grid-cols-1 overflow-hidden lg:grid-cols-[240px_minmax(0,1fr)_360px]">
        <aside className="min-h-0 overflow-y-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 lg:border-b-0 lg:border-r">
          <div className="mb-2 flex items-center justify-between gap-2">
            <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('storyMemory.structures')}</span>
            <button type="button" className="nova-icon-button flex h-7 w-7 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyMemory.addStructure')} onClick={startNewStructure}>
              <Plus className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="space-y-1">
            {structures.map((structure) => (
              <button key={structure.id} type="button" onClick={() => { setSelectedStructureId(structure.id); setRecordDraft(null); setStructureDraft(null) }} className={`w-full rounded-[var(--nova-radius)] px-2 py-2 text-left text-xs ${selectedStructure?.id === structure.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-surface-2)] hover:text-[var(--nova-text)]'}`}>
                <span className="block truncate font-medium">{structure.name}</span>
                <span className="block truncate text-[11px] opacity-75">{t(`storyMemory.mode.${structure.mode}`)}</span>
              </button>
            ))}
          </div>
        </aside>

        <main className="min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] p-4">
          <div className="mb-3 flex items-center justify-between gap-2">
            <div className="min-w-0">
              <h3 className="truncate text-sm font-semibold">{selectedStructure?.name || t('storyMemory.noStructure')}</h3>
              <p className="truncate text-xs text-[var(--nova-text-muted)]">{selectedStructure?.description || t('storyMemory.recordCount', { count: records.length })}</p>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              {selectedStructure && (
                <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={showHidden ? t('storyMemory.hideHidden') : t('storyMemory.showHidden')} onClick={() => setShowHidden((value) => !value)}>
                  {showHidden ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
                </button>
              )}
              <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyMemory.addRecord')} onClick={startNewRecord}>
                <Plus className="h-4 w-4" />
              </button>
            </div>
          </div>
          {records.length === 0 ? (
            <div className="flex min-h-[220px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] text-center text-xs text-[var(--nova-text-muted)]">{loading ? t('storyMemory.loading') : t('storyMemory.empty')}</div>
          ) : (
            <div className="grid gap-2 xl:grid-cols-2">
              {records.map((record) => (
                <article key={record.id} className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 ${record.hidden ? 'opacity-55' : ''}`}>
                  <div className="flex items-start justify-between gap-2">
                    <button type="button" className="min-w-0 flex-1 text-left" onClick={() => startEditRecord(record)}>
                      <h4 className="truncate text-sm font-medium">{record.key || selectedStructure?.name || t('storyMemory.untitled')}</h4>
                      <p className="mt-1 line-clamp-3 text-xs leading-5 text-[var(--nova-text-muted)]">{recordPreview(record, selectedStructure)}</p>
                    </button>
                    <button type="button" className="nova-icon-button flex h-7 w-7 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={record.hidden ? t('storyMemory.restore') : t('storyMemory.hide')} onClick={() => void toggleRecordHidden(record)}>
                      {record.hidden ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
                    </button>
                  </div>
                </article>
              ))}
            </div>
          )}
        </main>

        <aside className="min-h-0 overflow-y-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 lg:border-l lg:border-t-0">
          {structureDraft ? (
            <StructureEditor draft={structureDraft} saving={saving} onDraftChange={setStructureDraft} onSave={saveStructure} onCancel={() => setStructureDraft(null)} onDelete={structureDraft.id ? () => void removeStructure(structureDraft) : undefined} />
          ) : recordDraft && selectedStructure ? (
            <RecordEditor structure={selectedStructure} draft={recordDraft} saving={saving} onDraftChange={setRecordDraft} onSave={saveRecord} onCancel={() => { setRecordDraft(null); setSelectedRecord(null) }} />
          ) : selectedStructure ? (
            <div className="space-y-3">
              <button type="button" className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2 text-left text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={() => startEditStructure(selectedStructure)}>{t('storyMemory.editStructure')}</button>
              {selectedRecord && <button type="button" className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2 text-left text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={() => startEditRecord(selectedRecord)}>{t('storyMemory.editRecord')}</button>}
            </div>
          ) : (
            <div className="text-xs text-[var(--nova-text-muted)]">{t('storyMemory.noStructure')}</div>
          )}
        </aside>
      </div>
    </section>
  )
}

function StructureEditor({ draft, saving, onDraftChange, onSave, onCancel, onDelete }: { draft: StoryMemoryStructure; saving: boolean; onDraftChange: (draft: StoryMemoryStructure) => void; onSave: () => void; onCancel: () => void; onDelete?: () => void }) {
  const { t } = useTranslation()
  const updateField = (index: number, patch: Partial<StoryMemoryField>) => {
    const fields = draft.fields.map((field, fieldIndex) => fieldIndex === index ? { ...field, ...patch } : field)
    onDraftChange({ ...draft, fields })
  }
  return (
    <div className="space-y-2">
      <input value={draft.name} onChange={(event) => onDraftChange({ ...draft, name: event.target.value })} placeholder={t('storyMemory.structureName')} className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none" />
      <textarea value={draft.description || ''} onChange={(event) => onDraftChange({ ...draft, description: event.target.value })} placeholder={t('storyMemory.structureDescription')} rows={3} className="w-full resize-none rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none" />
      <select value={draft.mode} onChange={(event) => onDraftChange({ ...draft, mode: event.target.value as StoryMemoryStructure['mode'] })} className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none">
        <option value="singleton">{t('storyMemory.mode.singleton')}</option>
        <option value="keyed">{t('storyMemory.mode.keyed')}</option>
        <option value="append">{t('storyMemory.mode.append')}</option>
      </select>
      {draft.mode === 'keyed' && <input value={draft.key_field_id || ''} onChange={(event) => onDraftChange({ ...draft, key_field_id: event.target.value })} placeholder={t('storyMemory.keyField')} className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none" />}
      <div className="space-y-2">
        {draft.fields.map((field, index) => (
          <div key={`${field.id}-${index}`} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] p-2">
            <div className="grid grid-cols-2 gap-2">
              <input value={field.id} onChange={(event) => updateField(index, { id: event.target.value })} placeholder={t('storyMemory.fieldId')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs outline-none" />
              <input value={field.name} onChange={(event) => updateField(index, { name: event.target.value })} placeholder={t('storyMemory.fieldName')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs outline-none" />
            </div>
            <textarea value={field.description || ''} onChange={(event) => updateField(index, { description: event.target.value })} placeholder={t('storyMemory.fieldDescription')} rows={2} className="mt-2 w-full resize-none rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs outline-none" />
          </div>
        ))}
      </div>
      <button type="button" className="w-full rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-3 py-2 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={() => onDraftChange({ ...draft, fields: [...draft.fields, { id: '', name: '', description: '', order: (draft.fields.length + 1) * 10 }] })}>{t('storyMemory.addField')}</button>
      <EditorActions saving={saving} onSave={onSave} onCancel={onCancel} onDelete={onDelete} />
    </div>
  )
}

function RecordEditor({ structure, draft, saving, onDraftChange, onSave, onCancel }: { structure: StoryMemoryStructure; draft: StoryMemoryRecord; saving: boolean; onDraftChange: (draft: StoryMemoryRecord) => void; onSave: () => void; onCancel: () => void }) {
  const { t } = useTranslation()
  const updateValue = (fieldId: string, value: string) => onDraftChange({ ...draft, values: { ...draft.values, [fieldId]: value }, key: structure.key_field_id === fieldId ? value : draft.key })
  return (
    <div className="space-y-2">
      {structure.mode !== 'singleton' && <input value={draft.key || ''} onChange={(event) => onDraftChange({ ...draft, key: event.target.value })} placeholder={t('storyMemory.recordKey')} className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none" />}
      {structure.fields.map((field) => (
        <label key={field.id} className="block text-xs text-[var(--nova-text-muted)]">
          <span className="mb-1 block">{field.name}</span>
          <textarea value={draft.values?.[field.id] || ''} onChange={(event) => updateValue(field.id, event.target.value)} placeholder={field.description || field.name} rows={3} className="w-full resize-y rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs text-[var(--nova-text)] outline-none" />
        </label>
      ))}
      <EditorActions saving={saving} onSave={onSave} onCancel={onCancel} />
    </div>
  )
}

function EditorActions({ saving, onSave, onCancel, onDelete }: { saving: boolean; onSave: () => void; onCancel: () => void; onDelete?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-end gap-1 pt-2">
      {onDelete && <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-danger)]" aria-label={t('common.delete')} onClick={onDelete}><Trash2 className="h-4 w-4" /></button>}
      <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('common.cancel')} onClick={onCancel}><X className="h-4 w-4" /></button>
      <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('common.save')} onClick={onSave}>{saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}</button>
    </div>
  )
}

function recordPreview(record: StoryMemoryRecord, structure?: StoryMemoryStructure) {
  const fields = structure?.fields || []
  const parts = fields.map((field) => record.values?.[field.id]).filter(Boolean)
  if (parts.length > 0) return parts.join(' / ')
  return Object.values(record.values || {}).filter(Boolean).join(' / ')
}
