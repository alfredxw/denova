import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import type { Editor } from '@tiptap/core'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'

import { preserveAutosaveConflict, type PreservedAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { rebaseTextWithConflicts } from '@/lib/three-way-rebase'
import {
  AUTOSAVE_CONFLICT_PRESERVED_EVENT,
  type AutosaveConflictPreservedDetail,
} from '@/lib/autosave/rebase-with-recovery'
import { WorkspaceFileRevisionConflictError } from '@/lib/autosave/workspace-file-revision-conflict'
import { useSaveLane } from '@/hooks/use-save-lane'
import type { SaveStatus } from './EditorToolbar'
import { readEditorText } from './editorDocument'

export type EditorFlushHandler = () => Promise<boolean>

type EditorSaveResponse = boolean | { revision?: string }

type PendingSave = {
  workspace: string
  fileName: string
  text: string
  baseContent: string
  baseRevision: string
  mode: 'manual' | 'auto'
  save: (fileName: string, content: string, baseRevision: string) => Promise<EditorSaveResponse>
  recovery?: Promise<PreservedAutosaveConflict>
}

type PendingSaveBatch = {
  requests: PendingSave[]
}

type SavedEditorRequest = {
  request: PendingSave
  revision: string
}

type EditorSaveResult = {
  results: SavedEditorRequest[]
}

type FailedEditorRequest = {
  request: PendingSave
  error: unknown
}

class EditorSaveBatchError extends Error {
  readonly results: SavedEditorRequest[]
  readonly failures: FailedEditorRequest[]

  constructor(results: SavedEditorRequest[], failures: FailedEditorRequest[]) {
    const firstError = failures[0]?.error
    super(firstError instanceof Error ? firstError.message : String(firstError || 'Editor save failed'))
    this.name = 'EditorSaveBatchError'
    this.results = results
    this.failures = failures
  }
}

type ConflictRecovery = {
  key: string
  promise: Promise<PreservedAutosaveConflict> | null
  retry: () => Promise<PreservedAutosaveConflict>
}

export type ExternalContentConflict = {
  workspace: string
  fileName: string
  localContent: string
  externalContent: string
  mergedContent: string
  recoveryID?: string
  recoveryPath?: string
}

interface UseEditorDraftPersistenceOptions {
  workspace: string
  fileName: string | null
  content: string
  revision: string
  editor: Editor | null
  editorContainerRef: RefObject<HTMLDivElement | null>
  onSave: (fileName: string, content: string, baseRevision: string) => Promise<EditorSaveResponse>
  saveSignal: number
  autoSaveEnabled: boolean
  autoSaveDelayMs?: number
  applyExternalContent: (
    fileName: string | null,
    content: string,
    options: { resetHistory: boolean; preserveSelection: boolean },
  ) => void
  onExternalConflict?: (conflict: { fileName: string; localContent: string; externalContent: string }) => void
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
}

interface EditorDraftPersistence {
  saveStatus: SaveStatus | null
  externalConflict: ExternalContentConflict | null
  externalConflictSaving: boolean
  handleSave: () => Promise<void>
  flushCurrentDraft: EditorFlushHandler
  loadExternalVersion: () => void
  keepLocalVersion: () => Promise<void>
}

const DEFAULT_AUTO_SAVE_DELAY_MS = 1500
const EDITOR_LANE_SCOPE = 'editor-drafts'
const MAX_EDITOR_REVISION_SAVE_ATTEMPTS = 3

function documentSaveKey(workspace: string, fileName: string): string {
  return `${workspace}\u0000${fileName}`
}

function normalizeAutoSaveDelayMs(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return DEFAULT_AUTO_SAVE_DELAY_MS
  return Math.floor(value)
}

interface PreserveEditorConflictOptions {
  workspace: string
  fileName: string
  baseContent: string
  baseRevision: string
  localContent: string
  externalContent: string
  externalRevision: string
  mergedContent: string
  conflictPaths: string[][]
}

async function preserveEditorConflict({
  workspace,
  fileName,
  baseContent,
  baseRevision,
  localContent,
  externalContent,
  externalRevision,
  mergedContent,
  conflictPaths,
}: PreserveEditorConflictOptions): Promise<PreservedAutosaveConflict> {
  const saved = await preserveAutosaveConflict({
    resource: 'workspace_file',
    scope: workspace || 'editor',
    id: fileName,
    base: { revision: baseRevision, value: baseContent },
    local: { revision: baseRevision, value: localContent },
    external: { revision: externalRevision, value: externalContent },
    merged: { revision: externalRevision, value: mergedContent },
    strategy: 'merge_non_overlap_prefer_local',
    conflict_paths: conflictPaths,
  })
  window.dispatchEvent(new CustomEvent<AutosaveConflictPreservedDetail>(AUTOSAVE_CONFLICT_PRESERVED_EVENT, {
    detail: {
      ...saved,
      resource: 'workspace_file',
      scope: workspace || 'editor',
      resourceID: fileName,
    },
  }))
  return saved
}

/** Editor adapter: TipTap/external-sync concerns stay here; timing and serialization live in SaveLane. */
export function useEditorDraftPersistence({
  workspace,
  fileName,
  content,
  revision,
  editor,
  editorContainerRef,
  onSave,
  saveSignal,
  autoSaveEnabled,
  autoSaveDelayMs,
  applyExternalContent,
  onExternalConflict,
  onFlushHandlerChange,
}: UseEditorDraftPersistenceOptions): EditorDraftPersistence {
  const { t } = useTranslation()
  const [saveStatus, setSaveStatus] = useState<SaveStatus | null>(null)
  const [externalConflict, setExternalConflict] = useState<ExternalContentConflict | null>(null)
  const [externalConflictSaving, setExternalConflictSaving] = useState(false)
  const saveStatusClearTimer = useRef<number | null>(null)
  const workspaceRef = useRef(workspace)
  const fileNameRef = useRef<string | null>(fileName)
  const onSaveRef = useRef(onSave)
  const autoSaveEnabledRef = useRef(autoSaveEnabled)
  const lastSyncedFileRef = useRef<string | null>(null)
  const lastSyncedWorkspaceRef = useRef(workspace)
  const lastSyncedContentRef = useRef('')
  const lastSyncedRevisionRef = useRef('')
  const dirtyRef = useRef(false)
  const externalConflictRef = useRef<ExternalContentConflict | null>(null)
  const recoveryRef = useRef<ConflictRecovery | null>(null)
  const localSaveEchoesRef = useRef(new Map<string, { content: string; revision: string }>())
  const confirmedSnapshotsRef = useRef(new Map<string, { content: string; revision: string }>())
  // SaveLane remains latest-only; the editor adapter's value is a keyed batch
  // so coalescing one document never discards a different document.
  const queuedRequestsRef = useRef(new Map<string, PendingSave>())
  const filePositionsRef = useRef(new Map<string, { scrollTop: number }>())
  const lastSaveSignalRef = useRef(saveSignal)
  const tRef = useRef(t)

  onSaveRef.current = onSave
  autoSaveEnabledRef.current = autoSaveEnabled
  tRef.current = t

  const clearSaveStatusTimer = useCallback(() => {
    if (saveStatusClearTimer.current === null) return
    window.clearTimeout(saveStatusClearTimer.current)
    saveStatusClearTimer.current = null
  }, [])

  const scheduleSaveStatusClear = useCallback((status: SaveStatus, delay: number) => {
    clearSaveStatusTimer()
    saveStatusClearTimer.current = window.setTimeout(() => {
      setSaveStatus(current => current === status ? null : current)
      saveStatusClearTimer.current = null
    }, delay)
  }, [clearSaveStatusTimer])

  const applySavedResults = (results: SavedEditorRequest[]) => {
    for (const { request, revision: savedRevision } of results) {
      if (workspaceRef.current !== request.workspace || fileNameRef.current !== request.fileName) continue
      lastSyncedContentRef.current = request.text
      lastSyncedRevisionRef.current = savedRevision
      if (editor && !editor.isDestroyed && readEditorText(editor, request.fileName) === request.text) dirtyRef.current = false
      const conflict = externalConflictRef.current
      if (conflict && conflict.workspace === request.workspace && conflict.fileName === request.fileName) {
        externalConflictRef.current = null
        recoveryRef.current = null
        setExternalConflict(null)
        setExternalConflictSaving(false)
      }
      const status: SaveStatus = request.mode === 'auto' ? 'auto-saved' : 'manual-saved'
      setSaveStatus(status)
      scheduleSaveStatusClear(status, request.mode === 'auto' ? 1400 : 2000)
    }
  }

  const lane = useSaveLane<PendingSaveBatch, EditorSaveResult>({
    scopeKey: EDITOR_LANE_SCOPE,
    delayMs: normalizeAutoSaveDelayMs(autoSaveDelayMs),
    save: async ({ value: batch }) => {
      const results: SavedEditorRequest[] = []
      const failures: FailedEditorRequest[] = []
      for (const request of batch.requests) {
        const key = documentSaveKey(request.workspace, request.fileName)
        if (queuedRequestsRef.current.get(key) !== request) continue
        try {
          if (request.recovery) await request.recovery
          // A newer edit may arrive while conflict recovery is being archived.
          if (queuedRequestsRef.current.get(key) !== request) continue
          const active = workspaceRef.current === request.workspace && fileNameRef.current === request.fileName
          if (active) {
            clearSaveStatusTimer()
            setSaveStatus(request.mode === 'auto' ? 'auto-saving' : 'manual-saving')
          }
          let response: EditorSaveResponse | undefined
          let superseded = false
          let rebasedAgainstNewerBaseline = false
          for (let attempt = 0; attempt < MAX_EDITOR_REVISION_SAVE_ATTEMPTS; attempt += 1) {
            const confirmed = confirmedSnapshotsRef.current.get(key)
            if (confirmed && confirmed.revision !== request.baseRevision) {
              rebasedAgainstNewerBaseline = true
              const merged = rebaseTextWithConflicts(request.baseContent, request.text, confirmed.content)
              if (merged.conflicts.length > 0) {
                await preserveEditorConflict({
                  workspace: request.workspace,
                  fileName: request.fileName,
                  baseContent: request.baseContent,
                  baseRevision: request.baseRevision,
                  localContent: request.text,
                  externalContent: confirmed.content,
                  externalRevision: confirmed.revision,
                  mergedContent: merged.value,
                  conflictPaths: merged.conflicts.map(conflict => conflict.path),
                })
              }
              if (queuedRequestsRef.current.get(key) !== request) {
                superseded = true
                break
              }
              request.text = merged.value
              request.baseContent = confirmed.content
              request.baseRevision = confirmed.revision
            }

            try {
              response = await request.save(request.fileName, request.text, request.baseRevision)
              break
            } catch (error) {
              if (!(error instanceof WorkspaceFileRevisionConflictError)
                || error.latest.workspace !== request.workspace
                || attempt + 1 >= MAX_EDITOR_REVISION_SAVE_ATTEMPTS) {
                throw error
              }
              // The reload is authoritative for every queued draft of this file,
              // including a newer request that may supersede this one mid-recovery.
              confirmedSnapshotsRef.current.set(key, {
                content: error.latest.content,
                revision: error.latest.revision,
              })
              rebasedAgainstNewerBaseline = true
              if (queuedRequestsRef.current.get(key) !== request) {
                superseded = true
                break
              }
              const merged = rebaseTextWithConflicts(request.baseContent, request.text, error.latest.content)
              if (merged.conflicts.length > 0) {
                await preserveEditorConflict({
                  workspace: request.workspace,
                  fileName: request.fileName,
                  baseContent: request.baseContent,
                  baseRevision: request.baseRevision,
                  localContent: request.text,
                  externalContent: error.latest.content,
                  externalRevision: error.latest.revision,
                  mergedContent: merged.value,
                  conflictPaths: merged.conflicts.map(conflict => conflict.path),
                })
              }
              if (queuedRequestsRef.current.get(key) !== request) {
                superseded = true
                break
              }
              request.text = merged.value
              request.baseContent = error.latest.content
              request.baseRevision = error.latest.revision
            }
          }
          if (superseded) continue
          if (response === undefined) throw new Error(tRef.current('editor.saveFailed'))
          if (response === false) throw new Error(tRef.current('editor.saveFailed'))
          const nextRevision = typeof response === 'object' && response.revision
            ? response.revision
            : request.baseRevision
          confirmedSnapshotsRef.current.set(key, { content: request.text, revision: nextRevision })
          localSaveEchoesRef.current.set(key, {
            content: request.text,
            revision: nextRevision,
          })
          const queuedAfterSave = queuedRequestsRef.current.get(key)
          if (queuedAfterSave && queuedAfterSave !== request && !rebasedAgainstNewerBaseline) {
            // This newer editor snapshot was created after the submitted text.
            // Advance its ancestor directly to our acknowledged save so the next
            // request inherits the revision without treating that response as an
            // external concurrent edit.
            queuedAfterSave.baseContent = request.text
            queuedAfterSave.baseRevision = nextRevision
          }
          if (queuedAfterSave === request || queuedAfterSave?.text === request.text) {
            queuedRequestsRef.current.delete(key)
          }
          results.push({ request: { ...request }, revision: nextRevision })
        } catch (error) {
          failures.push({ request, error })
        }
      }
      if (failures.length > 0) throw new EditorSaveBatchError(results, failures)
      return { results }
    },
    onSaved: (_queued, result) => {
      applySavedResults(result.results)
    },
    onError: ({ value: batch }, error) => {
      const failures = error instanceof EditorSaveBatchError
        ? error.failures
        : [{
            request: batch.requests.find(candidate => (
              workspaceRef.current === candidate.workspace && fileNameRef.current === candidate.fileName
            )) ?? batch.requests[batch.requests.length - 1],
            error,
          }]
      if (error instanceof EditorSaveBatchError) applySavedResults(error.results)
      const activeFailure = failures.find(({ request }) => (
        workspaceRef.current === request?.workspace && fileNameRef.current === request?.fileName
      ))
      const failed = activeFailure ?? failures[0]
      const request = failed?.request
      if (!request) return
      for (const failure of failures) {
        if (!failure.request) continue
        console.error('[useEditorDraftPersistence.ts] editor save lane failed', {
          workspace: failure.request.workspace,
          path: failure.request.fileName,
          baseRevision: failure.request.baseRevision,
          mode: failure.request.mode,
          error: failure.error,
        })
      }
      if (activeFailure) {
        setSaveStatus('error')
        scheduleSaveStatusClear('error', request.mode === 'auto' ? 1400 : 2000)
      }
      if (!activeFailure || request.mode === 'manual') toast.error(tRef.current('editor.saveFailed'))
    },
  })
  const {
    cancel: cancelLane,
    edit: editLane,
    flush,
    getSnapshot,
    hasWork,
    reload: reloadLane,
    reset: resetLane,
  } = lane

  const queueSave = useCallback((request: PendingSave, preserveDeadline = false) => {
    queuedRequestsRef.current.set(documentSaveKey(request.workspace, request.fileName), request)
    const batch = { requests: Array.from(queuedRequestsRef.current.values()) }
    if (preserveDeadline && reloadLane(batch)) return
    editLane(batch)
  }, [editLane, reloadLane])

  const cancelDocumentSave = useCallback((targetWorkspace: string, targetFile: string, onlyAuto = false) => {
    const key = documentSaveKey(targetWorkspace, targetFile)
    const queued = queuedRequestsRef.current.get(key)
    if (onlyAuto && queued?.mode !== 'auto') return
    queuedRequestsRef.current.delete(key)
    const remaining = Array.from(queuedRequestsRef.current.values())
    if (remaining.length === 0) {
      cancelLane()
      return
    }
    const batch = { requests: remaining }
    if (!reloadLane(batch)) editLane(batch)
  }, [cancelLane, editLane, reloadLane])

  const resetQueuedSaves = useCallback((scopeKey: string) => {
    queuedRequestsRef.current.clear()
    resetLane(scopeKey)
  }, [resetLane])

  const updateExternalConflict = useCallback((next: ExternalContentConflict | null) => {
    externalConflictRef.current = next
    setExternalConflict(next)
  }, [])

  const pendingSave = useCallback((
    targetWorkspace: string,
    targetFile: string,
    text: string,
    mode: 'manual' | 'auto',
  ): PendingSave => {
    const key = documentSaveKey(targetWorkspace, targetFile)
    const recoveryState = recoveryRef.current?.key === key ? recoveryRef.current : null
    const recovery = recoveryState ? (recoveryState.promise ?? recoveryState.retry()) : undefined
    return {
      workspace: targetWorkspace,
      fileName: targetFile,
      text,
      baseContent: lastSyncedContentRef.current,
      baseRevision: lastSyncedRevisionRef.current,
      mode,
      save: onSaveRef.current,
      recovery,
    }
  }, [])

  const archiveConflict = useCallback((conflict: ExternalContentConflict, baselineContent: string, baselineRevision: string, externalRevision: string) => {
    const key = documentSaveKey(conflict.workspace, conflict.fileName)
    const retry = (): Promise<PreservedAutosaveConflict> => {
      const promise = preserveEditorConflict({
        workspace: conflict.workspace,
        fileName: conflict.fileName,
        baseContent: baselineContent,
        baseRevision: baselineRevision,
        localContent: conflict.localContent,
        externalContent: conflict.externalContent,
        externalRevision,
        mergedContent: conflict.mergedContent,
        conflictPaths: [[]],
      })
      recoveryRef.current = { key, promise, retry }
      void promise.then(saved => {
        const active = externalConflictRef.current
        if (!active || documentSaveKey(active.workspace, active.fileName) !== key) return
        updateExternalConflict({ ...active, recoveryID: saved.id, recoveryPath: saved.path })
      }).catch(error => {
        if (recoveryRef.current?.key === key && recoveryRef.current.promise === promise) {
          recoveryRef.current = { key, promise: null, retry }
        }
        console.error('[useEditorDraftPersistence.ts] failed to preserve concurrent editor versions', {
          workspace: conflict.workspace,
          path: conflict.fileName,
          error,
        })
        if (workspaceRef.current === conflict.workspace && fileNameRef.current === conflict.fileName) {
          setSaveStatus('error')
          toast.error(tRef.current('editor.externalConflict.archiveFailed'))
        }
      })
      return promise
    }
    return retry()
  }, [updateExternalConflict])

  // Synchronize canonical props with the active editor. Dirty reloads are
  // rebased; only actual editor update events establish a new delay deadline.
  useLayoutEffect(() => {
    if (!editor || editor.isDestroyed) return

    const previousFile = lastSyncedFileRef.current
    const previousWorkspace = lastSyncedWorkspaceRef.current
    const fileChanged = previousFile !== fileName || previousWorkspace !== workspace
    const contentChanged = lastSyncedContentRef.current !== content
    const revisionChanged = lastSyncedRevisionRef.current !== revision
    if (!fileChanged && !contentChanged && !revisionChanged) return

    if (previousWorkspace !== workspace) {
      resetQueuedSaves(EDITOR_LANE_SCOPE)
      localSaveEchoesRef.current.clear()
      confirmedSnapshotsRef.current.clear()
      recoveryRef.current = null
      updateExternalConflict(null)
      setExternalConflictSaving(false)
    }

    const currentKey = fileName ? documentSaveKey(workspace, fileName) : ''
    const echo = currentKey ? localSaveEchoesRef.current.get(currentKey) : undefined
    if (!fileChanged && echo && echo.content === content && (!revision || echo.revision === revision)) {
      lastSyncedContentRef.current = content
      lastSyncedRevisionRef.current = revision || echo.revision
      localSaveEchoesRef.current.delete(currentKey)
      confirmedSnapshotsRef.current.set(currentKey, { content, revision: revision || echo.revision })
      if (readEditorText(editor, fileName) === content) dirtyRef.current = false
      workspaceRef.current = workspace
      fileNameRef.current = fileName
      return
    }

    if (!fileChanged && dirtyRef.current) {
      const targetFile = fileName || ''
      const baselineContent = lastSyncedContentRef.current
      const baselineRevision = lastSyncedRevisionRef.current
      const localContent = readEditorText(editor, fileName)
      const merged = rebaseTextWithConflicts(baselineContent, localContent, content)
      lastSyncedContentRef.current = content
      lastSyncedRevisionRef.current = revision
      if (currentKey) confirmedSnapshotsRef.current.set(currentKey, { content, revision })
      workspaceRef.current = workspace
      fileNameRef.current = fileName
      dirtyRef.current = merged.value !== content

      if (merged.value !== localContent) {
        applyExternalContent(fileName, merged.value, { resetHistory: false, preserveSelection: true })
      }
      if (merged.conflicts.length > 0) {
        const conflict: ExternalContentConflict = {
          workspace,
          fileName: targetFile,
          localContent,
          externalContent: content,
          mergedContent: merged.value,
        }
        updateExternalConflict(conflict)
        setExternalConflictSaving(false)
        archiveConflict(conflict, baselineContent, baselineRevision, revision)
        onExternalConflict?.({ fileName: targetFile, localContent, externalContent: content })
      } else {
        recoveryRef.current = null
        updateExternalConflict(null)
      }

      if (dirtyRef.current && autoSaveEnabledRef.current && targetFile) {
        const next = pendingSave(workspace, targetFile, merged.value, 'auto')
        queueSave(next, true)
      }
      return
    }

    // Navigation normally flushes first. This fallback starts the old draft
    // before loading the new file, so the shared lane can serialize both.
    if (fileChanged && previousFile && previousWorkspace === workspace && dirtyRef.current) {
      const oldText = readEditorText(editor, previousFile)
      queueSave(pendingSave(previousWorkspace, previousFile, oldText, 'manual'))
      void flush()
    }

    const scrollEl = editorContainerRef.current
    if (fileChanged && previousFile) {
      filePositionsRef.current.set(documentSaveKey(previousWorkspace, previousFile), { scrollTop: scrollEl?.scrollTop ?? 0 })
    }
    if (fileChanged && scrollEl) scrollEl.style.visibility = 'hidden'

    lastSyncedFileRef.current = fileName
    lastSyncedWorkspaceRef.current = workspace
    lastSyncedContentRef.current = content
    lastSyncedRevisionRef.current = revision
    if (currentKey) confirmedSnapshotsRef.current.set(currentKey, { content, revision })
    workspaceRef.current = workspace
    fileNameRef.current = fileName
    dirtyRef.current = false
    recoveryRef.current = null
    updateExternalConflict(null)
    applyExternalContent(fileName, content, {
      resetHistory: fileChanged ? previousFile !== null : contentChanged,
      preserveSelection: !fileChanged,
    })

    if (fileChanged && scrollEl) {
      const saved = fileName ? filePositionsRef.current.get(documentSaveKey(workspace, fileName)) : null
      requestAnimationFrame(() => {
        scrollEl.scrollTop = saved?.scrollTop ?? 0
        scrollEl.style.visibility = ''
      })
    }
  }, [applyExternalContent, archiveConflict, content, editor, editorContainerRef, fileName, flush, onExternalConflict, pendingSave, queueSave, resetQueuedSaves, revision, updateExternalConflict, workspace])

  useEffect(() => clearSaveStatusTimer, [clearSaveStatusTimer])

  useEffect(() => {
    if (autoSaveEnabled) return
    const targetFile = fileNameRef.current
    if (targetFile) cancelDocumentSave(workspaceRef.current, targetFile, true)
  }, [autoSaveEnabled, cancelDocumentSave])

  const saveEditorContent = useCallback(async (mode: 'manual' | 'auto'): Promise<boolean> => {
    const targetWorkspace = workspaceRef.current
    const targetFile = fileNameRef.current
    if (!editor || editor.isDestroyed || !targetFile) return true
    queueSave(pendingSave(targetWorkspace, targetFile, readEditorText(editor, targetFile), mode))
    await flush()
    return getSnapshot().status !== 'error'
  }, [editor, flush, getSnapshot, pendingSave, queueSave])

  const flushCurrentDraft = useCallback<EditorFlushHandler>(async () => {
    if (!dirtyRef.current) {
      if (!hasWork()) return true
      await flush()
      return getSnapshot().status !== 'error'
    }
    return saveEditorContent('manual')
  }, [flush, getSnapshot, hasWork, saveEditorContent])

  useLayoutEffect(() => {
    onFlushHandlerChange?.(flushCurrentDraft)
    return () => onFlushHandlerChange?.(null)
  }, [flushCurrentDraft, onFlushHandlerChange])

  // Last-resort boundary for owners that disappear without awaiting the
  // registered navigation flush. It still uses the same serialized lane.
  useEffect(() => () => {
    const targetWorkspace = workspaceRef.current
    const targetFile = fileNameRef.current
    if (!editor || editor.isDestroyed || !targetFile || !dirtyRef.current) return
    dirtyRef.current = false
    queueSave(pendingSave(targetWorkspace, targetFile, readEditorText(editor, targetFile), 'manual'))
    void flush()
  }, [editor, flush, pendingSave, queueSave])

  const handleSave = useCallback(async () => {
    await saveEditorContent('manual')
  }, [saveEditorContent])

  useEffect(() => {
    if (saveSignal === lastSaveSignalRef.current) return
    lastSaveSignalRef.current = saveSignal
    void handleSave()
  }, [handleSave, saveSignal])

  // External content is applied with emitUpdate:false, so only user edits arm
  // the after-delay save timer.
  useEffect(() => {
    if (!editor) return
    const handleUpdate = () => {
      const targetFile = fileNameRef.current
      if (!targetFile) return
      dirtyRef.current = true
      clearSaveStatusTimer()
      setSaveStatus('dirty')
      if (!autoSaveEnabledRef.current) return
      queueSave(pendingSave(
        workspaceRef.current,
        targetFile,
        readEditorText(editor, targetFile),
        'auto',
      ))
    }
    editor.on('update', handleUpdate)
    return () => {
      editor.off('update', handleUpdate)
    }
  }, [clearSaveStatusTimer, editor, pendingSave, queueSave])

  const loadExternalVersion = useCallback(() => {
    const conflict = externalConflictRef.current
    if (!conflict || !editor || editor.isDestroyed) return
    if (conflict.workspace !== workspaceRef.current || conflict.fileName !== fileNameRef.current) return
    dirtyRef.current = false
    recoveryRef.current = null
    cancelDocumentSave(conflict.workspace, conflict.fileName)
    updateExternalConflict(null)
    setExternalConflictSaving(false)
    setSaveStatus(null)
    applyExternalContent(conflict.fileName, conflict.externalContent, { resetHistory: true, preserveSelection: true })
  }, [applyExternalContent, cancelDocumentSave, editor, updateExternalConflict])

  const keepLocalVersion = useCallback(async () => {
    const conflict = externalConflictRef.current
    if (!conflict || externalConflictSaving) return
    setExternalConflictSaving(true)
    const saved = await saveEditorContent('manual')
    if (saved && externalConflictRef.current === conflict) updateExternalConflict(null)
    setExternalConflictSaving(false)
  }, [externalConflictSaving, saveEditorContent, updateExternalConflict])

  return {
    saveStatus,
    externalConflict,
    externalConflictSaving,
    handleSave,
    flushCurrentDraft,
    loadExternalVersion,
    keepLocalVersion,
  }
}
