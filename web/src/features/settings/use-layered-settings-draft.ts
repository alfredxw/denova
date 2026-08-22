import { useCallback, useEffect, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { saveWithRevisionRecovery } from '@/lib/revision-conflict'
import { rebaseJSONValue } from '@/lib/three-way-rebase'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { createSettingsMergePatch, fetchSettingsTarget, patchSettingsTarget, refreshSettingsTarget } from './api'
import { subscribeSettingsTarget } from './query'
import type { SettingsTarget } from './query'
import type { LayeredSettings, Settings, SettingsLayer } from './types'
import { settingsForLayer, settingsRevisionForLayer, useAutoSaveSettings } from './use-auto-save-settings'

type SaveLayerSettings = (settings: Settings, baseRevision?: string) => Promise<LayeredSettings>
type LayerValues<T> = Record<SettingsLayer, T>

type SettingsSnapshotSource =
  | { kind: 'load' }
  | { kind: 'own-save'; layer: SettingsLayer; submitted: Settings }

interface UseLayeredSettingsDraftOptions {
  target: SettingsTarget
  layer: SettingsLayer
  sourcePrefix: string
  loadSettings?: () => Promise<LayeredSettings>
  saveUserSettings?: SaveLayerSettings
  saveWorkspaceSettings?: SaveLayerSettings
}

const emptyDrafts = (): LayerValues<Settings> => ({ user: {}, workspace: {} })

/** Owns independent user/workspace drafts, rebases external updates, and serializes saves per layer. */
export function useLayeredSettingsDraft({
  target,
  layer,
  sourcePrefix,
  loadSettings,
  saveUserSettings,
  saveWorkspaceSettings,
}: UseLayeredSettingsDraftOptions) {
  const targetKind = target.kind
  const projectId = target.kind === 'project' ? target.projectId : ''
  const [layered, setLayered] = useState<LayeredSettings | null>(null)
  const [drafts, setDrafts] = useState<LayerValues<Settings>>(emptyDrafts)
  const [ready, setReady] = useState(false)
  const [syncVersions, setSyncVersions] = useState<LayerValues<number>>({ user: 0, workspace: 0 })
  const [savingLayers, setSavingLayers] = useState<LayerValues<boolean>>({ user: false, workspace: false })
  const [error, setError] = useState<string | null>(null)
  const mountedRef = useRef(true)
  const readyRef = useRef(false)
  const layeredRef = useRef<LayeredSettings | null>(null)
  const draftsRef = useRef<LayerValues<Settings>>(emptyDrafts())
  const baselinesRef = useRef<LayerValues<Settings>>(emptyDrafts())
  const loadSequenceRef = useRef(0)
  const applySequenceRef = useRef(0)
  const suppressQuerySyncRef = useRef(0)

  layeredRef.current = layered
  draftsRef.current = drafts

  const applySnapshot = useCallback(async (next: LayeredSettings, source: SettingsSnapshotSource) => {
    const applySequence = applySequenceRef.current + 1
    applySequenceRef.current = applySequence
    const nextBaselines: LayerValues<Settings> = { user: next.user, workspace: next.workspace }
    const previousSnapshot = layeredRef.current
    const capturedBaselines = baselinesRef.current
    const capturedDrafts = draftsRef.current
    let nextDrafts = nextBaselines
    if (readyRef.current) {
      const rebaseLayer = (targetLayer: SettingsLayer) => {
        if (source.kind === 'own-save' && source.layer === targetLayer) {
          return Promise.resolve(rebaseJSONValue(
            source.submitted,
            capturedDrafts[targetLayer],
            nextBaselines[targetLayer],
          ))
        }
        return rebaseJSONWithRecovery({
          resource: 'settings',
          scope: `${sourcePrefix}:${targetLayer}`,
          id: targetLayer,
          baseline: {
            revision: settingsRevisionForLayer(previousSnapshot, targetLayer),
            value: capturedBaselines[targetLayer],
          },
          local: {
            revision: settingsRevisionForLayer(previousSnapshot, targetLayer),
            value: capturedDrafts[targetLayer],
          },
          external: {
            revision: settingsRevisionForLayer(next, targetLayer),
            value: nextBaselines[targetLayer],
          },
        })
      }
      const [user, workspace] = await Promise.all([rebaseLayer('user'), rebaseLayer('workspace')])
      nextDrafts = { user, workspace }
    }

    if (!mountedRef.current || applySequence !== applySequenceRef.current) return false
    // A user edit may land while an overlapping conflict is being archived. Replay
    // that newer edit over the prepared snapshot instead of replacing it.
    if (draftsRef.current !== capturedDrafts) {
      nextDrafts = {
        user: rebaseJSONValue(capturedDrafts.user, draftsRef.current.user, nextDrafts.user),
        workspace: rebaseJSONValue(capturedDrafts.workspace, draftsRef.current.workspace, nextDrafts.workspace),
      }
    }

    readyRef.current = true
    layeredRef.current = next
    draftsRef.current = nextDrafts
    baselinesRef.current = nextBaselines
    setLayered(next)
    setDrafts(nextDrafts)
    setReady(true)
    setSyncVersions((current) => ({ user: current.user + 1, workspace: current.workspace + 1 }))
    setError(null)
    return true
  }, [sourcePrefix])

  const reload = useCallback(async (fresh = false) => {
    const sequence = loadSequenceRef.current + 1
    loadSequenceRef.current = sequence
    try {
      const settingsTarget: SettingsTarget = targetKind === 'project'
        ? { kind: 'project', projectId }
        : { kind: 'global' }
      const next = await (loadSettings ?? (fresh
        ? () => refreshSettingsTarget(settingsTarget)
        : () => fetchSettingsTarget(settingsTarget)))()
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return null
      const applied = await applySnapshot(next, { kind: 'load' })
      return applied ? next : null
    } catch (cause) {
      if (!mountedRef.current || sequence !== loadSequenceRef.current) return null
      const message = cause instanceof Error ? cause.message : String(cause)
      console.error(`[settings] failed to load layered settings for ${sourcePrefix}`, cause)
      setError(message)
      return null
    }
  }, [applySnapshot, loadSettings, projectId, sourcePrefix, targetKind])

  useEffect(() => {
    mountedRef.current = true
    void reload()
    return () => {
      mountedRef.current = false
      loadSequenceRef.current += 1
      applySequenceRef.current += 1
    }
  }, [reload])

  useEffect(() => {
    const settingsTarget: SettingsTarget = targetKind === 'project'
      ? { kind: 'project', projectId }
      : { kind: 'global' }
    return subscribeSettingsTarget(settingsTarget, (snapshot) => {
      if (suppressQuerySyncRef.current === 0) void applySnapshot(snapshot, { kind: 'load' })
    })
  }, [applySnapshot, projectId, targetKind])

  const setDraft: Dispatch<SetStateAction<Settings>> = useCallback((action) => {
    setDrafts((current) => {
      const settings = typeof action === 'function' ? action(current[layer]) : action
      const next = { ...current, [layer]: settings }
      draftsRef.current = next
      return next
    })
  }, [layer])

  const saveLayer = useCallback(async (targetLayer: SettingsLayer, settings: Settings, baseRevision?: string) => {
    // This baseline belongs to the revision used by the first write. A reload may
    // advance baselinesRef while that request is in flight, but it must not change
    // how the original draft is interpreted during a conflict retry.
    const saveBaseline = baselinesRef.current[targetLayer]
    const customUpdater = targetLayer === 'user' ? saveUserSettings : saveWorkspaceSettings
    let patchBaseline = saveBaseline
    let recoveryBaselineRevision = baseRevision
    let latestRevision: string | undefined
    return saveWithRevisionRecovery({
      baseline: saveBaseline,
      draft: settings,
      revision: baseRevision,
      save: (nextDraft, revision) => {
        suppressQuerySyncRef.current += 1
        const request = customUpdater
          ? customUpdater(nextDraft, revision)
          : patchSettingsTarget(
              targetKind === 'project' ? { kind: 'project', projectId } : { kind: 'global' },
              targetLayer,
              createSettingsMergePatch(patchBaseline, nextDraft),
              revision,
            )
        return request.finally(() => { suppressQuerySyncRef.current -= 1 })
      },
      loadLatest: async () => {
        const settingsTarget: SettingsTarget = targetKind === 'project'
          ? { kind: 'project', projectId }
          : { kind: 'global' }
        const latest = await (loadSettings ?? (() => refreshSettingsTarget(settingsTarget)))()
        latestRevision = settingsRevisionForLayer(latest, targetLayer)
        return {
          value: settingsForLayer(latest, targetLayer),
          revision: latestRevision,
        }
      },
      rebase: async (baseline, draft, latest) => {
        const rebased = await rebaseJSONWithRecovery({
          resource: 'settings',
          scope: `${sourcePrefix}:${targetLayer}`,
          id: targetLayer,
          baseline: { revision: recoveryBaselineRevision, value: baseline },
          local: { revision: recoveryBaselineRevision, value: draft },
          external: { revision: latestRevision, value: latest },
        })
        recoveryBaselineRevision = latestRevision
        patchBaseline = latest
        return rebased
      },
    })
  }, [loadSettings, projectId, saveUserSettings, saveWorkspaceSettings, sourcePrefix, targetKind])

  const saveUser = useCallback((settings: Settings, revision?: string) => saveLayer('user', settings, revision), [saveLayer])
  const saveWorkspace = useCallback((settings: Settings, revision?: string) => saveLayer('workspace', settings, revision), [saveLayer])
  const applySavedSettings = useCallback(async (
    targetLayer: SettingsLayer,
    next: LayeredSettings,
    submitted: Settings,
  ) => {
    await applySnapshot(next, { kind: 'own-save', layer: targetLayer, submitted })
  }, [applySnapshot])
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
    savedRevision: (next) => settingsRevisionForLayer(next, 'user'),
    ready,
    resetKey: 'user',
    syncKey: syncVersions.user,
    save: saveUser,
    onSavingChange: (saving) => updateSavingLayer('user', saving),
    onSaved: (next, submitted) => applySavedSettings('user', next, submitted),
    onStaleSuccess: async () => { await reload() },
    onError: (message) => handleSaveError('user', message),
  })
  const workspaceAutoSave = useAutoSaveSettings({
    draft: drafts.workspace,
    saved: layered?.workspace ?? {},
    baseRevision: layered?.revisions?.workspace,
    savedRevision: (next) => settingsRevisionForLayer(next, 'workspace'),
    ready,
    resetKey: 'workspace',
    syncKey: syncVersions.workspace,
    save: saveWorkspace,
    onSavingChange: (saving) => updateSavingLayer('workspace', saving),
    onSaved: (next, submitted) => applySavedSettings('workspace', next, submitted),
    onStaleSuccess: async () => { await reload() },
    onError: (message) => handleSaveError('workspace', message),
  })

  const saveNow = useCallback(async () => {
    setError(null)
    const flush = layer === 'user' ? userAutoSave.flush : workspaceAutoSave.flush
    try {
      return await flush()
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause)
      if (mountedRef.current) setError(message)
      throw cause
    }
  }, [layer, userAutoSave.flush, workspaceAutoSave.flush])

  const activeAutoSave = layer === 'user' ? userAutoSave : workspaceAutoSave

  return {
    layered,
    draft: drafts[layer],
    setDraft,
    saving: savingLayers.user || savingLayers.workspace,
    autosaveStatus: activeAutoSave.status,
    autosaveError: activeAutoSave.error,
    error,
    setError,
    reload,
    saveNow,
  }
}
