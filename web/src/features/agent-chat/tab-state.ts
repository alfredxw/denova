import { AGENT_CHAT_GROUP_IDS, AGENT_CHAT_PAGE_IDS, agentChatPageIdsForProjectType, type AgentChatGroupId, type AgentChatPageId, type AgentChatTab } from './types'
import type { AgentChatProject } from './api'

/**
 * Local persistence for AgentChat tabs.
 *
 * The tab set is pure UI state. Every project owns an independent pair of tab groups; switching
 * projects selects another workbench state instead of filtering or reusing a global tab strip.
 * Nothing here lands in workspace files or enters the model context.
 */

const WORKBENCH_STORAGE_KEY = 'nova.agentchat.workbenches.v1'
const TAB_BAR_EXPANDED_STORAGE_KEY = 'nova.agentchat.tabBarExpanded.v1'

/** Upper bound on open tabs so a long session cannot pile up unbounded panes. */
export const MAX_AGENT_CHAT_TABS = 12

function readStorage(key: string): string | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage.getItem(key)
  } catch (error) {
    console.warn('[features/agent-chat/tab-state.ts] reading local tab state failed', { key, error })
    return null
  }
}

function writeStorage(key: string, value: string | null) {
  if (typeof window === 'undefined') return
  try {
    if (value === null) window.localStorage.removeItem(key)
    else window.localStorage.setItem(key, value)
  } catch (error) {
    console.warn('[features/agent-chat/tab-state.ts] writing local tab state failed', { key, error })
  }
}

function isPageId(value: unknown): value is AgentChatPageId {
  return typeof value === 'string' && (AGENT_CHAT_PAGE_IDS as readonly string[]).includes(value)
}

function terminalProfileId(value: unknown): string {
  if (typeof value !== 'string') return ''
  const normalized = value.trim()
  return normalized.length <= 128 && /^[a-z0-9][a-z0-9._-]*$/.test(normalized) ? normalized : ''
}

function isGroupId(value: unknown): value is AgentChatGroupId {
  return typeof value === 'string' && (AGENT_CHAT_GROUP_IDS as readonly string[]).includes(value)
}

/** The fields every persisted tab shares, read back defensively like the rest of the record. */
function parseCommon(value: Record<string, unknown>) {
  const projectId = typeof value.projectId === 'string' ? value.projectId.trim() : ''
  const workspace = typeof value.workspace === 'string' ? value.workspace.trim() : ''
  if (!projectId || !workspace || !isGroupId(value.group)) return null
  return {
    projectId,
    workspace,
    customTitle: typeof value.customTitle === 'string' && value.customTitle ? value.customTitle : undefined,
    pinned: value.pinned === true ? true : undefined,
    group: value.group,
  }
}

/** Validate one persisted record; malformed entries are dropped instead of breaking the page. */
function parseTab(raw: unknown): AgentChatTab | null {
  if (!raw || typeof raw !== 'object') return null
  const value = raw as Record<string, unknown>
  const id = typeof value.id === 'string' ? value.id : ''
  const parsedCommon = parseCommon(value)
  if (!id || !parsedCommon) return null
  const common = { id, ...parsedCommon }
  switch (value.kind) {
    case 'agent':
      return typeof value.sessionId === 'string' && value.sessionId ? { kind: 'agent', ...common, sessionId: value.sessionId } : null
    case 'terminal': {
      const profileId = terminalProfileId(value.profileId)
      if (!profileId) return null
      return {
        kind: 'terminal',
        ...common,
        profileId,
        profileName: typeof value.profileName === 'string' ? value.profileName.trim() || undefined : undefined,
        title: typeof value.title === 'string' ? value.title : '',
        terminalSessionId: typeof value.terminalSessionId === 'string' ? value.terminalSessionId : undefined,
      }
    }
    case 'page':
      return isPageId(value.pageId) ? { kind: 'page', ...common, pageId: value.pageId } : null
    case 'files': {
      const selectedPath = typeof value.selectedPath === 'string' ? value.selectedPath.trim() : ''
      return { kind: 'files', ...common, selectedPath: selectedPath || undefined }
    }
    case 'review':
      return typeof value.threadID === 'string' && value.threadID
        ? {
            kind: 'review',
            ...common,
            threadID: value.threadID,
            groupID: typeof value.groupID === 'string' ? value.groupID : undefined,
          }
        : null
    default:
      return null
  }
}

