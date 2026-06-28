import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { LayeredSettings, Settings, UpdateApplyResult, UpdateCheckResult, UpdateInstallResult } from './types'
import type { SSEEvent } from '@/lib/api-client'

export async function fetchSettings(): Promise<LayeredSettings> {
  return requestJSON('/api/settings')
}

export async function updateUserSettings(s: Settings, baseRevision?: string): Promise<LayeredSettings> {
  return requestJSON('/api/settings/user', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(settingsUpdateBody(s, baseRevision)),
  })
}

export async function updateWorkspaceSettings(s: Settings, baseRevision?: string): Promise<LayeredSettings> {
  return requestJSON('/api/settings/workspace', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(settingsUpdateBody(s, baseRevision)),
  })
}

function settingsUpdateBody(settings: Settings, baseRevision?: string) {
  return baseRevision ? { settings, base_revision: baseRevision } : settings
}

export async function checkForUpdate(): Promise<UpdateCheckResult> {
  return requestJSON('/api/update/check')
}

export async function installUpdate(): Promise<UpdateInstallResult> {
  return requestJSON('/api/update/install', { method: 'POST' })
}

export async function installUpdateStream(signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI('/api/update/install/stream', { method: 'POST', signal })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function applyUpdate(): Promise<UpdateApplyResult> {
  return requestJSON('/api/update/apply', { method: 'POST' })
}
