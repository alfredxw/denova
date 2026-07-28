import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquareText } from 'lucide-react'
import { toast } from 'sonner'
import { Group, Panel, Separator } from 'react-resizable-panels'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { EmptyState } from '@/components/common/EmptyState'
import type { WritingComposerSettingsController } from '@/components/Chat/AgentPanel'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type {
  ReviewFeedbackBatch,
  ReviewFeedbackComment,
  ReviewFeedbackSelection,
} from '@/features/changes/agent/ReviewFeedbackTray'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import {
  deleteAgentChatSession,
  getAgentChatProjects,
  renameAgentChatSession,
  type AgentChatProject,
  type AgentChatSession,
} from './api'
import { AgentChatSessionSidebar, AgentChatSidebarRail } from './AgentChatSessionSidebar'
import { AgentChatTabContent } from './AgentChatTabContent'
import { AgentChatTabBar } from './AgentChatTabBar'
import {
  appendTab,
  createTabId,
  emptyProjectTabState,
  moveTab,
  nextActiveTabId,
  otherTabIds,
  persistSidebarVisible,
  persistWorkbenchState,
  readSidebarVisible,
  readStoredWorkbenchState,
  setTabPinned,
  setTabTitle,
  setTerminalTabTitle,
  tabGroup,
  tabIdsAfter,
  tabsInGroup,
  type AgentChatProjectTabState,
} from './tab-state'
import { terminalTabLabel } from './terminal/TerminalTabView'
import { useTerminalSessionLifecycle } from './terminal/use-terminal-session-lifecycle'
import {
  AGENT_CHAT_GROUP_IDS,
  type AgentChatDocumentReviewNavigation,
  type AgentChatGroupId,
  type AgentChatPageId,
  type AgentChatPageRenderContext,
  type AgentChatReviewTab,
  type AgentChatTab,
  type TerminalProfileId,
} from './types'

interface AgentChatViewProps {
  composerSettings: WritingComposerSettingsController
  tellers: Teller[]
  imagePresets: ImagePreset[]
  /** Project pages receive their tab's project, never the foreground Writing book. */
  renderPage: (workspace: string, pageId: AgentChatPageId, context: AgentChatPageRenderContext) => ReactNode
  renderReview: (tab: AgentChatReviewTab, close: () => void, disabled: boolean) => ReactNode
  documentReviewWorkspace?: string
  documentReviewFeedback?: ReviewFeedbackSelection | null
  onDocumentReviewFeedbackRemove?: (selection: ReviewFeedbackSelection, commentID: string) => void
  onDocumentReviewFeedbackSubmitted?: (feedback: ReviewFeedbackBatch) => void
  onDocumentReviewFeedbackSubmissionFailed?: (feedback: ReviewFeedbackBatch) => void
  onActivateWorkspace?: (workspace: string) => Promise<boolean>
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged?: (workspace: string, paths: string[]) => void | Promise<void>
}

function bindingKey(workspace: string, sessionId: string) {
  return `${workspace}\u0000${sessionId}`
}

function mountedTabKey(workspace: string, tabId: string) {
  return `${workspace}\u0000${tabId}`
}

