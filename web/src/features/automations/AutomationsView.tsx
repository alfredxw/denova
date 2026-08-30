import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Bot, Clock3, Inbox, Loader2, Play, Plus, RefreshCw, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { LoadingState } from '@/components/common/LoadingState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { SidebarVisibilityToggle } from '@/components/layout/sidebar-visibility-toggle'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
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
  markAutomationInboxItemRead,
  startAutomationRun,
  createAgentCommandID,
  type AutomationInboxItem,
  type AutomationRunRecord,
  type AutomationTask,
  type AutomationTaskTemplate,
  type AutomationTriggerDefinition,
} from '@/lib/api'
import { fetchProjectSettings } from '@/features/settings/api'
import { getAgentChatProjects, type AgentChatProjectType } from '@/features/agent-chat/api'
import { requestAgentChatSessionNavigation } from '@/features/agent-chat/session-navigation'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { rebaseJSONValue } from '@/lib/three-way-rebase'
import type { Settings } from '@/features/settings/types'
import {
  isProjectChangeForProject,
  workspaceChangePaths,
  type WorkspaceChangeEvent,
} from '@/features/changes/types'
import { InboxPanel } from './AutomationInboxPanel'
import { AutomationConfigPanel } from './AutomationConfigPanel'
import { AutomationTaskCatalog } from './AutomationTaskCatalog'
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
import {
  automationProjectOptions,
  automationProjectTarget,
  automationTaskProjectID,
  defaultAutomationProject,
  type AutomationProjectOption,
} from './automation-projects'

type AutomationPanelView = 'config' | 'inbox'

