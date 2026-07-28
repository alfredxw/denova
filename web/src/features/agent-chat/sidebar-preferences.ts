import { useCallback, useEffect, useMemo, useState } from 'react'
import type { AgentChatProject, AgentChatSession } from './api'

export const AGENT_CHAT_SIDEBAR_SORT_MODES = ['updated', 'opened', 'manual'] as const
export type AgentChatSidebarSortMode = (typeof AGENT_CHAT_SIDEBAR_SORT_MODES)[number]

export interface AgentChatSidebarPreferences {
  sortMode: AgentChatSidebarSortMode
  pinnedProjects: string[]
  pinnedSessions: Record<string, string[]>
  manualProjectOrder: string[]
  manualSessionOrder: Record<string, string[]>
  projectOpenedAt: Record<string, number>
  sessionOpenedAt: Record<string, Record<string, number>>
}

const SIDEBAR_PREFERENCES_STORAGE_KEY = 'nova.agentchat.sidebarPreferences.v1'
const MAX_STORED_SESSION_PREFERENCES = 256

const DEFAULT_PREFERENCES: AgentChatSidebarPreferences = {
  sortMode: 'updated',
  pinnedProjects: [],
  pinnedSessions: {},
  manualProjectOrder: [],
  manualSessionOrder: {},
  projectOpenedAt: {},
  sessionOpenedAt: {},
}

function isSortMode(value: unknown): value is AgentChatSidebarSortMode {
  return typeof value === 'string' && (AGENT_CHAT_SIDEBAR_SORT_MODES as readonly string[]).includes(value)
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return [...new Set(value.filter((item): item is string => typeof item === 'string' && Boolean(item)))]
}

function stringArrayRecord(value: unknown): Record<string, string[]> {
  if (!value || typeof value !== 'object') return {}
  return Object.fromEntries(Object.entries(value).flatMap(([key, item]) => {
    const values = stringArray(item).slice(0, MAX_STORED_SESSION_PREFERENCES)
    return key && values.length ? [[key, values]] : []
  }))
}

function numberRecord(value: unknown): Record<string, number> {
  if (!value || typeof value !== 'object') return {}
  return Object.fromEntries(Object.entries(value).flatMap(([key, item]) => (
    key && typeof item === 'number' && Number.isFinite(item) && item > 0 ? [[key, item]] : []
  )))
}

function nestedNumberRecord(value: unknown): Record<string, Record<string, number>> {
  if (!value || typeof value !== 'object') return {}
  return Object.fromEntries(Object.entries(value).flatMap(([key, item]) => {
    const entries = numberRecord(item)
    return key && Object.keys(entries).length ? [[key, entries]] : []
  }))
}

function readPreferences(): AgentChatSidebarPreferences {
  if (typeof window === 'undefined') return DEFAULT_PREFERENCES
  try {
    const raw = window.localStorage.getItem(SIDEBAR_PREFERENCES_STORAGE_KEY)
    if (!raw) return DEFAULT_PREFERENCES
    const value = JSON.parse(raw) as Record<string, unknown>
    return {
      sortMode: isSortMode(value.sortMode) ? value.sortMode : 'updated',
      pinnedProjects: stringArray(value.pinnedProjects),
      pinnedSessions: stringArrayRecord(value.pinnedSessions),
      manualProjectOrder: stringArray(value.manualProjectOrder),
      manualSessionOrder: stringArrayRecord(value.manualSessionOrder),
      projectOpenedAt: numberRecord(value.projectOpenedAt),
      sessionOpenedAt: nestedNumberRecord(value.sessionOpenedAt),
    }
  } catch (error) {
    console.warn('[features/agent-chat/sidebar-preferences.ts] parsing sidebar preferences failed', { error })
    return DEFAULT_PREFERENCES
  }
}

function persistPreferences(preferences: AgentChatSidebarPreferences) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(SIDEBAR_PREFERENCES_STORAGE_KEY, JSON.stringify(preferences))
  } catch (error) {
    console.warn('[features/agent-chat/sidebar-preferences.ts] persisting sidebar preferences failed', { error })
  }
}

function limitNewest(values: Record<string, number>): Record<string, number> {
  return Object.fromEntries(Object.entries(values)
    .sort((left, right) => right[1] - left[1])
    .slice(0, MAX_STORED_SESSION_PREFERENCES))
}

function appendMissing(order: string[], ids: string[], preserveUnknown: boolean): string[] {
  const known = new Set(ids)
  const next = order.filter((id) => preserveUnknown || known.has(id))
  const included = new Set(next)
  for (const id of ids) {
    if (!included.has(id)) next.push(id)
  }
  return next.slice(0, MAX_STORED_SESSION_PREFERENCES)
}

