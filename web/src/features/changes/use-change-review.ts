import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { getProjectChangeGroup, getProjectChangeReviewThread, listProjectChangeGroups, type ListProjectChangeGroupsOptions } from './api'
import type { WorkspaceChangeEvent } from './types'

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

export function invalidateProjectChangeQueries(queryClient: QueryClient, projectId: string) {
  if (!projectId) return Promise.resolve()
  return queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[0] === projectChangeKeys.all[0] && query.queryKey[2] === projectId,
  })
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
      void invalidateProjectChangeQueries(queryClient, event.detail.project_id)
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
