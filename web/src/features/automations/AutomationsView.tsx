import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Clock3, Inbox, Loader2, MessageSquareText, Play, Plus, RefreshCw, Settings2, Square } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { MessageList } from '@/components/Chat/MessageList'
import { InputArea } from '@/components/Chat/InputArea'
import { Button } from '@/components/ui/button'
import {
  createAutomation,
  deleteAutomation,
  checkAutomation,
  confirmAutomationInboxItem,
  dismissAutomationInboxItem,
  getAutomationInbox,
  getAutomationTemplates,
  getAutomations,
  getActiveAutomationRuns,
  getBooks,
  markAutomationInboxItemRead,
  type AutomationActiveRun,
  type AutomationInboxItem,
  type AutomationRunRecord,
  type AutomationTask,
  type AutomationTaskTemplate,
  type AutomationTriggerDefinition,
  type BookRecord,
} from '@/lib/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { fetchSettings } from '@/features/settings/api'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { rebaseJSONValue } from '@/lib/three-way-rebase'
import type { Settings } from '@/features/settings/types'
import { useAutomationRunStream } from './useAutomationRunStream'
import { InboxPanel } from './AutomationInboxPanel'
import { AutomationConfigPanel } from './AutomationConfigPanel'
import { AutomationTaskCatalog } from './AutomationTaskCatalog'
import { AutomationTemplateDialog } from './AutomationTemplateDialog'
import { automationTaskKey, findAutomationTaskByTarget, findAutomationTaskForRun } from './automation-catalog'
import {
  AUTOMATION_NAVIGATION_EVENT,
  consumeAutomationNavigation,
  type AutomationNavigationTarget,
} from './automation-navigation'
import {
  automationTaskDraftSignature,
  cloneAutomationTask,
  defaultAutomationTarget,
  newAutomationTask,
  newAutomationTaskFromTemplate,
  normalizeAutomationTaskShape,
  upsertAutomationTask,
} from './automation-task-draft'
import { useAutomationAutosave } from './use-automation-autosave'
import { buildAutomationModelProfileOptions, inheritedAutomationModelProfileLabel } from './automation-model-profiles'

type AutomationPanelView = 'config' | 'inbox' | 'run' | 'agent'

