import { useCallback, useEffect, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { APIError } from '@/lib/api-client'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from './api'
import type { LayeredSettings, Settings, SettingsLayer } from './types'
import { settingsForLayer, settingsRevisionForLayer, useAutoSaveSettings } from './use-auto-save-settings'

type SaveLayerSettings = (settings: Settings, baseRevision?: string) => Promise<LayeredSettings>
type LayerValues<T> = Record<SettingsLayer, T>

interface UseLayeredSettingsDraftOptions {
  layer: SettingsLayer
  sourcePrefix: string
  loadSettings?: () => Promise<LayeredSettings>
  saveUserSettings?: SaveLayerSettings
  saveWorkspaceSettings?: SaveLayerSettings
}

let nextSettingsDraftSourceID = 1
const emptyDrafts = (): LayerValues<Settings> => ({ user: {}, workspace: {} })

/** Owns independent user/workspace drafts, rebases external updates, and serializes saves per layer. */
export function useLayeredSettingsDraft({
  layer,
  sourcePrefix,
  loadSettings,
  saveUserSettings,
  saveWorkspaceSettings,
}: UseLayeredSettingsDraftOptions) {
  const [layered, setLayered] = useState<LayeredSettings | null>(null)
  const [drafts, setDrafts] = useState<LayerValues<Settings>>(emptyDrafts)
  const [ready, setReady] = useState(false)
  const [syncVersions, setSyncVersions] = useState<LayerValues<number>>({ user: 0, workspace: 0 })
  const [savingLayers, setSavingLayers] = useState<LayerValues<boolean>>({ user: false, workspace: false })
  const [error, setError] = useState<string | null>(null)
  const [eventSource] = useState(() => {
    const source = `${sourcePrefix}-${nextSettingsDraftSourceID}`
    nextSettingsDraftSourceID += 1
    return source
  })
  const mountedRef = useRef(true)
  const readyRef = useRef(false)
  const layeredRef = useRef<LayeredSettings | null>(null)
  const draftsRef = useRef<LayerValues<Settings>>(emptyDrafts())
  const baselinesRef = useRef<LayerValues<Settings>>(emptyDrafts())
  const loadSequenceRef = useRef(0)

  layeredRef.current = layered
  draftsRef.current = drafts

  const notifyUpdated = useCallback(() => {
    if (typeof window === 'undefined') return
    window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: eventSource } }))
  }, [eventSource])

  const applySnapshot = useCallback((next: LayeredSettings, shouldNotify = false) => {
    const nextBaselines: LayerValues<Settings> = { user: next.user, workspace: next.workspace }
    const nextDrafts = readyRef.current
      ? {
          user: rebaseSettingsDraft(baselinesRef.current.user, draftsRef.current.user, next.user),
          workspace: rebaseSettingsDraft(baselinesRef.current.workspace, draftsRef.current.workspace, next.workspace),
        }
      : nextBaselines

    readyRef.current = true
    layeredRef.current = next
    draftsRef.current = nextDrafts
    baselinesRef.current = nextBaselines
    setLayered(next)
    setDrafts(nextDrafts)
    setReady(true)
    setSyncVersions((current) => ({ user: current.user + 1, workspace: current.workspace + 1 }))
    setError(null)
    if (shouldNotify) notifyUpdated()
  }, [notifyUpdated])

  const reload = useCallback(async () => {
    const sequence = loadSequenceRef.current + 1
    loadSequenceRef.current = sequence
    try {
      const next = await (loadSettings ?? fetchSettings)()
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return null
      applySnapshot(next)
      return next
    } catch (cause) {
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return null
      const message = cause instanceof Error ? cause.message : String(cause)
      console.error(`[settings] failed to load layered settings for ${sourcePrefix}`, cause)
      setError(message)
      return null
    }
  }, [applySnapshot, loadSettings, sourcePrefix])

  useEffect(() => {
    mountedRef.current = true
    void reload()
    return () => {
      mountedRef.current = false
      loadSequenceRef.current += 1
    }
  }, [reload])

  useEffect(() => {
    const onSettingsUpdated = (event: Event) => {
      const source = (event as CustomEvent<{ source?: string }>).detail?.source
      if (source === eventSource) return
      void reload()
    }
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [eventSource, reload])

  const setDraft: Dispatch<SetStateAction<Settings>> = useCallback((action) => {
    setDrafts((current) => {
      const settings = typeof action === 'function' ? action(current[layer]) : action
      const next = { ...current, [layer]: settings }
      draftsRef.current = next
      return next
    })
  }, [layer])

  const saveLayer = useCallback(async (targetLayer: SettingsLayer, settings: Settings, baseRevision?: string) => {
    const updater = targetLayer === 'user'
      ? (saveUserSettings ?? updateUserSettings)
      : (saveWorkspaceSettings ?? updateWorkspaceSettings)
    // This baseline belongs to the revision used by the first write. A reload may
    // advance baselinesRef while that request is in flight, but it must not change
    // how the original draft is interpreted during a conflict retry.
    const saveBaseline = baselinesRef.current[targetLayer]
    try {
      return baseRevision ? await updater(settings, baseRevision) : await updater(settings)
    } catch (cause) {
      if (!(cause instanceof APIError) || (cause.status !== 409 && cause.status !== 412)) throw cause

      const latest = await (loadSettings ?? fetchSettings)()
      const latestLayer = settingsForLayer(latest, targetLayer)
      const rebased = rebaseSettingsDraft(saveBaseline, settings, latestLayer)
      const revision = settingsRevisionForLayer(latest, targetLayer)
      return revision ? updater(rebased, revision) : updater(rebased)
    }
  }, [loadSettings, saveUserSettings, saveWorkspaceSettings])

  const saveUser = useCallback((settings: Settings, revision?: string) => saveLayer('user', settings, revision), [saveLayer])
  const saveWorkspace = useCallback((settings: Settings, revision?: string) => saveLayer('workspace', settings, revision), [saveLayer])
  const applySavedSettings = useCallback((next: LayeredSettings) => applySnapshot(next, true), [applySnapshot])
  const updateSavingLayer = useCallback((targetLayer: SettingsLayer, saving: boolean) => {
    if (!mountedRef.current) return
    setSavingLayers((current) => current[targetLayer] === saving ? current : { ...current, [targetLayer]: saving })
  }, [])
  const handleSaveError = useCallback((targetLayer: SettingsLayer, message: string) => {
    console.error(`[settings] failed to save ${targetLayer} settings for ${sourcePrefix}: ${message}`)
    setError(message)
  }, [sourcePrefix])

  const userAutoSave = useAutoSaveSettings({
    draft: drafts.user,
    saved: layered?.user ?? {},
    baseRevision: layered?.revisions?.user,
    ready,
    resetKey: 'user',
    syncKey: syncVersions.user,
    save: saveUser,
    onSavingChange: (saving) => updateSavingLayer('user', saving),
    onSaved: applySavedSettings,
    onStaleSuccess: async () => { await reload() },
    onError: (message) => handleSaveError('user', message),
  })
  const workspaceAutoSave = useAutoSaveSettings({
    draft: drafts.workspace,
    saved: layered?.workspace ?? {},
    baseRevision: layered?.revisions?.workspace,
    ready,
    resetKey: 'workspace',
    syncKey: syncVersions.workspace,
    save: saveWorkspace,
    onSavingChange: (saving) => updateSavingLayer('workspace', saving),
    onSaved: applySavedSettings,
    onStaleSuccess: async () => { await reload() },
    onError: (message) => handleSaveError('workspace', message),
  })

  const saveNow = useCallback(async () => {
    setError(null)
    const controller = layer === 'user' ? userAutoSave : workspaceAutoSave
    try {
      return await controller.flush()
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause)
      if (mountedRef.current) setError(message)
      throw cause
    }
  }, [layer, userAutoSave, workspaceAutoSave])

  return {
    layered,
    draft: drafts[layer],
    setDraft,
    saving: savingLayers.user || savingLayers.workspace,
    error,
    setError,
    reload,
    notifyUpdated,
    saveNow,
  }
}

