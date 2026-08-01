import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquareText } from 'lucide-react'
import { toast } from 'sonner'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import type { AdaptiveSurfaceControls } from '@/components/layout/adaptive-surface'
import { EmptyState } from '@/components/common/EmptyState'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type { ReviewFeedbackBatch, ReviewFeedbackComment, ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
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
import { AgentChatSessionHistoryDialog } from './AgentChatSessionHistoryDialog'
import { AgentChatTabContent } from './AgentChatTabContent'
import { AgentChatTabBar } from './AgentChatTabBar'
import { AgentChatSecondaryPaneControl } from './AgentChatSecondaryPaneControl'
import { AgentChatWorkspaceSurface } from './AgentChatWorkspaceSurface'
import { agentChatSessionBindingKey } from './sidebar-activity'
import {
  appendTab,
  createTabId,
  emptyProjectTabState,
  moveTab,
  nextActiveTabId,
  otherTabIds,
  persistWorkbenchState,
  readStoredWorkbenchState,
  reconcileWorkbenchProjects,
  setTabPinned,
  setTabTitle,
  setTerminalTabTitle,
  tabGroup,
  tabIdsAfter,
  tabsInGroup,
  type AgentChatProjectTabState,
} from './tab-state'
import { terminalTabLabel, type AgentChatTerminalStatus } from './terminal/TerminalTabView'
import { getTerminalRuntimeStatus } from './terminal/api'
import { useTerminalSessionLifecycle } from './terminal/use-terminal-session-lifecycle'
import { useAgentChatActivityNavigator } from './use-agent-chat-activity-navigator'
import {
  AGENT_CHAT_GROUP_IDS,
  type AgentChatDocumentReviewNavigation,
  type AgentChatGroupId,
  type AgentChatPageId,
  type AgentChatPageRenderContext,
  type AgentChatReviewTab,
  type AgentChatTab,
  type TerminalCommandProfile,
  type TerminalProfileId,
} from './types'

interface AgentChatViewProps {
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  /** Project pages receive their tab's project, never the foreground Writing book. */
  renderPage: (workspace: string, pageId: AgentChatPageId, context: AgentChatPageRenderContext) => ReactNode
  renderReview: (tab: AgentChatReviewTab, disabled: boolean) => ReactNode
  documentReviewWorkspace?: string
  documentReviewFeedback?: ReviewFeedbackSelection | null
  onDocumentReviewFeedbackRemove?: (selection: ReviewFeedbackSelection, commentID: string) => void
  onDocumentReviewFeedbackSubmitted?: (feedback: ReviewFeedbackBatch) => void
  onDocumentReviewFeedbackSubmissionFailed?: (feedback: ReviewFeedbackBatch) => void
  onActivateWorkspace?: (workspace: string) => Promise<boolean>
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged?: (workspace: string, paths: string[]) => void | Promise<void>
}

function mountedTabKey(projectID: string, tabId: string) {
  return `${projectID}\u0000${tabId}`
}

function pendingSessionTitle(message: string) {
  const normalized = message.replace(/^\/plan\s*/, '').trim()
  const characters = Array.from(normalized)
  return characters.length > 60 ? `${characters.slice(0, 60).join('')}...` : normalized
}

/** Secondary content is already inside the adaptive right pane, so its own tab actions need no drawer launcher. */
const SECONDARY_PANE_CONTROLS = {
  isMobile: false,
  openPaneId: null,
  openLeft: () => {},
  openRight: () => {},
  closePane: () => {},
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
  renderPage,
  renderReview,
  documentReviewWorkspace,
  documentReviewFeedback,
  onDocumentReviewFeedbackRemove,
  onDocumentReviewFeedbackSubmitted,
  onDocumentReviewFeedbackSubmissionFailed,
  onActivateWorkspace,
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
  const [terminalStatuses, setTerminalStatuses] = useState<ReadonlyMap<string, AgentChatTerminalStatus>>(() => new Map())
  const [terminalCommands, setTerminalCommands] = useState<TerminalCommandProfile[]>([])
  const refreshSequenceRef = useRef(0)
  const pageFlushHandlersRef = useRef(new Map<string, EditorFlushHandler>())
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
        const key = mountedTabKey(projectID, tabID)
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
          const key = mountedTabKey(project.id, tab.id)
          if (current.has(key)) continue
          next ??= new Set(current)
          next.add(key)
        }
      }
      return next ?? current
    })
  }, [projects, projectsLoading, workbench.activeProjectId, workbench.projects])
  const refreshTerminalCommands = useCallback(async () => {
    try {
      const runtime = await getTerminalRuntimeStatus()
      setTerminalCommands(runtime.commands ?? [])
    } catch (error) {
      console.warn('[features/agent-chat/AgentChatView.tsx] loading terminal commands failed', { error })
    }
  }, [])

  useEffect(() => {
    void refreshTerminalCommands()
    const onSettingsUpdated = () => {
      void refreshTerminalCommands()
    }
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [refreshTerminalCommands])

  const registerPageFlushHandler = useCallback((projectID: string, tabId: string, handler: EditorFlushHandler | null) => {
    const key = mountedTabKey(projectID, tabId)
    if (handler) pageFlushHandlersRef.current.set(key, handler)
    else pageFlushHandlersRef.current.delete(key)
  }, [])

  const flushPageDrafts = useCallback(async (keys?: ReadonlySet<string>): Promise<boolean> => {
    for (const [key, flush] of pageFlushHandlersRef.current) {
      if (keys && !keys.has(key)) continue
      try {
        if (!(await flush())) return false
      } catch (error) {
        console.error('[features/agent-chat/AgentChatView.tsx] flushing project page failed', { key, error })
        return false
      }
    }
    return true
  }, [])

  useEffect(() => {
    onFlushHandlerChange?.(flushPageDrafts)
    return () => onFlushHandlerChange?.(null)
  }, [flushPageDrafts, onFlushHandlerChange])

  const activeProjectId = workbench.activeProjectId
  const activeProject = projects.find((project) => project.id === activeProjectId) ?? null

  const selectProject = useCallback((projectID: string) => {
    setWorkbench((current) => ({
      activeProjectId: projectID,
      projects: current.projects[projectID] ? current.projects : { ...current.projects, [projectID]: emptyProjectTabState() },
    }))
  }, [])

  const openTab = useCallback((tab: AgentChatTab) => {
    setWorkbench((current) => {
      const state = current.projects[tab.projectId] ?? emptyProjectTabState()
      const appended = appendTab(state.tabs, tab)
      const opened = appended.tabs.find((item) => item.id === appended.activeId)
      const group = opened ? tabGroup(opened) : tabGroup(tab)
      return {
        activeProjectId: tab.projectId,
        projects: {
          ...current.projects,
          [tab.projectId]: {
            ...state,
            tabs: appended.tabs,
            activeTabIds: { ...state.activeTabIds, [group]: appended.activeId },
            focusedGroup: group,
            secondaryVisible: group === 'secondary' || state.secondaryVisible,
          },
        },
      }
    })
    setMountedTabKeys((current) => new Set(current).add(mountedTabKey(tab.projectId, tab.id)))
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
                      pendingTitle: pendingSessionTitle(message),
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

  const closeTabs = useCallback(
    async (projectID: string, tabIds: string[]): Promise<boolean> => {
      if (!tabIds.length) return true
      const closingKeys = new Set(tabIds.map((id) => mountedTabKey(projectID, id)))
      if (!(await flushPageDrafts(closingKeys))) return false
      const closing = new Set(tabIds)
      const doomed = workbenchRef.current.projects[projectID]?.tabs.filter((tab) => closing.has(tab.id)) || []
      if (!doomed.length) return true
      markTerminalTabsClosing(doomed)

      setWorkbench((current) => {
        const currentState = current.projects[projectID]
        if (!currentState) return current
        const activeTabIds = { ...currentState.activeTabIds }
        for (const group of AGENT_CHAT_GROUP_IDS) {
          const activeId = activeTabIds[group]
          if (!activeId || !closing.has(activeId)) continue
          const candidates = tabsInGroup(currentState.tabs, group).filter((tab) => tab.id === activeId || !closing.has(tab.id))
          activeTabIds[group] = nextActiveTabId(candidates, activeId, activeId)
        }
        const tabs = currentState.tabs.filter((tab) => !closing.has(tab.id))
        return {
          ...current,
          projects: {
            ...current.projects,
            [projectID]: {
              ...currentState,
              tabs,
              activeTabIds,
              secondaryVisible: currentState.secondaryVisible && tabsInGroup(tabs, 'secondary').length > 0,
            },
          },
        }
      })
      for (const key of closingKeys) pageFlushHandlersRef.current.delete(key)
      setMountedTabKeys((current) => new Set([...current].filter((key) => !closingKeys.has(key))))
      return true
    },
    [flushPageDrafts, markTerminalTabsClosing],
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

  const activateTab = useCallback((projectID: string, group: AgentChatGroupId, tabId: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID] ?? emptyProjectTabState()
      return {
        activeProjectId: projectID,
        projects: {
          ...current.projects,
          [projectID]: {
            ...state,
            activeTabIds: { ...state.activeTabIds, [group]: tabId },
            focusedGroup: group,
            secondaryVisible: group === 'secondary' || state.secondaryVisible,
          },
        },
      }
    })
    setMountedTabKeys((current) => new Set(current).add(mountedTabKey(projectID, tabId)))
  }, [])

  const focusGroup = useCallback((projectID: string, group: AgentChatGroupId) => {
    setWorkbench((current) => {
      const state = current.projects[projectID] ?? emptyProjectTabState()
      if (current.activeProjectId === projectID && state.focusedGroup === group) return current
      return {
        activeProjectId: projectID,
        projects: {
          ...current.projects,
          [projectID]: { ...state, focusedGroup: group },
        },
      }
    })
  }, [])

  const showSecondaryPane = useCallback((projectID: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state || state.secondaryVisible || tabsInGroup(state.tabs, 'secondary').length === 0) return current
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: { ...state, secondaryVisible: true },
        },
      }
    })
  }, [])

  const hideSecondaryPane = useCallback((projectID: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state || !state.secondaryVisible) return current
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: { ...state, secondaryVisible: false, focusedGroup: 'primary' },
        },
      }
    })
  }, [])

  const renameTab = useCallback((projectID: string, tabId: string, title: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state) return current
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: {
            ...state,
            tabs: setTabTitle(state.tabs, tabId, title),
          },
        },
      }
    })
  }, [])

  const togglePinTab = useCallback((projectID: string, tabId: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state) return current
      const pinned = !state.tabs.find((tab) => tab.id === tabId)?.pinned
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: {
            ...state,
            tabs: setTabPinned(state.tabs, tabId, pinned),
          },
        },
      }
    })
  }, [])

  const relocateTab = useCallback((projectID: string, sourceId: string, group: AgentChatGroupId, beforeId: string | null) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      const source = state?.tabs.find((tab) => tab.id === sourceId)
      if (!state || !source) return current
      const from = tabGroup(source)
      const tabs = moveTab(state.tabs, sourceId, group, beforeId)
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: {
            ...state,
            tabs,
            focusedGroup: group,
            secondaryVisible: group === 'secondary' || (state.secondaryVisible && tabsInGroup(tabs, 'secondary').length > 0),
            activeTabIds:
              from === group
                ? state.activeTabIds
                : {
                    ...state.activeTabIds,
                    [from]:
                      state.activeTabIds[from] === sourceId ? nextActiveTabId(tabsInGroup(state.tabs, from), sourceId, sourceId) : state.activeTabIds[from],
                    [group]: sourceId,
                  },
          },
        },
      }
    })
  }, [])

  const openTerminal = useCallback(
    (project: AgentChatProject, group: AgentChatGroupId, profileId: TerminalProfileId, profileName?: string, command?: string) => {
      openTab({
        kind: 'terminal',
        id: createTabId('terminal'),
        projectId: project.id,
        workspace: project.path,
        group,
        profileId,
        profileName,
        command,
        title: profileId === 'custom' ? command || '' : profileId === 'shell' ? '' : profileName || profileId,
      })
    },
    [openTab],
  )

  const updateTerminalTitle = useCallback((projectID: string, tabId: string, title: string) => {
    setWorkbench((current) => {
      const state = current.projects[projectID]
      if (!state) return current
      const tabs = setTerminalTabTitle(state.tabs, tabId, title)
      if (tabs.every((tab, index) => tab === state.tabs[index])) return current
      return {
        ...current,
        projects: {
          ...current.projects,
          [projectID]: { ...state, tabs },
        },
      }
    })
  }, [])

  const handleTerminalStatusChange = useCallback((tabId: string, status: AgentChatTerminalStatus | null) => {
    setTerminalStatuses((current) => {
      if (status === null && !current.has(tabId)) return current
      if (status !== null && current.get(tabId) === status) return current
      const next = new Map(current)
      if (status === null) next.delete(tabId)
      else next.set(tabId, status)
      return next
    })
  }, [])

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

  const activateProjectWorkspace = useCallback(
    async (workspace: string): Promise<boolean> => {
      if (!(await flushPageDrafts())) return false
      return onActivateWorkspace ? onActivateWorkspace(workspace) : false
    },
    [flushPageDrafts, onActivateWorkspace],
  )

  const openDocumentReviewFeedback = useCallback(
    (workspace: string, selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => {
      const resource = comment.target
      if (selection.source !== 'document' || workspace !== documentReviewWorkspace || !resource?.id) return
      const project = projects.find((candidate) => candidate.path === workspace && candidate.type === 'book')
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
        workspace,
        target: resource.kind === 'lore_item' ? { kind: 'lore_item', id: resource.id, field: 'content' } : { kind: 'workspace_file', id: resource.id },
        commentID: comment.id,
        nonce: navigationNonceRef.current,
      })
    },
    [documentReviewWorkspace, openProjectPage, projects],
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
    [refreshProjects, selectProject, t],
  )

  const archiveProject = useCallback(async () => {
    const target = archiveTarget
    if (!target) return
    await archiveAgentChatProject(target.id)
    await refreshProjects()
  }, [archiveTarget, refreshProjects])

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
    mobileControls: Pick<AdaptiveSurfaceControls, 'isMobile' | 'openPaneId' | 'openLeft' | 'openRight' | 'closePane'>,
  ) => {
    const groupTabs = tabsInGroup(state.tabs, group)
    const secondaryTabs = tabsInGroup(state.tabs, 'secondary')
    const activeId = state.activeTabIds[group]
    const projectRunning = project.sessions.some((session) => isSessionRunning(project, session))
    const secondaryBusy = (activitiesByProject.get(project.id) ?? []).some(
      (activity) => activity.group === 'secondary' && ['running', 'connecting', 'ready'].includes(activity.status),
    )
    const openAgentTab = (target: AgentChatGroupId) => {
      openDraftSessionInProject(project, target)
      if (target === 'secondary' && mobileControls.isMobile) mobileControls.openRight()
    }
    const openTerminalTab = (target: AgentChatGroupId, profileId: TerminalProfileId, profileName?: string, command?: string) => {
      openTerminal(project, target, profileId, profileName, command)
      if (target === 'secondary' && mobileControls.isMobile) mobileControls.openRight()
    }
    const openPageTab = (target: AgentChatGroupId, pageId: AgentChatPageId) => {
      openProjectPage(project, target, pageId)
      if (target === 'secondary' && mobileControls.isMobile) mobileControls.openRight()
    }
    const secondaryControl = (
      <AgentChatSecondaryPaneControl
        visible={secondaryTabs.length > 0 && (
          mobileControls.isMobile
            ? mobileControls.openPaneId === 'agent-chat-secondary'
            : state.secondaryVisible
        )}
        hasTabs={secondaryTabs.length > 0}
        busy={secondaryBusy}
        newChatDisabled={project.status !== 'available'}
        terminalCommands={terminalCommands}
        pagesEnabled={project.type === 'book'}
        onShow={() => {
          if (mobileControls.isMobile) mobileControls.openRight()
          else showSecondaryPane(project.id)
        }}
        onHide={() => {
          if (mobileControls.isMobile) mobileControls.closePane()
          else hideSecondaryPane(project.id)
        }}
        onNewAgentTab={openAgentTab}
        onNewTerminalTab={openTerminalTab}
        onOpenPage={openPageTab}
      />
    )
    const primaryOwnsSecondaryControl = group === 'primary'
      && (mobileControls.isMobile || !state.secondaryVisible || secondaryTabs.length === 0)
    const secondaryOwnsSecondaryControl = group === 'secondary' && paneVisible
    const tabBarEndActions = primaryOwnsSecondaryControl
      ? secondaryControl
      : secondaryOwnsSecondaryControl
        ? <div className="hidden lg:flex">{secondaryControl}</div>
        : undefined
    return (
      <div
        data-agent-chat-group={group}
        className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-bg)]"
        onPointerDownCapture={() => {
          if (state.focusedGroup !== group) focusGroup(project.id, group)
        }}
        onFocusCapture={() => {
          if (state.focusedGroup !== group) focusGroup(project.id, group)
        }}
      >
        <div className="flex items-center gap-1 bg-[var(--nova-surface)] pl-1.5 md:pl-0">
          {group === 'primary' && mobileControls.isMobile && (
            <MobilePaneTrigger
              side="left"
              className="size-7 shrink-0"
              label={t('agentChat.sidebar.projects')}
              onClick={mobileControls.openLeft}
            />
          )}
          <div className="min-w-0 flex-1">
            <AgentChatTabBar
              group={group}
              tabs={groupTabs}
              activeTabId={activeId}
              tabTitle={tabTitle}
              terminalCommands={terminalCommands}
              pagesEnabled={project.type === 'book'}
              newChatDisabled={project.status !== 'available'}
              endActions={tabBarEndActions}
              onActivate={(tabId) => activateTab(project.id, group, tabId)}
              onClose={(tabId) => {
                void closeTabs(project.id, [tabId])
              }}
              onCloseOthers={(tabId) => {
                void closeTabs(project.id, otherTabIds(state.tabs, tabId))
              }}
              onCloseToRight={(tabId) => {
                void closeTabs(project.id, tabIdsAfter(state.tabs, tabId))
              }}
              onRename={(tabId, title) => renameTab(project.id, tabId, title)}
              onTogglePin={(tabId) => togglePinTab(project.id, tabId)}
              onMoveTab={(sourceId, target, beforeId) => relocateTab(project.id, sourceId, target, beforeId)}
              onNewAgentTab={openAgentTab}
              onNewTerminalTab={openTerminalTab}
              onOpenPage={openPageTab}
            />
          </div>
        </div>

        <div className="relative min-h-0 flex-1">
          {groupTabs.length === 0 ? (
            <EmptyState
              variant="page"
              icon={MessageSquareText}
              title={t('agentChat.empty.title')}
              description={t('agentChat.empty.description')}
              action={{
                label: t('agentChat.tabs.newChat'),
                onClick: () => openDraftSessionInProject(project, group),
              }}
            />
          ) : (
            groupTabs.map((tab) => {
              const active = paneVisible && tab.id === activeId
              const mounted = mountedTabKeys.has(mountedTabKey(project.id, tab.id))
              if (!active && !mounted) return null
              return (
                <section key={tab.id} hidden={!active} aria-hidden={!active} className="absolute inset-0 flex min-h-0 flex-col">
                  <AgentChatTabContent
                    tab={tab}
                    projectType={project.type}
                    workspaceCurrent={project.current}
                    active={active}
                    running={projectRunning}
                    composerSettings={composerSettings}
                    tellers={tellers}
                    imagePresets={imagePresets}
                    renderPage={renderPage}
                    renderReview={renderReview}
                    navigationIntent={documentReviewNavigation?.workspace === tab.workspace ? documentReviewNavigation : null}
                    documentReviewFeedback={tab.workspace === documentReviewWorkspace ? documentReviewFeedback : null}
                    onDocumentReviewFeedbackOpen={openDocumentReviewFeedback}
                    onDocumentReviewFeedbackRemove={onDocumentReviewFeedbackRemove}
                    onDocumentReviewFeedbackSubmitted={onDocumentReviewFeedbackSubmitted}
                    onDocumentReviewFeedbackSubmissionFailed={onDocumentReviewFeedbackSubmissionFailed}
                    onOpenPage={(projectID, groupID, pageID) => {
                      const target = projects.find((candidate) => candidate.id === projectID)
                      if (target) openProjectPage(target, groupID, pageID)
                    }}
                    onActivateWorkspace={activateProjectWorkspace}
                    onPageFlushHandlerChange={registerPageFlushHandler}
                    onOpenChangeReview={openChangeReview}
                    onWorkspaceChanged={onWorkspaceChanged}
                    onRunningChange={handleRunningChange}
                    onDraftCommitted={(message) => commitDraftSession(project.id, tab.id, message)}
                    onTerminalSessionEstablished={(tabId, session) => bindTerminalSession(project.id, tabId, session)}
                    onTerminalTitleChange={(tabId, title) => updateTerminalTitle(project.id, tabId, title)}
                    onTerminalStatusChange={handleTerminalStatusChange}
                  />
                </section>
              )
            })
          )}
        </div>
      </div>
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
        {renderProjectGroup(project, state, 'secondary', visible, SECONDARY_PANE_CONTROLS)}
      </section>
    )
  })

  return (
    <>
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
