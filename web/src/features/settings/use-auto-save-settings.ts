import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useSaveLane } from '@/hooks/use-save-lane'
import type { LayeredSettings, Settings, SettingsLayer } from './types'

const AUTO_SAVE_DELAY_MS = 1000

type SaveSettings = (settings: Settings, baseRevision?: string) => Promise<LayeredSettings>

interface SettingsSaveRequest {
  draft: Settings
  key: string
  generation: number
}

/** Settings adapter over the shared after-delay, latest-only save lane. */
export function useAutoSaveSettings({
  draft,
  saved,
  baseRevision,
  savedRevision,
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
  /** Reads the next lane revision from a successful response before queued writes continue. */
  savedRevision?: (next: LayeredSettings) => string | undefined
  ready: boolean
  resetKey?: string
  /** Increment when the caller atomically applies a fresh server snapshot and rebased draft. */
  syncKey?: string | number
  save: SaveSettings
  onSavingChange: (saving: boolean) => void
  onSaved: (next: LayeredSettings, submitted: Settings) => void | Promise<void>
  /** Reconcile a successful write whose response was superseded by a newer server sync. */
  onStaleSuccess?: (next: LayeredSettings) => void | Promise<void>
  onError: (message: string) => void
}) {
  const draftKey = useMemo(() => stableStringifySettings(draft), [draft])
  const savedKey = useMemo(() => stableStringifySettings(saved), [saved])
  const laneScopeKey = `${resetKey}\u0000${String(syncKey)}`
  const baselineRef = useRef(savedKey)
  const initializedRef = useRef(false)
  const mountedRef = useRef(true)
  const waitingForDraftSyncRef = useRef(false)
  const latestDraftRef = useRef(draft)
  const latestDraftKeyRef = useRef(draftKey)
  const readyRef = useRef(ready)
  const baseRevisionRef = useRef(baseRevision || '')
  const blockedDraftKeyRef = useRef('')
  const observedDraftKeyRef = useRef('')
  const generationRef = useRef(0)
  const resetKeyRef = useRef(resetKey)
  const syncKeyRef = useRef(syncKey)
  const saveRef = useRef(save)
  const savedRevisionRef = useRef(savedRevision)
  const onSavingChangeRef = useRef(onSavingChange)
  const onSavedRef = useRef(onSaved)
  const onStaleSuccessRef = useRef(onStaleSuccess)
  const onErrorRef = useRef(onError)

  latestDraftRef.current = draft
  latestDraftKeyRef.current = draftKey
  readyRef.current = ready
  saveRef.current = save
  savedRevisionRef.current = savedRevision
  onSavingChangeRef.current = onSavingChange
  onSavedRef.current = onSaved
  onStaleSuccessRef.current = onStaleSuccess
  onErrorRef.current = onError
  if (draftKey !== blockedDraftKeyRef.current) blockedDraftKeyRef.current = ''

  const lane = useSaveLane<SettingsSaveRequest, LayeredSettings>({
    scopeKey: laneScopeKey,
    delayMs: AUTO_SAVE_DELAY_MS,
    save: async ({ value: request }) => {
      const revision = baseRevisionRef.current
      if (mountedRef.current) onSavingChangeRef.current(true)
      try {
        const next = revision
          ? await saveRef.current(request.draft, revision)
          : await saveRef.current(request.draft)
        if (!mountedRef.current) return next
        if (request.generation !== generationRef.current) {
          await onStaleSuccessRef.current?.(next)
          return next
        }
        baselineRef.current = request.key
        baseRevisionRef.current = savedRevisionRef.current?.(next) || baseRevisionRef.current
        blockedDraftKeyRef.current = ''
        await onSavedRef.current(next, request.draft)
        return next
      } catch (error) {
        if (mountedRef.current && request.generation === generationRef.current) {
          blockedDraftKeyRef.current = request.key
          const message = error instanceof Error ? error.message : String(error)
          onErrorRef.current(message)
        }
        throw error
      } finally {
        if (mountedRef.current) onSavingChangeRef.current(false)
      }
    },
  })
  const { cancel, edit, flush: flushLane, getSnapshot, hasWork, reset } = lane

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      generationRef.current += 1
    }
  }, [])

  useEffect(() => {
    const resetChanged = resetKeyRef.current !== resetKey
    const syncChanged = syncKeyRef.current !== syncKey
    if (!resetChanged && !syncChanged) return
    resetKeyRef.current = resetKey
    syncKeyRef.current = syncKey
    generationRef.current += 1
    baselineRef.current = savedKey
    baseRevisionRef.current = baseRevision || ''
    initializedRef.current = true
    waitingForDraftSyncRef.current = resetChanged && !syncChanged
    blockedDraftKeyRef.current = ''
    observedDraftKeyRef.current = ''
    reset(laneScopeKey)
  }, [baseRevision, laneScopeKey, reset, resetKey, savedKey, syncKey])

  useEffect(() => {
    if (!ready) return
    if (!initializedRef.current) {
      baselineRef.current = savedKey
      baseRevisionRef.current = baseRevision || ''
      waitingForDraftSyncRef.current = draftKey !== savedKey
      initializedRef.current = true
      return
    }
    if (latestDraftKeyRef.current === baselineRef.current) {
      baselineRef.current = savedKey
      baseRevisionRef.current = baseRevision || baseRevisionRef.current
    }
  }, [baseRevision, draftKey, ready, savedKey])

  useEffect(() => {
    if (!ready) {
      observedDraftKeyRef.current = ''
      cancel()
      return
    }
    if (waitingForDraftSyncRef.current) {
      if (draftKey === baselineRef.current) waitingForDraftSyncRef.current = false
      return
    }
    if (draftKey === baselineRef.current) {
      observedDraftKeyRef.current = draftKey
      if (!hasWork()) cancel()
      return
    }
    if (draftKey === blockedDraftKeyRef.current || observedDraftKeyRef.current === draftKey) return
    observedDraftKeyRef.current = draftKey
    edit({ draft, key: draftKey, generation: generationRef.current })
  }, [cancel, draft, draftKey, edit, hasWork, ready, syncKey])

  const flush = useCallback(async () => {
    if (!readyRef.current || waitingForDraftSyncRef.current) return null
    const key = latestDraftKeyRef.current
    if (key !== baselineRef.current && (!hasWork() || getSnapshot().status === 'error')) {
      blockedDraftKeyRef.current = ''
      observedDraftKeyRef.current = key
      edit({ draft: latestDraftRef.current, key, generation: generationRef.current })
    }
    const result = await flushLane()
    const snapshot = getSnapshot()
    if (snapshot.status === 'error') throw snapshot.error
    return result
  }, [edit, flushLane, getSnapshot, hasWork])

  const error = lane.error instanceof Error
    ? lane.error.message
    : lane.error === null ? null : String(lane.error)
  return { flush, status: lane.status, error }
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
