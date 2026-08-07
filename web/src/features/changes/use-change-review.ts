import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { getProjectChangeGroup, getProjectChangeReviewThread, listProjectChangeGroups, type ListProjectChangeGroupsOptions } from './api'
import { workspaceChangePaths, type WorkspaceChangeEvent } from './types'

export const projectChangeKeys = {
  all: ['project-change-groups'] as const,
  lists: () => [...projectChangeKeys.all, 'list'] as const,
  list: (projectId: string, options: ListProjectChangeGroupsOptions) => [...projectChangeKeys.lists(), projectId, options] as const,
  details: () => [...projectChangeKeys.all, 'detail'] as const,
  detail: (projectId: string, id: string) => [...projectChangeKeys.details(), projectId, id] as const,
  reviewThreads: () => [...projectChangeKeys.all, 'thread'] as const,
  reviewThread: (projectId: string, id: string) => [...projectChangeKeys.reviewThreads(), projectId, id] as const,
}

const REVIEW_THREAD_STALE_TIME = 5_000

function projectChangeReviewThreadQueryOptions(projectId: string, id: string) {
  return {
    queryKey: projectChangeKeys.reviewThread(projectId, id),
    queryFn: () => getProjectChangeReviewThread(projectId, id),
    staleTime: REVIEW_THREAD_STALE_TIME,
  }
}

/** Warms the same cache entry consumed by the full review surface. */
export function prefetchProjectChangeReviewThread(queryClient: QueryClient, projectId: string, id: string) {
  if (!projectId || !id) return Promise.resolve()
  return queryClient.prefetchQuery(projectChangeReviewThreadQueryOptions(projectId, id))
}

type WorkspaceChangeSubscription = {
  consumers: number
  listener: (event: Event) => void
}

const workspaceChangeSubscriptions = new WeakMap<QueryClient, WorkspaceChangeSubscription>()

export function invalidateProjectChangeQueries(queryClient: QueryClient, projectId: string, event?: WorkspaceChangeEvent) {
  if (!projectId) return Promise.resolve()
  const eventPaths = event ? workspaceChangePaths(event).map(normalizeWorkspacePath).filter(Boolean) : []
  const pathScopedWatcherEvent = event?.source === 'watcher' && !event.resync && eventPaths.length > 0
  return queryClient.invalidateQueries({
    predicate: (query) => {
      if (query.queryKey[0] !== projectChangeKeys.all[0] || query.queryKey[2] !== projectId) return false
      if (!pathScopedWatcherEvent) return true
      return cachedProjectChangePaths(query.state.data).some((cachedPath) =>
        eventPaths.some((eventPath) => workspacePathsOverlap(cachedPath, eventPath)),
      )
    },
  })
}

function cachedProjectChangePaths(data: unknown): string[] {
  const paths = new Set<string>()
  const visit = (value: unknown, depth: number) => {
    if (depth > 4 || value == null) return
    if (Array.isArray(value)) {
      value.forEach((item) => visit(item, depth + 1))
      return
    }
    if (typeof value !== 'object') return
    const record = value as Record<string, unknown>
    if (typeof record.path === 'string') {
      const path = normalizeWorkspacePath(record.path)
      if (path) paths.add(path)
    }
    if (Array.isArray(record.paths)) {
      record.paths.forEach((path) => {
        if (typeof path !== 'string') return
        const normalized = normalizeWorkspacePath(path)
        if (normalized) paths.add(normalized)
      })
    }
    for (const key of ['change_sets', 'files', 'groups']) {
      visit(record[key], depth + 1)
    }
  }
  visit(data, 0)
  return [...paths]
}

function normalizeWorkspacePath(path: string): string {
  return path.trim().replaceAll('\\', '/').replace(/^\.\//, '').replace(/\/+$/, '')
}

function workspacePathsOverlap(left: string, right: string): boolean {
  return left === right || left.startsWith(`${right}/`) || right.startsWith(`${left}/`)
}

function subscribeWorkspaceChangeEvents(queryClient: QueryClient) {
  const existing = workspaceChangeSubscriptions.get(queryClient)
  if (existing) {
    existing.consumers += 1
    return () => {
      existing.consumers -= 1
      if (existing.consumers > 0) return
      window.removeEventListener('nova:workspace-change', existing.listener)
      workspaceChangeSubscriptions.delete(queryClient)
    }
  }

  const subscription: WorkspaceChangeSubscription = {
    consumers: 1,
    listener: (rawEvent) => {
      const event = rawEvent as CustomEvent<WorkspaceChangeEvent>
      if (!event.detail?.project_id) return
      void invalidateProjectChangeQueries(queryClient, event.detail.project_id, event.detail)
    },
  }
  workspaceChangeSubscriptions.set(queryClient, subscription)
  window.addEventListener('nova:workspace-change', subscription.listener)
  return () => {
    subscription.consumers -= 1
    if (subscription.consumers > 0) return
    window.removeEventListener('nova:workspace-change', subscription.listener)
    workspaceChangeSubscriptions.delete(queryClient)
  }
}

export function useProjectChangeGroups(projectId: string, options: ListProjectChangeGroupsOptions = {}) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: projectChangeKeys.list(projectId, options),
    queryFn: () => listProjectChangeGroups(projectId, options),
    enabled: Boolean(projectId),
    staleTime: 10_000,
  })

  useEffect(() => subscribeWorkspaceChangeEvents(queryClient), [queryClient])

  return query
}

export function useProjectChangeReviewThread(projectId: string, id: string) {
  const queryClient = useQueryClient()
  const query = useQuery({
    ...projectChangeReviewThreadQueryOptions(projectId, id),
    enabled: Boolean(projectId && id),
  })

  useEffect(() => subscribeWorkspaceChangeEvents(queryClient), [queryClient])

  return query
}

export function useProjectChangeGroup(projectId: string, id: string) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: projectChangeKeys.detail(projectId, id),
    queryFn: () => getProjectChangeGroup(projectId, id),
    enabled: Boolean(projectId && id),
    staleTime: 5_000,
  })

  useEffect(() => subscribeWorkspaceChangeEvents(queryClient), [queryClient])

  return query
}
