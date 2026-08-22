import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client'
import type { ImageAPIProfileSettings, ImagePingResult, LayeredSettings, ModelCatalog, ModelDiscoveryResult, ModelPingResult, ModelProfileSettings, Settings, SettingsLayer, UpdateApplyResult, UpdateCheckResult } from './types'
import type { SSEEvent } from '@/lib/api-client'
import { projectAPIPath } from '@/lib/api-client/project-scope'
import { queryClient } from '@/lib/query-client'
import { GLOBAL_SETTINGS_TARGET, projectSettingsTarget, settingsQueryKeys, settingsQueryOptions } from './query'
import type { SettingsTarget } from './query'

export { GLOBAL_SETTINGS_TARGET, projectSettingsTarget }
export type { SettingsTarget }

type JSONMergePatch<T> = T extends readonly unknown[]
  ? T | null
  : T extends object
    ? { [K in keyof T]?: JSONMergePatch<NonNullable<T[K]>> | null }
    : T | null

export type SettingsPatch = JSONMergePatch<Settings>

/** Shares the current settings snapshot across startup consumers. */
export function fetchSettings(): Promise<LayeredSettings> {
  return fetchSettingsTarget(GLOBAL_SETTINGS_TARGET)
}

/** Returns the user layer merged with exactly one Project layer. */
export function fetchProjectSettings(projectId: string): Promise<LayeredSettings> {
  return fetchSettingsTarget(projectSettingsTarget(projectId))
}

/** Forces a canonical settings refresh while TanStack Query coalesces callers. */
export function refreshSettings(): Promise<LayeredSettings> {
  return refreshSettingsTarget(GLOBAL_SETTINGS_TARGET)
}

export function refreshProjectSettings(projectId: string): Promise<LayeredSettings> {
  return refreshSettingsTarget(projectSettingsTarget(projectId))
}

export function fetchSettingsTarget(target: SettingsTarget): Promise<LayeredSettings> {
  return queryClient.fetchQuery(settingsQueryOptions(target))
}

export async function refreshSettingsTarget(target: SettingsTarget): Promise<LayeredSettings> {
  const options = settingsQueryOptions(target)
  await queryClient.invalidateQueries({ queryKey: options.queryKey, exact: true, refetchType: 'none' })
  return queryClient.fetchQuery({ ...options, staleTime: 0 })
}

/** Removes cached settings snapshots, primarily for tests and sign-out. */
export function invalidateSettingsCache(projectId?: string) {
  if (projectId === undefined) {
    queryClient.removeQueries({ queryKey: settingsQueryKeys.all })
    return
  }
  queryClient.removeQueries({ queryKey: settingsQueryKeys.project(projectId), exact: true })
}

export async function patchSettings(layer: SettingsLayer, changes: SettingsPatch, baseRevision?: string): Promise<LayeredSettings> {
  const snapshot = await requestJSON<LayeredSettings>('/api/settings', {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ layer, changes, ...(baseRevision ? { base_revision: baseRevision } : {}) }),
  })
  primeSettingsQuery(settingsQueryKeys.global(), snapshot, layer === 'user')
  return snapshot
}

export async function patchProjectSettings(projectId: string, layer: SettingsLayer, changes: SettingsPatch, baseRevision?: string): Promise<LayeredSettings> {
  const normalized = normalizeProjectID(projectId)
  const snapshot = await requestJSON<LayeredSettings>(projectAPIPath(normalized, 'settings'), {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ layer, changes, ...(baseRevision ? { base_revision: baseRevision } : {}) }),
  })
  primeSettingsQuery(settingsQueryKeys.project(normalized), snapshot, layer === 'user')
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
  primeSettingsQuery(settingsQueryKeys.global(), snapshot, true)
  return snapshot
}

function primeSettingsQuery(queryKey: readonly string[], snapshot: LayeredSettings, invalidateAll: boolean) {
  void queryClient.cancelQueries({ queryKey, exact: true })
  if (invalidateAll) {
    void queryClient.invalidateQueries({
      queryKey: settingsQueryKeys.all,
      predicate: (query) => !sameQueryKey(query.queryKey, queryKey),
      refetchType: 'all',
    })
  }
  queryClient.setQueryData(queryKey, snapshot)
}

function sameQueryKey(left: readonly unknown[], right: readonly unknown[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}

function normalizeProjectID(projectId: string): string {
  const normalized = projectId.trim()
  if (!normalized) throw new Error('Project ID is required')
  return normalized
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

/** Loads optional protocol-native model suggestions without validating or
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

/** Validates an unsaved image profile through one minimal real Images API request. */
export function pingImageProfile(profile: ImageAPIProfileSettings, signal?: AbortSignal): Promise<ImagePingResult> {
  return requestJSON('/api/images/ping', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ profile }),
    signal,
  })
}
