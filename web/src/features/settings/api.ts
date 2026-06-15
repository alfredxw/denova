import { jsonHeaders, requestJSON } from '@/lib/api-client'
import type { LayeredSettings, Settings, UpdateCheckResult, UpdateInstallResult } from './types'

export async function fetchSettings(): Promise<LayeredSettings> {
  return requestJSON('/api/settings')
}

export async function updateUserSettings(s: Settings): Promise<LayeredSettings> {
  return requestJSON('/api/settings/user', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(s),
  })
}

export async function updateWorkspaceSettings(s: Settings): Promise<LayeredSettings> {
  return requestJSON('/api/settings/workspace', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(s),
  })
}

export async function checkForUpdate(): Promise<UpdateCheckResult> {
  return requestJSON('/api/update/check')
}

export async function installUpdate(): Promise<UpdateInstallResult> {
  return requestJSON('/api/update/install', { method: 'POST' })
}
