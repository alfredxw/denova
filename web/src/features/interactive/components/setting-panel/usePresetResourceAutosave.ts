import { useCallback, useEffect, useRef } from 'react'

export type PresetResourceSaveMode = 'manual' | 'auto'

const PRESET_RESOURCE_AUTOSAVE_DELAY_MS = 1200

interface PresetResourceAutosaveOptions<Draft extends { id: string; updated_at?: string }, Payload, Saved extends { updated_at?: string }> {
  draft: Draft | null
  active: boolean
  scopeKey?: string
  valid?: boolean
  makePayload: (draft: Draft) => Payload
  signature: (value: Partial<Draft> | Payload | Saved) => string
  save: (id: string, payload: Payload, baseRevision?: string) => Promise<Saved>
  onSaved?: (saved: Saved, mode: PresetResourceSaveMode, previousDraft: Draft) => void
  onAutoSaveError?: (error: unknown) => void
  onFlushError?: (error: unknown) => void
}

export function usePresetResourceAutosave<Draft extends { id: string; updated_at?: string }, Payload, Saved extends { updated_at?: string }>({
  draft,
  active,
  scopeKey = '',
  valid = true,
  makePayload,
  signature,
  save,
  onSaved,
  onAutoSaveError,
  onFlushError,
}: PresetResourceAutosaveOptions<Draft, Payload, Saved>) {
  const timerRef = useRef<number | null>(null)
  const saveQueueRef = useRef<Promise<void>>(Promise.resolve())
  const pendingSavesRef = useRef(new Set<Promise<Saved | null>>())
  const savedSignatureRef = useRef('')
  const baseRevisionRef = useRef('')
  const baselineResourceIdRef = useRef('')
  const baselineGenerationRef = useRef(0)
  const scopeKeyRef = useRef(scopeKey)
  const mountedRef = useRef(true)
  const draftRef = useRef(draft)
  const validRef = useRef(valid)
  const makePayloadRef = useRef(makePayload)
  const signatureRef = useRef(signature)
  const saveRef = useRef(save)
  const onSavedRef = useRef(onSaved)
  const onAutoSaveErrorRef = useRef(onAutoSaveError)
  const onFlushErrorRef = useRef(onFlushError)

  draftRef.current = draft
  validRef.current = valid
  makePayloadRef.current = makePayload
  signatureRef.current = signature
  saveRef.current = save
  onSavedRef.current = onSaved
  onAutoSaveErrorRef.current = onAutoSaveError
  onFlushErrorRef.current = onFlushError

  if (scopeKeyRef.current !== scopeKey) {
    scopeKeyRef.current = scopeKey
    baselineGenerationRef.current += 1
    baselineResourceIdRef.current = ''
    baseRevisionRef.current = ''
    savedSignatureRef.current = ''
    saveQueueRef.current = Promise.resolve()
    pendingSavesRef.current.clear()
  }

  const cancelPending = useCallback(() => {
    if (timerRef.current === null) return
    window.clearTimeout(timerRef.current)
    timerRef.current = null
  }, [])

  const resetBaseline = useCallback((nextDraft: Draft | null) => {
    const nextResourceId = nextDraft?.id || ''
    if (nextResourceId !== baselineResourceIdRef.current) {
      baselineGenerationRef.current += 1
      baselineResourceIdRef.current = nextResourceId
    }
    baseRevisionRef.current = nextDraft?.updated_at || ''
    savedSignatureRef.current = nextDraft ? signatureRef.current(nextDraft) : ''
  }, [])

  const saveNow = useCallback((mode: PresetResourceSaveMode) => {
    if (mode === 'manual') cancelPending()
    const snapshot = draftRef.current
    if (!snapshot || !validRef.current) return Promise.resolve(null)

    const payload = makePayloadRef.current(snapshot)
    const nextSignature = signatureRef.current(payload)
    const baselineGeneration = baselineGenerationRef.current
    const queuedScopeKey = scopeKeyRef.current
    const queuedBaseRevision = baseRevisionRef.current
    const saveResource = saveRef.current

    const operation = saveQueueRef.current.then(async () => {
      const baselineIsCurrent = baselineResourceIdRef.current === snapshot.id
        && baselineGenerationRef.current === baselineGeneration
        && scopeKeyRef.current === queuedScopeKey
      if (mode === 'auto' && baselineIsCurrent && nextSignature === savedSignatureRef.current) return null

      const baseRevision = baselineIsCurrent ? baseRevisionRef.current : queuedBaseRevision
      const saved = await saveResource(snapshot.id, payload, baseRevision)
      if (
        mountedRef.current
        && scopeKeyRef.current === queuedScopeKey
        && baselineResourceIdRef.current === snapshot.id
        && baselineGenerationRef.current === baselineGeneration
      ) {
        baseRevisionRef.current = saved.updated_at || ''
        // Keep the submitted signature while the editor remains active. The server may
        // normalize the response, but applying that response here could replace edits
        // made after this request started. The saved response becomes the next baseline
        // when the resource is opened again.
        savedSignatureRef.current = nextSignature
        onSavedRef.current?.(saved, mode, snapshot)
      }
      return saved
    })

    // A failed request must reject its caller without poisoning later queued saves.
    saveQueueRef.current = operation.then(() => undefined, () => undefined)
    pendingSavesRef.current.add(operation)
    void operation.then(
      () => pendingSavesRef.current.delete(operation),
      () => pendingSavesRef.current.delete(operation),
    )
    return operation
  }, [cancelPending])

  const flushPending = useCallback(() => {
    const pending = [...pendingSavesRef.current]
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
      pending.push(saveNow('auto'))
    }
    if (pending.length === 0) return null

    const result = Promise.all(pending).then((saved) => saved.at(-1) || null)
    result.catch((error) => {
      onFlushErrorRef.current?.(error)
    })
    return result
  }, [saveNow])

  useEffect(() => {
    if (!active || !draft) return
    if (!valid) {
      cancelPending()
      return
    }
    const nextSignature = signature(draft)
    if (nextSignature === savedSignatureRef.current) return
    cancelPending()
    const scheduledScopeKey = scopeKeyRef.current
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null
      void saveNow('auto').catch((error) => {
        if (mountedRef.current && scopeKeyRef.current === scheduledScopeKey) {
          onAutoSaveErrorRef.current?.(error)
        }
      })
    }, PRESET_RESOURCE_AUTOSAVE_DELAY_MS)
    return cancelPending
  }, [active, cancelPending, draft, saveNow, scopeKey, signature, valid])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      baselineGenerationRef.current += 1
      pendingSavesRef.current.clear()
      cancelPending()
    }
  }, [cancelPending])

  return {
    cancelPending,
    flushPending,
    resetBaseline,
    saveNow,
  }
}
