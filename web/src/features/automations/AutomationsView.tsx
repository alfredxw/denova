import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Clock3, Inbox, Loader2, MessageSquareText, PanelLeft, Play, Plus, RefreshCw, Save, Settings2, Square, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { MessageList } from '@/components/Chat/MessageList'
import { InputArea } from '@/components/Chat/InputArea'
import { Textarea } from '@/components/ui/textarea'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
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
  updateAutomation,
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
import type { Settings, ModelProfileSettings } from '@/features/settings/types'
import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import { useAutomationRunStream } from './useAutomationRunStream'
import { InboxPanel } from './AutomationInboxPanel'
import { TriggerEditor } from './AutomationTriggerEditor'
import { AutomationTaskCatalog } from './AutomationTaskCatalog'
import { AutomationTemplateDialog } from './AutomationTemplateDialog'
import { automationTaskKey, findAutomationTaskByTarget, findAutomationTaskForRun } from './automation-catalog'
import {
  AUTOMATION_NAVIGATION_EVENT,
  consumeAutomationNavigation,
  type AutomationNavigationTarget,
} from './automation-navigation'
import {
  automationTargetLabel,
  automationTargetOptions,
  automationTargetValue,
  cloneAutomationTask,
  defaultAutomationTarget,
  newAutomationTask,
  newAutomationTaskFromTemplate,
  nextAutomationWriteModePatch,
  nextAutomationWriteScopePatch,
  normalizeAutomationTaskShape,
  upsertAutomationTask,
} from './automation-task-draft'