/** The tab each group has in front. Both groups are tracked so a split survives a reload. */
export type ActiveTabIds = Record<AgentChatGroupId, string | null>

export const NO_ACTIVE_TABS: ActiveTabIds = { primary: null, secondary: null }

/** One project's complete two-pane tab state. */
export interface AgentChatProjectTabState {
  tabs: AgentChatTab[]
  activeTabIds: ActiveTabIds
  focusedGroup: AgentChatGroupId
  /** Visibility is independent from ownership: hiding the pane never closes its tabs or runtimes. */
  secondaryVisible: boolean
}

/** User-level AgentChat state. Project records are deliberately independent. */
export interface AgentChatWorkbenchState {
  activeProjectId: string
  projects: Record<string, AgentChatProjectTabState>
}

export function emptyProjectTabState(): AgentChatProjectTabState {
  return {
    tabs: [],
    activeTabIds: { ...NO_ACTIVE_TABS },
    focusedGroup: 'primary',
    secondaryVisible: false,
  }
}

/** Read the current project-owned workbench state. */
export function readStoredWorkbenchState(): AgentChatWorkbenchState {
  const raw = readStorage(WORKBENCH_STORAGE_KEY)
  if (!raw) return { activeProjectId: '', projects: {} }
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') throw new Error('not a record')
    const value = parsed as Record<string, unknown>
    const storedProjects = value.projects && typeof value.projects === 'object' ? (value.projects as Record<string, unknown>) : {}
    const projects: Record<string, AgentChatProjectTabState> = {}
    for (const [rawProjectId, rawState] of Object.entries(storedProjects)) {
      const projectId = rawProjectId.trim()
      if (!projectId || !rawState || typeof rawState !== 'object') continue
      const state = rawState as Record<string, unknown>
      const rawTabs = Array.isArray(state.tabs) ? state.tabs : []
      const tabs = orderTabs(
        dedupeTabs(
          rawTabs.map(parseTab).filter((tab): tab is AgentChatTab => tab !== null && tab.projectId === projectId),
        ).slice(0, MAX_AGENT_CHAT_TABS),
      )
      const active = state.activeTabIds && typeof state.activeTabIds === 'object' ? (state.activeTabIds as Record<string, unknown>) : {}
      const validActiveID = (group: AgentChatGroupId) => {
        const id = typeof active[group] === 'string' ? active[group] : ''
        return tabs.some((tab) => tab.id === id && tabGroup(tab) === group) ? id : null
      }
      projects[projectId] = {
        tabs,
        activeTabIds: {
          primary: validActiveID('primary'),
          secondary: validActiveID('secondary'),
        },
        focusedGroup: isGroupId(state.focusedGroup) ? state.focusedGroup : 'primary',
        secondaryVisible: state.secondaryVisible === true && tabsInGroup(tabs, 'secondary').length > 0,
      }
    }
    const activeProjectId = typeof value.activeProjectId === 'string' && projects[value.activeProjectId] ? value.activeProjectId : ''
    return { activeProjectId, projects }
  } catch (error) {
    console.warn('[features/agent-chat/tab-state.ts] parsing local workbench state failed', { error })
    return { activeProjectId: '', projects: {} }
  }
}

