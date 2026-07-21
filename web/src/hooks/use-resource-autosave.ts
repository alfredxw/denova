import { useCallback, useEffect, useRef } from 'react'
import type { AutosaveStatus } from '@/components/forms/autosave-status'
import { useSaveLane } from '@/hooks/use-save-lane'
import { SaveLaneBlockedError } from '@/lib/autosave/save-lane'

export type ResourceSaveMode = 'manual' | 'auto'

const DEFAULT_RESOURCE_AUTOSAVE_DELAY_MS = 1200
const MAX_RESOURCE_SAVE_ATTEMPTS = 3

interface ResourceAutosaveOptions<Draft extends { id: string; updated_at?: string }, Payload, Saved extends { updated_at?: string }> {
  draft: Draft | null
  active: boolean
  scopeKey?: string
  valid?: boolean
  delayMs?: number
  makePayload: (draft: Draft) => Payload
  signature: (value: Partial<Draft> | Payload | Saved) => string
  getRevision?: (value: Draft | Saved) => string | undefined
  save: (id: string, payload: Payload, baseRevision?: string) => Promise<Saved>
  resolveConflict?: (context: ResourceConflictContext<Draft, Payload>) => Promise<ResourceConflictResolution<Payload> | null>
  /** Converts a server response into the exact baseline used by a queued follow-up edit. */
  baselineFromSaved?: (saved: Saved, submitted: Draft) => Draft
  onSaved?: (saved: Saved, mode: ResourceSaveMode, previousDraft: Draft) => void
  onAutoSaveError?: (error: unknown) => void
  onFlushError?: (error: unknown) => void
}

export interface ResourceConflictContext<Draft, Payload> {
  error: unknown
  baseline: Draft | null
  draft: Draft
  payload: Payload
  baseRevision: string
}

export interface ResourceConflictResolution<Payload> {
  payload: Payload
  baseRevision?: string
}

interface ResourceSaveRequest<Draft, Payload, Saved extends { updated_at?: string }> {
  draft: Draft
  mode: ResourceSaveMode
  baselineGeneration: number
  baselineAdvanceVersion: number
  baseline: Draft | null
  baseRevision: string
  makePayload: (draft: Draft) => Payload
  signature: (value: Partial<Draft> | Payload | Saved) => string
  getRevision: (value: Draft | Saved) => string | undefined
  save: (id: string, payload: Payload, baseRevision?: string) => Promise<Saved>
  resolveConflict?: (context: ResourceConflictContext<Draft, Payload>) => Promise<ResourceConflictResolution<Payload> | null>
  baselineFromSaved?: (saved: Saved, submitted: Draft) => Draft
}

interface ResourceSaveResult<Draft, Saved> {
  saved: Saved
  submittedDraft: Draft
  savedBaseline: Draft
  mode: ResourceSaveMode
}

