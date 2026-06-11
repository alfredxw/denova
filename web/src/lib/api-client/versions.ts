import { jsonHeaders, requestJSON } from './client'
import type { VersionCommandResult, VersionDiff, VersionEntry, VersionStatus } from './types'

export async function getVersionStatus(): Promise<VersionStatus> {
  const status = await requestJSON<VersionStatus>('/api/versions/status')
  return {
    ...status,
    changes: status.changes ?? [],
  }
}

export async function getVersions(limit = 30): Promise<VersionEntry[]> {
  const data = await requestJSON<{ versions: VersionEntry[] }>(`/api/versions?limit=${encodeURIComponent(String(limit))}`)
  return data.versions || []
}

export async function createVersion(message = ''): Promise<VersionCommandResult> {
  return requestJSON('/api/versions', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ message }),
  })
}

export async function getVersionDiff(id: string, path?: string): Promise<VersionDiff> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  return requestJSON(`/api/versions/${encodeURIComponent(id)}/diff${query}`)
}

export async function restoreVersion(id: string): Promise<VersionCommandResult> {
  return requestJSON(`/api/versions/${encodeURIComponent(id)}/restore`, { method: 'POST' })
}