/** Refresh project paths and discard tabs that no longer belong to a live project. */
export function reconcileWorkbenchProjects(state: AgentChatWorkbenchState, projects: readonly AgentChatProject[]): AgentChatWorkbenchState {
  const reconciled: Record<string, AgentChatProjectTabState> = {}
  for (const project of projects) {
    const source = state.projects[project.id]
    if (!source) continue
    const visibleSessionIDs = new Set(project.sessions.map((session) => session.id))
    const sessionListComplete = project.total <= project.sessions.length
    const allowedPages = agentChatPageIdsForProjectType(project.type)
    const projectTabs = source.tabs.filter((tab) => {
      if (tab.kind === 'page') return allowedPages.includes(tab.pageId)
      return project.type === 'book' || tab.kind === 'agent' || tab.kind === 'terminal' || tab.kind === 'files'
    })
    // Durable sessions are authoritative only when the project response is complete. If the
    // backend truncated a large history, keep unknown tabs rather than discarding valid data.
    const eligibleTabs = projectTabs.filter((tab) =>
      tab.kind !== 'agent' || tab.draft || !sessionListComplete || visibleSessionIDs.has(tab.sessionId),
    )
    const tabs = eligibleTabs.map((tab) => ({
      ...tab,
      projectId: project.id,
      workspace: project.path,
    }))
    reconciled[project.id] = {
      ...source,
      tabs,
      activeTabIds: {
        primary: tabs.some((tab) => tab.id === source.activeTabIds.primary) ? source.activeTabIds.primary : null,
        secondary: tabs.some((tab) => tab.id === source.activeTabIds.secondary) ? source.activeTabIds.secondary : null,
      },
      secondaryVisible: source.secondaryVisible && tabsInGroup(tabs, 'secondary').length > 0,
    }
  }
  const activeProject = projects.find((project) => project.id === state.activeProjectId)
  return {
    activeProjectId: activeProject?.id ?? '',
    projects: reconciled,
  }
}

export function persistWorkbenchState(state: AgentChatWorkbenchState) {
  const projects = Object.fromEntries(
    Object.entries(state.projects).map(([projectId, project]) => {
      // Blank drafts are deliberately ephemeral: a reload or app restart must not turn them into
      // durable, indistinguishable conversations.
      const tabs = project.tabs.filter((tab) => tab.kind !== 'agent' || !tab.draft)
      const activeTabIds = Object.fromEntries(
        AGENT_CHAT_GROUP_IDS.map((group) => [group, tabs.some((tab) => tab.id === project.activeTabIds[group]) ? project.activeTabIds[group] : null]),
      ) as ActiveTabIds
      return [projectId, {
        ...project,
        tabs,
        activeTabIds,
        secondaryVisible: project.secondaryVisible && tabsInGroup(tabs, 'secondary').length > 0,
      }]
    }),
  )
  writeStorage(WORKBENCH_STORAGE_KEY, JSON.stringify({ ...state, projects }))
}

/** Derives the temporary title shown before the first message is persisted. */
export function draftSessionTitle(message: string) {
  const normalized = message.replace(/^\/plan\s*/, '').trim()
  const characters = Array.from(normalized)
  return characters.length > 60 ? `${characters.slice(0, 60).join('')}...` : normalized
}

/** Advances refresh generations without coupling mounted surfaces to each other. */
export function incrementProjectRefreshSignals(
  current: ReadonlyMap<string, number>,
  projectIDs: readonly string[],
): ReadonlyMap<string, number> {
  const next = new Map(current)
  projectIDs.forEach((projectID) => next.set(projectID, (next.get(projectID) ?? 0) + 1))
  return next
}

const SIDEBAR_VISIBLE_STORAGE_KEY = 'nova.agentchat.sidebarVisible.v1'

/** The conversation tree is shown until the user collapses it. */
export function readSidebarVisible(): boolean {
  return readStorage(SIDEBAR_VISIBLE_STORAGE_KEY) !== 'false'
}

export function persistSidebarVisible(visible: boolean) {
  writeStorage(SIDEBAR_VISIBLE_STORAGE_KEY, visible ? 'true' : 'false')
}

export function readTabBarExpanded(): boolean {
  // Collapsed by default: with a single tab the bar is pure noise, so the user opts in
  // once they actually work with several tabs.
  return readStorage(TAB_BAR_EXPANDED_STORAGE_KEY) === 'true'
}

