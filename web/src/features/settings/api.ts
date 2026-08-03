import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { LayeredSettings, ModelCatalog, ModelDiscoveryResult, ModelPingResult, ModelProfileSettings, Settings, SettingsLayer, UpdateApplyResult, UpdateCheckResult } from './types'
import type { SSEEvent } from '@/lib/api-client'

type JSONMergePatch<T> = T extends readonly unknown[]
  ? T | null
  : T extends object
    ? { [K in keyof T]?: JSONMergePatch<NonNullable<T[K]>> | null }
    : T | null

export type SettingsPatch = JSONMergePatch<Settings>

let settingsReadInFlight: Promise<LayeredSettings> | null = null
let settingsRefreshBatch: Promise<LayeredSettings> | null = null
let settingsReadCache: LayeredSettings | null = null
let settingsReadGeneration = 0

/** Shares the current settings snapshot across startup consumers. */
export function fetchSettings(): Promise<LayeredSettings> {
  if (settingsReadCache) return Promise.resolve(settingsReadCache)
  if (settingsReadInFlight) return settingsReadInFlight
  return startSettingsRead(settingsReadGeneration)
}

/** Invalidates once while sharing the read between synchronous listeners of one update event. */
export function refreshSettings(): Promise<LayeredSettings> {
  if (settingsRefreshBatch) return settingsRefreshBatch
  settingsReadGeneration += 1
  settingsReadCache = null
  settingsReadInFlight = null
  const generation = settingsReadGeneration
  const promise = startSettingsRead(generation)
  settingsRefreshBatch = promise
  queueMicrotask(() => {
    if (settingsRefreshBatch === promise) settingsRefreshBatch = null
  })
  return promise
}

export function invalidateSettingsCache() {
  settingsReadGeneration += 1
  settingsReadCache = null
  settingsReadInFlight = null
  settingsRefreshBatch = null
}

export async function patchSettings(layer: SettingsLayer, changes: SettingsPatch, baseRevision?: string): Promise<LayeredSettings> {
  const snapshot = await requestJSON<LayeredSettings>('/api/settings', {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ layer, changes, ...(baseRevision ? { base_revision: baseRevision } : {}) }),
  })
  primeSettingsCache(snapshot)
  return snapshot
}

/** Revokes one saved rule by stable ID so concurrent rule additions cannot be
 * lost through replacement of a stale settings array. */
export async function revokeAgentApprovalRule(id: string): Promise<LayeredSettings> {
  const snapshot = await requestJSON<LayeredSettings>(`/api/settings/agent-approval-rules/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  primeSettingsCache(snapshot)
  return snapshot
}

function startSettingsRead(generation: number): Promise<LayeredSettings> {
  const promise = requestJSON<LayeredSettings>('/api/settings').then((snapshot) => {
    if (generation === settingsReadGeneration) settingsReadCache = snapshot
    return snapshot
  })
  settingsReadInFlight = promise
  void promise.then(
    () => { if (settingsReadInFlight === promise) settingsReadInFlight = null },
    () => { if (settingsReadInFlight === promise) settingsReadInFlight = null },
  )
  return promise
}

function primeSettingsCache(snapshot: LayeredSettings) {
  settingsReadGeneration += 1
  settingsReadCache = snapshot
  settingsReadInFlight = null
  settingsRefreshBatch = null
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

export function fetchModelCatalog(signal?: AbortSignal): Promise<ModelCatalog> {
  return requestJSON('/api/models/catalog', { signal })
}

/** Loads optional OpenAI-compatible model suggestions without validating or
 * restricting the profile's custom model text. */
export function discoverModels(profile: ModelProfileSettings, signal?: AbortSignal): Promise<ModelDiscoveryResult> {
  return requestJSON('/api/models/discover', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ profile }),
    signal,
  })
}

/** Validates an unsaved profile through the same resolver and adapter as a real Agent run. */
export function pingModelProfile(profile: ModelProfileSettings, signal?: AbortSignal): Promise<ModelPingResult> {
  return requestJSON('/api/models/ping', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ profile }),
    signal,
  })
}
