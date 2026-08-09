import { useCallback, useEffect, useMemo } from 'react'
import { getAgentChatActivity, type AgentChatHistoryItem, type AgentChatProject, type AgentChatSession } from './api'
import { agentChatSessionBindingKey, projectSidebarActivities, type AgentChatSidebarActivity } from './sidebar-activity'
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
  const isSessionRunning = useCallback(
    (project: AgentChatProject, session: AgentChatSession) => session.running || liveRunningBindings.has(agentChatSessionBindingKey(project.id, session.id)),
    [liveRunningBindings],
  )

  const openConversationBindings = useMemo(() => {
    const bindings = new Set<string>()
    for (const [projectID, state] of Object.entries(workbench.projects)) {
      for (const tab of state.tabs) {
        if (tab.kind === 'agent') bindings.add(agentChatSessionBindingKey(projectID, tab.sessionId))
      }
    }
    return bindings
  }, [workbench.projects])

  const detachedRunningBindings = useMemo(() => {
    const bindings = new Set<string>()
    for (const project of projects) {
      for (const session of project.sessions) {
        const key = agentChatSessionBindingKey(project.id, session.id)
        if (isSessionRunning(project, session) && !openConversationBindings.has(key)) bindings.add(key)
      }
    }
    for (const key of liveRunningBindings) {
      if (!openConversationBindings.has(key)) bindings.add(key)
    }
    return bindings
  }, [isSessionRunning, liveRunningBindings, openConversationBindings, projects])

  useEffect(() => {
    if (detachedRunningBindings.size === 0) return
    let stopped = false
    let pending = false
    const refreshActivity = async () => {
      if (pending) return
      pending = true
      try {
        const bindings = await getAgentChatActivity()
        if (stopped) return
        const current = new Set(
          bindings
            .map((binding) => agentChatSessionBindingKey(binding.project_id, binding.session_id))
            .filter((key) => !openConversationBindings.has(key)),
        )
        if (current.size !== detachedRunningBindings.size || [...current].some((key) => !detachedRunningBindings.has(key))) {
          await refreshProjects()
        }
      } catch (error) {
        console.warn('[features/agent-chat/use-agent-chat-activity-navigator.ts] refreshing detached activity failed', { error })
      } finally {
        pending = false
      }
    }
    const interval = window.setInterval(() => {
      void refreshActivity()
    }, DETACHED_ACTIVITY_REFRESH_INTERVAL_MS)
    return () => {
      stopped = true
      window.clearInterval(interval)
    }
  }, [detachedRunningBindings, openConversationBindings, refreshProjects])

  const activitiesByProject = useMemo(() => {
    const activities = new Map<string, readonly AgentChatSidebarActivity[]>()
    for (const project of projects) {
      const state = workbench.projects[project.id] ?? emptyProjectTabState()
      const runningSessionIds = new Set(project.sessions.filter((session) => isSessionRunning(project, session)).map((session) => session.id))
      for (const tab of state.tabs) {
        if (tab.kind === 'agent' && liveRunningBindings.has(agentChatSessionBindingKey(project.id, tab.sessionId))) {
          runningSessionIds.add(tab.sessionId)
        }
      }
      activities.set(
        project.id,
        projectSidebarActivities({
          project,
          state,
          activeProjectId: workbench.activeProjectId,
          runningSessionIds,
          terminalStatuses,
          tabTitle,
        }),
      )
    }
    return activities
  }, [isSessionRunning, liveRunningBindings, projects, tabTitle, terminalStatuses, workbench])

  const openSidebarActivity = useCallback(
    (project: AgentChatProject, activity: AgentChatSidebarActivity) => {
      if (activity.tabId && activity.group) {
        activateTab(project.id, activity.group, activity.tabId)
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
    },
    [activateTab, openSessionTab, refreshProjects],
  )

  const openOrActivateSession = useCallback(
    (project: AgentChatProject, session: AgentChatSession) => {
      const existing = workbench.projects[project.id]?.tabs.find((tab) => tab.kind === 'agent' && tab.sessionId === session.id)
      if (existing) {
        activateTab(project.id, tabGroup(existing), existing.id)
        return
      }
      openSessionTab(project, session)
    },
    [activateTab, openSessionTab, workbench.projects],
  )

  const openHistorySession = useCallback(
    (item: AgentChatHistoryItem) => {
      const project = projects.find((candidate) => candidate.id === item.project_id)
      if (project) {
        openOrActivateSession(project, item.session)
        return
      }
      console.warn('[features/agent-chat/use-agent-chat-activity-navigator.ts] history project no longer registered', {
        projectId: item.project_id,
        sessionId: item.session.id,
      })
      void refreshProjects()
    },
    [openOrActivateSession, projects, refreshProjects],
  )

  return {
    activitiesByProject,
    isSessionRunning,
    openHistorySession,
    openOrActivateSession,
    openSidebarActivity,
  }
}
