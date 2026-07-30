import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { AgentApprovalMode, LayeredSettings, Settings, UpdateApplyResult, UpdateCheckResult } from './types'
import type { SSEEvent } from '@/lib/api-client'

let settingsReadInFlight: Promise<LayeredSettings> | null = null

/** Merge concurrent startup consumers without caching across completed reads. */
export function fetchSettings(): Promise<LayeredSettings> {
  if (settingsReadInFlight) return settingsReadInFlight
  settingsReadInFlight = requestJSON<LayeredSettings>('/api/settings')
    .finally(() => { settingsReadInFlight = null })
  return settingsReadInFlight
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

export async function updateAgentApprovalMode(mode: AgentApprovalMode): Promise<LayeredSettings> {
  return requestJSON('/api/settings/agent-approval-mode', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({ mode }),
  })
}

function settingsUpdateBody(settings: Settings, baseRevision?: string) {
  return baseRevision ? { settings, base_revision: baseRevision } : settings
}

export async function checkForUpdate(): Promise<UpdateCheckResult> {
  return requestJSON('/api/update/check')
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