export function persistTabBarExpanded(expanded: boolean) {
  writeStorage(TAB_BAR_EXPANDED_STORAGE_KEY, expanded ? 'true' : 'false')
}

/**
 * Deduplication rules: each project page and Files workspace keeps a single instance,
 * conversation tabs are unique per session, and terminal tabs are always distinct.
 */
export function dedupeTabs(tabs: AgentChatTab[]): AgentChatTab[] {
  const seenPages = new Set<AgentChatPageId>()
  const seenSessions = new Set<string>()
  const seenDraftGroups = new Set<AgentChatGroupId>()
  const seenThreads = new Set<string>()
  let seenFiles = false
  const seenIds = new Set<string>()
  const out: AgentChatTab[] = []
  for (const tab of tabs) {
    if (seenIds.has(tab.id)) continue
    if (tab.kind === 'page') {
      if (seenPages.has(tab.pageId)) continue
      seenPages.add(tab.pageId)
    }
    if (tab.kind === 'agent') {
      if (tab.draft) {
        const group = tabGroup(tab)
        if (seenDraftGroups.has(group)) continue
        seenDraftGroups.add(group)
      }
      if (seenSessions.has(tab.sessionId)) continue
      seenSessions.add(tab.sessionId)
    }
    if (tab.kind === 'review') {
      if (seenThreads.has(tab.threadID)) continue
      seenThreads.add(tab.threadID)
    }
    if (tab.kind === 'files') {
      if (seenFiles) continue
      seenFiles = true
    }
    seenIds.add(tab.id)
    out.push(tab)
  }
  return out
}

/** Group hosting a tab. */
export function tabGroup(tab: AgentChatTab): AgentChatGroupId {
  return tab.group
}

/** The tabs one strip renders, in the order it renders them. */
export function tabsInGroup(tabs: AgentChatTab[], group: AgentChatGroupId): AgentChatTab[] {
  return tabs.filter((tab) => tabGroup(tab) === group)
}

/**
 * Canonical order: the primary group, then the secondary one, pinned tabs at the front of each.
 * Array order is exactly what the strips render, so every mutation normalises through here
 * rather than each call site remembering to keep pinned tabs in front.
 */
export function orderTabs(tabs: AgentChatTab[]): AgentChatTab[] {
  const rank = (tab: AgentChatTab) => AGENT_CHAT_GROUP_IDS.indexOf(tabGroup(tab)) * 2 + (tab.pinned ? 0 : 1)
  return tabs
    .map((tab, index) => ({ tab, index }))
    .sort((left, right) => rank(left.tab) - rank(right.tab) || left.index - right.index)
    .map((entry) => entry.tab)
}

/** Drop the oldest unpinned tab until the set fits, never the one just opened. */
function trimTabs(tabs: AgentChatTab[]): AgentChatTab[] {
  const out = [...tabs]
  while (out.length > MAX_AGENT_CHAT_TABS) {
    const index = out.findIndex((tab, position) => !tab.pinned && position < out.length - 1)
    if (index === -1) break
    out.splice(index, 1)
  }
  return out
}

/** Append a tab within the limit; an equivalent tab is reused. Returns the tab to activate. */
export function appendTab(tabs: AgentChatTab[], next: AgentChatTab): { tabs: AgentChatTab[]; activeId: string } {
  const existing = tabs.find((tab) => {
    if (tab.kind !== next.kind) return false
    if (tab.kind === 'page' && next.kind === 'page') return tab.pageId === next.pageId
    if (tab.kind === 'agent' && next.kind === 'agent') {
      if (tab.draft && next.draft) return tabGroup(tab) === tabGroup(next)
      return tab.sessionId === next.sessionId
    }
    if (tab.kind === 'review' && next.kind === 'review') return tab.threadID === next.threadID
    if (tab.kind === 'files' && next.kind === 'files') return true
    return false
  })
  if (existing) {
    const updated = existing.kind === 'files' && next.kind === 'files' && next.selectedPath
      ? tabs.map((tab) => tab.id === existing.id ? { ...tab, selectedPath: next.selectedPath } : tab)
      : tabs
    return { tabs: updated, activeId: existing.id }
  }
  return {
    tabs: orderTabs(trimTabs(dedupeTabs([...tabs, next]))),
    activeId: next.id,
  }
}