export function AutomationsView({ workspace, onClose }: { workspace: string; onClose?: () => void }) {
  const { t, i18n } = useTranslation()
  const [tasks, setTasks] = useState<AutomationTask[]>([])
  const [templates, setTemplates] = useState<AutomationTaskTemplate[]>([])
  const [books, setBooks] = useState<BookRecord[]>([])
  const [activeRuns, setActiveRuns] = useState<AutomationActiveRun[]>([])
  const [inboxItems, setInboxItems] = useState<AutomationInboxItem[]>([])
  const [effectiveSettings, setEffectiveSettings] = useState<Settings | null>(null)
  const [activeId, setActiveId] = useState<string>('')
  const activeIdRef = useRef('')
  const [draft, setDraft] = useState<AutomationTask>(() => newAutomationTask(defaultAutomationTarget(workspace), t('automations.defaultName')))
  const [creating, setCreating] = useState(false)
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false)
  const [panelView, setPanelView] = useState<AutomationPanelView>('config')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null)
  const [navigationTarget, setNavigationTarget] = useState<AutomationNavigationTarget | null>(null)
  const [runInputAreaHeight, setRunInputAreaHeight] = useState(0)
  const mountedRef = useRef(true)
  const loadSequenceRef = useRef(0)
  const draftDirtyRef = useRef(false)
  const draftRef = useRef(draft)
  const taskBaselineRef = useRef<AutomationTask | null>(null)
  const creatingRef = useRef(creating)
  draftRef.current = draft
  creatingRef.current = creating

  const load = useCallback(async () => {
    const sequence = loadSequenceRef.current + 1
    loadSequenceRef.current = sequence
    try {
      const locale = i18n.resolvedLanguage || i18n.language || 'zh-CN'
      const [data, taskTemplates, inbox, settings, bookRecords, runningTasks] = await Promise.all([
        getAutomations(),
        getAutomationTemplates(locale),
        getAutomationInbox(),
        fetchSettings(),
        getBooks(),
        getActiveAutomationRuns(),
      ])
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return
      const normalized = data.map((task) => normalizeAutomationTaskShape(task, workspace))
      // Automation tasks have explicit targets and may be global. A book switch
      // must not discard an existing or not-yet-created dirty definition.
      const preserveDraft = draftDirtyRef.current && Boolean(activeIdRef.current || creatingRef.current)
      setTasks(normalized)
      setTemplates(taskTemplates)
      setBooks(bookRecords)
      setActiveRuns(runningTasks)
      setInboxItems(inbox)
      setEffectiveSettings(settings.effective)
      const selected = normalized.find((task) => automationTaskKey(task) === activeIdRef.current)
        ?? normalized.find((task) => task.target?.kind === 'workspace' && task.target.workspace === workspace)
        ?? normalized[0]
      if (preserveDraft && selected && automationTaskKey(selected) === activeIdRef.current) {
        const previousBaseline = taskBaselineRef.current
        const draftAtReloadStart = draftRef.current
        const nextDraft = previousBaseline && automationTaskKey(previousBaseline) === activeIdRef.current
          ? await rebaseJSONWithRecovery({
              resource: 'automation',
              scope: workspace,
              id: activeIdRef.current,
              baseline: { revision: previousBaseline.revision, value: previousBaseline },
              local: { revision: draftAtReloadStart.revision, value: draftAtReloadStart },
              external: { revision: selected.revision, value: selected },
            })
          : draftRef.current
        if (!mountedRef.current || sequence !== loadSequenceRef.current) return
        taskBaselineRef.current = cloneAutomationTask(selected, workspace)
        const latestDraft = draftRef.current
        const draftWithNewerEdits = latestDraft === draftAtReloadStart
          ? nextDraft
          : rebaseJSONValue(draftAtReloadStart, latestDraft, nextDraft)
        const clonedDraft = cloneAutomationTask(draftWithNewerEdits, workspace)
        draftRef.current = clonedDraft
        setDraft(clonedDraft)
      } else if (!preserveDraft) {
        draftDirtyRef.current = false
        if (selected) {
          const key = automationTaskKey(selected)
          activeIdRef.current = key
          setActiveId(key)
          taskBaselineRef.current = cloneAutomationTask(selected, workspace)
          const clonedDraft = cloneAutomationTask(selected, workspace)
          draftRef.current = clonedDraft
          setDraft(clonedDraft)
          setCreating(false)
        } else {
          activeIdRef.current = ''
          setActiveId('')
          taskBaselineRef.current = null
          const emptyDraft = newAutomationTask(defaultAutomationTarget(workspace), t('automations.defaultName'))
          draftRef.current = emptyDraft
          setDraft(emptyDraft)
          setCreating(false)
        }
      }
    } catch (e) {
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return
      setError((e as Error).message)
    }
  }, [i18n.language, i18n.resolvedLanguage, t, workspace])

  const runStream = useAutomationRunStream({ onFinished: load })
  const { loadHistory: loadAutomationRunHistory, resume: resumeAutomationRun } = runStream
  const running = runStream.isStreaming
  const catalogActiveRuns = useMemo(() => {
    const live = runStream.activeRun
    if (!live || live.status !== 'running' || activeRuns.some((active) => active.run.id === live.id)) return activeRuns
    return [...activeRuns, { task_id: live.task_id, run: live }]
  }, [activeRuns, runStream.activeRun])
  const automationWorkspace = draft.target?.kind === 'workspace' ? draft.target.workspace || '' : ''
  const skillCommands = useSkillCommands({ agentKey: 'automation', workspace: automationWorkspace, fallbackEnabled: true })
  const runMessageListBottomPadding = runInputAreaHeight > 0 ? runInputAreaHeight + 20 : undefined

  useEffect(() => {
    mountedRef.current = true
    void load()
    return () => {
      mountedRef.current = false
      loadSequenceRef.current += 1
    }
  }, [load])

  useEffect(() => {
    const receiveNavigation = (event: Event) => {
      const queued = consumeAutomationNavigation()
      const detail = (event as CustomEvent<AutomationNavigationTarget>).detail
      setNavigationTarget(queued || detail)
    }
    window.addEventListener(AUTOMATION_NAVIGATION_EVENT, receiveNavigation)
    const queued = consumeAutomationNavigation()
    if (queued) setNavigationTarget(queued)
    return () => window.removeEventListener(AUTOMATION_NAVIGATION_EVENT, receiveNavigation)
  }, [])

  useEffect(() => {
    const reloadChangedAutomation = (event: Event) => {
      const detail = (event as CustomEvent<{ paths?: unknown }>).detail
      const paths = Array.isArray(detail?.paths)
        ? detail.paths.filter((path): path is string => typeof path === 'string')
        : []
      if (paths.length > 0 && !paths.some(isAutomationTaskFile)) return
      void load()
    }
    window.addEventListener('nova:workspace-change', reloadChangedAutomation)
    return () => window.removeEventListener('nova:workspace-change', reloadChangedAutomation)
  }, [load])

  useEffect(() => {
    if (running || tasks.length === 0) return
    let cancelled = false
    void (async () => {
      try {
        const runs = await getActiveAutomationRuns()
        if (cancelled) return
        setActiveRuns(runs)
        if (runs.length === 0) return
        const active = runs[0]
        const task = findAutomationTaskForRun(tasks, active.run)
        if (task && !draftDirtyRef.current) {
          const key = automationTaskKey(task)
          activeIdRef.current = key
          setActiveId(key)
          const nextDraft = cloneAutomationTask(task, workspace)
          taskBaselineRef.current = nextDraft
          draftRef.current = nextDraft
          setDraft(nextDraft)
          draftDirtyRef.current = false
          setCreating(false)
        }
        setPanelView('run')
        await resumeAutomationRun(active.run, t('automations.run.attached', { name: task?.name || active.run.task_id }))
      } catch (e) {
        if (!cancelled) console.error('resume automation run failed', e)
      }
    })()
    return () => { cancelled = true }
  }, [resumeAutomationRun, running, t, tasks, workspace])

  const unreadInboxCount = useMemo(() => inboxItems.filter((item) => !item.read_at && item.status === 'pending').length, [inboxItems])
  const modelProfileOptions = useMemo(() => buildAutomationModelProfileOptions(effectiveSettings, draft.model_profile_id, t), [draft.model_profile_id, effectiveSettings, t])
  const inheritedAutomationProfile = useMemo(() => inheritedAutomationModelProfileLabel(effectiveSettings, t), [effectiveSettings, t])

  const automationAutosave = useAutomationAutosave({
    activeId,
    creating,
    draft,
    tasks,
    workspace,
    onSaved: (saved, _submitted, submittedIsCurrent) => {
      setTasks((current) => upsertAutomationTask(current, saved))
      taskBaselineRef.current = cloneAutomationTask(saved, workspace)
      if (submittedIsCurrent) {
        const nextDraft = cloneAutomationTask(saved, workspace)
        draftRef.current = nextDraft
        setDraft(nextDraft)
        draftDirtyRef.current = false
      }
    },
    onError: (cause) => {
      console.error('[automations] failed to autosave task configuration', cause)
      setError(cause instanceof Error ? cause.message : String(cause))
    },
  })
  const flushAutomationAutosave = useCallback(() => {
    setError(null)
    return automationAutosave.flush()
  }, [automationAutosave.flush])

  const selectTask = async (task: AutomationTask) => {
    if (!(await flushAutomationAutosave())) return
    const key = automationTaskKey(task)
    activeIdRef.current = key
    setActiveId(key)
    const nextDraft = cloneAutomationTask(task, workspace)
    taskBaselineRef.current = nextDraft
    draftRef.current = nextDraft
    setDraft(nextDraft)
    draftDirtyRef.current = false
    setCreating(false)
    setPanelView('config')
  }

  const createNew = async () => {
    if (!(await flushAutomationAutosave())) return
    setTemplateDialogOpen(true)
  }

  const chooseCreationTemplate = (template: AutomationTaskTemplate | null, target: NonNullable<AutomationTask['target']>) => {
    activeIdRef.current = ''
    setActiveId('')
    taskBaselineRef.current = null
    const nextDraft = template
      ? newAutomationTaskFromTemplate(template, target)
      : newAutomationTask(target, t('automations.defaultName'))
    draftRef.current = nextDraft
    setDraft(nextDraft)
    draftDirtyRef.current = true
    setCreating(true)
    setPanelView('config')
  }

  const createDraft = async () => {
    if (!creating) return
    const submitted = draftRef.current
    setSaving(true)
    setError(null)
    try {
      const saved = await createAutomation(submitted)
      const normalized = normalizeAutomationTaskShape(saved, workspace)
      const key = automationTaskKey(normalized)
      activeIdRef.current = key
      setActiveId(key)
      const canonical = cloneAutomationTask(normalized, workspace)
      taskBaselineRef.current = canonical
      const latestDraft = draftRef.current
      const nextDraft = latestDraft === submitted
        ? canonical
        : cloneAutomationTask(rebaseJSONValue(submitted, latestDraft, canonical), workspace)
      draftRef.current = nextDraft
      setDraft(nextDraft)
      draftDirtyRef.current = automationTaskDraftSignature(nextDraft) !== automationTaskDraftSignature(canonical)
      setTasks((current) => upsertAutomationTask(current, normalized))
      setCreating(false)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const requestRemove = async () => {
    if (!activeId) return
    if (!(await flushAutomationAutosave())) return
    setDeleteTarget({ id: activeId, name: draft.name || activeId })
  }

  const confirmRemove = async () => {
    if (!deleteTarget) return
    setSaving(true)
    setError(null)
    try {
      await deleteAutomation(deleteTarget.id)
      const next = tasks.filter((task) => automationTaskKey(task) !== deleteTarget.id)
      setTasks(next)
      const fallback = next[0]
      const fallbackID = fallback ? automationTaskKey(fallback) : ''
      activeIdRef.current = fallbackID
      setActiveId(fallbackID)
      const nextDraft = fallback ? cloneAutomationTask(fallback, workspace) : newAutomationTask(defaultAutomationTarget(workspace), t('automations.defaultName'))
      taskBaselineRef.current = fallback ? nextDraft : null
      draftRef.current = nextDraft
      setDraft(nextDraft)
      draftDirtyRef.current = false
      setCreating(false)
    } catch (e) {
      setError((e as Error).message)
      throw e
    } finally {
      setSaving(false)
    }
  }

  const runNow = async () => {
    if (!activeId) return
    if (!(await flushAutomationAutosave())) return
    setError(null)
    setPanelView('run')
    try {
      await runStream.start(activeId, buildRunUserMessage(draft, t))
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const checkTriggers = async () => {
    if (!activeId) return
    if (!(await flushAutomationAutosave())) return
    setSaving(true)
    setError(null)
    try {
      await checkAutomation(activeId)
      const inbox = await getAutomationInbox()
      setInboxItems(inbox)
      setPanelView('inbox')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const openRun = useCallback(async (run: AutomationRunRecord) => {
    setError(null)
    setPanelView('run')
    try {
      await loadAutomationRunHistory(run)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [loadAutomationRunHistory])

  useEffect(() => {
    if (!navigationTarget || tasks.length === 0) return
    let cancelled = false
    void (async () => {
      if (!await flushAutomationAutosave() || cancelled) return
      const task = tasks.find((candidate) => automationTaskKey(candidate) === navigationTarget.taskId)
        || findAutomationTaskByTarget(tasks, navigationTarget.taskId, navigationTarget.workspace)
      if (!task || cancelled) return
      const key = automationTaskKey(task)
      activeIdRef.current = key
      setActiveId(key)
      const nextDraft = cloneAutomationTask(task, workspace)
      taskBaselineRef.current = nextDraft
      draftRef.current = nextDraft
      setDraft(nextDraft)
      draftDirtyRef.current = false
      setCreating(false)
      if (navigationTarget.inboxId) {
        setPanelView('inbox')
      } else if (navigationTarget.runId) {
        const run = task.recent_runs?.find((candidate) => candidate.id === navigationTarget.runId)
        if (run) void openRun(run)
        else setPanelView('run')
      } else {
        setPanelView('config')
      }
      setNavigationTarget(null)
    })()
    return () => { cancelled = true }
  }, [flushAutomationAutosave, navigationTarget, openRun, tasks, workspace])

  const sendRunMessage = async (message: string) => {
    setError(null)
    setPanelView('run')
    try {
      await runStream.send(message)
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const confirmInboxItem = async (item: AutomationInboxItem) => {
    setError(null)
    try {
      const result = await confirmAutomationInboxItem(item.id)
      setInboxItems((current) => current.map((candidate) => candidate.id === result.item.id ? result.item : candidate))
      if (result.run) {
        const task = findAutomationTaskForRun(tasks, result.run)
        setPanelView('run')
        await resumeAutomationRun(result.run, t('automations.run.attached', { name: task?.name || result.run.task_id }))
      }
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const dismissInboxItem = async (item: AutomationInboxItem) => {
    setError(null)
    try {
      const updated = await dismissAutomationInboxItem(item.id)
      setInboxItems((current) => current.map((candidate) => candidate.id === updated.id ? updated : candidate))
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const readInboxItem = async (item: AutomationInboxItem) => {
    if (item.read_at) return
    try {
      const updated = await markAutomationInboxItemRead(item.id)
      setInboxItems((current) => current.map((candidate) => candidate.id === updated.id ? updated : candidate))
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const setDraftField = (patch: Partial<AutomationTask>) => {
    draftDirtyRef.current = true
    setDraft((current) => {
      const next = { ...current, ...patch }
      draftRef.current = next
      return next
    })
  }
  const setDraftTriggers = (triggers: AutomationTriggerDefinition[]) => {
    draftDirtyRef.current = true
    setDraft((current) => {
      const schedule = triggers.find((trigger) => trigger.type === 'schedule')?.schedule ?? current.schedule
      const next = { ...current, schedule, triggers }
      draftRef.current = next
      return next
    })
  }
  const hasEditableDraft = Boolean(activeId) || creating
  const taskListPanel = (
    <AutomationTaskCatalog
      tasks={tasks}
      books={books}
      activeRuns={catalogActiveRuns}
      activeId={activeId}
      agentActive={panelView === 'agent'}
      onSelect={selectTask}
      onCreate={createNew}
      onOpenAgent={() => setPanelView('agent')}
    />
  )

  return (
    <FeaturePageShell
      icon={Clock3}
      title={t('automations.title')}
      subtitle={t('automations.summary', { tasks: tasks.length, running: catalogActiveRuns.length })}
      error={error}
      errorTitle={t('automations.error')}
      onClose={onClose ? () => {
        void flushAutomationAutosave().then((flushed) => { if (flushed) onClose() })
      } : undefined}
      closeLabel={t('automations.close')}
      onSaveShortcut={activeId && !creating ? flushAutomationAutosave : undefined}
      className="bg-[var(--nova-bg)] text-[var(--nova-text)]"
      actions={(
        <>
          {activeId && !creating ? (
            <AutosaveStatusIndicator
              status={automationAutosave.status}
              error={automationAutosave.error}
              onRetry={flushAutomationAutosave}
            />
          ) : null}
          <Button type="button" size="sm" variant="outline" onClick={checkTriggers} disabled={!activeId || running || saving} className="nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]" aria-label={t('automations.checkTriggers')} title={t('automations.checkTriggers')}>
            <RefreshCw data-icon="inline-start" />
            <span className="hidden sm:inline">{t('automations.checkTriggers')}</span>
          </Button>
          <Button type="button" size="sm" variant="secondary" onClick={runNow} disabled={!activeId || running || saving} className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-active)]" aria-label={running ? t('automations.running') : t('automations.runNow')} title={running ? t('automations.running') : t('automations.runNow')}>
            <Play data-icon="inline-start" />
            <span className="hidden sm:inline">{running ? t('automations.running') : t('automations.runNow')}</span>
          </Button>
          {running ? (
            <Button type="button" size="sm" variant="outline" onClick={runStream.stop} className="nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]" aria-label={t('automations.stopRun')} title={t('automations.stopRun')}>
              <Square data-icon="inline-start" />
              <span className="hidden sm:inline">{t('automations.stopRun')}</span>
            </Button>
          ) : null}
          {creating ? (
            <Button type="button" size="sm" variant="secondary" onClick={createDraft} disabled={saving || running} className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-active)]" aria-label={t('common.create')} title={t('common.create')}>
              {saving ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Plus data-icon="inline-start" />}
              <span className="hidden sm:inline">{saving ? t('common.creating') : t('common.create')}</span>
            </Button>
          ) : null}
        </>
      )}
    >
      <AdaptiveSurface
        left={{
          id: 'automation-tasks',
          title: t('automations.title'),
          side: 'left',
          icon: <Clock3 className="h-4 w-4" />,
          content: taskListPanel,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(90vw,360px)]',
        }}
        className="flex-1 text-xs"
        mainClassName="min-h-0 min-w-0"
        desktopGridClassName="grid-cols-[18rem_minmax(0,1fr)]"
      >
        {({ openLeft }) => (
          <main className="flex h-full min-h-0 flex-col">
            <div className="flex h-10 shrink-0 items-center gap-2 overflow-x-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 sm:px-4">
              <MobilePaneTrigger
                side="left"
                label={t('workbench.mobile.openSidePanel', { label: t('automations.title') })}
                onClick={openLeft}
                className="md:hidden"
              />
              <div className="flex h-7 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5">
                <button
                  type="button"
                  onClick={() => setPanelView('config')}
                  className={`inline-flex items-center gap-1.5 rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${panelView === 'config' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
                >
                  <Settings2 className="h-3.5 w-3.5" />
                  {t('automations.view.config')}
                </button>
                <button
                  type="button"
                  onClick={() => setPanelView('inbox')}
                  className={`inline-flex items-center gap-1.5 rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${panelView === 'inbox' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
                >
                  <Inbox className="h-3.5 w-3.5" />
                  {t('automations.view.inbox')}
                  {unreadInboxCount > 0 && <span className="rounded-full bg-[var(--nova-danger-border)] px-1.5 text-[10px] text-white">{unreadInboxCount}</span>}
                </button>
                <button
                  type="button"
                  onClick={() => setPanelView('run')}
                  className={`inline-flex items-center gap-1.5 rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${panelView === 'run' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
                >
                  <MessageSquareText className="h-3.5 w-3.5" />
                  {t('automations.view.run')}
                </button>
              </div>
              <div className="min-w-0 flex-1" />
              {runStream.activeRun && (
                <span className="truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 font-mono text-[11px] text-[var(--nova-text-faint)]">
                  {runStream.activeRun.status || (running ? 'running' : '')} · {runStream.activeRun.id}
                </span>
              )}
            </div>

            {panelView === 'config' ? hasEditableDraft ? (
              <AutomationConfigPanel
                activeId={activeId}
                activeRunId={runStream.activeRun?.id || ''}
                books={books}
                draft={draft}
                inheritedModelProfile={inheritedAutomationProfile}
                modelProfileOptions={modelProfileOptions}
                running={running}
                saving={saving}
                onChange={setDraftField}
                onOpenRun={openRun}
                onRemove={() => void requestRemove()}
                onTriggersChange={setDraftTriggers}
              />
            ) : (
              <EmptyState
                variant="page"
                icon={Plus}
                title={t('automations.empty.title')}
                description={t('automations.empty.description')}
                action={{ label: t('automations.newTask'), onClick: createNew }}
                className="min-h-0 flex-1 overflow-y-auto px-4 py-10"
              />
            ) : panelView === 'inbox' ? (
            <InboxPanel
              items={inboxItems}
              tasks={tasks}
              onRead={readInboxItem}
              onConfirm={confirmInboxItem}
              onDismiss={dismissInboxItem}
              onOpenRun={(runId) => {
                const run = tasks.flatMap((task) => task.recent_runs || []).find((candidate) => candidate.id === runId)
                if (run) void openRun(run)
              }}
            />
          ) : panelView === 'run' ? (
            <section className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
              <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                <MessageList
                  messages={runStream.messages}
                  isStreaming={runStream.isStreaming}
                  activityContent={runStream.activityContent}
                  scrollResetKey={runStream.activeRun?.id || activeId || 'automation'}
                  collapseTraceGroups
                  bottomPaddingClassName="pb-36"
                  bottomPaddingPx={runMessageListBottomPadding}
                />
              </div>
              {runStream.activeRun ? (
                <InputArea
                  onSend={sendRunMessage}
                  onStop={runStream.isStreaming ? runStream.stop : undefined}
                  disabled={runStream.isStreaming}
                  commandScope="skills"
                  skills={skillCommands}
                  agentKey="automation"
                  workspace={automationWorkspace}
                  floating
                  onHeightChange={setRunInputAreaHeight}
                />
              ) : (
                <EmptyState variant="compact" title={t('automations.run.empty')} className="border-t border-[var(--nova-border)] text-[var(--nova-text-faint)]" />
              )}
            </section>
          ) : (
            <ConfigManagerChat
              workspace={automationWorkspace}
              origin="automation"
              resourceId={activeId}
              context={{
                active_automation_id: activeId,
                active_automation_name: draft.name || '',
                automation_scope: draft.scope,
                automation_target_kind: draft.target?.kind || '',
                automation_target_workspace: draft.target?.workspace || '',
              }}
              onMutated={() => void load()}
            />
          )}
          </main>
        )}
      </AdaptiveSurface>
      <AutomationTemplateDialog
        open={templateDialogOpen}
        workspace={workspace}
        books={books}
        templates={templates}
        onOpenChange={setTemplateDialogOpen}
        onChoose={chooseCreationTemplate}
      />
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
        title={t('automations.deleteTask.title')}
        description={t('automations.deleteTask.confirm', { name: deleteTarget?.name || '' })}
        confirmLabel={t('automations.deleteTask')}
        tone="danger"
        onConfirm={confirmRemove}
      />
    </FeaturePageShell>
  )
}

function buildRunUserMessage(task: AutomationTask, t: (key: string, options?: Record<string, unknown>) => string) {
  const prompt = task.prompt?.trim() || task.name
  return `${t('automations.run.userMessage', { name: task.name })}\n\n${prompt}`
}

function isAutomationTaskFile(path: string): boolean {
  const normalized = path.replaceAll('\\', '/').toLowerCase()
  return normalized === 'automations/tasks.json' || normalized.endsWith('/automations/tasks.json')
}
