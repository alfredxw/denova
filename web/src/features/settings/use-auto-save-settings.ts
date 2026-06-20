import { useEffect, useMemo, useRef } from 'react'
import type { LayeredSettings, Settings, SettingsLayer } from './types'

const AUTO_SAVE_DELAY_MS = 1000

type SaveSettings = (settings: Settings) => Promise<LayeredSettings>

export function useAutoSaveSettings({
  draft,
  saved,
  ready,
  save,
  onSavingChange,
  onSaved,
  onError,
}: {
  draft: Settings
  saved: Settings
  ready: boolean
  save: SaveSettings
  onSavingChange: (saving: boolean) => void
  onSaved: (next: LayeredSettings) => void
  onError: (message: string) => void
}) {
  const baselineRef = useRef('')
  const waitingForDraftSyncRef = useRef(false)
  const initializedRef = useRef(false)
  const mountedRef = useRef(true)
  const saveInFlightRef = useRef(false)
  const pendingAfterSaveRef = useRef(false)
  const timerRef = useRef<number | null>(null)
  const latestDraftRef = useRef(draft)
  const latestDraftKeyRef = useRef('')
  const saveRef = useRef(save)
  const onSavingChangeRef = useRef(onSavingChange)
  const onSavedRef = useRef(onSaved)
  const onErrorRef = useRef(onError)
  const runSaveRef = useRef<() => Promise<void>>(async () => undefined)
  const scheduleSaveRef = useRef<() => void>(() => undefined)
  const draftKey = useMemo(() => stableStringifySettings(draft), [draft])
  const savedKey = useMemo(() => stableStringifySettings(saved), [saved])

  useEffect(() => {
    latestDraftRef.current = draft
    latestDraftKeyRef.current = draftKey
  }, [draft, draftKey])

  useEffect(() => { saveRef.current = save }, [save])
  useEffect(() => { onSavingChangeRef.current = onSavingChange }, [onSavingChange])
  useEffect(() => { onSavedRef.current = onSaved }, [onSaved])
  useEffect(() => { onErrorRef.current = onError }, [onError])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    const clearTimer = () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current)
        timerRef.current = null
      }
    }

    scheduleSaveRef.current = () => {
      clearTimer()
      timerRef.current = window.setTimeout(() => {
        timerRef.current = null
        void runSaveRef.current()
      }, AUTO_SAVE_DELAY_MS)
    }

    runSaveRef.current = async () => {
      if (!ready) return
      const snapshot = latestDraftRef.current
      const snapshotKey = latestDraftKeyRef.current
      if (snapshotKey === baselineRef.current) return
      if (saveInFlightRef.current) {
        pendingAfterSaveRef.current = true
        return
      }

      saveInFlightRef.current = true
      onSavingChangeRef.current(true)
      try {
        const next = await saveRef.current(snapshot)
        baselineRef.current = snapshotKey
        onSavedRef.current(next)
      } catch (error) {
        onErrorRef.current((error as Error).message)
      } finally {
        if (!mountedRef.current) return
        saveInFlightRef.current = false
        onSavingChangeRef.current(false)
        if (pendingAfterSaveRef.current || latestDraftKeyRef.current !== baselineRef.current) {
          pendingAfterSaveRef.current = false
          scheduleSaveRef.current()
        }
      }
    }
  }, [ready])

  useEffect(() => {
    if (!ready) return
    if (!initializedRef.current) {
      baselineRef.current = savedKey
      waitingForDraftSyncRef.current = true
      initializedRef.current = true
      return
    }
    if (latestDraftKeyRef.current === baselineRef.current) {
      baselineRef.current = savedKey
    }
  }, [ready, savedKey])

  useEffect(() => {
    if (!ready) return
    if (waitingForDraftSyncRef.current) {
      if (draftKey === baselineRef.current) {
        waitingForDraftSyncRef.current = false
      }
      return
    }
    if (draftKey === baselineRef.current) return
    if (saveInFlightRef.current) {
      pendingAfterSaveRef.current = true
      return
    }
    scheduleSaveRef.current()
  }, [draftKey, ready])
}

export function settingsForLayer(layered: LayeredSettings, layer: SettingsLayer): Settings {
  return layer === 'user' ? layered.user : layered.workspace
}

function stableStringifySettings(settings: Settings): string {
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