function normalizePreferences(
  preferences: AgentChatSidebarPreferences,
  projects: AgentChatProject[],
): AgentChatSidebarPreferences {
  const projectPaths = projects.map((project) => project.path)
  const projectSet = new Set(projectPaths)
  const projectRecords = <T,>(record: Record<string, T>): Record<string, T> => Object.fromEntries(
    Object.entries(record).filter(([path]) => projectSet.has(path)),
  )
  const manualProjectOrder = preferences.manualProjectOrder.length
    ? appendMissing(preferences.manualProjectOrder, projectPaths, false)
    : []
  const manualSessionOrder = projectRecords(preferences.manualSessionOrder)
  if (preferences.sortMode === 'manual' || Object.keys(manualSessionOrder).length) {
    for (const project of projects) {
      const ids = project.sessions.map((session) => session.id)
      manualSessionOrder[project.path] = appendMissing(manualSessionOrder[project.path] || [], ids, true)
    }
  }
  const sessionOpenedAt = Object.fromEntries(Object.entries(projectRecords(preferences.sessionOpenedAt))
    .map(([path, values]) => [path, limitNewest(values)]))
  return {
    ...preferences,
    pinnedProjects: preferences.pinnedProjects.filter((path) => projectSet.has(path)),
    pinnedSessions: projectRecords(preferences.pinnedSessions),
    manualProjectOrder,
    manualSessionOrder,
    projectOpenedAt: projectRecords(preferences.projectOpenedAt),
    sessionOpenedAt,
  }
}

