import { useCallback, useEffect, useMemo } from 'react'
import type { AgentChatHistoryItem, AgentChatProject, AgentChatSession } from './api'
import {
  agentChatSessionBindingKey,
  projectSidebarActivities,
  type AgentChatSidebarActivity,
} from './sidebar-activity'
import { emptyProjectTabState, tabGroup, type AgentChatWorkbenchState } from './tab-state'
import type { AgentChatTerminalStatus } from './terminal/TerminalTabView'
import type { AgentChatGroupId, AgentChatTab } from './types'

/** Detached tasks have no mounted stream left to report completion, so refresh their cheap snapshot. */
const DETACHED_ACTIVITY_REFRESH_INTERVAL_MS = 2_000

interface AgentChatActivityNavigatorOptions {
  projects: AgentChatProject[]
  workbench: AgentChatWorkbenchState
  liveRunningBindings: ReadonlySet<string>
  terminalStatuses: ReadonlyMap<string, AgentChatTerminalStatus>
  tabTitle: (tab: AgentChatTab) => string
  refreshProjects: () => Promise<AgentChatProject[] | null>
  activateTab: (workspace: string, group: AgentChatGroupId, tabId: string) => void
  openSessionTab: (project: AgentChatProject, session: AgentChatSession) => void
}

/**
 * Owns the dynamic navigation projection layered over the durable project workbench.
 *
 * The workbench remains the source of tab order and pane selection. Server snapshots contribute
 * detached running conversations, while mounted terminal/conversation runtimes contribute live
 * status. This hook deliberately does not own either source model.
 */
export function useAgentChatActivityNavigator({
  projects,
  workbench,
  liveRunningBindings,
  terminalStatuses,
  tabTitle,
  refreshProjects,
  activateTab,
  openSessionTab,
}: AgentChatActivityNavigatorOptions) {
  const isSessionRunning = useCallback((project: AgentChatProject, session: AgentChatSession) => (
    session.running || liveRunningBindings.has(agentChatSessionBindingKey(project.path, session.id))
  ), [liveRunningBindings])

  const openConversationBindings = useMemo(() => {
    const bindings = new Set<string>()
    for (const [workspace, state] of Object.entries(workbench.projects)) {
      for (const tab of state.tabs) {
        if (tab.kind === 'agent') bindings.add(agentChatSessionBindingKey(workspace, tab.sessionId))
      }
    }
    return bindings
  }, [workbench.projects])

  const hasDetachedRunningActivity = useMemo(() => {
    for (const project of projects) {
      for (const session of project.sessions) {
        const key = agentChatSessionBindingKey(project.path, session.id)
        if (isSessionRunning(project, session) && !openConversationBindings.has(key)) return true
      }
    }
    return [...liveRunningBindings].some((key) => !openConversationBindings.has(key))
  }, [isSessionRunning, liveRunningBindings, openConversationBindings, projects])

  useEffect(() => {
    if (!hasDetachedRunningActivity) return
    const interval = window.setInterval(() => { void refreshProjects() }, DETACHED_ACTIVITY_REFRESH_INTERVAL_MS)
    return () => window.clearInterval(interval)
  }, [hasDetachedRunningActivity, refreshProjects])

  const activitiesByProject = useMemo(() => {
    const activities = new Map<string, readonly AgentChatSidebarActivity[]>()
    for (const project of projects) {
      const state = workbench.projects[project.path] ?? emptyProjectTabState()
      const runningSessionIds = new Set(
        project.sessions.filter((session) => isSessionRunning(project, session)).map((session) => session.id),
      )
      for (const tab of state.tabs) {
        if (tab.kind === 'agent' && liveRunningBindings.has(agentChatSessionBindingKey(project.path, tab.sessionId))) {
          runningSessionIds.add(tab.sessionId)
        }
      }
      activities.set(project.path, projectSidebarActivities({
        project,
        state,
        activeProjectPath: workbench.activeProjectPath,
        runningSessionIds,
        terminalStatuses,
        tabTitle,
      }))
    }
    return activities
  }, [isSessionRunning, liveRunningBindings, projects, tabTitle, terminalStatuses, workbench])

  const openSidebarActivity = useCallback((project: AgentChatProject, activity: AgentChatSidebarActivity) => {
    if (activity.tabId && activity.group) {
      activateTab(project.path, activity.group, activity.tabId)
      return
    }
    if (activity.kind !== 'agent' || !activity.sessionId) return
    const session = project.sessions.find((candidate) => candidate.id === activity.sessionId)
    if (session) {
      openSessionTab(project, session)
      return
    }
    console.warn('[features/agent-chat/use-agent-chat-activity-navigator.ts] detached activity session missing from project snapshot', {
      workspace: project.path,
      sessionId: activity.sessionId,
    })
    void refreshProjects()
  }, [activateTab, openSessionTab, refreshProjects])

  const openOrActivateSession = useCallback((project: AgentChatProject, session: AgentChatSession) => {
    const existing = workbench.projects[project.path]?.tabs.find((tab) => (
      tab.kind === 'agent' && tab.sessionId === session.id
    ))
    if (existing) {
      activateTab(project.path, tabGroup(existing), existing.id)
      return
    }
    openSessionTab(project, session)
  }, [activateTab, openSessionTab, workbench.projects])

  const openHistorySession = useCallback((item: AgentChatHistoryItem) => {
    const project = projects.find((candidate) => candidate.path === item.workspace)
    if (project) {
      openOrActivateSession(project, item.session)
      return
    }
    console.warn('[features/agent-chat/use-agent-chat-activity-navigator.ts] history project no longer registered', {
      workspace: item.workspace,
      sessionId: item.session.id,
    })
    void refreshProjects()
  }, [openOrActivateSession, projects, refreshProjects])

  return { activitiesByProject, isSessionRunning, openHistorySession, openSidebarActivity }
}
