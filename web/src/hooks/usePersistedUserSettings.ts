import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchSettings, updateUserSettings } from '@/features/settings/api'
import type { LayeredSettings, Settings } from '@/features/settings/types'
import { useSaveLane } from '@/hooks/use-save-lane'
import { saveWithRevisionRecovery } from '@/lib/revision-conflict'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'

const PERSISTED_USER_SETTINGS_DELAY_MS = 1000

type PersistedStringSettingKey = {
  [K in keyof Settings]-?: NonNullable<Settings[K]> extends string ? K : never
}[keyof Settings]

export type PersistedStringSettingDefaults = Partial<Record<PersistedStringSettingKey, string>>
export type PersistedStringSettingValues<TDefaults extends PersistedStringSettingDefaults> = {
  [K in keyof TDefaults]: string
}

interface PersistedUserSettingsOptions<TDefaults extends PersistedStringSettingDefaults> {
  workspace: string
  defaults: TDefaults
}

export interface PersistedUserSettingsController<TDefaults extends PersistedStringSettingDefaults> {
  values: PersistedStringSettingValues<TDefaults>
  loading: boolean
  isSaving: (key: keyof TDefaults) => boolean
  persist: <TKey extends keyof TDefaults>(key: TKey, next: string) => Promise<boolean>
  reload: () => Promise<LayeredSettings | null>
  /** Waits for the owned save lane; false means the local value is still queued for retry. */
  flushPending: () => Promise<boolean>
}

interface PendingSettingChange {
  mutationID: number
  scope: string
  value: string
  baselineUserValue?: string
  baselineUserPresent: boolean
  baseRevision?: string
}

interface PersistedSettingsSaveRequest {
  scope: string
  changes: Partial<Record<PersistedStringSettingKey, string>>
  baselineUserValues: Partial<Record<PersistedStringSettingKey, string>>
  baselineUserPresent: Partial<Record<PersistedStringSettingKey, boolean>>
  baseRevision?: string
  maxMutationID: number
}

interface PersistedSettingsSaveResult {
  snapshot: LayeredSettings
}

let nextPersistedSettingsSourceID = 1

/**
 * Owns optimistic user-level string preferences over the shared after-delay save lane.
 * Each write reloads the latest settings first, while queued edits collapse into one patch.
 */
