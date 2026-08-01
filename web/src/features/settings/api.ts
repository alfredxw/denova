import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { LayeredSettings, Settings, SettingsLayer, UpdateApplyResult, UpdateCheckResult } from './types'
import type { SSEEvent } from '@/lib/api-client'

type JSONMergePatch<T> = T extends readonly unknown[]
  ? T | null
  : T extends object
    ? { [K in keyof T]?: JSONMergePatch<NonNullable<T[K]>> | null }
    : T | null

export type SettingsPatch = JSONMergePatch<Settings>

let settingsReadInFlight: Promise<LayeredSettings> | null = null

/** Merge concurrent startup consumers without caching across completed reads. */
export function fetchSettings(): Promise<LayeredSettings> {
  if (settingsReadInFlight) return settingsReadInFlight
  settingsReadInFlight = requestJSON<LayeredSettings>('/api/settings')
    .finally(() => { settingsReadInFlight = null })
  return settingsReadInFlight
}

export async function patchSettings(layer: SettingsLayer, changes: SettingsPatch, baseRevision?: string): Promise<LayeredSettings> {
  return requestJSON('/api/settings', {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ layer, changes, ...(baseRevision ? { base_revision: baseRevision } : {}) }),
  })
}

/** Builds the minimal RFC 7386 object needed to transform baseline into draft. */
export function createSettingsMergePatch(baseline: Settings, draft: Settings): SettingsPatch {
  const patch = createMergePatchValue(baseline, draft)
  return patch === unchanged || !isPlainObject(patch) ? {} : patch as SettingsPatch
}

const unchanged = Symbol('unchanged')

function createMergePatchValue(baseline: unknown, draft: unknown): unknown | typeof unchanged {
  if (Object.is(baseline, draft)) return unchanged
  if (Array.isArray(baseline) || Array.isArray(draft)) {
    return JSON.stringify(baseline) === JSON.stringify(draft) ? unchanged : draft
  }
  if (!isPlainObject(baseline) || !isPlainObject(draft)) return draft
  const result: Record<string, unknown> = {}
  const keys = new Set([...Object.keys(baseline), ...Object.keys(draft)])
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(draft, key) || draft[key] === undefined) {
      result[key] = null
      continue
    }
    if (!Object.prototype.hasOwnProperty.call(baseline, key)) {
      result[key] = draft[key]
      continue
    }
    const child = createMergePatchValue(baseline[key], draft[key])
    if (child !== unchanged) result[key] = child
  }
  return Object.keys(result).length === 0 ? unchanged : result
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
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
