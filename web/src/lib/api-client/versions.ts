import { jsonHeaders, requestJSON } from './client'
import { projectAPIPath } from './project-scope'
import type { VersionCommandResult, VersionDiff, VersionDiffComparison, VersionEntry, VersionRestorePlan, VersionRestoreResult, VersionStatus } from './types'

export const versionQueryKeys = {
  all: ['versions'] as const,
  status: (projectId: string) => ['versions', 'status', projectId] as const,
  history: (projectId: string, limit: number) => ['versions', 'history', projectId, limit] as const,
  diff: (projectId: string, versionId: string, comparison: VersionDiffComparison) =>
    ['versions', 'diff', projectId, versionId, comparison] as const,
}

export async function getVersionStatus(projectId: string): Promise<VersionStatus> {
  const status = await requestJSON<VersionStatus>(projectAPIPath(projectId, 'versions/status'))
  return {
    ...status,
    changes: status.changes ?? [],
  }
}

export async function getVersions(projectId: string, limit = 30): Promise<VersionEntry[]> {
  const data = await requestJSON<{ versions: VersionEntry[] }>(`${projectAPIPath(projectId, 'versions')}?limit=${encodeURIComponent(String(limit))}`)
  return data.versions || []
}

export async function createVersion(projectId: string, message = ''): Promise<VersionCommandResult> {
  return requestJSON(projectAPIPath(projectId, 'versions'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ message }),
  })
}

export async function getVersionDiff(projectId: string, id: string, path?: string, comparison: VersionDiffComparison = 'workspace'): Promise<VersionDiff> {
  const query = new URLSearchParams({ comparison })
  if (path) query.set('path', path)
  return requestJSON(`${projectAPIPath(projectId, `versions/${encodeURIComponent(id)}/diff`)}?${query.toString()}`)
}

function restoreBody(paths?: string[]) {
  if (!paths || paths.length === 0) return undefined
  return JSON.stringify({ paths })
}

export async function getVersionRestorePlan(projectId: string, id: string, paths?: string[]): Promise<VersionRestorePlan> {
  return requestJSON(projectAPIPath(projectId, `versions/${encodeURIComponent(id)}/restore-plan`), {
    method: 'POST',
    headers: jsonHeaders,
    body: restoreBody(paths),
  })
}

export async function restoreVersion(projectId: string, id: string, paths?: string[]): Promise<VersionRestoreResult> {
  return requestJSON(projectAPIPath(projectId, `versions/${encodeURIComponent(id)}/restore`), {
    method: 'POST',
    headers: jsonHeaders,
    body: restoreBody(paths),
  })
}
