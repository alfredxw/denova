import { useCallback, useEffect, useMemo, useRef } from 'react'
import type { LayeredSettings, Settings, SettingsLayer } from './types'

const AUTO_SAVE_DELAY_MS = 1000

type SaveSettings = (settings: Settings, baseRevision?: string) => Promise<LayeredSettings>

/** One serialized autosave lane. Manual flushes share the same timer, queue, and revision. */
export function useAutoSaveSettings({
  draft,
  saved,
  baseRevision,
  ready,
  resetKey = 'default',
  syncKey = 0,
  save,
  onSavingChange,
  onSaved,
  onStaleSuccess,
  onError,
}: {
  draft: Settings
  saved: Settings
  baseRevision?: string
  ready: boolean
  resetKey?: string
  /** Increment when the caller atomically applies a fresh server snapshot and rebased draft. */
  syncKey?: string | number
  save: SaveSettings
  onSavingChange: (saving: boolean) => void
  onSaved: (next: LayeredSettings) => void
  /** Reconcile a successful write whose response was superseded by a newer server sync. */
  onStaleSuccess?: (next: LayeredSettings) => void | Promise<void>
  onError: (message: string) => void
}) {
  const draftKey = useMemo(() => stableStringifySettings(draft), [draft])
  const savedKey = useMemo(() => stableStringifySettings(saved), [saved])
  const baselineRef = useRef(savedKey)
  const initializedRef = useRef(false)
  const waitingForDraftSyncRef = useRef(false)
  const mountedRef = useRef(true)
  const readyRef = useRef(ready)
  const latestDraftRef = useRef(draft)
  const latestDraftKeyRef = useRef(draftKey)
  const baseRevisionRef = useRef(baseRevision || '')
  const blockedDraftKeyRef = useRef('')
  const timerRef = useRef<number | null>(null)
  const generationRef = useRef(0)
  const resetKeyRef = useRef(resetKey)
  const syncKeyRef = useRef(syncKey)
  const saveRef = useRef(save)
  const onSavingChangeRef = useRef(onSavingChange)
  const onSavedRef = useRef(onSaved)
  const onStaleSuccessRef = useRef(onStaleSuccess)
  const onErrorRef = useRef(onError)
  const inFlightRef = useRef<Promise<LayeredSettings | null> | null>(null)
  const pendingAfterSaveRef = useRef(false)
  const runSaveRef = useRef<(force?: boolean) => Promise<LayeredSettings | null>>(async () => null)
  const scheduleSaveRef = useRef<() => void>(() => undefined)

  readyRef.current = ready
  latestDraftRef.current = draft
  latestDraftKeyRef.current = draftKey
  baseRevisionRef.current = baseRevision || ''
  saveRef.current = save
  onSavingChangeRef.current = onSavingChange
  onSavedRef.current = onSaved
  onStaleSuccessRef.current = onStaleSuccess
  onErrorRef.current = onError
  if (draftKey !== blockedDraftKeyRef.current) blockedDraftKeyRef.current = ''

  const clearTimer = useCallback(() => {
    if (timerRef.current === null) return
    window.clearTimeout(timerRef.current)
    timerRef.current = null
  }, [])

  scheduleSaveRef.current = () => {
    clearTimer()
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null
      void runSaveRef.current(false).catch(() => undefined)
    }, AUTO_SAVE_DELAY_MS)
  }

  runSaveRef.current = async (force = false): Promise<LayeredSettings | null> => {
    clearTimer()
    if (!readyRef.current || waitingForDraftSyncRef.current) return null

    if (inFlightRef.current) {
      pendingAfterSaveRef.current = true
      try {
        await inFlightRef.current
      } catch {
        // The original caller receives the error; a forced retry may continue below.
      }
      if (force && latestDraftKeyRef.current !== baselineRef.current) {
        blockedDraftKeyRef.current = ''
        return runSaveRef.current(true)
      }
      return null
    }

    const snapshot = latestDraftRef.current
    const snapshotKey = latestDraftKeyRef.current
    if (force) blockedDraftKeyRef.current = ''
    if (snapshotKey === baselineRef.current || snapshotKey === blockedDraftKeyRef.current) return null

    const generation = generationRef.current
    const revision = baseRevisionRef.current
    onSavingChangeRef.current(true)
    const operation = (async () => {
      try {
        const next = revision ? await saveRef.current(snapshot, revision) : await saveRef.current(snapshot)
        baselineRef.current = snapshotKey
        blockedDraftKeyRef.current = ''
        if (!mountedRef.current) return next
        if (generation !== generationRef.current) {
          await onStaleSuccessRef.current?.(next)
          return next
        }
        onSavedRef.current(next)
        return next
      } catch (error) {
        if (generation === generationRef.current) {
          blockedDraftKeyRef.current = snapshotKey
          onErrorRef.current(error instanceof Error ? error.message : String(error))
        }
        throw error
      } finally {
        inFlightRef.current = null
        if (mountedRef.current) {
          onSavingChangeRef.current(false)
          const shouldSchedule = readyRef.current
            && !waitingForDraftSyncRef.current
            && latestDraftKeyRef.current !== baselineRef.current
            && latestDraftKeyRef.current !== blockedDraftKeyRef.current
          if (pendingAfterSaveRef.current || shouldSchedule) {
            pendingAfterSaveRef.current = false
            if (shouldSchedule) scheduleSaveRef.current()
          }
        }
      }
    })()
    inFlightRef.current = operation
    return operation
  }

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      generationRef.current += 1
      pendingAfterSaveRef.current = false
      clearTimer()
    }
  }, [clearTimer])

  useEffect(() => {
    const resetChanged = resetKeyRef.current !== resetKey
    const syncChanged = syncKeyRef.current !== syncKey
    if (!resetChanged && !syncChanged) return
    resetKeyRef.current = resetKey
    syncKeyRef.current = syncKey
    generationRef.current += 1
    clearTimer()
    baselineRef.current = savedKey
    initializedRef.current = true
    waitingForDraftSyncRef.current = resetChanged && !syncChanged
    pendingAfterSaveRef.current = false
    blockedDraftKeyRef.current = ''
  }, [clearTimer, resetKey, savedKey, syncKey])

  useEffect(() => {
    if (!ready) return
    if (!initializedRef.current) {
      baselineRef.current = savedKey
      waitingForDraftSyncRef.current = draftKey !== savedKey
      initializedRef.current = true
      return
    }
    if (latestDraftKeyRef.current === baselineRef.current) baselineRef.current = savedKey
  }, [draftKey, ready, savedKey])

  useEffect(() => {
    if (!ready) return
    if (waitingForDraftSyncRef.current) {
      if (draftKey === baselineRef.current) waitingForDraftSyncRef.current = false
      return
    }
    if (draftKey === baselineRef.current || draftKey === blockedDraftKeyRef.current) {
      clearTimer()
      return
    }
    if (inFlightRef.current) {
      pendingAfterSaveRef.current = true
      return
    }
    scheduleSaveRef.current()
  }, [clearTimer, draftKey, ready, syncKey])

  const flush = useCallback(() => {
    clearTimer()
    return runSaveRef.current(true)
  }, [clearTimer])

  return { flush }
}

export function settingsForLayer(layered: LayeredSettings, layer: SettingsLayer): Settings {
  return layer === 'user' ? layered.user : layered.workspace
}

export function settingsRevisionForLayer(layered: LayeredSettings | null, layer: SettingsLayer): string | undefined {
  return layer === 'user' ? layered?.revisions?.user : layered?.revisions?.workspace
}

export function stableStringifySettings(settings: Settings): string {
  return JSON.stringify(sortForStableStringify(settings))
}

function sortForStableStringify(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortForStableStringify)
  if (!value || typeof value !== 'object') return value
  return Object.keys(value as Record<string, unknown>).sort().reduce<Record<string, unknown>>((acc, key) => {
    acc[key] = sortForStableStringify((value as Record<string, unknown>)[key])
    return acc
  }, {})
}
