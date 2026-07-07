import { useCallback, useEffect, useRef } from 'react'

export type PresetResourceSaveMode = 'manual' | 'auto'

const PRESET_RESOURCE_AUTOSAVE_DELAY_MS = 1200

interface PresetResourceAutosaveOptions<Draft extends { id: string; updated_at?: string }, Payload, Saved extends { tags?: string[]; updated_at?: string }> {
  draft: Draft | null
  tagDraft: string
  active: boolean
  valid?: boolean
  makePayload: (draft: Draft, tagDraft: string) => Payload
  signature: (value: Partial<Draft> | Payload | Saved, tagDraft: string) => string
  save: (id: string, payload: Payload, baseRevision?: string) => Promise<Saved>
  onSaved?: (saved: Saved, mode: PresetResourceSaveMode, previousDraft: Draft) => void
  onAutoSaveError?: (error: unknown) => void
  onFlushError?: (error: unknown) => void
}

export function usePresetResourceAutosave<Draft extends { id: string; updated_at?: string }, Payload, Saved extends { tags?: string[]; updated_at?: string }>({
  draft,
  tagDraft,
  active,
  valid = true,
  makePayload,
  signature,
  save,
  onSaved,
  onAutoSaveError,
  onFlushError,
}: PresetResourceAutosaveOptions<Draft, Payload, Saved>) {
  const timerRef = useRef<number | null>(null)
  const savedSignatureRef = useRef('')
  const baseRevisionRef = useRef('')
  const draftRef = useRef(draft)
  const tagDraftRef = useRef(tagDraft)
  const validRef = useRef(valid)
  const makePayloadRef = useRef(makePayload)
  const signatureRef = useRef(signature)
  const saveRef = useRef(save)
  const onSavedRef = useRef(onSaved)
  const onAutoSaveErrorRef = useRef(onAutoSaveError)
  const onFlushErrorRef = useRef(onFlushError)

  useEffect(() => { draftRef.current = draft }, [draft])
  useEffect(() => { tagDraftRef.current = tagDraft }, [tagDraft])
  useEffect(() => { validRef.current = valid }, [valid])
  useEffect(() => { makePayloadRef.current = makePayload }, [makePayload])
  useEffect(() => { signatureRef.current = signature }, [signature])
  useEffect(() => { saveRef.current = save }, [save])
  useEffect(() => { onSavedRef.current = onSaved }, [onSaved])
  useEffect(() => { onAutoSaveErrorRef.current = onAutoSaveError }, [onAutoSaveError])
  useEffect(() => { onFlushErrorRef.current = onFlushError }, [onFlushError])

  const cancelPending = useCallback(() => {
    if (timerRef.current === null) return
    window.clearTimeout(timerRef.current)
    timerRef.current = null
  }, [])

  const resetBaseline = useCallback((nextDraft: Draft | null, nextTagDraft = '') => {
    baseRevisionRef.current = nextDraft?.updated_at || ''
    savedSignatureRef.current = nextDraft ? signatureRef.current(nextDraft, nextTagDraft) : ''
  }, [])

  const saveNow = useCallback(async (mode: PresetResourceSaveMode) => {
    if (mode === 'manual') cancelPending()
    const snapshot = draftRef.current
    if (!snapshot || !validRef.current) return null

    const tags = tagDraftRef.current
    const payload = makePayloadRef.current(snapshot, tags)
    const nextSignature = signatureRef.current(payload, tags)
    if (mode === 'auto' && nextSignature === savedSignatureRef.current) return null

    const saved = await saveRef.current(snapshot.id, payload, baseRevisionRef.current)
    baseRevisionRef.current = saved.updated_at || ''
    savedSignatureRef.current = signatureRef.current(saved, (saved.tags || []).join('，'))
    onSavedRef.current?.(saved, mode, snapshot)
    return saved
  }, [cancelPending])

  const flushPending = useCallback(() => {
    if (timerRef.current === null) return null
    window.clearTimeout(timerRef.current)
    timerRef.current = null
    const result = saveNow('auto')
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
    const nextSignature = signature(draft, tagDraft)
    if (nextSignature === savedSignatureRef.current) return
    cancelPending()
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null
      void saveNow('auto').catch((error) => {
        onAutoSaveErrorRef.current?.(error)
      })
    }, PRESET_RESOURCE_AUTOSAVE_DELAY_MS)
    return cancelPending
  }, [active, cancelPending, draft, saveNow, signature, tagDraft, valid])

  useEffect(() => cancelPending, [cancelPending])

  return {
    cancelPending,
    flushPending,
    resetBaseline,
    saveNow,
  }
}