export function AutomationsView({
  projectId = '',
  projectType = 'book',
  onOpenAgentChat,
  onClose,
}: {
  projectId?: string
  projectType?: AgentChatProjectType
  workspace: string
  onOpenAgentChat?: () => void
  onClose?: () => void
}) {
  const { t, i18n } = useTranslation()
  const unassignedProjectTarget = useMemo(
    () => defaultAutomationTarget({ projectId: '', workspace: '' }),
    [],
  )
  const [tasks, setTasks] = useState<AutomationTask[]>([])
  const [projects, setProjects] = useState<AutomationProjectOption[]>([])
  const [templates, setTemplates] = useState<AutomationTaskTemplate[]>([])
  const [inboxItems, setInboxItems] = useState<AutomationInboxItem[]>([])
  const [effectiveSettings, setEffectiveSettings] = useState<Settings | null>(null)
  const [activeId, setActiveId] = useState<string>('')
  const activeIdRef = useRef('')
  const [draft, setDraft] = useState<AutomationTask>(() => newAutomationTask(unassignedProjectTarget, t('automations.defaultName')))
  const [creating, setCreating] = useState(false)
  const [panelView, setPanelView] = useState<AutomationPanelView>('config')
  const [agentOpen, setAgentOpen] = useState(false)
  const [sidebarVisible, setSidebarVisible] = useState(true)
  const [saving, setSaving] = useState(false)
  const [initialLoadComplete, setInitialLoadComplete] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null)
  const [navigationTarget, setNavigationTarget] = useState<AutomationNavigationTarget | null>(null)
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
      const projectDirectory = automationProjectOptions(await getAgentChatProjects())
      const targets = projectDirectory.map((project) => automationProjectTarget(project))
      const [taskTemplates, catalogGroups] = await Promise.all([
        getAutomationTemplates(locale),
        Promise.allSettled(targets.map(async (target) => {
          const [projectTasks, projectInbox] = await Promise.all([
            getAutomations(target),
            getAutomationInbox(target),
          ])
          return {
            tasks: projectTasks.map((task) => normalizeAutomationTaskShape(task, target)),
            inbox: projectInbox,
          }
        })),
      ])
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return
      const failedProjects: AutomationProjectOption[] = []
      const normalized = catalogGroups.flatMap((group, index) => {
        if (group.status === 'fulfilled') return group.value.tasks
        const project = projectDirectory[index]
        failedProjects.push(project)
        console.error(`[automations] failed to load Project catalog project_id=${project.id}`, group.reason)
        return []
      })
      const inbox = catalogGroups.flatMap((group) => group.status === 'fulfilled' ? group.value.inbox : [])
      // A Project switch must not discard an existing dirty definition while
      // the current request is still settling.
      const preserveDraft = draftDirtyRef.current && Boolean(activeIdRef.current || creatingRef.current)
      setTasks(normalized)
      setProjects(projectDirectory)
      setTemplates(taskTemplates)
      setInboxItems(inbox)
      setError(failedProjects.length > 0
        ? t('automations.project.loadPartial', { count: failedProjects.length })
        : null)
      const selected = normalized.find((task) => automationTaskKey(task) === activeIdRef.current) ?? normalized[0]
      if (preserveDraft && selected && automationTaskKey(selected) === activeIdRef.current) {
        const previousBaseline = taskBaselineRef.current
        const draftAtReloadStart = draftRef.current
        const nextDraft = previousBaseline && automationTaskKey(previousBaseline) === activeIdRef.current
          ? await rebaseJSONWithRecovery({
              resource: 'automation',
              scope: projectId,
              id: activeIdRef.current,
              baseline: { revision: previousBaseline.revision, value: previousBaseline },
              local: { revision: draftAtReloadStart.revision, value: draftAtReloadStart },
              external: { revision: selected.revision, value: selected },
            })
          : draftRef.current
        if (!mountedRef.current || sequence !== loadSequenceRef.current) return
        taskBaselineRef.current = cloneAutomationTask(selected, unassignedProjectTarget)
        const latestDraft = draftRef.current
        const draftWithNewerEdits = latestDraft === draftAtReloadStart
          ? nextDraft
          : rebaseJSONValue(draftAtReloadStart, latestDraft, nextDraft)
        const clonedDraft = cloneAutomationTask(draftWithNewerEdits, unassignedProjectTarget)
        draftRef.current = clonedDraft
        setDraft(clonedDraft)
      } else if (!preserveDraft) {
        draftDirtyRef.current = false
        if (selected) {
          const key = automationTaskKey(selected)
          activeIdRef.current = key
          setActiveId(key)
          taskBaselineRef.current = cloneAutomationTask(selected, unassignedProjectTarget)
          const clonedDraft = cloneAutomationTask(selected, unassignedProjectTarget)
          draftRef.current = clonedDraft
          setDraft(clonedDraft)
          setCreating(false)
        } else {
          activeIdRef.current = ''
          setActiveId('')
          taskBaselineRef.current = null
          const emptyDraft = newAutomationTask(unassignedProjectTarget, t('automations.defaultName'))
          draftRef.current = emptyDraft
          setDraft(emptyDraft)
          setCreating(false)
        }
      }
    } catch (e) {
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return
      setError((e as Error).message)
    } finally {
      if (mountedRef.current && sequence === loadSequenceRef.current) setInitialLoadComplete(true)
    }
  }, [i18n.language, i18n.resolvedLanguage, t, unassignedProjectTarget])

  const draftProject = projects.find((project) => project.id === automationTaskProjectID(draft))
  useEffect(() => {
    let cancelled = false
    setEffectiveSettings(null)
    if (!draftProject) return () => { cancelled = true }
    void fetchProjectSettings(draftProject.id)
      .then((settings) => { if (!cancelled) setEffectiveSettings(settings.effective) })
      .catch((cause) => {
        if (cancelled) return
        console.error('[automations] failed to load selected Project settings', cause)
        setError(cause instanceof Error ? cause.message : String(cause))
      })
    return () => { cancelled = true }
  }, [draftProject?.id])

  const catalogActiveRuns = useMemo(() => {
    return tasks.flatMap((task) => (task.recent_runs || [])
      .filter((run) => run.status === 'running')
      .map((run) => ({ task_id: run.task_id, run })))
  }, [tasks])
  const selectedActiveRun = catalogActiveRuns.find((active) => {
    const task = findAutomationTaskForRun(tasks, active.run)
    return task ? automationTaskKey(task) === activeId : false
  })?.run
  const running = Boolean(selectedActiveRun)
  const activeRunId = selectedActiveRun?.id || ''

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
      const detail = (event as CustomEvent<WorkspaceChangeEvent>).detail
      if (!projects.some((project) => isProjectChangeForProject(detail, project.id))) return
      const paths = workspaceChangePaths(detail)
      if (paths.length > 0 && !paths.some(isAutomationTaskFile)) return
      void load()
    }
    window.addEventListener('nova:workspace-change', reloadChangedAutomation)
    return () => window.removeEventListener('nova:workspace-change', reloadChangedAutomation)
  }, [load, projects])

  const unreadInboxCount = useMemo(() => inboxItems.filter((item) => !item.read_at && item.status === 'pending').length, [inboxItems])
  const modelProfileOptions = useMemo(() => buildAutomationModelProfileOptions(effectiveSettings, draft.model_profile_id, t), [draft.model_profile_id, effectiveSettings, t])
  const inheritedAutomationProfile = useMemo(
    () => inheritedAutomationModelProfileLabel(effectiveSettings, draftProject?.type || projectType, t),
    [draftProject?.type, effectiveSettings, projectType, t],
  )

  const automationAutosave = useAutomationAutosave({
    activeId,
    creating,
    draft,
    tasks,
    fallbackTarget: draft.target?.kind ? draft.target : unassignedProjectTarget,
    onSaved: (saved, _submitted, submittedIsCurrent) => {
      setTasks((current) => upsertAutomationTask(current, saved))
      taskBaselineRef.current = cloneAutomationTask(saved, unassignedProjectTarget)
      if (submittedIsCurrent) {
        const nextDraft = cloneAutomationTask(saved, unassignedProjectTarget)
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
    const nextDraft = cloneAutomationTask(task, unassignedProjectTarget)
    taskBaselineRef.current = nextDraft
    draftRef.current = nextDraft
    setDraft(nextDraft)
    draftDirtyRef.current = false
    setCreating(false)
    setPanelView('config')
  }

  const createNew = async (owner?: AutomationProjectOption) => {
    if (!(await flushAutomationAutosave())) return
    const project = owner?.status === 'available' ? owner : defaultAutomationProject(projects, projectId)
    const target = project ? automationProjectTarget(project) : unassignedProjectTarget
    activeIdRef.current = ''
    setActiveId('')
    taskBaselineRef.current = null
    const nextDraft = newAutomationTask(target, t('automations.defaultName'))
    draftRef.current = nextDraft
    setDraft(nextDraft)
    draftDirtyRef.current = true
    setCreating(true)
    setPanelView('config')
  }

  const setDraftTemplate = (templateId: string | null) => {
    if (!creating) return
    const target = draftRef.current.target?.kind ? draftRef.current.target : unassignedProjectTarget
    const template = templateId ? templates.find((candidate) => candidate.id === templateId) : null
    if (templateId && !template) return
    const nextDraft = template
      ? newAutomationTaskFromTemplate(template, target)
      : newAutomationTask(target, t('automations.defaultName'))
    draftRef.current = nextDraft
    setDraft(nextDraft)
    draftDirtyRef.current = true
  }

  const createDraft = async () => {
    if (!creating) return
    const submitted = draftRef.current
    if (!automationTaskProjectID(submitted)) {
      setError(t('automations.project.required'))
      return
    }
    setSaving(true)
    setError(null)
    try {
      const saved = await createAutomation(submitted)
      const normalized = normalizeAutomationTaskShape(saved, unassignedProjectTarget)
      const key = automationTaskKey(normalized)
      activeIdRef.current = key
      setActiveId(key)
      const canonical = cloneAutomationTask(normalized, unassignedProjectTarget)
      taskBaselineRef.current = canonical
      const latestDraft = draftRef.current
      const nextDraft = latestDraft === submitted
        ? canonical
        : cloneAutomationTask(rebaseJSONValue(submitted, latestDraft, canonical), unassignedProjectTarget)
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
      const nextDraft = fallback ? cloneAutomationTask(fallback, unassignedProjectTarget) : newAutomationTask(unassignedProjectTarget, t('automations.defaultName'))
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
    setSaving(true)
    try {
      const run = await startAutomationRun(activeId, createAgentCommandID())
      if (!run.project_id || !run.session_id) {
        throw new Error(t('automations.run.missingConversation'))
      }
      requestAgentChatSessionNavigation({ projectId: run.project_id, sessionId: run.session_id })
      onOpenAgentChat?.()
      void load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const checkTriggers = async () => {
    if (!activeId) return
    if (!(await flushAutomationAutosave())) return
    setSaving(true)
    setError(null)
    try {
      await checkAutomation(activeId)
      const target = draftRef.current.target
      if (!target?.kind) throw new Error(t('automations.project.required'))
      const inbox = await getAutomationInbox(target)
      const targetProjectID = target.kind === 'workspace' ? target.project_id : ''
      setInboxItems((current) => [
        ...current.filter((item) => !targetProjectID || item.project_id !== targetProjectID),
        ...inbox,
      ])
      setPanelView('inbox')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const openRun = useCallback((run: AutomationRunRecord) => {
    setError(null)
    if (!run.project_id || !run.session_id) {
      setError(t('automations.run.missingConversation'))
      return
    }
    requestAgentChatSessionNavigation({ projectId: run.project_id, sessionId: run.session_id })
    onOpenAgentChat?.()
  }, [onOpenAgentChat, t])

  useEffect(() => {
    if (!navigationTarget || tasks.length === 0) return
    let cancelled = false
    void (async () => {
      if (!await flushAutomationAutosave() || cancelled) return
      const task = tasks.find((candidate) => automationTaskKey(candidate) === navigationTarget.taskId)
        || findAutomationTaskByTarget(tasks, navigationTarget.taskId, navigationTarget.workspace, navigationTarget.projectId)
      if (!task || cancelled) return
      const key = automationTaskKey(task)
      activeIdRef.current = key
      setActiveId(key)
      const nextDraft = cloneAutomationTask(task, unassignedProjectTarget)
      taskBaselineRef.current = nextDraft
      draftRef.current = nextDraft
      setDraft(nextDraft)
      draftDirtyRef.current = false
      setCreating(false)
      if (navigationTarget.inboxId) {
        setPanelView('inbox')
      } else if (navigationTarget.runId) {
        const run = task.recent_runs?.find((candidate) => candidate.id === navigationTarget.runId)
        if (run) openRun(run)
        else setPanelView('config')
      } else {
        setPanelView('config')
      }
      setNavigationTarget(null)
    })()
    return () => { cancelled = true }
  }, [flushAutomationAutosave, navigationTarget, openRun, tasks, unassignedProjectTarget])

  const confirmInboxItem = async (item: AutomationInboxItem) => {
    setError(null)
    try {
      const result = await confirmAutomationInboxItem(item.id)
      setInboxItems((current) => current.map((candidate) => candidate.id === result.item.id ? result.item : candidate))
      if (result.run) {
        openRun(result.run)
        void load()
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
  const setDraftProject = (nextProjectId: string) => {
    if (!creating) return
    const project = projects.find((candidate) => candidate.id === nextProjectId && candidate.status === 'available')
    if (!project) return
    setDraftField({ scope: 'workspace', target: automationProjectTarget(project) })
  }
  const hasEditableDraft = Boolean(activeId) || creating
  const taskListPanel = (
    <AutomationTaskCatalog
      tasks={tasks}
      projects={projects}
      activeRuns={catalogActiveRuns}
      activeId={activeId}
      agentActive={agentOpen}
      onSelect={selectTask}
      onCreate={() => void createNew()}
      onCreateForProject={(project) => void createNew(project)}
      onOpenAgent={() => setAgentOpen((open) => !open)}
    />
  )

  let panelContent: ReactNode
  if (panelView === 'inbox') {
    panelContent = (
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
    )
  } else if (hasEditableDraft) {
    panelContent = (
      <AutomationConfigPanel
        activeId={activeId}
        activeRunId={activeRunId}
        draft={draft}
        inheritedModelProfile={inheritedAutomationProfile}
        modelProfileOptions={modelProfileOptions}
        projects={projects}
        templates={templates}
        creating={creating}
        running={running}
        saving={saving}
        onChange={setDraftField}
        onOpenRun={openRun}
        onProjectChange={setDraftProject}
        onTemplateChange={setDraftTemplate}
        onRemove={() => void requestRemove()}
        onTriggersChange={setDraftTriggers}
      />
    )
  } else {
    panelContent = (
      <EmptyState
        variant="page"
        icon={Plus}
        title={t('automations.empty.title')}
        description={t('automations.empty.description')}
        action={{ label: t('automations.newTask'), onClick: () => void createNew() }}
        className="min-h-0 flex-1 overflow-y-auto px-4 py-10"
      />
    )
  }

  return (
    <FeaturePageShell
      icon={Clock3}
      title={t('automations.title')}
      leadingContent={(
        <SidebarVisibilityToggle
          visible={sidebarVisible}
          onToggle={() => setSidebarVisible((visible) => !visible)}
        />
      )}
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
          <Button type="button" size="sm" variant="outline" onClick={checkTriggers} disabled={!activeId || running || saving} className="nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]" aria-label={t('automations.checkTriggers')}>
            <RefreshCw data-icon="inline-start" />
            <span className="hidden sm:inline">{t('automations.checkTriggers')}</span>
          </Button>
          <Button type="button" size="sm" variant="secondary" onClick={runNow} disabled={!activeId || running || saving} className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-active)]" aria-label={running ? t('automations.running') : t('automations.runNow')}>
            <Play data-icon="inline-start" />
            <span className="hidden sm:inline">{running ? t('automations.running') : t('automations.runNow')}</span>
          </Button>
          {creating ? (
            <Button type="button" size="sm" variant="secondary" onClick={createDraft} disabled={saving || running} className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-active)]" aria-label={t('common.create')}>
              {saving ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Plus data-icon="inline-start" />}
              <span className="hidden sm:inline">{saving ? t('common.creating') : t('common.create')}</span>
            </Button>
          ) : null}
        </>
      )}
    >
      {!initialLoadComplete ? (
        <LoadingState label={t('common.loading')} className="min-h-0 flex-1" />
      ) : (
      <AdaptiveSurface
        left={{
          id: 'automation-tasks',
          title: t('automations.title'),
          side: 'left',
          icon: <Clock3 className="h-4 w-4" />,
          content: taskListPanel,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          desktopVisible: sidebarVisible,
          mobileClassName: 'w-[min(90vw,360px)]',
        }}
        right={agentOpen ? {
          id: 'automations-config-manager',
          title: t('automations.view.agent'),
          side: 'right',
          icon: <Bot className="h-4 w-4" />,
          content: (
            <ConfigManagerChat
              projectId={draftProject?.id || projectId}
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
          ),
          desktopClassName: 'min-h-0 border-l border-[var(--nova-border)]',
          mobileClassName: 'w-[min(92vw,420px)]',
        } : undefined}
        className="flex-1 text-xs"
        mainClassName="min-h-0 min-w-0"
        leftResize={{
          layoutKey: 'nova-automations-task-list-layout',
          label: t('layout.resize.sidebar'),
          defaultSize: '288px',
          minSize: '220px',
          maxSize: '40%',
        }}
        rightResize={{
          layoutKey: 'nova-automations-config-manager-layout',
          label: t('layout.resize.right'),
          defaultSize: '420px',
          minSize: '300px',
          maxSize: '65%',
          mainMinSize: '240px',
        }}
      >
        {({ isMobile, openLeft, openRight }) => (
          <main className="flex h-full min-h-0 flex-col">
            <div className="flex h-10 shrink-0 items-center gap-2 overflow-x-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 sm:px-4">
              {isMobile && (
                <MobilePaneTrigger
                  side="left"
                  label={t('workbench.mobile.openSidePanel', { label: t('automations.title') })}
                  onClick={openLeft}
                />
              )}
              {isMobile && agentOpen ? (
                <MobilePaneTrigger
                  side="right"
                  label={t('workbench.mobile.openSidePanel', { label: t('automations.view.agent') })}
                  onClick={openRight}
                />
              ) : null}
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
              </div>
              <div className="min-w-0 flex-1" />
              {selectedActiveRun && (
                <span className="truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 font-mono text-[11px] text-[var(--nova-text-faint)]">
                  {selectedActiveRun.status} · {selectedActiveRun.id}
                </span>
              )}
            </div>

            {panelContent}
          </main>
        )}
      </AdaptiveSurface>
      )}
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

function isAutomationTaskFile(path: string): boolean {
  const normalized = path.replaceAll('\\', '/').toLowerCase()
  return normalized === 'automations/tasks.json' || normalized.endsWith('/automations/tasks.json')
}
