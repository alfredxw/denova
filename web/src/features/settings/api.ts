import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { LayeredSettings, ModelCatalog, ModelDiscoveryResult, ModelPingResult, ModelProfileSettings, Settings, SettingsLayer, UpdateApplyResult, UpdateCheckResult } from './types'
import type { SSEEvent } from '@/lib/api-client'
import { projectAPIPath } from '@/lib/api-client/project-scope'
import { GLOBAL_RESOURCE_TARGET, projectResourceTarget } from '@/lib/api-client/project-scope'
import type { ResourceTarget } from '@/lib/api-client/project-scope'

type JSONMergePatch<T> = T extends readonly unknown[]
  ? T | null
  : T extends object
    ? { [K in keyof T]?: JSONMergePatch<NonNullable<T[K]>> | null }
    : T | null

export type SettingsPatch = JSONMergePatch<Settings>

/** Selects a settings projection without inferring foreground navigation state. */
export type SettingsTarget = ResourceTarget

export const GLOBAL_SETTINGS_TARGET: SettingsTarget = GLOBAL_RESOURCE_TARGET

export function projectSettingsTarget(projectId: string): SettingsTarget {
  return projectResourceTarget(projectId)
}

interface SettingsCacheEntry {
  readInFlight: Promise<LayeredSettings> | null
  refreshBatch: Promise<LayeredSettings> | null
  snapshot: LayeredSettings | null
  generation: number
}

const GLOBAL_SETTINGS_SCOPE = 'global'
const settingsCaches = new Map<string, SettingsCacheEntry>()

function settingsCache(scope: string): SettingsCacheEntry {
  const existing = settingsCaches.get(scope)
  if (existing) return existing
  const created: SettingsCacheEntry = { readInFlight: null, refreshBatch: null, snapshot: null, generation: 0 }
  settingsCaches.set(scope, created)
  return created
}

/** Shares the current settings snapshot across startup consumers. */
export function fetchSettings(): Promise<LayeredSettings> {
  return fetchSettingsForScope(GLOBAL_SETTINGS_SCOPE, '/api/settings')
}

/** Returns the user layer merged with exactly one Project layer. */
export function fetchProjectSettings(projectId: string): Promise<LayeredSettings> {
  const scope = projectSettingsScope(projectId)
  return fetchSettingsForScope(scope, projectAPIPath(projectId, 'settings'))
}

/** Invalidates once while sharing the read between synchronous listeners of one update event. */
export function refreshSettings(): Promise<LayeredSettings> {
  return refreshSettingsForScope(GLOBAL_SETTINGS_SCOPE, '/api/settings')
}

export function refreshProjectSettings(projectId: string): Promise<LayeredSettings> {
  const scope = projectSettingsScope(projectId)
  return refreshSettingsForScope(scope, projectAPIPath(projectId, 'settings'))
}

export function fetchSettingsTarget(target: SettingsTarget): Promise<LayeredSettings> {
  return target.kind === 'project' ? fetchProjectSettings(target.projectId) : fetchSettings()
}

export function refreshSettingsTarget(target: SettingsTarget): Promise<LayeredSettings> {
  return target.kind === 'project' ? refreshProjectSettings(target.projectId) : refreshSettings()
}

export function invalidateSettingsCache(projectId?: string) {
  if (projectId === undefined) {
    settingsCaches.clear()
    return
  }
  settingsCaches.delete(projectSettingsScope(projectId))
}

export async function patchSettings(layer: SettingsLayer, changes: SettingsPatch, baseRevision?: string): Promise<LayeredSettings> {
  const snapshot = await requestJSON<LayeredSettings>('/api/settings', {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ layer, changes, ...(baseRevision ? { base_revision: baseRevision } : {}) }),
  })
  primeSettingsCache(GLOBAL_SETTINGS_SCOPE, snapshot, layer === 'user')
  return snapshot
}

export async function patchProjectSettings(projectId: string, layer: SettingsLayer, changes: SettingsPatch, baseRevision?: string): Promise<LayeredSettings> {
  const scope = projectSettingsScope(projectId)
  const snapshot = await requestJSON<LayeredSettings>(projectAPIPath(projectId, 'settings'), {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ layer, changes, ...(baseRevision ? { base_revision: baseRevision } : {}) }),
  })
  primeSettingsCache(scope, snapshot, layer === 'user')
  return snapshot
}

export function patchSettingsTarget(
  target: SettingsTarget,
  layer: SettingsLayer,
  changes: SettingsPatch,
  baseRevision?: string,
): Promise<LayeredSettings> {
  return target.kind === 'project'
    ? patchProjectSettings(target.projectId, layer, changes, baseRevision)
    : patchSettings(layer, changes, baseRevision)
}

/** Revokes one saved rule by stable ID so concurrent rule additions cannot be
 * lost through replacement of a stale settings array. */
export async function revokeAgentApprovalRule(id: string): Promise<LayeredSettings> {
  const snapshot = await requestJSON<LayeredSettings>(`/api/settings/agent-approval-rules/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  primeSettingsCache(GLOBAL_SETTINGS_SCOPE, snapshot, true)
  return snapshot
}

function fetchSettingsForScope(scope: string, path: string): Promise<LayeredSettings> {
  const cache = settingsCache(scope)
  if (cache.snapshot) return Promise.resolve(cache.snapshot)
  if (cache.readInFlight) return cache.readInFlight
  return startSettingsRead(scope, path, cache.generation)
}

function refreshSettingsForScope(scope: string, path: string): Promise<LayeredSettings> {
  const cache = settingsCache(scope)
  if (cache.refreshBatch) return cache.refreshBatch
  cache.generation += 1
  cache.snapshot = null
  cache.readInFlight = null
  const generation = cache.generation
  const promise = startSettingsRead(scope, path, generation)
  cache.refreshBatch = promise
  queueMicrotask(() => {
    if (cache.refreshBatch === promise) cache.refreshBatch = null
  })
  return promise
}

function startSettingsRead(scope: string, path: string, generation: number): Promise<LayeredSettings> {
  const cache = settingsCache(scope)
  const promise = requestJSON<LayeredSettings>(path).then((snapshot) => {
    if (generation === cache.generation) cache.snapshot = snapshot
    return snapshot
  })
  cache.readInFlight = promise
  void promise.then(
    () => { if (cache.readInFlight === promise) cache.readInFlight = null },
    () => { if (cache.readInFlight === promise) cache.readInFlight = null },
  )
  return promise
}

function primeSettingsCache(scope: string, snapshot: LayeredSettings, invalidateAll: boolean) {
  if (invalidateAll) settingsCaches.clear()
  const cache = settingsCache(scope)
  cache.generation += 1
  cache.snapshot = snapshot
  cache.readInFlight = null
  cache.refreshBatch = null
}

function projectSettingsScope(projectId: string): string {
  const normalized = projectId.trim()
  if (!normalized) throw new Error('Project ID is required')
  return `project:${normalized}`
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