export function usePersistedUserSettings<TDefaults extends PersistedStringSettingDefaults>({
  workspace,
  defaults,
}: PersistedUserSettingsOptions<TDefaults>): PersistedUserSettingsController<TDefaults> {
  const [values, setValues] = useState<PersistedStringSettingValues<TDefaults>>(() => defaultsAsValues(defaults))
  const [savingKeys, setSavingKeys] = useState<ReadonlySet<PersistedStringSettingKey>>(() => new Set())
  const [loading, setLoading] = useState(Boolean(workspace))
  const [eventSource] = useState(() => {
    const source = `persisted-user-settings-${nextPersistedSettingsSourceID}`
    nextPersistedSettingsSourceID += 1
    return source
  })
  const mountedRef = useRef(true)
  const workspaceRef = useRef(workspace)
  const valuesRef = useRef(values)
  const snapshotRef = useRef<LayeredSettings | null>(null)
  const pendingChangesRef = useRef(new Map<PersistedStringSettingKey, PendingSettingChange>())
  const mutationWaitersRef = useRef(new Map<number, (saved: boolean) => void>())
  const nextMutationIDRef = useRef(1)
  const loadGenerationRef = useRef(0)
  const reloadLaneRef = useRef<(value: PersistedSettingsSaveRequest) => boolean>(() => false)

  workspaceRef.current = workspace
  valuesRef.current = values

  const applyValues = useCallback((next: PersistedStringSettingValues<TDefaults>) => {
    valuesRef.current = next
    setValues(next)
  }, [])

  const applySnapshot = useCallback((snapshot: LayeredSettings) => {
    const next = {} as PersistedStringSettingValues<TDefaults>
    for (const key of settingKeys(defaults)) {
      const pending = pendingChangesRef.current.get(key)?.value
      const effective = snapshot.effective[key]
      next[key] = (pending ?? (typeof effective === 'string' && effective ? effective : defaults[key] ?? '')) as PersistedStringSettingValues<TDefaults>[typeof key]
    }
    applyValues(next)
  }, [applyValues, defaults])

  const settleMutationsThrough = useCallback((maxMutationID: number, saved: boolean) => {
    for (const [mutationID, resolve] of mutationWaitersRef.current) {
      if (mutationID > maxMutationID) continue
      mutationWaitersRef.current.delete(mutationID)
      resolve(saved)
    }
  }, [])

  const lane = useSaveLane<PersistedSettingsSaveRequest, PersistedSettingsSaveResult>({
    // User settings are global; keep this owner lane stable while workspace props change.
    scopeKey: `persisted-user-settings:${eventSource}`,
    delayMs: PERSISTED_USER_SETTINGS_DELAY_MS,
    save: async ({ value: request }) => {
      const latestSnapshot = await fetchSettings()
      const updated = await updateSettingsOnLatestSnapshot(
        latestSnapshot,
        request,
        request.scope,
      )
      return { snapshot: updated }
    },
    onSaved: ({ value: request }, result) => {
      if (!mountedRef.current) return

      for (const [key, submittedValue] of settingEntries(request.changes)) {
        const pending = pendingChangesRef.current.get(key)
        if (!pending || pending.mutationID > request.maxMutationID || pending.value !== submittedValue) continue
        pendingChangesRef.current.delete(key)
      }
      for (const [key, pending] of pendingChangesRef.current) {
        if (!Object.prototype.hasOwnProperty.call(request.changes, key)) continue
        const savedUser = result.snapshot.user
        const savedValuePresent = Object.prototype.hasOwnProperty.call(savedUser, key)
        pending.baselineUserPresent = savedValuePresent
        pending.baselineUserValue = savedValuePresent ? savedUser[key] as string | undefined : undefined
        pending.baseRevision = result.snapshot.revisions?.user
      }
      settleMutationsThrough(request.maxMutationID, true)

      snapshotRef.current = result.snapshot
      if (workspaceRef.current) {
        applySnapshot(result.snapshot)
      }

      setSavingKeys(new Set(pendingChangesRef.current.keys()))
      const remaining = saveRequestFor(pendingChangesRef.current)
      if (remaining) reloadLaneRef.current(remaining)
      window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: eventSource } }))
    },
    onError: ({ value: request }, error) => {
      console.warn('[usePersistedUserSettings.ts] failed to save user settings; local preference remains queued', {
        settingKeys: Object.keys(request.changes),
        error,
      })
    },
  })
  reloadLaneRef.current = lane.reload

  const load = useCallback(async () => {
    const generation = loadGenerationRef.current + 1
    loadGenerationRef.current = generation
    if (!workspace) {
      snapshotRef.current = null
      applyValues(defaultsAsValues(defaults))
      setLoading(false)
      return null
    }

    setLoading(true)
    try {
      const snapshot = await fetchSettings()
      if (!mountedRef.current || generation !== loadGenerationRef.current || workspaceRef.current !== workspace) return null
      snapshotRef.current = snapshot
      applySnapshot(snapshot)
      return snapshot
    } catch (error) {
      if (!mountedRef.current || generation !== loadGenerationRef.current || workspaceRef.current !== workspace) return null
      console.warn('[usePersistedUserSettings.ts] failed to load user settings', { error })
      if (!snapshotRef.current) applyValues(defaultsAsValues(defaults))
      return null
    } finally {
      if (mountedRef.current && generation === loadGenerationRef.current && workspaceRef.current === workspace) setLoading(false)
    }
  }, [applySnapshot, applyValues, defaults, workspace])

  useEffect(() => {
    mountedRef.current = true
    void load()
    return () => {
      mountedRef.current = false
      loadGenerationRef.current += 1
    }
  }, [load])

  useEffect(() => () => {
    for (const resolve of mutationWaitersRef.current.values()) resolve(false)
    mutationWaitersRef.current.clear()
  }, [])

  useEffect(() => {
    const handleSettingsUpdated = (event: Event) => {
      const source = (event as CustomEvent<{ source?: string }>).detail?.source
      if (source !== eventSource) void load()
    }
    window.addEventListener('nova:settings-updated', handleSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', handleSettingsUpdated)
  }, [eventSource, load])

  const persist = useCallback(<TKey extends keyof TDefaults>(key: TKey, next: string): Promise<boolean> => {
    const settingKey = key as PersistedStringSettingKey
    if (!workspaceRef.current || next === valuesRef.current[key]) return Promise.resolve(false)

    const existing = pendingChangesRef.current.get(settingKey)
    const baselineUser = snapshotRef.current?.user
    const mutationID = nextMutationIDRef.current
    nextMutationIDRef.current += 1
    pendingChangesRef.current.set(settingKey, {
      mutationID,
      scope: `${workspaceRef.current || 'global'}:user`,
      value: next,
      baselineUserValue: existing?.baselineUserValue ?? (
        baselineUser && Object.prototype.hasOwnProperty.call(baselineUser, settingKey)
          ? baselineUser[settingKey] as string | undefined
          : undefined
      ),
      baselineUserPresent: existing?.baselineUserPresent ?? Boolean(
        baselineUser && Object.prototype.hasOwnProperty.call(baselineUser, settingKey)
      ),
      baseRevision: existing?.baseRevision ?? snapshotRef.current?.revisions?.user,
    })
    loadGenerationRef.current += 1
    setLoading(false)
    applyValues({ ...valuesRef.current, [key]: next })
    setSavingKeys(new Set(pendingChangesRef.current.keys()))

    const operation = new Promise<boolean>(resolve => mutationWaitersRef.current.set(mutationID, resolve))
    const request = saveRequestFor(pendingChangesRef.current)
    if (request) lane.edit(request)
    return operation
  }, [applyValues, lane])

  const isSaving = useCallback(
    (key: keyof TDefaults) => savingKeys.has(key as PersistedStringSettingKey),
    [savingKeys],
  )

  const flushPending = useCallback(async (): Promise<boolean> => {
    if (!lane.hasWork()) return true
    await lane.flush()
    return lane.getSnapshot().status !== 'error'
  }, [lane])

  return { values, loading, isSaving, persist, reload: load, flushPending }
}