function pendingSessionTitle(message: string) {
  const normalized = message.replace(/^\/plan\s*/, '').trim()
  const characters = Array.from(normalized)
  return characters.length > 60 ? `${characters.slice(0, 60).join('')}...` : normalized
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
  const [sidebarVisible, setSidebarVisible] = useState(() => readSidebarVisible())
  /** Once mounted, a tab stays mounted so hidden conversations keep receiving their own stream. */
  const [mountedTabKeys, setMountedTabKeys] = useState<ReadonlySet<string>>(() => new Set())
  /** Optimistic live state reported by mounted hooks, layered over the project snapshot. */
  const [liveRunningBindings, setLiveRunningBindings] = useState<ReadonlySet<string>>(() => new Set())
  const liveRunningBindingsRef = useRef<ReadonlySet<string>>(new Set())
  const refreshSequenceRef = useRef(0)
  const pageFlushHandlersRef = useRef(new Map<string, EditorFlushHandler>())
  const navigationNonceRef = useRef(0)
  const [documentReviewNavigation, setDocumentReviewNavigation] = useState<AgentChatDocumentReviewNavigation | null>(null)
  const { bindTerminalSession, markTerminalTabsClosing } = useTerminalSessionLifecycle(
    workbench,
    projectsLoading,
    setWorkbench,
  )

  const refreshProjects = useCallback(async () => {
    const sequence = ++refreshSequenceRef.current
    try {
      const next = await getAgentChatProjects()
      if (refreshSequenceRef.current !== sequence) return
      setProjects(next)
      setProjectsError('')
      setWorkbench((current) => {
        const registered = new Set(next.map((project) => project.path))
        const projectStates = Object.fromEntries(
          Object.entries(current.projects).filter(([path]) => registered.has(path)),
        )
        const activeProjectPath = registered.has(current.activeProjectPath)
          ? current.activeProjectPath
          : next.find((project) => project.current)?.path || next[0]?.path || ''
        return { activeProjectPath, projects: projectStates }
      })
    } catch (error) {
      if (refreshSequenceRef.current !== sequence) return
      console.error('[features/agent-chat/AgentChatView.tsx] loading projects failed', { error })
      setProjectsError(error instanceof Error ? error.message : String(error))
    } finally {
      if (refreshSequenceRef.current === sequence) setProjectsLoading(false)
    }
  }, [])

  useEffect(() => { void refreshProjects() }, [refreshProjects])
  useEffect(() => { persistWorkbenchState(workbench) }, [workbench])
  useEffect(() => { persistSidebarVisible(sidebarVisible) }, [sidebarVisible])

  const registerPageFlushHandler = useCallback((workspace: string, tabId: string, handler: EditorFlushHandler | null) => {
    const key = mountedTabKey(workspace, tabId)
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

  const activeProjectPath = workbench.activeProjectPath
  const activeProject = projects.find((project) => project.path === activeProjectPath) ?? null

  const selectProject = useCallback((path: string) => {
    setWorkbench((current) => ({
      activeProjectPath: path,
      projects: current.projects[path]
        ? current.projects
        : { ...current.projects, [path]: emptyProjectTabState() },
    }))
  }, [])

  const openTab = useCallback((tab: AgentChatTab) => {
    setWorkbench((current) => {
      const state = current.projects[tab.workspace] ?? emptyProjectTabState()
      const appended = appendTab(state.tabs, tab)
      const opened = appended.tabs.find((item) => item.id === appended.activeId)
      const group = opened ? tabGroup(opened) : tabGroup(tab)
      return {
        activeProjectPath: tab.workspace,
        projects: {
          ...current.projects,
          [tab.workspace]: {
            tabs: appended.tabs,
            activeTabIds: { ...state.activeTabIds, [group]: appended.activeId },
            focusedGroup: group,
          },
        },
      }
    })
    setMountedTabKeys((current) => new Set(current).add(mountedTabKey(tab.workspace, tab.id)))
  }, [])

  const openSessionTab = useCallback((project: AgentChatProject, session: AgentChatSession) => {
    openTab({
      kind: 'agent',
      id: createTabId('agent'),
      workspace: project.path,
      group: 'primary',
      sessionId: session.id,
    })
  }, [openTab])

  const openDraftSessionInProject = useCallback((project: AgentChatProject, group: AgentChatGroupId = 'primary') => {
    const tabId = createTabId('agent')
    openTab({
      kind: 'agent',
      id: tabId,
      workspace: project.path,
      group,
      sessionId: `s-${tabId}`,
      draft: true,
    })
  }, [openTab])

  const commitDraftSession = useCallback((workspace: string, tabId: string, message: string) => {
    setWorkbench((current) => {
      const state = current.projects[workspace]
      if (!state) return current
      return {
        ...current,
        projects: {
          ...current.projects,
          [workspace]: {
            ...state,
            tabs: state.tabs.map((tab) => tab.id === tabId && tab.kind === 'agent' && tab.draft
              ? { ...tab, draft: undefined, pendingTitle: pendingSessionTitle(message) }
              : tab),
          },
        },
      }
    })
    void refreshProjects()
  }, [refreshProjects])

  const renameSession = useCallback(async (project: AgentChatProject, session: AgentChatSession, title: string) => {
    try {
      await renameAgentChatSession(project.path, session.id, title)
      await refreshProjects()
    } catch (error) {
      console.error('[features/agent-chat/AgentChatView.tsx] renaming session failed', { sessionId: session.id, error })
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }, [refreshProjects])

  const closeTabs = useCallback(async (workspace: string, tabIds: string[]): Promise<boolean> => {
    if (!tabIds.length) return true
    const closingKeys = new Set(tabIds.map((id) => mountedTabKey(workspace, id)))
    if (!(await flushPageDrafts(closingKeys))) return false
    const closing = new Set(tabIds)
    const doomed = workbenchRef.current.projects[workspace]?.tabs.filter((tab) => closing.has(tab.id)) || []
    if (!doomed.length) return true
    markTerminalTabsClosing(doomed)

    setWorkbench((current) => {
      const currentState = current.projects[workspace]
      if (!currentState) return current
      const activeTabIds = { ...currentState.activeTabIds }
      for (const group of AGENT_CHAT_GROUP_IDS) {
        const activeId = activeTabIds[group]
        if (!activeId || !closing.has(activeId)) continue
        const candidates = tabsInGroup(currentState.tabs, group)
          .filter((tab) => tab.id === activeId || !closing.has(tab.id))
        activeTabIds[group] = nextActiveTabId(candidates, activeId, activeId)
      }
      return {
        ...current,
        projects: {
          ...current.projects,
          [workspace]: {
            ...currentState,
            tabs: currentState.tabs.filter((tab) => !closing.has(tab.id)),
            activeTabIds,
          },
        },
      }
    })
    for (const key of closingKeys) pageFlushHandlersRef.current.delete(key)
    setMountedTabKeys((current) => new Set([...current].filter((key) => !closingKeys.has(key))))
    return true
  }, [flushPageDrafts, markTerminalTabsClosing])

  const deleteSession = useCallback(async (project: AgentChatProject, session: AgentChatSession) => {
    try {
      await deleteAgentChatSession(project.path, session.id)
      const state = workbench.projects[project.path]
      await closeTabs(project.path, state?.tabs
        .filter((tab) => tab.kind === 'agent' && tab.sessionId === session.id)
        .map((tab) => tab.id) || [])
      await refreshProjects()
    } catch (error) {
      console.error('[features/agent-chat/AgentChatView.tsx] deleting session failed', { sessionId: session.id, error })
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }, [closeTabs, refreshProjects, workbench.projects])

  const activateTab = useCallback((workspace: string, group: AgentChatGroupId, tabId: string) => {
    setWorkbench((current) => {
      const state = current.projects[workspace] ?? emptyProjectTabState()
      return {
        activeProjectPath: workspace,
        projects: {
          ...current.projects,
          [workspace]: {
            ...state,
            activeTabIds: { ...state.activeTabIds, [group]: tabId },
            focusedGroup: group,
          },
        },
      }
    })
    setMountedTabKeys((current) => new Set(current).add(mountedTabKey(workspace, tabId)))
  }, [])

  const focusGroup = useCallback((workspace: string, group: AgentChatGroupId) => {
    setWorkbench((current) => {
      const state = current.projects[workspace] ?? emptyProjectTabState()
      if (current.activeProjectPath === workspace && state.focusedGroup === group) return current
      return {
        activeProjectPath: workspace,
        projects: { ...current.projects, [workspace]: { ...state, focusedGroup: group } },
      }
    })
  }, [])

  const renameTab = useCallback((workspace: string, tabId: string, title: string) => {
    setWorkbench((current) => {
      const state = current.projects[workspace]
      if (!state) return current
      return {
        ...current,
        projects: { ...current.projects, [workspace]: { ...state, tabs: setTabTitle(state.tabs, tabId, title) } },
      }
    })
  }, [])

  const togglePinTab = useCallback((workspace: string, tabId: string) => {
    setWorkbench((current) => {
      const state = current.projects[workspace]
      if (!state) return current
      const pinned = !state.tabs.find((tab) => tab.id === tabId)?.pinned
      return {
        ...current,
        projects: { ...current.projects, [workspace]: { ...state, tabs: setTabPinned(state.tabs, tabId, pinned) } },
      }
    })
  }, [])

  const relocateTab = useCallback((workspace: string, sourceId: string, group: AgentChatGroupId, beforeId: string | null) => {
    setWorkbench((current) => {
      const state = current.projects[workspace]
      const source = state?.tabs.find((tab) => tab.id === sourceId)
      if (!state || !source) return current
      const from = tabGroup(source)
      return {
        ...current,
        projects: {
          ...current.projects,
          [workspace]: {
            tabs: moveTab(state.tabs, sourceId, group, beforeId),
            focusedGroup: group,
            activeTabIds: from === group
              ? state.activeTabIds
              : {
                ...state.activeTabIds,
                [from]: state.activeTabIds[from] === sourceId
                  ? nextActiveTabId(tabsInGroup(state.tabs, from), sourceId, sourceId)
                  : state.activeTabIds[from],
                [group]: sourceId,
              },
          },
        },
      }
    })
  }, [])

  const openTerminal = useCallback((workspace: string, group: AgentChatGroupId, profileId: TerminalProfileId, command?: string) => {
    openTab({
      kind: 'terminal',
      id: createTabId('terminal'),
      workspace,
      group,
      profileId,
      command,
      title: profileId === 'custom' ? command || '' : profileId === 'shell' ? '' : profileId,
    })
  }, [openTab])

  const updateTerminalTitle = useCallback((workspace: string, tabId: string, title: string) => {
    setWorkbench((current) => {
      const state = current.projects[workspace]
      if (!state) return current
      const tabs = setTerminalTabTitle(state.tabs, tabId, title)
      if (tabs.every((tab, index) => tab === state.tabs[index])) return current
      return {
        ...current,
        projects: {
          ...current.projects,
          [workspace]: { ...state, tabs },
        },
      }
    })
  }, [])

  const openProjectPage = useCallback((workspace: string, group: AgentChatGroupId, pageId: AgentChatPageId) => {
    openTab({
      kind: 'page',
      id: createTabId('page'),
      workspace,
      group,
      pageId,
    })
  }, [openTab])

  const activateProjectWorkspace = useCallback(async (workspace: string): Promise<boolean> => {
    if (!(await flushPageDrafts())) return false
    return onActivateWorkspace ? onActivateWorkspace(workspace) : false
  }, [flushPageDrafts, onActivateWorkspace])

  const openDocumentReviewFeedback = useCallback((
    workspace: string,
    selection: ReviewFeedbackSelection,
    comment: ReviewFeedbackComment,
  ) => {
    const resource = comment.target
    if (selection.source !== 'document' || workspace !== documentReviewWorkspace || !resource?.id) return
    const pageId: AgentChatPageId = resource.kind === 'lore_item' ? 'lore' : 'reader'
    const state = workbenchRef.current.projects[workspace] ?? emptyProjectTabState()
    const existingPage = state.tabs.find((tab) => tab.kind === 'page' && tab.pageId === pageId)
    const conversationGroup = AGENT_CHAT_GROUP_IDS.find((group) => {
      const activeTab = state.tabs.find((tab) => tab.id === state.activeTabIds[group])
      return activeTab?.kind === 'agent'
    })
    const targetGroup = existingPage
      ? tabGroup(existingPage)
      : conversationGroup === 'primary' ? 'secondary' : 'primary'
    openProjectPage(workspace, targetGroup, pageId)
    navigationNonceRef.current += 1
    setDocumentReviewNavigation({
      workspace,
      target: resource.kind === 'lore_item'
        ? { kind: 'lore_item', id: resource.id, field: 'content' }
        : { kind: 'workspace_file', id: resource.id },
      commentID: comment.id,
      nonce: navigationNonceRef.current,
    })
  }, [documentReviewWorkspace, openProjectPage])

  const openChangeReview = useCallback((workspace: string, reviewThreadID: string, groupID: string) => {
    const state = workbench.projects[workspace] ?? emptyProjectTabState()
    const conversationGroup = AGENT_CHAT_GROUP_IDS.find((group) => {
      const tab = state.tabs.find((candidate) => candidate.id === state.activeTabIds[group])
      return tab?.kind === 'agent'
    })
    openTab({
      kind: 'review',
      id: createTabId('review'),
      workspace,
      group: conversationGroup === 'primary' ? 'secondary' : 'primary',
      threadID: reviewThreadID,
      groupID: groupID || undefined,
    })
  }, [openTab, workbench.projects])

  const handleRunningChange = useCallback((workspace: string, sessionId: string, running: boolean | null) => {
    const key = bindingKey(workspace, sessionId)
    const current = liveRunningBindingsRef.current
    const wasRunning = current.has(key)
    console.debug(
      `[features/agent-chat/AgentChatView.tsx] conversation running state changed session=${sessionId} running=${String(running)} was_running=${wasRunning}`,
    )
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
  }, [refreshProjects])

  const isSessionRunning = useCallback((project: AgentChatProject, session: AgentChatSession) => (
    session.running || liveRunningBindings.has(bindingKey(project.path, session.id))
  ), [liveRunningBindings])

  const sessionTitles = useMemo(() => {
    const titles = new Map<string, string>()
    for (const project of projects) {
      for (const session of project.sessions) titles.set(bindingKey(project.path, session.id), session.title)
    }
    return titles
  }, [projects])

  const tabTitle = useCallback((tab: AgentChatTab) => {
    if (tab.customTitle) return tab.customTitle
    switch (tab.kind) {
      case 'agent':
        return sessionTitles.get(bindingKey(tab.workspace, tab.sessionId)) || tab.pendingTitle || t('chat.untitledSession')
      case 'terminal':
        return terminalTabLabel(tab, t)
      case 'page':
        return t(`agentChat.page.${tab.pageId}`)
      case 'review':
        return t('agentChat.review.tab')
    }
  }, [sessionTitles, t])

  const treeProps = {
    projects,
    loading: projectsLoading,
    error: projectsError,
    activeProjectPath,
    isSessionRunning,
    onSelectProject: (project: AgentChatProject) => selectProject(project.path),
    onOpenSession: openSessionTab,
    onCreateSession: (project: AgentChatProject) => openDraftSessionInProject(project),
    onRenameSession: renameSession,
    onDeleteSession: deleteSession,
  }
  const sidebar = <AgentChatSessionSidebar {...treeProps} onCollapse={() => setSidebarVisible(false)} />

  const renderProjectGroup = (
    project: AgentChatProject,
    state: AgentChatProjectTabState,
    group: AgentChatGroupId,
    projectVisible: boolean,
    mobileControls: { isMobile: boolean; openLeft: () => void },
  ) => {
    const groupTabs = tabsInGroup(state.tabs, group)
    const activeId = state.activeTabIds[group]
    const projectRunning = project.sessions.some((session) => isSessionRunning(project, session))
    return (
      <div
        className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-bg)]"
        onPointerDownCapture={() => {
          if (state.focusedGroup !== group) focusGroup(project.path, group)
        }}
        onFocusCapture={() => {
          if (state.focusedGroup !== group) focusGroup(project.path, group)
        }}
      >
        <div className="flex items-center gap-1 bg-[var(--nova-surface)] pl-1.5 md:pl-0">
          {group === 'primary' && mobileControls.isMobile && (
            <MobilePaneTrigger
              side="left"
              className="size-7 shrink-0"
              label={t('agentChat.sidebar.projects')}
              onClick={() => { setSidebarVisible(true); mobileControls.openLeft() }}
            />
          )}
          <div className="min-w-0 flex-1">
            <AgentChatTabBar
              group={group}
              tabs={groupTabs}
              activeTabId={activeId}
              tabTitle={tabTitle}
              onActivate={(tabId) => activateTab(project.path, group, tabId)}
              onClose={(tabId) => { void closeTabs(project.path, [tabId]) }}
              onCloseOthers={(tabId) => { void closeTabs(project.path, otherTabIds(state.tabs, tabId)) }}
              onCloseToRight={(tabId) => { void closeTabs(project.path, tabIdsAfter(state.tabs, tabId)) }}
              onRename={(tabId, title) => renameTab(project.path, tabId, title)}
              onTogglePin={(tabId) => togglePinTab(project.path, tabId)}
              onMoveTab={(sourceId, target, beforeId) => relocateTab(project.path, sourceId, target, beforeId)}
              onNewAgentTab={(target) => openDraftSessionInProject(project, target)}
              onNewTerminalTab={(target, profileId, command) => openTerminal(project.path, target, profileId, command)}
              onOpenPage={(target, pageId) => openProjectPage(project.path, target, pageId)}
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
              action={{ label: t('agentChat.tabs.newChat'), onClick: () => openDraftSessionInProject(project, group) }}
            />
          ) : groupTabs.map((tab) => {
            const active = projectVisible && tab.id === activeId
            const mounted = mountedTabKeys.has(mountedTabKey(project.path, tab.id))
            if (!active && !mounted) return null
            return (
              <section
                key={tab.id}
                hidden={!active}
                aria-hidden={!active}
                className="absolute inset-0 flex min-h-0 flex-col"
              >
                <AgentChatTabContent
                  tab={tab}
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
                  onOpenPage={openProjectPage}
                  onActivateWorkspace={activateProjectWorkspace}
                  onPageFlushHandlerChange={registerPageFlushHandler}
                  onOpenChangeReview={openChangeReview}
                  onClose={(tabId) => { void closeTabs(project.path, [tabId]) }}
                  onWorkspaceChanged={onWorkspaceChanged}
                  onRunningChange={handleRunningChange}
                  onDraftCommitted={(message) => commitDraftSession(tab.workspace, tab.id, message)}
                  onTerminalSessionEstablished={(tabId, session) => bindTerminalSession(project.path, tabId, session)}
                  onTerminalTitleChange={(tabId, title) => updateTerminalTitle(project.path, tabId, title)}
                />
              </section>
            )
          })}
        </div>
      </div>
    )
  }

  return (
    <AdaptiveSurface
      className="h-full min-h-0"
      collapseAt={720}
      desktopGridClassName={sidebarVisible
        ? 'grid-cols-[clamp(200px,18vw,280px)_minmax(0,1fr)]'
        : 'grid-cols-[minmax(0,1fr)]'}
      left={{
        id: 'agent-chat-sessions',
        side: 'left',
        title: t('agentChat.sidebar.projects'),
        content: sidebar,
        desktopClassName: 'h-full min-h-0 min-w-0',
        enabled: sidebarVisible,
      }}
    >
      {(controls) => {
        const projectLayers = projects.map((project) => {
          const state = workbench.projects[project.path] ?? emptyProjectTabState()
          const visible = project.path === activeProjectPath
          const primary = renderProjectGroup(project, state, 'primary', visible, controls)
          const content = tabsInGroup(state.tabs, 'secondary').length === 0 ? primary : (
            <Group id={`nova-agentchat-split-${project.path}`} orientation="horizontal" className="flex h-full min-h-0">
              <Panel id="agentchat-primary" defaultSize="50%" minSize="280px" className="min-w-0">
                {primary}
              </Panel>
              <Separator
                aria-label={t('agentChat.tabs.resizeSplit')}
                className="nova-resize-handle nova-resize-divider nova-resize-divider-vertical relative z-30 -mx-1 w-2 shrink-0 touch-none cursor-col-resize select-none"
              />
              <Panel id="agentchat-secondary" defaultSize="50%" minSize="280px" className="min-w-0">
                {renderProjectGroup(project, state, 'secondary', visible, controls)}
              </Panel>
            </Group>
          )
          return (
            <section
              key={project.path}
              hidden={!visible}
              aria-hidden={!visible}
              className="absolute inset-0 flex min-h-0 flex-col"
            >
              {content}
            </section>
          )
        })

        const workbenchContent = activeProject ? (
          <div className="relative h-full min-h-0">{projectLayers}</div>
        ) : (
          <EmptyState variant="page" icon={MessageSquareText} title={t('agentChat.empty.noWorkspace')} />
        )
        if (controls.isMobile || sidebarVisible) return workbenchContent
        return (
          <div className="flex h-full min-h-0">
            <AgentChatSidebarRail
              {...treeProps}
              onExpand={() => setSidebarVisible(true)}
              onCreateDefaultSession={() => { if (activeProject) openDraftSessionInProject(activeProject) }}
              createDisabled={!activeProject}
            />
            <div className="min-w-0 flex-1">{workbenchContent}</div>
          </div>
        )
      }}
    </AdaptiveSurface>
  )
}
