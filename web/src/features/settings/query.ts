import { requestJSON } from '@/lib/api-client'
import { GLOBAL_RESOURCE_TARGET, projectAPIPath, projectResourceTarget } from '@/lib/api-client/project-scope'
import type { ResourceTarget } from '@/lib/api-client/project-scope'
import { queryClient } from '@/lib/query-client'
import type { LayeredSettings } from './types'

export type SettingsTarget = ResourceTarget

export const GLOBAL_SETTINGS_TARGET: SettingsTarget = GLOBAL_RESOURCE_TARGET

export function projectSettingsTarget(projectId: string): SettingsTarget {
  return projectResourceTarget(projectId)
}

export const settingsQueryKeys = {
  all: ['settings'] as const,
  global: () => [...settingsQueryKeys.all, 'global'] as const,
  project: (projectId: string) => [...settingsQueryKeys.all, 'project', projectId.trim()] as const,
}

export function settingsQueryOptions(target: SettingsTarget): {
  queryKey: readonly unknown[]
  queryFn: () => Promise<LayeredSettings>
} {
  if (target.kind === 'project') {
    const projectId = target.projectId.trim()
    return {
      queryKey: settingsQueryKeys.project(projectId),
      queryFn: () => {
        if (!projectId) throw new Error('Project ID is required')
        return requestJSON<LayeredSettings>(projectAPIPath(projectId, 'settings'))
      },
    }
  }
  return {
    queryKey: settingsQueryKeys.global(),
    queryFn: () => requestJSON<LayeredSettings>('/api/settings'),
  }
}

/** Subscribes to the canonical settings snapshot owned by TanStack Query. */
export function subscribeSettingsTarget(
  target: SettingsTarget,
  listener: (snapshot: LayeredSettings) => void,
): () => void {
  const queryKey = settingsQueryOptions(target).queryKey
  return queryClient.getQueryCache().subscribe((event) => {
    if (
      event.type !== 'updated'
      || event.action.type !== 'success'
      || !sameQueryKey(event.query.queryKey, queryKey)
    ) return
    const snapshot = event.query.state.data as LayeredSettings | undefined
    if (snapshot) listener(snapshot)
  })
}

function sameQueryKey(left: readonly unknown[], right: readonly unknown[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}
