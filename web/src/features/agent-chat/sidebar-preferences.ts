import { useCallback, useEffect, useMemo, useState } from 'react'
import type { AgentChatProject } from './api'

export const AGENT_CHAT_SIDEBAR_SORT_MODES = ['updated', 'opened', 'manual'] as const
export type AgentChatSidebarSortMode = (typeof AGENT_CHAT_SIDEBAR_SORT_MODES)[number]

/** Project-level navigation preferences. Activity order itself is owned by the tab workbench. */
export interface AgentChatSidebarPreferences {
  sortMode: AgentChatSidebarSortMode
  pinnedProjects: string[]
  manualProjectOrder: string[]
  projectOpenedAt: Record<string, number>
}

const SIDEBAR_PREFERENCES_STORAGE_KEY = 'nova.agentchat.sidebarPreferences.v1'

const DEFAULT_PREFERENCES: AgentChatSidebarPreferences = {
  sortMode: 'updated',
  pinnedProjects: [],
  manualProjectOrder: [],
  projectOpenedAt: {},
}

function isSortMode(value: unknown): value is AgentChatSidebarSortMode {
  return typeof value === 'string' && (AGENT_CHAT_SIDEBAR_SORT_MODES as readonly string[]).includes(value)
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return [...new Set(value.filter((item): item is string => typeof item === 'string' && Boolean(item)))]
}

function numberRecord(value: unknown): Record<string, number> {
  if (!value || typeof value !== 'object') return {}
  return Object.fromEntries(Object.entries(value).flatMap(([key, item]) => (
    key && typeof item === 'number' && Number.isFinite(item) && item > 0 ? [[key, item]] : []
  )))
}

function readPreferences(): AgentChatSidebarPreferences {
  if (typeof window === 'undefined') return DEFAULT_PREFERENCES
  try {
    const raw = window.localStorage.getItem(SIDEBAR_PREFERENCES_STORAGE_KEY)
    if (!raw) return DEFAULT_PREFERENCES
    const value = JSON.parse(raw) as Record<string, unknown>
    // Conversation-level fields written by the old history tree are deliberately ignored. The
    // same storage key keeps the user's project pins and order across the navigation redesign.
    return {
      sortMode: isSortMode(value.sortMode) ? value.sortMode : 'updated',
      pinnedProjects: stringArray(value.pinnedProjects),
      manualProjectOrder: stringArray(value.manualProjectOrder),
      projectOpenedAt: numberRecord(value.projectOpenedAt),
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

function appendMissing(order: string[], ids: string[]): string[] {
  const known = new Set(ids)
  const next = order.filter((id) => known.has(id))
  const included = new Set(next)
  for (const id of ids) {
    if (!included.has(id)) next.push(id)
  }
  return next
}

function normalizePreferences(
  preferences: AgentChatSidebarPreferences,
  projects: AgentChatProject[],
): AgentChatSidebarPreferences {
  const projectPaths = projects.map((project) => project.path)
  const projectSet = new Set(projectPaths)
  return {
    ...preferences,
    pinnedProjects: preferences.pinnedProjects.filter((path) => projectSet.has(path)),
    manualProjectOrder: preferences.manualProjectOrder.length
      ? appendMissing(preferences.manualProjectOrder, projectPaths)
      : [],
    projectOpenedAt: Object.fromEntries(
      Object.entries(preferences.projectOpenedAt).filter(([path]) => projectSet.has(path)),
    ),
  }
}

function timestamp(value: string): number {
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function projectUpdatedAt(project: AgentChatProject): number {
  return project.sessions.reduce(
    (latest, session) => Math.max(latest, timestamp(session.updated_at || session.created_at)),
    0,
  )
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
  return [...new Set(next)]
}

function toggleID(values: string[], id: string): string[] {
  return values.includes(id) ? values.filter((value) => value !== id) : [id, ...values]
}

/** Owns project navigation preferences; activity ordering remains a pure workbench projection. */
export function useAgentChatSidebarPreferences(projects: AgentChatProject[]) {
  const [preferences, setPreferences] = useState<AgentChatSidebarPreferences>(readPreferences)

  useEffect(() => {
    setPreferences((current) => normalizePreferences(current, projects))
  }, [projects])
  useEffect(() => persistPreferences(preferences), [preferences])

  const orderedProjects = useMemo(() => orderAgentChatProjects(projects, preferences), [preferences, projects])
  const setSortMode = useCallback((sortMode: AgentChatSidebarSortMode) => {
    setPreferences((current) => {
      if (sortMode !== 'manual' || current.manualProjectOrder.length) {
        return normalizePreferences({ ...current, sortMode }, projects)
      }
      return normalizePreferences({
        ...current,
        sortMode,
        manualProjectOrder: orderAgentChatProjects(projects, current).map((project) => project.path),
      }, projects)
    })
  }, [projects])
  const recordProjectOpened = useCallback((path: string) => {
    setPreferences((current) => ({
      ...current,
      projectOpenedAt: { ...current.projectOpenedAt, [path]: Date.now() },
    }))
  }, [])
  const toggleProjectPinned = useCallback((path: string) => {
    setPreferences((current) => ({ ...current, pinnedProjects: toggleID(current.pinnedProjects, path) }))
  }, [])
  const moveProject = useCallback((activePath: string, overPath: string) => {
    setPreferences((current) => {
      if (current.sortMode !== 'manual') return current
      const visible = orderAgentChatProjects(projects, current).map((project) => project.path)
      return {
        ...current,
        manualProjectOrder: reorderKnownItems(current.manualProjectOrder, visible, activePath, overPath),
      }
    })
  }, [projects])

  return {
    sortMode: preferences.sortMode,
    orderedProjects,
    setSortMode,
    recordProjectOpened,
    isProjectPinned: (path: string) => preferences.pinnedProjects.includes(path),
    toggleProjectPinned,
    moveProject,
  }
}