function timestamp(value: string): number {
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function projectUpdatedAt(project: AgentChatProject): number {
  return project.sessions.reduce((latest, session) => Math.max(latest, timestamp(session.updated_at || session.created_at)), 0)
}

function stablePreferenceSort<T>(
  items: T[],
  pinned: (item: T) => boolean,
  primary: (item: T) => number,
  secondary: (item: T) => number,
): T[] {
  return items
    .map((item, index) => ({ item, index }))
    .sort((left, right) => Number(pinned(right.item)) - Number(pinned(left.item))
      || primary(right.item) - primary(left.item)
      || secondary(right.item) - secondary(left.item)
      || left.index - right.index)
    .map(({ item }) => item)
}

/** Pin grouping and sort semantics are identical at the project and conversation levels. */
export function orderAgentChatProjects(
  projects: AgentChatProject[],
  preferences: AgentChatSidebarPreferences,
): AgentChatProject[] {
  const pinned = new Set(preferences.pinnedProjects)
  const manualRank = new Map(preferences.manualProjectOrder.map((path, index) => [path, index]))
  const fallbackRank = preferences.manualProjectOrder.length + projects.length
  if (preferences.sortMode === 'manual') {
    return stablePreferenceSort(projects, (project) => pinned.has(project.path),
      (project) => fallbackRank - (manualRank.get(project.path) ?? fallbackRank), () => 0)
  }
  if (preferences.sortMode === 'opened') {
    return stablePreferenceSort(projects, (project) => pinned.has(project.path),
      (project) => preferences.projectOpenedAt[project.path] || 0, projectUpdatedAt)
  }
  return stablePreferenceSort(projects, (project) => pinned.has(project.path), projectUpdatedAt, () => 0)
}

export function orderAgentChatSessions(
  project: AgentChatProject,
  preferences: AgentChatSidebarPreferences,
): AgentChatSession[] {
  const pinned = new Set(preferences.pinnedSessions[project.path] || [])
  const manualOrder = preferences.manualSessionOrder[project.path] || []
  const manualRank = new Map(manualOrder.map((id, index) => [id, index]))
  const fallbackRank = manualOrder.length + project.sessions.length
  if (preferences.sortMode === 'manual') {
    return stablePreferenceSort(project.sessions, (session) => pinned.has(session.id),
      (session) => fallbackRank - (manualRank.get(session.id) ?? fallbackRank), () => 0)
  }
  if (preferences.sortMode === 'opened') {
    const opened = preferences.sessionOpenedAt[project.path] || {}
    return stablePreferenceSort(project.sessions, (session) => pinned.has(session.id),
      (session) => opened[session.id] || 0, (session) => timestamp(session.updated_at || session.created_at))
  }
  return stablePreferenceSort(project.sessions, (session) => pinned.has(session.id),
    (session) => timestamp(session.updated_at || session.created_at), () => 0)
}

/** Replaces only visible ids, preserving stored ids currently outside the bounded server window. */
export function reorderKnownItems(stored: string[], visible: string[], active: string, over: string): string[] {
  const from = visible.indexOf(active)
  const to = visible.indexOf(over)
  if (from === -1 || to === -1 || from === to) return stored.length ? stored : visible
  const moved = [...visible]
  const [item] = moved.splice(from, 1)
  moved.splice(to, 0, item)
  if (!stored.length) return moved
  const visibleSet = new Set(visible)
  let index = 0
  const next = stored.map((id) => visibleSet.has(id) ? moved[index++] : id)
  while (index < moved.length) next.push(moved[index++])
  return [...new Set(next)].slice(0, MAX_STORED_SESSION_PREFERENCES)
}

function toggleID(values: string[], id: string): string[] {
  return values.includes(id) ? values.filter((value) => value !== id) : [id, ...values]
}

/** Owns user-level AgentChat navigation preferences; no ordering metadata enters a book. */
export function useAgentChatSidebarPreferences(projects: AgentChatProject[]) {
  const [preferences, setPreferences] = useState<AgentChatSidebarPreferences>(readPreferences)

  useEffect(() => {
    setPreferences((current) => normalizePreferences(current, projects))
  }, [projects])
  useEffect(() => persistPreferences(preferences), [preferences])

  const orderedProjects = useMemo(() => orderAgentChatProjects(projects, preferences), [preferences, projects])
  const sessionsForProject = useCallback(
    (project: AgentChatProject) => orderAgentChatSessions(project, preferences),
    [preferences],
  )
  const setSortMode = useCallback((sortMode: AgentChatSidebarSortMode) => {
    setPreferences((current) => {
      if (sortMode !== 'manual' || current.manualProjectOrder.length) {
        return normalizePreferences({ ...current, sortMode }, projects)
      }
      const manualProjectOrder = orderAgentChatProjects(projects, current).map((project) => project.path)
      const manualSessionOrder = Object.fromEntries(projects.map((project) => [
        project.path,
        orderAgentChatSessions(project, current).map((session) => session.id),
      ]))
      return normalizePreferences({ ...current, sortMode, manualProjectOrder, manualSessionOrder }, projects)
    })
  }, [projects])
  const recordProjectOpened = useCallback((path: string) => {
    const openedAt = Date.now()
    setPreferences((current) => ({ ...current, projectOpenedAt: { ...current.projectOpenedAt, [path]: openedAt } }))
  }, [])
  const recordSessionOpened = useCallback((path: string, sessionID: string) => {
    const openedAt = Date.now()
    setPreferences((current) => ({
      ...current,
      projectOpenedAt: { ...current.projectOpenedAt, [path]: openedAt },
      sessionOpenedAt: {
        ...current.sessionOpenedAt,
        [path]: limitNewest({ ...(current.sessionOpenedAt[path] || {}), [sessionID]: openedAt }),
      },
    }))
  }, [])
  const toggleProjectPinned = useCallback((path: string) => {
    setPreferences((current) => ({ ...current, pinnedProjects: toggleID(current.pinnedProjects, path) }))
  }, [])
  const toggleSessionPinned = useCallback((path: string, sessionID: string) => {
    setPreferences((current) => ({
      ...current,
      pinnedSessions: {
        ...current.pinnedSessions,
        [path]: toggleID(current.pinnedSessions[path] || [], sessionID).slice(0, MAX_STORED_SESSION_PREFERENCES),
      },
    }))
  }, [])
  const moveProject = useCallback((activePath: string, overPath: string) => {
    setPreferences((current) => {
      if (current.sortMode !== 'manual') return current
      const visible = orderAgentChatProjects(projects, current).map((project) => project.path)
      return { ...current, manualProjectOrder: reorderKnownItems(current.manualProjectOrder, visible, activePath, overPath) }
    })
  }, [projects])
  const moveSession = useCallback((project: AgentChatProject, activeID: string, overID: string) => {
    setPreferences((current) => {
      if (current.sortMode !== 'manual') return current
      const visible = orderAgentChatSessions(project, current).map((session) => session.id)
      return {
        ...current,
        manualSessionOrder: {
          ...current.manualSessionOrder,
          [project.path]: reorderKnownItems(current.manualSessionOrder[project.path] || [], visible, activeID, overID),
        },
      }
    })
  }, [])

  return {
    sortMode: preferences.sortMode,
    orderedProjects,
    sessionsForProject,
    setSortMode,
    recordProjectOpened,
    recordSessionOpened,
    isProjectPinned: (path: string) => preferences.pinnedProjects.includes(path),
    isSessionPinned: (path: string, sessionID: string) => (preferences.pinnedSessions[path] || []).includes(sessionID),
    toggleProjectPinned,
    toggleSessionPinned,
    moveProject,
    moveSession,
  }
}