/** React/resource adapter over the shared after-delay, latest-only save lane. */
export function useResourceAutosave<Draft extends { id: string; updated_at?: string }, Payload, Saved extends { updated_at?: string }>({
  draft,
  active,
  scopeKey = '',
  valid = true,
  delayMs = DEFAULT_RESOURCE_AUTOSAVE_DELAY_MS,
  makePayload,
  signature,
  getRevision,
  save,
  resolveConflict,
  baselineFromSaved,
  onSaved,
  onAutoSaveError,
  onFlushError,
}: ResourceAutosaveOptions<Draft, Payload, Saved>) {
  const draftRef = useRef(draft)
  const validRef = useRef(valid)
  const activeRef = useRef(active)
  const scopeKeyRef = useRef(scopeKey)
  const baselineScopeKeyRef = useRef('')
  const baselineGenerationRef = useRef(0)
  // Advances only when queued full snapshots can safely inherit the latest
  // revision. A conflict merge/server normalization creates a rebase barrier.
  const baselineAdvanceVersionRef = useRef(0)
  const externalRevisionBarrierRef = useRef(false)
  const baselineResourceIdRef = useRef('')
  const baseRevisionRef = useRef('')
  const baselineDraftRef = useRef<Draft | null>(null)
  const savedSignatureRef = useRef('')
  const observedEditKeyRef = useRef('')
  const observedDraftRevisionRef = useRef('')
  const makePayloadRef = useRef(makePayload)
  const signatureRef = useRef(signature)
  const getRevisionRef = useRef(getRevision ?? defaultResourceRevision)
  const saveRef = useRef(save)
  const resolveConflictRef = useRef(resolveConflict)
  const baselineFromSavedRef = useRef(baselineFromSaved)
  const onSavedRef = useRef(onSaved)
  const onAutoSaveErrorRef = useRef(onAutoSaveError)
  const onFlushErrorRef = useRef(onFlushError)

  draftRef.current = draft
  activeRef.current = active
  validRef.current = valid
  scopeKeyRef.current = scopeKey
  makePayloadRef.current = makePayload
  signatureRef.current = signature
  getRevisionRef.current = getRevision ?? defaultResourceRevision
  saveRef.current = save
  resolveConflictRef.current = resolveConflict
  baselineFromSavedRef.current = baselineFromSaved
  onSavedRef.current = onSaved
  onAutoSaveErrorRef.current = onAutoSaveError
  onFlushErrorRef.current = onFlushError

  const lane = useSaveLane<
    ResourceSaveRequest<Draft, Payload, Saved>,
    ResourceSaveResult<Draft, Saved> | null
  >({
    scopeKey,
    delayMs,
    save: async ({ value: request }) => {
      const snapshot = request.draft
      const payload = request.makePayload(snapshot)
      const nextSignature = request.signature(payload)
      if (baselineResourceIdRef.current === snapshot.id && nextSignature === savedSignatureRef.current) return null

      const canAdvanceFromCurrentBaseline = request.baselineGeneration === baselineGenerationRef.current
        && request.baselineAdvanceVersion === baselineAdvanceVersionRef.current
        && baselineResourceIdRef.current === snapshot.id
      const queuedBaseline = canAdvanceFromCurrentBaseline ? baselineDraftRef.current : request.baseline
      let attemptPayload = payload
      let attemptBaseRevision: string | undefined = (
        canAdvanceFromCurrentBaseline ? baseRevisionRef.current : request.baseRevision
      ) || undefined
      let saved: Saved
      for (let attempt = 1; ; attempt += 1) {
        try {
          saved = await request.save(snapshot.id, attemptPayload, attemptBaseRevision)
          break
        } catch (error) {
          if (attempt >= MAX_RESOURCE_SAVE_ATTEMPTS) throw error
          const resolution = await request.resolveConflict?.({
            error,
            baseline: queuedBaseline,
            draft: snapshot,
            payload: attemptPayload,
            baseRevision: attemptBaseRevision || '',
          })
          if (!resolution) throw error
          attemptPayload = resolution.payload
          attemptBaseRevision = resolution.baseRevision
        }
      }
      const savedBaseline = request.baselineFromSaved
        ? request.baselineFromSaved(saved, snapshot)
        : { ...snapshot, ...saved, updated_at: saved.updated_at || snapshot.updated_at } as Draft
      return {
        saved,
        savedBaseline,
        submittedDraft: snapshot,
        mode: request.mode,
      }
    },
    onSaved: ({ value: request }, result) => {
      if (!result) return
      const { saved, savedBaseline, submittedDraft, mode } = result
      if (request.baselineGeneration !== baselineGenerationRef.current) return
      if (baselineResourceIdRef.current !== submittedDraft.id) return
      baseRevisionRef.current = request.getRevision(saved) || ''
      baselineDraftRef.current = savedBaseline
      const nextSavedSignature = request.signature(savedBaseline)
      const submittedSignature = request.signature(request.makePayload(submittedDraft))
      savedSignatureRef.current = nextSavedSignature
      if (nextSavedSignature !== submittedSignature) baselineAdvanceVersionRef.current += 1
      onSavedRef.current?.(saved, mode, submittedDraft)
    },
    onError: ({ value: request }, error) => {
      if (request.mode === 'auto') onAutoSaveErrorRef.current?.(error)
    },
  })

  const { block, cancel, edit, flush, getSnapshot, hasWork, reload, reset, unblock } = lane

  const saveRequest = useCallback((nextDraft: Draft, mode: ResourceSaveMode): ResourceSaveRequest<Draft, Payload, Saved> => ({
    draft: nextDraft,
    mode,
    baselineGeneration: baselineGenerationRef.current,
    baselineAdvanceVersion: baselineAdvanceVersionRef.current,
    baseline: baselineDraftRef.current,
    baseRevision: baseRevisionRef.current,
    makePayload: makePayloadRef.current,
    signature: signatureRef.current,
    getRevision: getRevisionRef.current,
    save: saveRef.current,
    resolveConflict: resolveConflictRef.current,
    baselineFromSaved: baselineFromSavedRef.current,
  }), [])

  const resetBaseline = useCallback((nextDraft: Draft | null) => {
    const previousResourceId = baselineResourceIdRef.current
    const previousRevision = baseRevisionRef.current
    const previousScopeKey = baselineScopeKeyRef.current
    const nextResourceId = nextDraft?.id || ''
    const nextRevision = nextDraft ? getRevisionRef.current(nextDraft) || '' : ''
    const nextSavedSignature = nextDraft ? signatureRef.current(nextDraft) : ''
    const sameResource = previousResourceId === nextResourceId && previousScopeKey === scopeKeyRef.current
    const acknowledgedOwnSave = sameResource && Boolean(nextRevision) && nextRevision === previousRevision
    if (sameResource && !acknowledgedOwnSave) baselineGenerationRef.current += 1
    const hadWork = hasWork()
    baselineResourceIdRef.current = nextResourceId
    baselineScopeKeyRef.current = scopeKeyRef.current
    baseRevisionRef.current = nextRevision
    baselineDraftRef.current = nextDraft
    savedSignatureRef.current = nextSavedSignature

    const currentDraft = draftRef.current
    const currentDraftRevision = currentDraft ? getRevisionRef.current(currentDraft) || '' : ''
    const currentDraftMatchesBaselineRevision = !nextRevision || currentDraftRevision === nextRevision
    if (sameResource && !acknowledgedOwnSave && !currentDraftMatchesBaselineRevision) {
      externalRevisionBarrierRef.current = true
    } else if (currentDraftMatchesBaselineRevision) {
      externalRevisionBarrierRef.current = false
    }
    const hasNewerDraft = Boolean(
      currentDraft
      && currentDraft.id === nextResourceId
      && signatureRef.current(makePayloadRef.current(currentDraft)) !== nextSavedSignature,
    )
    const currentEditKey = currentDraft
      ? `${scopeKeyRef.current}\u0000${currentDraft.id}\u0000${currentDraftRevision}\u0000${signatureRef.current(makePayloadRef.current(currentDraft))}`
      : ''
    observedEditKeyRef.current = currentEditKey
    observedDraftRevisionRef.current = currentDraftRevision

    if (!sameResource) reset(scopeKeyRef.current)
    if (!activeRef.current) {
      cancel()
      observedEditKeyRef.current = ''
      observedDraftRevisionRef.current = ''
      return
    }
    if (!validRef.current) {
      cancel()
      block()
      observedEditKeyRef.current = ''
      observedDraftRevisionRef.current = ''
      return
    }
    if (!hasNewerDraft || !currentDraft) {
      cancel()
      return
    }
    // A rebase owner may publish its baseline before React commits the rebased draft.
    // Keep the existing request, which is still paired with the old revision, until
    // the matching draft arrives. Sending the stale draft against nextRevision would
    // silently bypass optimistic concurrency.
    if (sameResource && !acknowledgedOwnSave && !currentDraftMatchesBaselineRevision) return
    if (!sameResource || !hadWork) {
      edit(saveRequest(currentDraft, 'auto'))
      return
    }
    if (!acknowledgedOwnSave) reload(saveRequest(currentDraft, 'auto'))
  }, [block, cancel, edit, hasWork, reload, reset, saveRequest])

  const resultOrThrow = useCallback(async (
    promise: Promise<ResourceSaveResult<Draft, Saved> | null>,
  ): Promise<Saved | null> => {
    const result = await promise
    const snapshot = getSnapshot()
    if (snapshot.status === 'error') throw snapshot.error
    return result?.saved ?? null
  }, [getSnapshot])

  const hasDirtyDraft = useCallback(() => {
    const current = draftRef.current
    return Boolean(
      current
      && current.id === baselineResourceIdRef.current
      && signatureRef.current(makePayloadRef.current(current)) !== savedSignatureRef.current,
    )
  }, [])

  /** Lets a rebase owner distinguish our saved echo from a genuinely external baseline. */
  const isBaselineAcknowledged = useCallback((nextDraft: Draft | null) => {
    if (!nextDraft) return false
    const nextRevision = getRevisionRef.current(nextDraft) || ''
    return baselineScopeKeyRef.current === scopeKeyRef.current
      && baselineResourceIdRef.current === nextDraft.id
      && Boolean(nextRevision)
      && nextRevision === baseRevisionRef.current
      && signatureRef.current(nextDraft) === savedSignatureRef.current
  }, [])

  const saveNow = useCallback((mode: ResourceSaveMode) => {
    const snapshot = draftRef.current
    if (!snapshot) return Promise.resolve(null)
    if (!validRef.current) {
      const error = new SaveLaneBlockedError()
      block(error)
      return Promise.reject(error)
    }
    const snapshotRevision = getRevisionRef.current(snapshot) || ''
    const awaitingMatchingRebasedDraft = externalRevisionBarrierRef.current && Boolean(
      baselineResourceIdRef.current === snapshot.id
      && snapshotRevision
      && baseRevisionRef.current
      && snapshotRevision !== baseRevisionRef.current,
    )
    if (awaitingMatchingRebasedDraft) {
      // Manual retry must obey the same revision barrier as after-delay edits.
      // Prefer retrying the already captured old-revision request; replacing it
      // with the stale React draft would bypass CAS against the new baseline.
      if (hasWork()) return resultOrThrow(flush())
      return Promise.reject(new SaveLaneBlockedError('external-rebase-pending'))
    }
    const nextSignature = signatureRef.current(makePayloadRef.current(snapshot))
    if (baselineResourceIdRef.current === snapshot.id && nextSignature === savedSignatureRef.current) {
      cancel()
      return Promise.resolve(null)
    }
    unblock()
    edit(saveRequest(snapshot, mode))
    return resultOrThrow(flush())
  }, [block, cancel, edit, flush, hasWork, resultOrThrow, saveRequest, unblock])

  const flushPending = useCallback(() => {
    if (!validRef.current && hasDirtyDraft()) {
      const error = new SaveLaneBlockedError()
      block(error)
      const result = Promise.reject(error) as Promise<Saved | null>
      result.catch(cause => onFlushErrorRef.current?.(cause))
      return result
    }
    if (!hasWork()) return null
    const result = resultOrThrow(flush())
    result.catch(error => onFlushErrorRef.current?.(error))
    return result
  }, [block, flush, hasDirtyDraft, hasWork, resultOrThrow])

  useEffect(() => {
    if (!active || !draft) {
      observedEditKeyRef.current = ''
      observedDraftRevisionRef.current = ''
      cancel()
      return
    }
    if (!valid) {
      block()
      return
    }
    unblock()
    const nextSignature = signatureRef.current(makePayloadRef.current(draft))
    const draftRevision = getRevisionRef.current(draft) || ''
    const revisionChanged = observedDraftRevisionRef.current !== draftRevision
    const editKey = `${scopeKey}\u0000${draft.id}\u0000${draftRevision}\u0000${nextSignature}`
    if (baselineResourceIdRef.current === draft.id && nextSignature === savedSignatureRef.current) {
      observedEditKeyRef.current = editKey
      observedDraftRevisionRef.current = draftRevision
      cancel()
      return
    }
    if (observedEditKeyRef.current === editKey) return
    observedEditKeyRef.current = editKey
    observedDraftRevisionRef.current = draftRevision
    const awaitingMatchingRebasedDraft = Boolean(
      baselineResourceIdRef.current === draft.id
      && draftRevision
      && baseRevisionRef.current
      && draftRevision !== baseRevisionRef.current,
    )
    // resetBaseline may advance to r2 before conflict recovery/archive has
    // produced the matching r2 draft. User edits made inside that async window
    // still belong to r1 and must not replace the safe r1 lane request.
    if (awaitingMatchingRebasedDraft) return
    externalRevisionBarrierRef.current = false
    const rebasedExternalDraft = Boolean(
      hasWork()
      && baselineResourceIdRef.current === draft.id
      && draftRevision
      && revisionChanged,
    )
    const request = saveRequest(draft, 'auto')
    if (rebasedExternalDraft) reload(request)
    else edit(request)
  }, [active, block, cancel, draft, edit, hasWork, reload, saveRequest, scopeKey, unblock, valid])

  const error = lane.error instanceof Error
    ? lane.error.message
    : lane.error === null ? null : String(lane.error)

  return {
    cancelPending: cancel,
    flushPending,
    isBaselineAcknowledged,
    resetBaseline,
    saveNow,
    status: lane.status as AutosaveStatus,
    error,
    retry: () => saveNow('manual'),
  }
}

function defaultResourceRevision(value: { updated_at?: string }): string | undefined {
  return value.updated_at
}
