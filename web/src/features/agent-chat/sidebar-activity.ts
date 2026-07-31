import type { AgentChatProject } from './api'
import { tabGroup, type AgentChatProjectTabState } from './tab-state'
import type { AgentChatGroupId, AgentChatTab } from './types'

/** Stable identity shared by server snapshots and mounted conversation runtimes. */
export function agentChatSessionBindingKey(projectID: string, sessionId: string): string {
  return `${projectID}\u0000${sessionId}`
}

/** Runtime states exposed by the compact cross-project activity navigator. */
export type AgentChatActivityStatus = 'idle' | 'running' | 'connecting' | 'ready' | 'exited' | 'error'

/**
 * A sidebar activity is a navigation projection, not another tab model.
 *
 * Open conversation/terminal tabs retain their tab identity and pane. A conversation whose tab
 * was closed while its task is still running has only a session identity; opening it creates a
 * new tab for that same durable session. Project pages and review tabs are intentionally absent.
 */
export interface AgentChatSidebarActivity {
  id: string
  projectId: string
  workspace: string
  kind: 'agent' | 'terminal'
  title: string
  status: AgentChatActivityStatus
  tabId?: string
  sessionId?: string
  group?: AgentChatGroupId
  /** Visible in either pane of the foreground project. */
  paneVisible: boolean
  /** Last-focused visible pane; this is the sidebar's strong selection. */
  focused: boolean
}

interface ProjectSidebarActivitiesOptions {
  project: AgentChatProject
  state: AgentChatProjectTabState
  activeProjectId: string
  runningSessionIds: ReadonlySet<string>
  terminalStatuses: ReadonlyMap<string, Exclude<AgentChatActivityStatus, 'idle' | 'running'>>
  tabTitle: (tab: AgentChatTab) => string
}

/**
 * Derive the stable sidebar order by filtering the existing tab order, then appending detached
 * running conversations in the server's stable session order. Status changes never re-sort rows.
 */
export function projectSidebarActivities({
  project,
  state,
  activeProjectId,
  runningSessionIds,
  terminalStatuses,
  tabTitle,
}: ProjectSidebarActivitiesOptions): AgentChatSidebarActivity[] {
  const foreground = project.id === activeProjectId
  const openSessionIds = new Set<string>()
  const activities: AgentChatSidebarActivity[] = []

  for (const tab of state.tabs) {
    if (tab.kind !== 'agent' && tab.kind !== 'terminal') continue
    const group = tabGroup(tab)
    const groupVisible = group === 'primary' || state.secondaryVisible
    const paneVisible = foreground && groupVisible && state.activeTabIds[group] === tab.id
    const common = {
      projectId: project.id,
      workspace: project.path,
      title: tabTitle(tab),
      tabId: tab.id,
      group,
      paneVisible,
      focused: paneVisible && state.focusedGroup === group,
    }
    if (tab.kind === 'agent') {
      openSessionIds.add(tab.sessionId)
      activities.push({
        ...common,
        id: `agent:${tab.sessionId}`,
        kind: 'agent',
        sessionId: tab.sessionId,
        status: runningSessionIds.has(tab.sessionId) ? 'running' : 'idle',
      })
      continue
    }
    activities.push({
      ...common,
      id: `terminal:${tab.id}`,
      kind: 'terminal',
      status: terminalStatuses.get(tab.id) ?? (tab.terminalSessionId ? 'ready' : 'connecting'),
    })
  }

  for (const session of project.sessions) {
    if (!runningSessionIds.has(session.id) || openSessionIds.has(session.id)) continue
    activities.push({
      id: `agent:${session.id}`,
      projectId: project.id,
      workspace: project.path,
      kind: 'agent',
      title: session.title,
      status: 'running',
      sessionId: session.id,
      paneVisible: false,
      focused: false,
    })
  }
  return activities
}

export interface AgentChatActivitySummary {
  total: number
  running: number
  attention: number
}

/** Compact project-row summary; exited terminals stay visible but are not treated as failures. */
export function summarizeSidebarActivities(activities: readonly AgentChatSidebarActivity[]): AgentChatActivitySummary {
  return activities.reduce<AgentChatActivitySummary>(
    (summary, activity) => ({
      total: summary.total + 1,
      running: summary.running + Number(activity.status === 'running' || activity.status === 'connecting'),
      attention: summary.attention + Number(activity.status === 'error'),
    }),
    { total: 0, running: 0, attention: 0 },
  )
}