function settingKeys<TDefaults extends PersistedStringSettingDefaults>(defaults: TDefaults) {
  return Object.keys(defaults) as Array<keyof TDefaults & PersistedStringSettingKey>
}

function settingEntries(values: Partial<Record<PersistedStringSettingKey, string>>) {
  return Object.entries(values) as Array<[PersistedStringSettingKey, string]>
}

function defaultsAsValues<TDefaults extends PersistedStringSettingDefaults>(defaults: TDefaults) {
  return { ...defaults } as PersistedStringSettingValues<TDefaults>
}

function saveRequestFor(
  pendingChanges: ReadonlyMap<PersistedStringSettingKey, PendingSettingChange>,
): PersistedSettingsSaveRequest | null {
  if (pendingChanges.size === 0) return null
  const changes: Partial<Record<PersistedStringSettingKey, string>> = {}
  const baselineUserValues: Partial<Record<PersistedStringSettingKey, string>> = {}
  const baselineUserPresent: Partial<Record<PersistedStringSettingKey, boolean>> = {}
  let baseRevision: string | undefined
  let scope = 'global:user'
  let maxMutationID = 0
  for (const [key, pending] of pendingChanges) {
    if (maxMutationID === 0) scope = pending.scope
    changes[key] = pending.value
    baselineUserPresent[key] = pending.baselineUserPresent
    if (pending.baselineUserValue !== undefined) baselineUserValues[key] = pending.baselineUserValue
    baseRevision ??= pending.baseRevision
    maxMutationID = Math.max(maxMutationID, pending.mutationID)
  }
  return { scope, changes, baselineUserValues, baselineUserPresent, baseRevision, maxMutationID }
}

async function updateSettingsOnLatestSnapshot(
  snapshot: LayeredSettings,
  request: PersistedSettingsSaveRequest,
  scope: string,
): Promise<LayeredSettings> {
  const baseline = { ...snapshot.user }
  for (const [key] of settingEntries(request.changes)) {
    if (request.baselineUserPresent[key]) baseline[key] = request.baselineUserValues[key]
    else delete baseline[key]
  }
  const local = { ...baseline, ...request.changes }
  const rebased = await rebaseJSONWithRecovery({
    resource: 'settings',
    scope,
    id: 'user',
    baseline: { revision: request.baseRevision, value: baseline },
    local: { revision: request.baseRevision, value: local },
    external: { revision: snapshot.revisions?.user, value: snapshot.user },
  })
  let recoveryBaselineRevision = snapshot.revisions?.user
  let latestRevision: string | undefined
  return saveWithRevisionRecovery({
    baseline: snapshot.user,
    draft: rebased,
    revision: snapshot.revisions?.user,
    save: (draft, revision) => revision ? updateUserSettings(draft, revision) : updateUserSettings(draft),
    loadLatest: async () => {
      const latest = await fetchSettings()
      latestRevision = latest.revisions?.user
      return { value: latest.user, revision: latestRevision }
    },
    rebase: async (previous, draft, external) => {
      const merged = await rebaseJSONWithRecovery({
        resource: 'settings',
        scope,
        id: 'user',
        baseline: { revision: recoveryBaselineRevision, value: previous },
        local: { revision: recoveryBaselineRevision, value: draft },
        external: { revision: latestRevision, value: external },
      })
      recoveryBaselineRevision = latestRevision
      return merged
    },
  })
}
