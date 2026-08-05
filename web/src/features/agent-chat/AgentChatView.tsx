import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquareText } from 'lucide-react'
import { toast } from 'sonner'
import { EmptyState } from '@/components/common/EmptyState'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type { ReviewFeedbackComment, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import {
  addAgentChatProject,
  archiveAgentChatProject,
  deleteAgentChatSession,
  getAgentChatProjects,
  relinkAgentChatProject,
  renameAgentChatProject,
  renameAgentChatSession,
  selectAgentChatProjectDirectory,
  type AgentChatProject,
  type AgentChatSession,
} from './api'
import { AgentChatProjectRenameDialog } from './AgentChatProjectRenameDialog'
import {
  AgentChatProjectGroup,
  DESKTOP_SECONDARY_PANE_CONTROLS,
  type AgentChatPaneControls,
} from './AgentChatProjectGroup'
import { AgentChatSessionHistoryDialog } from './AgentChatSessionHistoryDialog'
import { AgentChatTabContent } from './AgentChatTabContent'
import { AgentChatTabDragContext } from './AgentChatTabDragContext'
import { AgentChatWorkspaceSurface } from './AgentChatWorkspaceSurface'
import { agentChatSessionBindingKey } from './sidebar-activity'
import {
  createTabId,
  draftSessionTitle,
  emptyProjectTabState,
  incrementProjectRefreshSignals,
  persistWorkbenchState,
  readStoredWorkbenchState,
  reconcileWorkbenchProjects,
  tabGroup,
  tabsInGroup,
  type AgentChatProjectTabState,
} from './tab-state'
import { terminalTabLabel } from './terminal/TerminalTabView'
import { useTerminalSessionLifecycle } from './terminal/use-terminal-session-lifecycle'
import { useAgentChatActivityNavigator } from './use-agent-chat-activity-navigator'
import { mountedAgentChatTabKey, useAgentChatTabWorkbench } from './use-agent-chat-tab-workbench'
import { useAgentChatTerminalTabs } from './use-agent-chat-terminal-tabs'
import {
  AGENT_CHAT_GROUP_IDS,
  type AgentChatDocumentReviewNavigation,
  type AgentChatGroupId,
  type AgentChatPageId,
  type AgentChatPageRenderContext,
  type AgentChatReviewRenderContext,
  type AgentChatReviewTab,
  type AgentChatTab,
} from './types'

interface AgentChatViewProps {
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  autoSaveEnabled?: boolean
  autoSaveDelayMs?: number
  /** Project pages receive their tab's project, never the foreground Writing book. */
  renderPage: (projectId: string, workspace: string, pageId: AgentChatPageId, context: AgentChatPageRenderContext) => ReactNode
  renderReview: (tab: AgentChatReviewTab, disabled: boolean, context: AgentChatReviewRenderContext) => ReactNode
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged?: (
    projectId: string,
    workspace: string,
    paths: string[],
    metadata: WorkspaceChangeMetadata,
  ) => void | Promise<void>
}

/**
 * User-level AgentChat workspace.
 *
 * The project tree is only a selector. Each project owns an independent persisted workbench,
 * and each conversation tab owns an independently scoped Agent runtime and display stream.
 */
export function AgentChatView({
  composerSettings,
  tellers,
  imagePresets,
  autoSaveEnabled = true,
  autoSaveDelayMs = 1200,
  renderPage,
  renderReview,
  onFlushHandlerChange,
  onWorkspaceChanged,
}: AgentChatViewProps) {
  const { t } = useTranslation()
  const [projects, setProjects] = useState<AgentChatProject[]>([])
  const [projectsLoading, setProjectsLoading] = useState(true)
  const [projectsError, setProjectsError] = useState('')
  const [workbench, setWorkbench] = useState(() => readStoredWorkbenchState())
  const workbenchRef = useRef(workbench)
  workbenchRef.current = workbench
  const [historyOpen, setHistoryOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<AgentChatProject | null>(null)
  const [projectDirectoryBusy, setProjectDirectoryBusy] = useState(false)
  const projectDirectoryBusyRef = useRef(false)
  const [archiveTarget, setArchiveTarget] = useState<AgentChatProject | null>(null)
  /** Once mounted, a tab stays mounted so hidden conversations keep receiving their own stream. */
  const [mountedTabKeys, setMountedTabKeys] = useState<ReadonlySet<string>>(() => new Set())
  /** Optimistic live state reported by mounted hooks, layered over the project snapshot. */
  const [liveRunningBindings, setLiveRunningBindings] = useState<ReadonlySet<string>>(() => new Set())
  const liveRunningBindingsRef = useRef<ReadonlySet<string>>(new Set())
  const refreshSequenceRef = useRef(0)
  const tabFlushHandlersRef = useRef(new Map<string, EditorFlushHandler>())
  const [filesEditorRefreshSignals, setFilesEditorRefreshSignals] = useState<ReadonlyMap<string, number>>(() => new Map())
  const [filesTreeRefreshSignals, setFilesTreeRefreshSignals] = useState<ReadonlyMap<string, number>>(() => new Map())
  const [projectPageRefreshSignals, setProjectPageRefreshSignals] = useState<ReadonlyMap<string, number>>(() => new Map())
  const navigationNonceRef = useRef(0)
  const [documentReviewNavigation, setDocumentReviewNavigation] = useState<AgentChatDocumentReviewNavigation | null>(null)
  const { bindTerminalSession, markTerminalTabsClosing } = useTerminalSessionLifecycle(workbench, projectsLoading, setWorkbench)

  const refreshProjects = useCallback(async (): Promise<AgentChatProject[] | null> => {
    const sequence = ++refreshSequenceRef.current
    try {
      const next = await getAgentChatProjects()
      if (refreshSequenceRef.current !== sequence) return null
      setProjects(next)
      setProjectsError('')
      setWorkbench((current) => {
        const reconciled = reconcileWorkbenchProjects(current, next)
        const activeProjectId = reconciled.activeProjectId || next.find((project) => project.current)?.id || next[0]?.id || ''
        return { ...reconciled, activeProjectId }
      })
      // Mounted conversations keep their optimistic state until their hook reports a transition.
      // Detached rows can drop the optimistic binding after this authoritative snapshot arrives;
      // a still-running task is now represented by `session.running` and remains pollable.
      const openBindings = new Set<string>()
      for (const [projectID, state] of Object.entries(workbenchRef.current.projects)) {
        for (const tab of state.tabs) {
          if (tab.kind === 'agent') openBindings.add(agentChatSessionBindingKey(projectID, tab.sessionId))
        }
      }
      const currentLive = liveRunningBindingsRef.current
      const retainedLive = new Set([...currentLive].filter((key) => openBindings.has(key)))
      if (retainedLive.size !== currentLive.size) {
        liveRunningBindingsRef.current = retainedLive
        setLiveRunningBindings(retainedLive)
      }
      return next
    } catch (error) {
      if (refreshSequenceRef.current !== sequence) return null
      console.error('[features/agent-chat/AgentChatView.tsx] loading projects failed', { error })
      setProjectsError(error instanceof Error ? error.message : String(error))
      return null
    } finally {
      if (refreshSequenceRef.current === sequence) setProjectsLoading(false)
    }
  }, [])

  useEffect(() => {
    void refreshProjects()
  }, [refreshProjects])
  useEffect(() => {
    if (!projectsLoading) persistWorkbenchState(workbench)
  }, [projectsLoading, workbench])
  useEffect(() => {
    if (projectsLoading) return
    setMountedTabKeys((current) => {
      let next: Set<string> | null = null
      const mount = (projectID: string, tabID: string) => {
        const key = mountedAgentChatTabKey(projectID, tabID)
        if (current.has(key)) return
        next ??= new Set(current)
        next.add(key)
      }
      const activeState = workbench.projects[workbench.activeProjectId]
      if (activeState?.activeTabIds.primary) mount(workbench.activeProjectId, activeState.activeTabIds.primary)
      if (activeState?.secondaryVisible && activeState.activeTabIds.secondary) {
        mount(workbench.activeProjectId, activeState.activeTabIds.secondary)
      }
      for (const project of projects) {
        const runningSessionIDs = new Set(project.sessions.filter((session) => session.running).map((session) => session.id))
        if (runningSessionIDs.size === 0) continue
        for (const tab of workbench.projects[project.id]?.tabs ?? []) {
          if (tab.kind !== 'agent' || !runningSessionIDs.has(tab.sessionId)) continue
          const key = mountedAgentChatTabKey(project.id, tab.id)
          if (current.has(key)) continue
          next ??= new Set(current)
          next.add(key)
        }
      }
      return next ?? current
    })
  }, [projects, projectsLoading, workbench.activeProjectId, workbench.projects])
  const registerTabFlushHandler = useCallback((projectID: string, tabId: string, handler: EditorFlushHandler | null) => {
    const key = mountedAgentChatTabKey(projectID, tabId)
    if (handler) tabFlushHandlersRef.current.set(key, handler)
    else tabFlushHandlersRef.current.delete(key)
  }, [])

  const flushTabDrafts = useCallback(async (keys?: ReadonlySet<string>): Promise<boolean> => {
    for (const [key, flush] of tabFlushHandlersRef.current) {
      if (keys && !keys.has(key)) continue
      try {
        if (!(await flush())) return false
      } catch (error) {
        console.error('[features/agent-chat/AgentChatView.tsx] flushing tab draft failed', { key, error })
        return false
      }
    }
    return true
  }, [])

  const flushProjectDrafts = useCallback((projectID: string) => {
    const tabs = workbenchRef.current.projects[projectID]?.tabs ?? []
    return flushTabDrafts(new Set(tabs.map((tab) => mountedAgentChatTabKey(projectID, tab.id))))
  }, [flushTabDrafts])

  useEffect(() => {
    onFlushHandlerChange?.(flushTabDrafts)
    return () => onFlushHandlerChange?.(null)
  }, [flushTabDrafts, onFlushHandlerChange])

  const {
    activateTab,
    closeTabs,
    focusGroup,
    hideSecondaryPane,
    openTab,
    relocateTab,
    renameTab,
    showSecondaryPane,
    togglePinTab,
  } = useAgentChatTabWorkbench({
    workbenchRef,
    setWorkbench,
    setMountedTabKeys,
    flushTabDrafts,
    tabFlushHandlersRef,
    markTerminalTabsClosing,
  })
  const {
    commands: terminalCommands,
    statuses: terminalStatuses,
    openTerminal,
    updateTitle: updateTerminalTitle,
    handleStatusChange: handleTerminalStatusChange,
  } = useAgentChatTerminalTabs({ setWorkbench, openTab })

  const activeProjectId = workbench.activeProjectId
  const activeProject = projects.find((project) => project.id === activeProjectId) ?? null

  const selectProject = useCallback((projectID: string) => {
    setWorkbench((current) => ({
      activeProjectId: projectID,
      projects: current.projects[projectID] ? current.projects : { ...current.projects, [projectID]: emptyProjectTabState() },
    }))
  }, [])

  const openSessionTab = useCallback(
    (project: AgentChatProject, session: AgentChatSession) => {
      openTab({
        kind: 'agent',
        id: createTabId('agent'),
        projectId: project.id,
        workspace: project.path,
        group: 'primary',
        sessionId: session.id,
      })
    },
    [openTab],
  )

  const openDraftSessionInProject = useCallback(
    (project: AgentChatProject, group: AgentChatGroupId = 'primary') => {
      const tabId = createTabId('agent')
      openTab({
        kind: 'agent',
        id: tabId,
        projectId: project.id,
        workspace: project.path,
        group,
        sessionId: `s-${tabId}`,
        draft: true,
      })
    },
    [openTab],
  )

  const commitDraftSession = useCallback(
    (projectID: string, tabId: string, message: string) => {
      setWorkbench((current) => {
        const state = current.projects[projectID]
        if (!state) return current
        return {
          ...current,
          projects: {
            ...current.projects,
            [projectID]: {
              ...state,
              tabs: state.tabs.map((tab) =>
                tab.id === tabId && tab.kind === 'agent' && tab.draft
                  ? {
                      ...tab,
                      draft: undefined,
                      pendingTitle: draftSessionTitle(message),
                    }
                  : tab,
              ),
            },
          },
        }
      })
      void refreshProjects()
    },
    [refreshProjects],
  )

  const renameSession = useCallback(
    async (projectID: string, session: AgentChatSession, title: string) => {
      try {
        await renameAgentChatSession(projectID, session.id, title)
        await refreshProjects()
      } catch (error) {
        console.error('[features/agent-chat/AgentChatView.tsx] renaming session failed', { sessionId: session.id, error })
        throw error
      }
    },
    [refreshProjects],
  )

  const deleteSession = useCallback(
    async (projectID: string, session: AgentChatSession) => {
      try {
        await deleteAgentChatSession(projectID, session.id)
        const state = workbench.projects[projectID]
        await closeTabs(projectID, state?.tabs.filter((tab) => tab.kind === 'agent' && tab.sessionId === session.id).map((tab) => tab.id) || [])
        await refreshProjects()
      } catch (error) {
        console.error('[features/agent-chat/AgentChatView.tsx] deleting session failed', { sessionId: session.id, error })
        throw error
      }
    },
    [closeTabs, refreshProjects, workbench.projects],
  )

  const openProjectPage = useCallback(
    (project: AgentChatProject, group: AgentChatGroupId, pageId: AgentChatPageId) => {
      if (project.type !== 'book') return
      openTab({
        kind: 'page',
        id: createTabId('page'),
        projectId: project.id,
        workspace: project.path,
        group,
        pageId,
      })
    },
    [openTab],
  )

  const openProjectFiles = useCallback(
    (project: AgentChatProject, group: AgentChatGroupId, selectedPath?: string) => {
      openTab({
        kind: 'files',
        id: createTabId('files'),
        projectId: project.id,
        workspace: project.path,
        group,
        selectedPath,
      })
    },
    [openTab],
  )

  const setFilesSelectedPath = useCallback((projectID: string, tabId: string, path: string | null) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state) return current
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: {
            ...state,
            tabs: state.tabs.map((tab) => (
              tab.id === tabId && tab.kind === 'files'
                ? { ...tab, selectedPath: path || undefined }
                : tab
            )),
          },
        },
      }
    })
  }, [])

  const openProjectFile = useCallback((projectID: string, path: string, preferredGroup: AgentChatGroupId) => {
    const project = projects.find((candidate) => candidate.id === projectID)
    if (!project) return
    const existing = workbenchRef.current.projects[projectID]?.tabs.find((tab) => tab.kind === 'files')
    openProjectFiles(project, existing ? tabGroup(existing) : preferredGroup, path)
  }, [openProjectFiles, projects])

  const handleWorkspaceChanged = useCallback(async (
    changedProjectId: string,
    changedWorkspace: string,
    paths: string[],
    metadata: WorkspaceChangeMetadata,
  ) => {
    if (changedProjectId && metadata.origin !== 'files-tab') {
      setFilesEditorRefreshSignals((current) => incrementProjectRefreshSignals(current, [changedProjectId]))
      if (metadata.impact === 'structure') {
        setFilesTreeRefreshSignals((current) => incrementProjectRefreshSignals(current, [changedProjectId]))
      }
    }
    if (changedProjectId && metadata.origin !== 'project-page') {
      setProjectPageRefreshSignals((current) => incrementProjectRefreshSignals(current, [changedProjectId]))
    }
    await onWorkspaceChanged?.(changedProjectId, changedWorkspace, paths, metadata)
  }, [onWorkspaceChanged])

  const openDocumentReviewFeedback = useCallback(
    (projectID: string, selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => {
      const resource = comment.target
      if (selection.source !== 'document' || !resource?.id) return
      const project = projects.find((candidate) => candidate.id === projectID && candidate.type === 'book')
      if (!project) return
      const pageId: AgentChatPageId = resource.kind === 'lore_item' ? 'lore' : 'reader'
      const state = workbenchRef.current.projects[project.id] ?? emptyProjectTabState()
      const existingPage = state.tabs.find((tab) => tab.kind === 'page' && tab.pageId === pageId)
      const conversationGroup = AGENT_CHAT_GROUP_IDS.find((group) => {
        const activeTab = state.tabs.find((tab) => tab.id === state.activeTabIds[group])
        return activeTab?.kind === 'agent'
      })
      const targetGroup = existingPage ? tabGroup(existingPage) : conversationGroup === 'primary' ? 'secondary' : 'primary'
      openProjectPage(project, targetGroup, pageId)
      navigationNonceRef.current += 1
      setDocumentReviewNavigation({
        projectId: project.id,
        target: resource.kind === 'lore_item' ? { kind: 'lore_item', id: resource.id, field: 'content' } : { kind: 'workspace_file', id: resource.id },
        commentID: comment.id,
        nonce: navigationNonceRef.current,
      })
    },
    [openProjectPage, projects],
  )

  const openChangeReview = useCallback(
    (projectID: string, workspace: string, reviewThreadID: string, groupID: string) => {
      const state = workbench.projects[projectID] ?? emptyProjectTabState()
      const conversationGroup = AGENT_CHAT_GROUP_IDS.find((group) => {
        const tab = state.tabs.find((candidate) => candidate.id === state.activeTabIds[group])
        return tab?.kind === 'agent'
      })
      openTab({
        kind: 'review',
        id: createTabId('review'),
        projectId: projectID,
        workspace,
        group: conversationGroup === 'primary' ? 'secondary' : 'primary',
        threadID: reviewThreadID,
        groupID: groupID || undefined,
      })
    },
    [openTab, workbench.projects],
  )

  const handleRunningChange = useCallback(
    (projectID: string, sessionId: string, running: boolean | null) => {
      const key = agentChatSessionBindingKey(projectID, sessionId)
      const current = liveRunningBindingsRef.current
      const wasRunning = current.has(key)
      console.debug(
        `[features/agent-chat/AgentChatView.tsx] conversation running state changed session=${sessionId} running=${String(running)} was_running=${wasRunning}`,
      )
      if (running === null) {
        // Unmount is loss of the local observer, not proof that the backend task stopped. Keep the
        // optimistic row until an authoritative project snapshot takes over.
        if (wasRunning) void refreshProjects()
        return
      }
      const next = new Set(current)
      if (running) next.add(key)
      else next.delete(key)
      if (next.size !== current.size || [...next].some((item) => !current.has(item))) {
        liveRunningBindingsRef.current = next
        setLiveRunningBindings(next)
      }
      // Initial idle reports are state synchronization, not completed runs. Only
      // a real true → false transition needs a fresh backend running snapshot.
      if (running === false && wasRunning) void refreshProjects()
    },
    [refreshProjects],
  )

  const sessionTitles = useMemo(() => {
    const titles = new Map<string, string>()
    for (const project of projects) {
      for (const session of project.sessions) {
        titles.set(agentChatSessionBindingKey(project.id, session.id), session.title)
      }
    }
    return titles
  }, [projects])

  const tabTitle = useCallback(
    (tab: AgentChatTab) => {
      if (tab.customTitle) return tab.customTitle
      switch (tab.kind) {
        case 'agent':
          return sessionTitles.get(agentChatSessionBindingKey(tab.projectId, tab.sessionId)) || tab.pendingTitle || t('chat.untitledSession')
        case 'terminal':
          return terminalTabLabel(tab, t)
        case 'files':
          return t('files.title')
        case 'page':
          return t(`agentChat.page.${tab.pageId}`)
        case 'review':
          return t('agentChat.review.tab')
      }
    },
    [sessionTitles, t],
  )

  const { activitiesByProject, isSessionRunning, openHistorySession, openSidebarActivity } = useAgentChatActivityNavigator({
    projects,
    workbench,
    liveRunningBindings,
    terminalStatuses,
    tabTitle,
    refreshProjects,
    activateTab,
    openSessionTab,
  })

  const renameProject = useCallback(
    async (name: string) => {
      if (!renameTarget) return
      await renameAgentChatProject(renameTarget.id, name)
      await refreshProjects()
    },
    [refreshProjects, renameTarget],
  )

  const chooseProjectDirectory = useCallback(
    async (relinkTarget?: AgentChatProject) => {
      if (projectDirectoryBusyRef.current) return
      projectDirectoryBusyRef.current = true
      setProjectDirectoryBusy(true)
      try {
        const selection = await selectAgentChatProjectDirectory(relinkTarget?.path)
        if (selection.canceled || !selection.path) return
        if (relinkTarget) {
          if (!(await flushProjectDrafts(relinkTarget.id))) return
          await relinkAgentChatProject(relinkTarget.id, selection.path)
          await refreshProjects()
          return
        }
        const added = await addAgentChatProject(selection.path)
        await refreshProjects()
        selectProject(added.id)
      } catch (error) {
        console.error('[features/agent-chat/AgentChatView.tsx] project directory selection failed', {
          action: relinkTarget ? 'relink' : 'add',
          projectID: relinkTarget?.id,
          error,
        })
        toast.error(t(relinkTarget ? 'agentChat.project.relinkFailed' : 'agentChat.project.addFailed'), {
          description: error instanceof Error ? error.message : String(error),
        })
      } finally {
        projectDirectoryBusyRef.current = false
        setProjectDirectoryBusy(false)
      }
    },
    [flushProjectDrafts, refreshProjects, selectProject, t],
  )

  const archiveProject = useCallback(async (): Promise<boolean> => {
    const target = archiveTarget
    if (!target) return false
    if (!(await flushProjectDrafts(target.id))) return false
    await archiveAgentChatProject(target.id)
    await refreshProjects()
    return true
  }, [archiveTarget, flushProjectDrafts, refreshProjects])

  const treeProps = {
    projects,
    activitiesByProject,
    loading: projectsLoading,
    error: projectsError,
    activeProjectId,
    onSelectProject: (project: AgentChatProject) => selectProject(project.id),
    onOpenActivity: openSidebarActivity,
    onCreateSession: (project: AgentChatProject) => openDraftSessionInProject(project),
    onOpenHistory: () => setHistoryOpen(true),
    onAddProject: () => void chooseProjectDirectory(),
    projectDirectoryBusy,
    onRenameProject: (project: AgentChatProject) => setRenameTarget(project),
    onRelinkProject: (project: AgentChatProject) => void chooseProjectDirectory(project),
    onArchiveProject: (project: AgentChatProject) => setArchiveTarget(project),
  }

  const renderProjectGroup = (
    project: AgentChatProject,
    state: AgentChatProjectTabState,
    group: AgentChatGroupId,
    paneVisible: boolean,
    mobileControls: AgentChatPaneControls,
  ) => {
    const projectRunning = project.sessions.some((session) => isSessionRunning(project, session))
    const secondaryBusy = (activitiesByProject.get(project.id) ?? []).some(
      (activity) => activity.group === 'secondary' && ['running', 'connecting', 'ready'].includes(activity.status),
    )
    return (
      <AgentChatProjectGroup
        project={project}
        state={state}
        group={group}
        paneVisible={paneVisible}
        mobileControls={mobileControls}
        mountedTabKeys={mountedTabKeys}
        terminalCommands={terminalCommands}
        secondaryBusy={secondaryBusy}
        tabTitle={tabTitle}
        renderTab={(tab, active) => (
          <AgentChatTabContent
            tab={tab}
            projectType={project.type}
            workspaceCurrent={project.current}
            active={active}
            running={projectRunning}
            composerSettings={composerSettings}
            tellers={tellers}
            imagePresets={imagePresets}
            autoSaveEnabled={autoSaveEnabled}
            autoSaveDelayMs={autoSaveDelayMs}
            filesEditorRefreshSignal={filesEditorRefreshSignals.get(project.id) ?? 0}
            filesTreeRefreshSignal={filesTreeRefreshSignals.get(project.id) ?? 0}
            projectPageRefreshSignal={projectPageRefreshSignals.get(project.id) ?? 0}
            renderPage={renderPage}
            renderReview={renderReview}
            navigationIntent={documentReviewNavigation?.projectId === tab.projectId ? documentReviewNavigation : null}
            onDocumentReviewFeedbackOpen={openDocumentReviewFeedback}
            onOpenPage={(projectID, groupID, pageID) => {
              const target = projects.find((candidate) => candidate.id === projectID)
              if (target) openProjectPage(target, groupID, pageID)
            }}
            onFlushHandlerChange={registerTabFlushHandler}
            onFilesSelectedPathChange={setFilesSelectedPath}
            onOpenProjectFile={openProjectFile}
            onOpenChangeReview={openChangeReview}
            onWorkspaceChanged={handleWorkspaceChanged}
            onRunningChange={handleRunningChange}
            onDraftCommitted={(message) => commitDraftSession(project.id, tab.id, message)}
            onTerminalSessionEstablished={(tabID, session) => bindTerminalSession(project.id, tabID, session)}
            onTerminalTitleChange={(tabID, title) => updateTerminalTitle(project.id, tabID, title)}
            onTerminalStatusChange={handleTerminalStatusChange}
          />
        )}
        onFocus={(target) => focusGroup(project.id, target)}
        onActivate={(target, tabID) => activateTab(project.id, target, tabID)}
        onClose={(tabIDs) => { void closeTabs(project.id, tabIDs) }}
        onRename={(tabID, title) => renameTab(project.id, tabID, title)}
        onTogglePin={(tabID) => togglePinTab(project.id, tabID)}
        onMoveTab={(sourceID, target, beforeID) => relocateTab(project.id, sourceID, target, beforeID)}
        onNewAgentTab={(target) => openDraftSessionInProject(project, target)}
        onNewTerminalTab={(target, profileID, profileName, command) => (
          openTerminal(project, target, profileID, profileName, command)
        )}
        onOpenFiles={(target) => openProjectFiles(project, target)}
        onOpenPage={(target, pageID) => openProjectPage(project, target, pageID)}
        onShowSecondary={() => showSecondaryPane(project.id)}
        onHideSecondary={() => hideSecondaryPane(project.id)}
      />
    )
  }

  const activeProjectState = activeProject ? workbench.projects[activeProject.id] ?? emptyProjectTabState() : null
  const secondaryVisible = Boolean(
    activeProjectState?.secondaryVisible && tabsInGroup(activeProjectState.tabs, 'secondary').length > 0,
  )
  const secondaryProjectLayers = projects.map((project) => {
    const state = workbench.projects[project.id] ?? emptyProjectTabState()
    const visible = project.id === activeProjectId && state.secondaryVisible
    return (
      <section key={project.id} hidden={!visible} aria-hidden={!visible} className="absolute inset-0 flex min-h-0 flex-col">
        {renderProjectGroup(project, state, 'secondary', visible, DESKTOP_SECONDARY_PANE_CONTROLS)}
      </section>
    )
  })

  return (
    <>
      <AgentChatTabDragContext
        workbench={workbench}
        onMoveTab={relocateTab}
      >
        <AgentChatWorkspaceSurface
          sidebarProps={treeProps}
          secondaryPane={{
            content: <div className="relative h-full min-h-0">{secondaryProjectLayers}</div>,
            visible: secondaryVisible,
            layoutKey: `nova-agent-chat-secondary-layout:v1:${activeProjectId || 'empty'}`,
            onOpen: () => {
              if (activeProject) showSecondaryPane(activeProject.id)
            },
            onClose: () => {
              if (activeProject) hideSecondaryPane(activeProject.id)
            },
          }}
          createDisabled={!activeProject || activeProject.status !== 'available'}
          onCreateDefaultSession={() => {
            if (activeProject?.status === 'available') openDraftSessionInProject(activeProject)
          }}
        >
          {(controls) => {
            const projectLayers = projects.map((project) => {
              const state = workbench.projects[project.id] ?? emptyProjectTabState()
              const visible = project.id === activeProjectId
              return (
                <section key={project.id} hidden={!visible} aria-hidden={!visible} className="absolute inset-0 flex min-h-0 flex-col">
                  {renderProjectGroup(project, state, 'primary', visible, controls)}
                </section>
              )
            })

            const workbenchContent =
              activeProject?.status === 'missing' ? (
                <EmptyState
                  variant="page"
                  icon={MessageSquareText}
                  title={t('agentChat.project.missing')}
                  description={activeProject.path}
                  action={{
                    label: t(projectDirectoryBusy ? 'agentChat.project.selectingDirectory' : 'agentChat.project.relink'),
                    onClick: () => void chooseProjectDirectory(activeProject),
                  }}
                />
              ) : activeProject ? (
                <div className="relative h-full min-h-0">{projectLayers}</div>
              ) : (
                <EmptyState
                  variant="page"
                  icon={MessageSquareText}
                  title={t('agentChat.empty.noWorkspace')}
                  action={{
                    label: t(projectDirectoryBusy ? 'agentChat.project.selectingDirectory' : 'agentChat.project.add'),
                    onClick: () => void chooseProjectDirectory(),
                  }}
                />
              )
            return workbenchContent
          }}
        </AgentChatWorkspaceSurface>
      </AgentChatTabDragContext>
      <AgentChatSessionHistoryDialog
        open={historyOpen}
        projects={projects}
        currentProjectId={activeProjectId}
        onOpenChange={setHistoryOpen}
        onOpenSession={openHistorySession}
        onRenameSession={(item, title) => renameSession(item.project_id, item.session, title)}
        onDeleteSession={(item) => deleteSession(item.project_id, item.session)}
      />
      <AgentChatProjectRenameDialog
        project={renameTarget}
        onOpenChange={(open) => {
          if (!open) setRenameTarget(null)
        }}
        onRename={renameProject}
      />
      <ConfirmDialog
        open={Boolean(archiveTarget)}
        onOpenChange={(open) => {
          if (!open) setArchiveTarget(null)
        }}
        title={t('agentChat.project.archiveTitle')}
        description={t('agentChat.project.archiveDescription', {
          name: archiveTarget?.name || archiveTarget?.path,
        })}
        confirmLabel={t('agentChat.project.archive')}
        tone="danger"
        detailContent={
          archiveTarget ? (
            <div
              className="truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-xs text-[var(--nova-text-faint)]"
              title={archiveTarget.path}
            >
              {archiveTarget.path}
            </div>
          ) : null
        }
        onConfirm={archiveProject}
      />
    </>
  )
}