function rebaseSettingsDraft(previousSaved: Settings, currentDraft: Settings, nextSaved: Settings): Settings {
  return rebaseJSONRecord(
    previousSaved as Record<string, unknown>,
    currentDraft as Record<string, unknown>,
    nextSaved as Record<string, unknown>,
  ) as Settings
}

function rebaseJSONRecord(
  previous: Record<string, unknown>,
  current: Record<string, unknown>,
  next: Record<string, unknown>,
): Record<string, unknown> {
  const rebased: Record<string, unknown> = { ...next }
  const keys = new Set([...Object.keys(previous), ...Object.keys(current)])
  for (const key of keys) {
    const previousHasKey = Object.prototype.hasOwnProperty.call(previous, key)
    const currentHasKey = Object.prototype.hasOwnProperty.call(current, key)
    const previousValue = previous[key]
    const currentValue = current[key]
    if (previousHasKey === currentHasKey && jsonValueEqual(previousValue, currentValue)) continue
    if (!currentHasKey) {
      delete rebased[key]
      continue
    }
    const nextValue = next[key]
    rebased[key] = isJSONRecord(previousValue) && isJSONRecord(currentValue) && isJSONRecord(nextValue)
      ? rebaseJSONRecord(previousValue, currentValue, nextValue)
      : currentValue
  }
  return rebased
}

function isJSONRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value))
}

function jsonValueEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true
  if (Array.isArray(left) || Array.isArray(right)) {
    return Array.isArray(left)
      && Array.isArray(right)
      && left.length === right.length
      && left.every((value, index) => jsonValueEqual(value, right[index]))
  }
  if (!isJSONRecord(left) || !isJSONRecord(right)) return false
  const leftKeys = Object.keys(left)
  const rightKeys = Object.keys(right)
  return leftKeys.length === rightKeys.length
    && leftKeys.every((key) => Object.prototype.hasOwnProperty.call(right, key) && jsonValueEqual(left[key], right[key]))
}
