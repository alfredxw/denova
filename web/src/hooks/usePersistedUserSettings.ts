import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchSettings, updateUserSettings } from '@/features/settings/api'
import type { LayeredSettings, Settings } from '@/features/settings/types'
import { APIError } from '@/lib/api-client'

type PersistedStringSettingKey = {
  [K in keyof Settings]-?: NonNullable<Settings[K]> extends string ? K : never
}[keyof Settings]

export type PersistedStringSettingDefaults = Partial<Record<PersistedStringSettingKey, string>>
type PersistedStringSettingValues<TDefaults extends PersistedStringSettingDefaults> = {
  [K in keyof TDefaults]: string
}

interface PersistedUserSettingsOptions<TDefaults extends PersistedStringSettingDefaults> {
  workspace: string
  defaults: TDefaults
}

let nextPersistedSettingsSourceID = 1

/**
 * Owns a small set of user-level string settings behind one serialized mutation queue.
 * Pending optimistic values survive external reloads while each save rebases on the latest revision.
 */
export function usePersistedUserSettings<TDefaults extends PersistedStringSettingDefaults>({
  workspace,
  defaults,
}: PersistedUserSettingsOptions<TDefaults>) {
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
  const pendingValuesRef = useRef(new Map<PersistedStringSettingKey, string>())
  const loadGenerationRef = useRef(0)
  const mutationQueueRef = useRef<Promise<void>>(Promise.resolve())

  workspaceRef.current = workspace
  valuesRef.current = values

  const applyValues = useCallback((next: PersistedStringSettingValues<TDefaults>) => {
    valuesRef.current = next
    setValues(next)
  }, [])

  const applySnapshot = useCallback((snapshot: LayeredSettings) => {
    const next = {} as PersistedStringSettingValues<TDefaults>
    for (const key of settingKeys(defaults)) {
      const pending = pendingValuesRef.current.get(key)
      const effective = snapshot.effective[key]
      next[key] = (pending ?? (typeof effective === 'string' && effective ? effective : defaults[key] ?? '')) as PersistedStringSettingValues<TDefaults>[typeof key]
    }
    applyValues(next)
  }, [applyValues, defaults])

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
    const scope = workspaceRef.current
    if (!scope || pendingValuesRef.current.has(settingKey) || next === valuesRef.current[key]) return Promise.resolve(false)

    const previous = valuesRef.current[key]
    pendingValuesRef.current.set(settingKey, next)
    loadGenerationRef.current += 1
    setLoading(false)
    applyValues({ ...valuesRef.current, [key]: next })
    setSavingKeys((current) => new Set(current).add(settingKey))

    const operation = mutationQueueRef.current.then(async () => {
      let latestSnapshot: LayeredSettings | null = null
      try {
        latestSnapshot = await fetchSettings()
        const updated = await updateSettingOnLatestSnapshot(latestSnapshot, settingKey, next)
        snapshotRef.current = updated
        pendingValuesRef.current.delete(settingKey)
        if (mountedRef.current) {
          if (workspaceRef.current) applySnapshot(updated)
          else applyValues(defaultsAsValues(defaults))
        }
        window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: eventSource } }))
        return true
      } catch (error) {
        console.warn('[usePersistedUserSettings.ts] failed to save user setting', { settingKey, next, error })
        pendingValuesRef.current.delete(settingKey)
        try {
          latestSnapshot = await fetchSettings()
          snapshotRef.current = latestSnapshot
        } catch (reloadError) {
          console.warn('[usePersistedUserSettings.ts] failed to reload after save error', { settingKey, reloadError })
        }
        if (mountedRef.current) {
          if (latestSnapshot && workspaceRef.current) applySnapshot(latestSnapshot)
          else if (valuesRef.current[key] === next) applyValues({ ...valuesRef.current, [key]: previous })
        }
        return false
      } finally {
        if (mountedRef.current) {
          setSavingKeys((current) => {
            const nextKeys = new Set(current)
            nextKeys.delete(settingKey)
            return nextKeys
          })
        }
      }
    })

    mutationQueueRef.current = operation.then(() => undefined)
    return operation
  }, [applySnapshot, applyValues, defaults, eventSource])

  const isSaving = useCallback((key: keyof TDefaults) => savingKeys.has(key as PersistedStringSettingKey), [savingKeys])

  return { values, loading, isSaving, persist, reload: load }
}

function settingKeys<TDefaults extends PersistedStringSettingDefaults>(defaults: TDefaults) {
  return Object.keys(defaults) as Array<keyof TDefaults & PersistedStringSettingKey>
}

function defaultsAsValues<TDefaults extends PersistedStringSettingDefaults>(defaults: TDefaults) {
  return { ...defaults } as PersistedStringSettingValues<TDefaults>
}

async function updateSettingOnLatestSnapshot(
  snapshot: LayeredSettings,
  settingKey: PersistedStringSettingKey,
  next: string,
): Promise<LayeredSettings> {
  const update = (latest: LayeredSettings) => {
    const draft = { ...latest.user, [settingKey]: next }
    const revision = latest.revisions?.user
    return revision ? updateUserSettings(draft, revision) : updateUserSettings(draft)
  }

  try {
    return await update(snapshot)
  } catch (cause) {
    if (!(cause instanceof APIError) || (cause.status !== 409 && cause.status !== 412)) throw cause
    return update(await fetchSettings())
  }
}