/**
 * Move a tab, which covers both reordering inside a strip and dragging across the split.
 * `beforeId` is the tab it was dropped onto; dropping on empty strip space passes null and
 * appends to the end of the group.
 */
export function moveTab(tabs: AgentChatTab[], sourceId: string, group: AgentChatGroupId, beforeId: string | null): AgentChatTab[] {
  const source = tabs.find((tab) => tab.id === sourceId)
  if (!source || sourceId === beforeId) return tabs
  const moved: AgentChatTab = { ...source, group }
  const rest = tabs.filter((tab) => tab.id !== sourceId)
  const index = beforeId ? rest.findIndex((tab) => tab.id === beforeId) : -1
  const placed = index === -1 ? [...rest, moved] : [...rest.slice(0, index), moved, ...rest.slice(index)]
  return orderTabs(placed)
}

/** Pin or unpin a tab; pinning re-sorts it to the front of its own strip. */
export function setTabPinned(tabs: AgentChatTab[], tabId: string, pinned: boolean): AgentChatTab[] {
  return orderTabs(tabs.map((tab) => (tab.id === tabId ? { ...tab, pinned: pinned || undefined } : tab)))
}

/** Rename a tab. An empty title clears the override so the derived one takes over again. */
export function setTabTitle(tabs: AgentChatTab[], tabId: string, title: string): AgentChatTab[] {
  const trimmed = title.trim()
  return tabs.map((tab) => (tab.id === tabId ? { ...tab, customTitle: trimmed || undefined } : tab))
}

/** Update the program-owned title without disturbing an explicit user rename. */
export function setTerminalTabTitle(tabs: AgentChatTab[], tabId: string, title: string): AgentChatTab[] {
  return tabs.map((tab) => (tab.id === tabId && tab.kind === 'terminal' && tab.title !== title ? { ...tab, title } : tab))
}

/** Ids closed by "close others": the rest of that strip, pinned tabs excepted. */
export function otherTabIds(tabs: AgentChatTab[], tabId: string): string[] {
  const anchor = tabs.find((tab) => tab.id === tabId)
  if (!anchor) return []
  return tabsInGroup(tabs, tabGroup(anchor))
    .filter((tab) => tab.id !== tabId && !tab.pinned)
    .map((tab) => tab.id)
}

/** Ids closed by "close to the right": later tabs in that strip, pinned tabs excepted. */
export function tabIdsAfter(tabs: AgentChatTab[], tabId: string): string[] {
  const anchor = tabs.find((tab) => tab.id === tabId)
  if (!anchor) return []
  const group = tabsInGroup(tabs, tabGroup(anchor))
  const index = group.findIndex((tab) => tab.id === tabId)
  if (index === -1) return []
  return group
    .slice(index + 1)
    .filter((tab) => !tab.pinned)
    .map((tab) => tab.id)
}

/** Pick the next active tab after a close: prefer the right neighbour, then the left one. */
export function nextActiveTabId(tabs: AgentChatTab[], closedId: string, activeId: string | null): string | null {
  if (activeId !== closedId) return activeId
  const index = tabs.findIndex((tab) => tab.id === closedId)
  if (index === -1) return activeId
  const remaining = tabs.filter((tab) => tab.id !== closedId)
  if (remaining.length === 0) return null
  return (remaining[index] ?? remaining[remaining.length - 1]).id
}

let tabSequence = 0

/** Generate a tab id. The prefix keeps debugging readable, the counter keeps it unique. */
export function createTabId(kind: AgentChatTab['kind']): string {
  tabSequence += 1
  return `${kind}-${Date.now().toString(36)}-${tabSequence.toString(36)}`
}