const fieldCls = 'nova-field min-h-7 w-full min-w-0 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'
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

  const load = useCallback(async () => {
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
      const normalized = data.map((task) => normalizeAutomationTaskShape(task, workspace))
      setTasks(normalized)
      setTemplates(taskTemplates)
      setBooks(bookRecords)
      setActiveRuns(runningTasks)
      setInboxItems(inbox)
      setEffectiveSettings(settings.effective)
      const selected = normalized.find((task) => automationTaskKey(task) === activeIdRef.current)
        ?? normalized.find((task) => task.target?.kind === 'workspace' && task.target.workspace === workspace)
        ?? normalized[0]
      if (selected) {
        const key = automationTaskKey(selected)
        activeIdRef.current = key
        setActiveId(key)
        setDraft(cloneAutomationTask(selected, workspace))
        setCreating(false)
      } else {
        activeIdRef.current = ''
        setActiveId('')
        setDraft(newAutomationTask(defaultAutomationTarget(workspace), t('automations.defaultName')))
        setCreating(false)
      }
    } catch (e) {
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

  useEffect(() => { void load() }, [load])

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
        if (task) {
          const key = automationTaskKey(task)
          activeIdRef.current = key
          setActiveId(key)
          setDraft(cloneAutomationTask(task, workspace))
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
  const modelProfileOptions = useMemo(() => buildModelProfileOptions(effectiveSettings, draft.model_profile_id, t), [draft.model_profile_id, effectiveSettings, t])
  const inheritedAutomationProfile = useMemo(() => inheritedAutomationProfileLabel(effectiveSettings, t), [effectiveSettings, t])

  const selectTask = (task: AutomationTask) => {
    const key = automationTaskKey(task)
    activeIdRef.current = key
    setActiveId(key)
    setDraft(cloneAutomationTask(task, workspace))
    setCreating(false)
    setPanelView('config')
  }

  const createNew = () => {
    setTemplateDialogOpen(true)
  }

  const chooseCreationTemplate = (template: AutomationTaskTemplate | null, target: NonNullable<AutomationTask['target']>) => {
    activeIdRef.current = ''
    setActiveId('')
    setDraft(template
      ? newAutomationTaskFromTemplate(template, target)
      : newAutomationTask(target, t('automations.defaultName')))
    setCreating(true)
    setPanelView('config')
  }

  const save = async () => {
    if (!activeId && !creating) return
    setSaving(true)
    setError(null)
    try {
      const saved = activeId ? await updateAutomation(activeId, draft) : await createAutomation(draft)
      const normalized = normalizeAutomationTaskShape(saved, workspace)
      const key = automationTaskKey(normalized)
      activeIdRef.current = key
      setActiveId(key)
      setDraft(cloneAutomationTask(normalized, workspace))
      setTasks((current) => upsertAutomationTask(current, normalized))
      setCreating(false)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const requestRemove = () => {
    if (!activeId) return
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
      setDraft(fallback ? cloneAutomationTask(fallback, workspace) : newAutomationTask(defaultAutomationTarget(workspace), t('automations.defaultName')))
      setCreating(false)
      setDeleteTarget(null)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const runNow = async () => {
    if (!activeId) return
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
    const task = tasks.find((candidate) => automationTaskKey(candidate) === navigationTarget.taskId)
      || findAutomationTaskByTarget(tasks, navigationTarget.taskId, navigationTarget.workspace)
    if (!task) return
    const key = automationTaskKey(task)
    activeIdRef.current = key
    setActiveId(key)
    setDraft(cloneAutomationTask(task, workspace))
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
  }, [navigationTarget, openRun, tasks, workspace])

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

  const setDraftField = (patch: Partial<AutomationTask>) => setDraft((current) => ({ ...current, ...patch }))
  const setDraftTriggers = (triggers: AutomationTriggerDefinition[]) => {
    setDraft((current) => {
      const schedule = triggers.find((trigger) => trigger.type === 'schedule')?.schedule ?? current.schedule
      return { ...current, schedule, triggers }
    })
  }
  const globalTask = draft.target?.kind === 'user'
  const hasEditableDraft = Boolean(activeId) || creating
  const targetValue = automationTargetValue(draft)
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
    <div className="flex h-full min-h-0 w-full flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-topbar flex h-10 shrink-0 flex-nowrap max-md:flex-wrap items-center gap-2 overflow-x-auto max-md:overflow-x-hidden border-b px-3 py-1 text-xs sm:px-4">
        <Clock3 className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="shrink-0 font-medium">{t('automations.title')}</span>
        <span className="shrink-0 border-l border-[var(--nova-border)] pl-2 text-[11px] text-[var(--nova-text-faint)] sm:ml-3 sm:pl-3">
          {t('automations.summary', { tasks: tasks.length, running: catalogActiveRuns.length })}
        </span>
        <button type="button" onClick={checkTriggers} disabled={!activeId || running || saving} className="nova-nav-item ml-auto inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1 text-[var(--nova-text-muted)] disabled:opacity-50 sm:px-3">
          <RefreshCw className="h-3.5 w-3.5" />
          {t('automations.checkTriggers')}
        </button>
        <button type="button" onClick={runNow} disabled={!activeId || running || saving} className="nova-nav-item inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2.5 py-1 text-[var(--nova-text)] disabled:opacity-50 sm:px-3">
          <Play className="h-3.5 w-3.5" />
          {running ? t('automations.running') : t('automations.runNow')}
        </button>
        {running && (
          <button type="button" onClick={runStream.stop} className="nova-nav-item inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1 text-[var(--nova-text-muted)]">
            <Square className="h-3.5 w-3.5" />
            {t('automations.stopRun')}
          </button>
        )}
        <button type="button" onClick={save} disabled={saving || running || !hasEditableDraft} className="nova-nav-item inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-1 text-[var(--nova-text)] disabled:opacity-50">
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
          {t('common.save')}
        </button>
        {onClose && (
          <button type="button" onClick={onClose} className="nova-nav-item flex h-8 w-8 shrink-0 items-center justify-center rounded p-1" aria-label={t('automations.close')} title={t('automations.close')}>
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title={t('automations.error')} />}

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
              <button type="button" className="nova-icon-button flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)] md:hidden" aria-label={t('workbench.mobile.openSidePanel', { label: t('automations.title') })} onClick={openLeft}>
                <PanelLeft className="h-4 w-4" />
              </button>
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
              <div className="min-h-0 flex-1 overflow-y-auto">
                <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
                  <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--nova-border)] pb-4">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium text-[var(--nova-text)]">{draft.name || t('automations.newTask')}</div>
                      <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">
                        {automationTargetLabel(draft, books, t)} · {draft.enabled ? t('automations.enabled') : t('automations.disabled')}
                      </div>
                    </div>
                    {activeId && (
                      <button
                        type="button"
                        onClick={requestRemove}
                        disabled={saving || running}
                        className="nova-nav-item inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[var(--nova-danger)] disabled:cursor-not-allowed disabled:opacity-45"
                        aria-label={t('automations.deleteTask')}
                        title={t('automations.deleteTask')}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                        {t('automations.deleteTask')}
                      </button>
                    )}
                  </div>
                <section className="grid gap-3 border-b border-[var(--nova-border)] pb-5 md:grid-cols-2">
                  <Field label={t('automations.field.name')}>
                    <input value={draft.name} onChange={(e) => setDraftField({ name: e.target.value })} className={fieldCls} />
                  </Field>
                  <Field label={t('automations.field.enabled')}>
                    <select value={String(draft.enabled)} onChange={(e) => setDraftField({ enabled: e.target.value === 'true' })} className={fieldCls}>
                      <option value="true">{t('automations.enabled')}</option>
                      <option value="false">{t('automations.disabled')}</option>
                    </select>
                  </Field>
                  <Field label={t('automations.field.target')}>
                    <select value={targetValue} disabled className={fieldCls}>
                      <option value="user">{t('automations.target.global')}</option>
                      {automationTargetOptions(books, draft).map((book) => <option key={book.path} value={`workspace:${book.path}`}>{t('automations.target.workspace', { name: book.name })}</option>)}
                    </select>
                  </Field>
                  <Field label={t('automations.field.modelProfile')}>
                    <select value={draft.model_profile_id || ''} onChange={(e) => setDraftField({ model_profile_id: e.target.value })} className={fieldCls}>
                      <option value="">{t('automations.model.inherit', { label: inheritedAutomationProfile })}</option>
                      {modelProfileOptions.map((profile) => <option key={profile.id} value={profile.id}>{profile.label}</option>)}
                    </select>
                  </Field>
                  <div className="md:col-span-2">
                    <Field label={t('automations.field.prompt')}>
                      <Textarea autoResize value={draft.prompt} onChange={(e) => setDraftField({ prompt: e.target.value })} placeholder={t('automations.prompt.placeholder')} className={`${fieldCls} min-h-32 resize-y leading-5 shadow-none focus-visible:ring-0`} />
                    </Field>
                  </div>
                </section>

                <section className="grid gap-3 border-b border-[var(--nova-border)] pb-5 md:grid-cols-2">
                  <Field label={t('automations.field.writeMode')}>
                    <select value={draft.write_mode} disabled={globalTask} onChange={(e) => setDraftField(nextAutomationWriteModePatch(draft, e.target.value as AutomationTask['write_mode']))} className={fieldCls}>
                      <option value="read_only">{t('automations.writeMode.readOnly')}</option>
                      <option value="confirm_write">{t('automations.writeMode.confirmWrite')}</option>
                      <option value="auto_write">{t('automations.writeMode.autoWrite')}</option>
                    </select>
                  </Field>
                  <Field label={t('automations.field.writeScope')}>
                    <select value={draft.write_scope} disabled={globalTask || draft.write_mode === 'read_only'} onChange={(e) => setDraftField(nextAutomationWriteScopePatch(draft, e.target.value as AutomationTask['write_scope']))} className={fieldCls}>
                      <option value="none">{t('automations.writeScope.none')}</option>
                      <option value="lore">{t('automations.writeScope.lore')}</option>
                      <option value="file">{t('automations.writeScope.file')}</option>
                      <option value="lore_and_file">{t('automations.writeScope.loreFile')}</option>
                    </select>
                  </Field>
                  <Field label={t('automations.field.outputPolicy')}>
                    <select value={draft.output_policy} disabled={globalTask} onChange={(e) => setDraftField({ output_policy: e.target.value as AutomationTask['output_policy'] })} className={fieldCls}>
                      <option value="run_record_only">{t('automations.output.record')}</option>
                      <option value="optional_file">{t('automations.output.file')}</option>
                    </select>
                  </Field>
                  <div className="md:col-span-2">
                    <Field label={t('automations.field.outputPath')}>
                      <input value={draft.output_path} disabled={globalTask} onChange={(e) => setDraftField({ output_path: e.target.value })} placeholder="reports/automation-review.md" className={fieldCls} />
                    </Field>
                  </div>
                  {globalTask && <div className="md:col-span-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('automations.target.globalHelp')}</div>}
                </section>

                <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
                  <SectionTitle title={t('automations.section.triggers')} />
                  <TriggerEditor task={draft} onChange={setDraftTriggers} />
                </section>

                <section className="space-y-3 pb-5">
                  <SectionTitle title={t('automations.section.runs')} />
                  <RunList task={draft} activeRunId={runStream.activeRun?.id || ''} onOpenRun={openRun} />
                </section>
                </div>
              </div>
            ) : (
              <div className="flex min-h-0 flex-1 items-center justify-center overflow-y-auto px-4 py-10">
                <div className="flex max-w-md flex-col items-center text-center">
                  <div className="flex h-10 w-10 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
                    <Plus className="h-4 w-4" />
                  </div>
                  <div className="mt-3 text-sm font-medium text-[var(--nova-text)]">{t('automations.empty.title')}</div>
                  <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('automations.empty.description')}</div>
                  <button type="button" onClick={createNew} className="nova-nav-item mt-4 inline-flex h-8 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 text-xs text-[var(--nova-text)]">
                    <Plus className="h-3.5 w-3.5" />
                    {t('automations.newTask')}
                  </button>
                </div>
              </div>
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
                  collapseTraceBeforeAssistant
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
                <div className="border-t border-[var(--nova-border)] px-4 py-3 text-[11px] text-[var(--nova-text-faint)]">
                  {t('automations.run.empty')}
                </div>
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
      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(open) => {
        if (!open && !saving) setDeleteTarget(null)
      }}>
        <AlertDialogContent className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('automations.deleteTask.title')}</AlertDialogTitle>
            <AlertDialogDescription className="text-[var(--nova-text-muted)]">
              {t('automations.deleteTask.confirm', { name: deleteTarget?.name || '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={saving}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-[var(--nova-danger-bg)] text-[var(--nova-danger)] hover:bg-[var(--nova-danger-bg)]"
              disabled={saving || !deleteTarget}
              onClick={(event) => {
                event.preventDefault()
                void confirmRemove()
              }}
            >
              {t('automations.deleteTask')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="flex min-w-0 flex-col gap-1.5 text-xs"><span className="text-[var(--nova-text-muted)]">{label}</span>{children}</label>
}

function SectionTitle({ title }: { title: string }) {
  return <div className="text-xs font-medium text-[var(--nova-text)]">{title}</div>
}

function RunList({ task, activeRunId, onOpenRun }: { task: AutomationTask; activeRunId: string; onOpenRun: (run: AutomationRunRecord) => void }) {
  const { t } = useTranslation()
  const runs = task.recent_runs || []
  if (runs.length === 0) return <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-8 text-center text-[var(--nova-text-faint)]">{t('automations.runs.empty')}</div>
  return (
    <div className="space-y-2">
      {runs.slice(0, 5).map((run) => (
        <div key={run.id} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
          <div className="flex items-center gap-2">
            <span className="font-medium">{run.status}</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">{new Date(run.started_at).toLocaleString()}</span>
            {run.output_path && <span className="ml-auto truncate text-[11px] text-[var(--nova-text-faint)]">{run.output_path}</span>}
            {run.session_id && (
              <button
                type="button"
                onClick={() => onOpenRun(run)}
                className={`nova-nav-item ml-auto rounded-[var(--nova-radius)] px-2 py-0.5 text-[11px] ${activeRunId === run.id ? 'is-active' : 'text-[var(--nova-text-muted)]'}`}
              >
                {t('automations.runs.viewTimeline')}
              </button>
            )}
          </div>
          <div className="mt-1 line-clamp-3 whitespace-pre-wrap text-[11px] leading-5 text-[var(--nova-text-muted)]">{run.error || run.summary}</div>
        </div>
      ))}
    </div>
  )
}

function buildModelProfileOptions(settings: Settings | null, selectedID: string | undefined, t: (key: string, options?: Record<string, unknown>) => string): Array<{ id: string; label: string }> {
  const labels = modelProfileLabels(settings, t)
  const selected = selectedID?.trim()
  if (selected && !labels.has(selected)) {
    labels.set(selected, t('automations.model.unknownProfile', { id: selected }))
  }
  return Array.from(labels.entries()).map(([id, label]) => ({
    id,
    label: id === 'default' ? t('automations.model.defaultProfile', { label }) : t('automations.model.profile', { id, label }),
  }))
}

function inheritedAutomationProfileLabel(settings: Settings | null, t: (key: string, options?: Record<string, unknown>) => string) {
  const labels = modelProfileLabels(settings, t)
  const inheritedID = settings?.agent_models?.automation?.profile_id || settings?.agent_models?.default?.profile_id || 'default'
  return labels.get(inheritedID) || t('automations.model.unknownProfile', { id: inheritedID })
}

function modelProfileLabels(settings: Settings | null, t: (key: string, options?: Record<string, unknown>) => string) {
  const profiles = new Map<string, string>()
  const add = (profile?: ModelProfileSettings) => {
    const id = modelProfileID(profile)
    if (!id) return
    profiles.set(id, modelProfileLabel(profile))
  }
  modelProfilesWithDefault(settings ?? undefined).forEach(add)
  if (!profiles.has('default')) profiles.set('default', t('automations.model.defaultModel'))
  return profiles
}

function buildRunUserMessage(task: AutomationTask, t: (key: string, options?: Record<string, unknown>) => string) {
  const prompt = task.prompt?.trim() || task.name
  return `${t('automations.run.userMessage', { name: task.name })}\n\n${prompt}`
}
