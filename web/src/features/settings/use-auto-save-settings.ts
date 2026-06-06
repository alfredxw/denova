import { useEffect, useMemo, useRef } from 'react'
import type { LayeredSettings, Settings, SettingsLayer } from './types'

const AUTO_SAVE_DELAY_MS = 600

type SaveSettings = (settings: Settings) => Promise<LayeredSettings>

export function useAutoSaveSettings({
  draft,
  ready,
  resetKey,
  save,
  onSavingChange,
  onSaved,
  onError,
}: {
  draft: Settings
  ready: boolean
  resetKey: string
  save: SaveSettings
  onSavingChange: (saving: boolean) => void
  onSaved: (next: LayeredSettings) => void
  onError: (message: string) => void
}) {
  const baselineRef = useRef('')
  const saveRef = useRef(save)
  const onSavingChangeRef = useRef(onSavingChange)
  const onSavedRef = useRef(onSaved)
  const onErrorRef = useRef(onError)
  const draftKey = useMemo(() => stableStringifySettings(draft), [draft])

  useEffect(() => { saveRef.current = save }, [save])
  useEffect(() => { onSavingChangeRef.current = onSavingChange }, [onSavingChange])
  useEffect(() => { onSavedRef.current = onSaved }, [onSaved])
  useEffect(() => { onErrorRef.current = onError }, [onError])

  useEffect(() => {
    if (ready) baselineRef.current = draftKey
  }, [ready, resetKey])

  useEffect(() => {
    if (!ready || draftKey === baselineRef.current) return
    const snapshot = draft
    const snapshotKey = draftKey
    const timer = window.setTimeout(() => {
      onSavingChangeRef.current(true)
      saveRef.current(snapshot)
        .then((next) => {
          baselineRef.current = snapshotKey
          onSavedRef.current(next)
        })
        .catch((error) => onErrorRef.current((error as Error).message))
        .finally(() => onSavingChangeRef.current(false))
    }, AUTO_SAVE_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [draft, draftKey, ready])
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
