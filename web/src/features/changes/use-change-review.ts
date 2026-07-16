import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { getWorkspaceChangeGroup, getWorkspaceChangeReviewThread, listWorkspaceChangeGroups, type ListWorkspaceChangeGroupsOptions } from './api'
import type { WorkspaceChangeEvent } from './types'

export const workspaceChangeKeys = {
  all: ['workspace-change-groups'] as const,
  lists: () => [...workspaceChangeKeys.all, 'list'] as const,
  list: (workspace: string, options: ListWorkspaceChangeGroupsOptions) => [...workspaceChangeKeys.lists(), workspace, options] as const,
  details: () => [...workspaceChangeKeys.all, 'detail'] as const,
  detail: (workspace: string, id: string) => [...workspaceChangeKeys.details(), workspace, id] as const,
  reviewThreads: () => [...workspaceChangeKeys.all, 'thread'] as const,
  reviewThread: (workspace: string, id: string) => [...workspaceChangeKeys.reviewThreads(), workspace, id] as const,
}

type WorkspaceChangeSubscription = {
  consumers: number
  listener: (event: Event) => void
}

const workspaceChangeSubscriptions = new WeakMap<QueryClient, WorkspaceChangeSubscription>()

export function invalidateWorkspaceChangeQueries(queryClient: QueryClient, workspace: string) {
  if (!workspace) return Promise.resolve()
  return queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[0] === workspaceChangeKeys.all[0] && query.queryKey[2] === workspace,
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
      if (!event.detail?.workspace) return
      void invalidateWorkspaceChangeQueries(queryClient, event.detail.workspace)
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

export function useWorkspaceChangeGroups(workspace: string, options: ListWorkspaceChangeGroupsOptions = {}) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: workspaceChangeKeys.list(workspace, options),
    queryFn: () => listWorkspaceChangeGroups(workspace, options),
    enabled: Boolean(workspace),
    staleTime: 10_000,
  })

  useEffect(() => subscribeWorkspaceChangeEvents(queryClient), [queryClient])

  return query
}

export function useWorkspaceChangeReviewThread(workspace: string, id: string) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: workspaceChangeKeys.reviewThread(workspace, id),
    queryFn: () => getWorkspaceChangeReviewThread(workspace, id),
    enabled: Boolean(workspace && id),
    staleTime: 5_000,
  })

  useEffect(() => subscribeWorkspaceChangeEvents(queryClient), [queryClient])

  return query
}

export function useWorkspaceChangeGroup(workspace: string, id: string) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: workspaceChangeKeys.detail(workspace, id),
    queryFn: () => getWorkspaceChangeGroup(workspace, id),
    enabled: Boolean(workspace && id),
    staleTime: 5_000,
  })

  useEffect(() => subscribeWorkspaceChangeEvents(queryClient), [queryClient])

  return query
}
