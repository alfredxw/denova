import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import { Archive, Bot, Brain, ChevronDown, ChevronRight, Edit3, Loader2, PanelLeft, PanelRight, Plus, RefreshCw, RotateCcw, Save, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { deleteStoryMemoryStructure, generateStoryMemory, getStoryMemory, saveStoryMemoryRecord, saveStoryMemoryStructure, setStoryMemoryRecordArchived, updateStoryMemorySettings } from '../api'
import type { BranchSummary, StoryMemoryField, StoryMemoryRecord, StoryMemoryState, StoryMemoryStructure } from '../types'

interface StoryMemoryViewProps {
  storyId?: string
  branchId?: string
  branches?: BranchSummary[]
}

const emptyStructure: StoryMemoryStructure = {
  id: '',
  name: '',
  description: '',
  generation_instruction: '',
  mode: 'append',
  key_field_id: '',
  enabled: true,
  fields: [{ id: 'event', name: '事件', description: '', generation_instruction: '', enabled: true, required: true, order: 10 }],
  order: 100,
}

export function StoryMemoryView({ storyId, branchId, branches = [] }: StoryMemoryViewProps) {
  const { t } = useTranslation()
  const [state, setState] = useState<StoryMemoryState | null>(null)
  const [memoryBranchId, setMemoryBranchId] = useState(branchId || '')
  const [selectedStructureId, setSelectedStructureId] = useState('')
  const [structureDraft, setStructureDraft] = useState<StoryMemoryStructure | null>(null)
  const [recordDraft, setRecordDraft] = useState<StoryMemoryRecord | null>(null)
  const [expandedRecordIds, setExpandedRecordIds] = useState<Set<string>>(() => new Set())
  const [showArchived, setShowArchived] = useState(false)
  const [agentOpen, setAgentOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const activeBranchId = memoryBranchId || branchId || ''

  useEffect(() => {
    setMemoryBranchId(branchId || '')
    setExpandedRecordIds(new Set())
    setRecordDraft(null)
    setStructureDraft(null)
  }, [branchId, storyId])

  const load = useCallback(async () => {
    if (!storyId) {
      setState(null)
      return
    }
    setLoading(true)
    setError('')
    try {
      const next = await getStoryMemory(storyId, activeBranchId, showArchived)
      setState(next)
      setSelectedStructureId((current) => next.structures.some((structure) => structure.id === current) ? current : next.structures[0]?.id || '')
    } catch (err) {
      console.error('[story-memory-view] load failed', err)
      setError(err instanceof Error ? err.message : t('storyMemory.loadFailed'))
    } finally {
      setLoading(false)
    }
  }, [activeBranchId, showArchived, storyId, t])

  useEffect(() => {
    void load()
  }, [load])

  const structures = state?.structures || []
  const selectedStructure = structures.find((item) => item.id === selectedStructureId) || structures[0]
  const tableFields = useMemo<StoryMemoryField[]>(() => {
    if (selectedStructure?.fields.length) return selectedStructure.fields
    return [{ id: 'value', name: t('storyMemory.value'), order: 10 }]
  }, [selectedStructure, t])
  const branchOptions = useMemo(() => {
    const options = [...branches]
    const loadedBranch = state?.branch_id || activeBranchId
    if (loadedBranch && !options.some((branch) => branch.id === loadedBranch)) {
      options.unshift({
        id: loadedBranch,
        head: '',
        title: loadedBranch,
        created_at: '',
        current: loadedBranch === branchId,
      })
    }
    return options
  }, [activeBranchId, branchId, branches, state?.branch_id])
  const records = useMemo(() => {
    const source = state?.records || []
    if (!selectedStructure) return source
    return source.filter((record) => record.structure_id === selectedStructure.id)
  }, [selectedStructure, state?.records])
  const visibleBranchId = state?.branch_id || activeBranchId
  const visibleBranch = branchOptions.find((branch) => branch.id === visibleBranchId)
  const editorVisible = Boolean(structureDraft || (recordDraft && selectedStructure))
  const recordColumnCount = tableFields.length + 4
  const columnWidths = useMemo(() => storyMemoryColumnWidths(tableFields.length), [tableFields.length])

  const startNewStructure = () => {
    setStructureDraft({ ...emptyStructure, fields: [...emptyStructure.fields] })
    setRecordDraft(null)
  }

  const startEditStructure = (structure: StoryMemoryStructure) => {
    setStructureDraft({ ...structure, fields: structure.fields.map((field) => ({ ...field })) })
    setRecordDraft(null)
  }

  const startNewRecord = () => {
    if (!selectedStructure) return
    setRecordDraft({
      id: '',
      structure_id: selectedStructure.id,
      branch_id: state?.branch_id || activeBranchId,
      key: '',
      values: Object.fromEntries(selectedStructure.fields.map((field) => [field.id, ''])),
      manual: true,
      created_at: '',
      updated_at: '',
    })
    setStructureDraft(null)
  }

  const startEditRecord = (record: StoryMemoryRecord) => {
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
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyMemory.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const toggleRecordArchived = async (record: StoryMemoryRecord) => {
    if (!storyId) return
    await setStoryMemoryRecordArchived(storyId, record.id, state?.branch_id || activeBranchId, !record.archived)
    await load()
  }

  const runGenerate = async () => {
    if (!storyId) return
    setLoading(true)
    setError('')
    try {
      setState(await generateStoryMemory(storyId, state?.branch_id || activeBranchId))
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyMemory.generateFailed'))
    } finally {
      setLoading(false)
    }
  }

  const toggleExpanded = (recordId: string) => {
    setExpandedRecordIds((current) => {
      const next = new Set(current)
      if (next.has(recordId)) next.delete(recordId)
      else next.add(recordId)
      return next
    })
  }

  const changeMemoryBranch = (nextBranchId: string) => {
    setMemoryBranchId(nextBranchId)
    setExpandedRecordIds(new Set())
    setRecordDraft(null)
    setStructureDraft(null)
  }

  const structurePanel = (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-[var(--nova-surface)] p-3">
      <div className="mb-3 grid grid-cols-2 gap-2">
        <button type="button" className={`nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2 ${agentOpen ? 'is-active' : 'bg-[var(--nova-surface-2)]'}`} onClick={() => setAgentOpen((value) => !value)}>
          <Bot className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('storyMemory.configAgent')}</span>
        </button>
        <button type="button" className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2" onClick={startNewStructure}>
          <Plus className="h-3.5 w-3.5" />
          <span className="min-w-0 truncate">{t('storyMemory.addStructure')}</span>
        </button>
      </div>
      <div className="mb-2 text-xs font-medium text-[var(--nova-text-muted)]">{t('storyMemory.structures')}</div>
      <div className="space-y-1">
        {structures.map((structure) => (
          <button key={structure.id} type="button" onClick={() => { setSelectedStructureId(structure.id); setRecordDraft(null); setStructureDraft(null) }} className={`w-full rounded-[var(--nova-radius)] px-2 py-2 text-left text-xs ${selectedStructure?.id === structure.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-surface-2)] hover:text-[var(--nova-text)]'}`}>
            <span className="flex min-w-0 items-center gap-1">
              <span className="min-w-0 truncate font-medium">{structure.name}</span>
              {!storyMemoryEnabled(structure.enabled) && <span className="shrink-0 rounded-full border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] opacity-75">{t('storyMemory.disabled')}</span>}
            </span>
            <span className="block truncate text-[11px] opacity-75">{t(`storyMemory.mode.${structure.mode}`)}</span>
          </button>
        ))}
      </div>
    </div>
  )
  const agentPanel = agentOpen ? (
    <div className="h-full min-h-0 bg-[var(--nova-surface)]">
      <ConfigManagerChat
        origin="story_memory"
        storyId={storyId}
        branchId={visibleBranchId}
        resourceId={selectedStructure?.id || ''}
        context={{
          selected_structure_id: selectedStructure?.id || '',
          selected_structure_name: selectedStructure?.name || '',
          record_count: String(records.length),
        }}
        onMutated={() => void load()}
      />
    </div>
  ) : null

  return (
    <section className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <header className="nova-topbar flex shrink-0 flex-col items-stretch gap-2 border-b border-[var(--nova-border)] px-3 py-2 sm:flex-row sm:items-center sm:justify-between sm:gap-3 sm:px-4 sm:py-3">
        <div className="flex min-w-0 items-center gap-2">
          <Brain className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold">{t('storyMemory.title')}</h2>
            <p className="hidden truncate text-xs text-[var(--nova-text-muted)] sm:block">
              {t('storyMemory.branchSummary', {
                branch: branchTitle(visibleBranch, visibleBranchId, t('branchTimeline.mainBranch')),
                head: visibleBranch?.head ? shortId(visibleBranch.head) : t('storyMemory.noHead'),
              })}
            </p>
          </div>
        </div>
        <div className="flex w-full min-w-0 flex-nowrap items-center justify-start gap-2 overflow-x-auto sm:w-auto sm:shrink-0 sm:justify-end">
          {branchOptions.length > 0 && (
            <label className="flex min-w-[11rem] flex-1 items-center gap-2 text-xs text-[var(--nova-text-muted)] sm:min-w-0 sm:flex-none">
              <span className="shrink-0">{t('storyMemory.branch')}</span>
              <select aria-label={t('storyMemory.branch')} value={visibleBranchId} onChange={(event) => changeMemoryBranch(event.target.value)} className="h-8 min-w-0 flex-1 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-xs text-[var(--nova-text)] outline-none sm:max-w-[220px]">
                {branchOptions.map((branch) => (
                  <option key={branch.id} value={branch.id}>
                    {branchTitle(branch, branch.id, t('branchTimeline.mainBranch'))}
                    {branch.id === branchId ? ` · ${t('storyMemory.currentBranch')}` : ''}
                    {branch.head ? ` · ${shortId(branch.head)}` : ''}
                  </option>
                ))}
              </select>
            </label>
          )}
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
      <AdaptiveSurface
        left={{
          id: 'story-memory-structures',
          title: t('storyMemory.structures'),
          side: 'left',
          icon: <Brain className="h-4 w-4" />,
          content: structurePanel,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(90vw,360px)]',
        }}
        right={agentOpen && agentPanel ? {
          id: 'story-memory-agent',
          title: t('storyMemory.configAgent'),
          side: 'right',
          icon: <Bot className="h-4 w-4" />,
          content: agentPanel,
          desktopClassName: 'min-h-0 border-l border-[var(--nova-border)]',
        } : undefined}
        className="flex-1 overflow-hidden"
        mainClassName="min-h-0 min-w-0"
        desktopGridClassName={agentOpen ? 'grid-cols-[240px_minmax(0,1fr)_minmax(320px,28rem)]' : 'grid-cols-[240px_minmax(0,1fr)]'}
      >
        {({ isMobile, openLeft, openRight }) => (
          <main className="h-full min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] p-3 sm:p-4">
            <div className={`grid min-h-0 gap-4 ${editorVisible ? 'xl:grid-cols-[minmax(0,1fr)_340px]' : ''}`}>
            <div className="min-w-0">
              <div className="mb-3 flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  {isMobile && (
                    <button type="button" className="nova-icon-button flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: t('storyMemory.structures') })} onClick={openLeft}>
                      <PanelLeft className="h-4 w-4" />
                    </button>
                  )}
                <div className="min-w-0">
                  <h3 className="truncate text-sm font-semibold">{selectedStructure?.name || t('storyMemory.noStructure')}</h3>
                  <p className="truncate text-xs text-[var(--nova-text-muted)]">
                    {selectedStructure && !storyMemoryEnabled(selectedStructure.enabled) ? `${t('storyMemory.disabled')} · ` : ''}
                    {selectedStructure?.description || t('storyMemory.recordCount', { count: records.length })}
                  </p>
                </div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {isMobile && agentOpen && (
                    <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: t('storyMemory.configAgent') })} onClick={openRight}>
                      <PanelRight className="h-4 w-4" />
                    </button>
                  )}
                  {selectedStructure && (
                    <>
                      <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyMemory.editStructure')} onClick={() => startEditStructure(selectedStructure)}>
                        <Edit3 className="h-4 w-4" />
                      </button>
                      <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={showArchived ? t('storyMemory.hideArchived') : t('storyMemory.showArchived')} onClick={() => setShowArchived((value) => !value)}>
                        {showArchived ? <RotateCcw className="h-4 w-4" /> : <Archive className="h-4 w-4" />}
                      </button>
                    </>
                  )}
                  <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyMemory.addRecord')} onClick={startNewRecord}>
                    <Plus className="h-4 w-4" />
                  </button>
                </div>
              </div>
              {records.length === 0 ? (
                <div className="flex min-h-[220px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] text-center text-xs text-[var(--nova-text-muted)]">{loading ? t('storyMemory.loading') : t('storyMemory.empty')}</div>
              ) : isMobile ? (
                <div data-testid="story-memory-cards" className="flex flex-col gap-2">
                  {records.map((record) => (
                    <div
                      key={record.id}
                      className={`rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 ${record.archived ? 'opacity-55' : ''}`}
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <div className="truncate font-medium text-[var(--nova-text)]">{record.key || selectedStructure?.name || t('storyMemory.untitled')}</div>
                          {(record.manual || record.inherited_from || record.archived) && (
                            <div className="mt-1 flex flex-wrap gap-1">
                              {record.manual && <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('storyMemory.manual')}</span>}
                              {record.inherited_from && <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('storyMemory.inherited')}</span>}
                              {record.archived && <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('storyMemory.archived')}</span>}
                            </div>
                          )}
                        </div>
                        <div className="flex shrink-0 gap-1">
                          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyMemory.editRecord')} onClick={() => startEditRecord(record)}>
                            <Edit3 className="h-3.5 w-3.5" />
                          </button>
                          <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={record.archived ? t('storyMemory.restore') : t('storyMemory.archive')} onClick={() => void toggleRecordArchived(record)}>
                            {record.archived ? <RotateCcw className="h-3.5 w-3.5" /> : <Archive className="h-3.5 w-3.5" />}
                          </button>
                        </div>
                      </div>
                      <div className="mt-2 flex flex-col gap-2">
                        {tableFields.map((field) => (
                          <div key={field.id} className="min-w-0">
                            <div className="flex items-center gap-1 text-[11px] font-medium text-[var(--nova-text-muted)]">
                              <span className="truncate">{field.name || field.id}</span>
                              {!storyMemoryEnabled(field.enabled) && <span className="shrink-0 rounded-full border border-[var(--nova-border)] px-1 py-0.5 text-[10px] font-normal">{t('storyMemory.disabled')}</span>}
                              {field.required && <span className="shrink-0 text-[var(--nova-danger)]">*</span>}
                            </div>
                            <p className="whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{recordFieldValue(record, field) || t('storyMemory.noValue')}</p>
                          </div>
                        ))}
                      </div>
                      <div className="mt-2 flex items-center justify-between text-[11px] text-[var(--nova-text-faint)]">
                        <span>{formatDate(record.updated_at || record.created_at)}</span>
                        <span className="truncate">{record.branch_id === branchId ? t('storyMemory.currentBranch') : shortId(record.branch_id)}</span>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div data-testid="story-memory-table-shell" className="max-w-full overflow-x-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
                  <table data-testid="story-memory-table" className="w-full table-fixed border-collapse text-left text-xs">
                    <colgroup>
                      <col style={{ width: columnWidths.record }} />
                      {tableFields.map((field) => (
                        <col key={field.id} style={{ width: columnWidths.field }} />
                      ))}
                      <col style={{ width: columnWidths.meta }} />
                      <col style={{ width: columnWidths.meta }} />
                      <col style={{ width: columnWidths.actions }} />
                    </colgroup>
                    <thead className="sticky top-0 z-10 border-b border-[var(--nova-border)] bg-[var(--nova-table-header-bg)] text-[var(--nova-text-muted)]">
                      <tr>
                        <th className="min-w-0 bg-[var(--nova-table-header-bg)] px-3 py-2 font-medium">{t('storyMemory.record')}</th>
                        {tableFields.map((field) => (
                          <th key={field.id} className="min-w-0 px-3 py-2 font-medium">
                            <div className="flex min-w-0 items-center gap-1">
                              <span className="truncate">{field.name || field.id}</span>
                              {!storyMemoryEnabled(field.enabled) && <span className="shrink-0 rounded-full border border-[var(--nova-border)] px-1 py-0.5 text-[10px] font-normal">{t('storyMemory.disabled')}</span>}
                              {field.required && <span className="shrink-0 text-[var(--nova-danger)]">*</span>}
                            </div>
                            {field.description && <div className="mt-0.5 line-clamp-1 break-words text-[10px] font-normal text-[var(--nova-text-faint)] [overflow-wrap:anywhere]">{field.description}</div>}
                          </th>
                        ))}
                        <th className="min-w-0 px-3 py-2 font-medium">{t('storyMemory.updated')}</th>
                        <th className="min-w-0 px-3 py-2 font-medium">{t('storyMemory.branch')}</th>
                        <th className="min-w-0 px-3 py-2 text-right font-medium">{t('storyMemory.actions')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {records.map((record) => {
                        const expanded = expandedRecordIds.has(record.id)
                        return (
                          <Fragment key={record.id}>
                            <tr className={`border-b border-[var(--nova-border)] align-top ${record.archived ? 'opacity-55' : ''}`}>
                              <td className="min-w-0 bg-[var(--nova-surface)] px-3 py-2">
                                <div className="flex min-w-0 items-start gap-2">
                                  <button type="button" className="nova-icon-button mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={expanded ? t('storyMemory.collapseRecord') : t('storyMemory.expandRecord')} onClick={() => toggleExpanded(record.id)}>
                                    {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                                  </button>
                                  <div className="min-w-0">
                                    <div className="truncate font-medium text-[var(--nova-text)]">{record.key || selectedStructure?.name || t('storyMemory.untitled')}</div>
                                    <div className="mt-1 flex flex-wrap gap-1">
                                      {record.manual && <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('storyMemory.manual')}</span>}
                                      {record.inherited_from && <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('storyMemory.inherited')}</span>}
                                      {record.archived && <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('storyMemory.archived')}</span>}
                                    </div>
                                  </div>
                                </div>
                              </td>
                              {tableFields.map((field) => (
                                <td key={field.id} className="min-w-0 px-3 py-2 text-[var(--nova-text-muted)]">
                                  <p className="line-clamp-3 whitespace-pre-wrap break-words leading-5 [overflow-wrap:anywhere]">{recordFieldValue(record, field) || t('storyMemory.noValue')}</p>
                                </td>
                              ))}
                              <td className="min-w-0 px-3 py-2 text-[var(--nova-text-muted)]">
                                <span className="block truncate">{formatDate(record.updated_at || record.created_at)}</span>
                              </td>
                              <td className="min-w-0 px-3 py-2 text-[var(--nova-text-muted)]">
                                <span className="block truncate">{record.branch_id === branchId ? t('storyMemory.currentBranch') : shortId(record.branch_id)}</span>
                              </td>
                              <td className="min-w-0 px-2 py-2">
                                <div className="flex justify-end gap-1">
                                  <button type="button" className="nova-icon-button flex h-7 w-7 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyMemory.editRecord')} onClick={() => startEditRecord(record)}>
                                    <Edit3 className="h-3.5 w-3.5" />
                                  </button>
                                  <button type="button" className="nova-icon-button flex h-7 w-7 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={record.archived ? t('storyMemory.restore') : t('storyMemory.archive')} onClick={() => void toggleRecordArchived(record)}>
                                    {record.archived ? <RotateCcw className="h-3.5 w-3.5" /> : <Archive className="h-3.5 w-3.5" />}
                                  </button>
                                </div>
                              </td>
                            </tr>
                            {expanded && (
                              <tr className="border-b border-[var(--nova-border)] bg-[var(--nova-surface)]">
                                <td colSpan={recordColumnCount} className="px-3 py-3">
                                  <div data-testid="story-memory-expanded-grid" className="grid max-w-full grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-3 overflow-hidden">
                                    {tableFields.map((field) => (
                                      <section key={field.id} className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
                                        <div className="mb-1 flex min-w-0 items-center gap-1 text-[11px] font-medium text-[var(--nova-text-muted)]">
                                          <span className="truncate">{field.name || field.id}</span>
                                          {!storyMemoryEnabled(field.enabled) && <span className="shrink-0 rounded-full border border-[var(--nova-border)] px-1 py-0.5 text-[10px] font-normal">{t('storyMemory.disabled')}</span>}
                                          {field.required && <span className="shrink-0 text-[var(--nova-danger)]">*</span>}
                                        </div>
                                        {field.description && <p className="mb-2 whitespace-pre-wrap break-words text-[11px] leading-4 text-[var(--nova-text-faint)] [overflow-wrap:anywhere]">{field.description}</p>}
                                        {field.generation_instruction && <p className="mb-2 whitespace-pre-wrap break-words rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2 py-1 text-[11px] leading-4 text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">{field.generation_instruction}</p>}
                                        <p className="whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text)] [overflow-wrap:anywhere]">{recordFieldValue(record, field) || t('storyMemory.noValue')}</p>
                                      </section>
                                    ))}
                                  </div>
                                </td>
                              </tr>
                            )}
                          </Fragment>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
            {editorVisible && (
              <aside className="min-h-0 overflow-y-auto rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
                {structureDraft ? (
                  <StructureEditor draft={structureDraft} saving={saving} onDraftChange={setStructureDraft} onSave={saveStructure} onCancel={() => setStructureDraft(null)} onDelete={structureDraft.id ? () => void removeStructure(structureDraft) : undefined} />
                ) : recordDraft && selectedStructure ? (
                  <RecordEditor structure={selectedStructure} draft={recordDraft} saving={saving} onDraftChange={setRecordDraft} onSave={saveRecord} onCancel={() => setRecordDraft(null)} />
                ) : null}
              </aside>
            )}
            </div>
          </main>
        )}
      </AdaptiveSurface>
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
      <label className="flex items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs text-[var(--nova-text-muted)]">
        <input type="checkbox" checked={storyMemoryEnabled(draft.enabled)} onChange={(event) => onDraftChange({ ...draft, enabled: event.target.checked })} />
        {t('storyMemory.enabled')}
      </label>
      <textarea value={draft.description || ''} onChange={(event) => onDraftChange({ ...draft, description: event.target.value })} placeholder={t('storyMemory.structureDescription')} rows={3} className="w-full resize-none rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none" />
      <textarea value={draft.generation_instruction || ''} onChange={(event) => onDraftChange({ ...draft, generation_instruction: event.target.value })} placeholder={t('storyMemory.generationInstruction')} rows={3} className="w-full resize-y rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none" />
      <select value={draft.mode} onChange={(event) => onDraftChange({ ...draft, mode: event.target.value as StoryMemoryStructure['mode'] })} className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none">
        <option value="singleton">{t('storyMemory.mode.singleton')}</option>
        <option value="keyed">{t('storyMemory.mode.keyed')}</option>
        <option value="append">{t('storyMemory.mode.append')}</option>
      </select>
      {draft.mode === 'keyed' && <input value={draft.key_field_id || ''} onChange={(event) => onDraftChange({ ...draft, key_field_id: event.target.value })} placeholder={t('storyMemory.keyField')} className="w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs outline-none" />}
      <div className="space-y-2">
        {draft.fields.map((field, index) => (
          <div key={`${field.id}-${index}`} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] p-2">
            <label className="mb-2 flex items-center gap-2 text-xs text-[var(--nova-text-muted)]">
              <input type="checkbox" checked={storyMemoryEnabled(field.enabled)} onChange={(event) => updateField(index, { enabled: event.target.checked })} />
              {t('storyMemory.fieldEnabled')}
            </label>
            <div className="grid grid-cols-2 gap-2">
              <input value={field.id} onChange={(event) => updateField(index, { id: event.target.value })} placeholder={t('storyMemory.fieldId')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs outline-none" />
              <input value={field.name} onChange={(event) => updateField(index, { name: event.target.value })} placeholder={t('storyMemory.fieldName')} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs outline-none" />
            </div>
            <textarea value={field.description || ''} onChange={(event) => updateField(index, { description: event.target.value })} placeholder={t('storyMemory.fieldDescription')} rows={2} className="mt-2 w-full resize-none rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs outline-none" />
            <textarea value={field.generation_instruction || ''} onChange={(event) => updateField(index, { generation_instruction: event.target.value })} placeholder={t('storyMemory.fieldGenerationInstruction')} rows={2} className="mt-2 w-full resize-y rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs outline-none" />
          </div>
        ))}
      </div>
      <button type="button" className="w-full rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-3 py-2 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={() => onDraftChange({ ...draft, fields: [...draft.fields, { id: '', name: '', description: '', generation_instruction: '', enabled: true, order: (draft.fields.length + 1) * 10 }] })}>{t('storyMemory.addField')}</button>
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

function recordFieldValue(record: StoryMemoryRecord, field: StoryMemoryField) {
  const value = record.values?.[field.id]
  if (value) return value
  if (field.id === 'value') return Object.values(record.values || {}).filter(Boolean).join('\n')
  return ''
}

function storyMemoryEnabled(value?: boolean) {
  return value !== false
}

function storyMemoryColumnWidths(fieldCount: number) {
  const safeFieldCount = Math.max(1, fieldCount)
  const record = safeFieldCount >= 5 ? 15 : safeFieldCount >= 3 ? 18 : 22
  const meta = safeFieldCount >= 5 ? 7 : 8
  const actions = 7
  const field = Math.max(7, (100 - record - meta * 2 - actions) / safeFieldCount)
  return {
    record: `${record}%`,
    field: `${field}%`,
    meta: `${meta}%`,
    actions: `${actions}%`,
  }
}

function branchTitle(branch: BranchSummary | undefined, fallback: string, mainLabel: string) {
  if (!branch) return fallback || mainLabel
  if (branch.title) return branch.title
  if (branch.id === 'main') return mainLabel
  return branch.id
}

function shortId(value: string) {
  if (!value) return ''
  return value.length > 8 ? value.slice(0, 8) : value
}

function formatDate(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString(undefined, { month: '2-digit', day: '2-digit' })
}
