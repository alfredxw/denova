import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import type { Editor } from '@tiptap/core'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'

import type { SaveStatus } from './EditorToolbar'
import {
  isTxtFile,
  normalizeEditorText,
  readEditorText,
} from './editorDocument'

export type EditorFlushHandler = () => Promise<boolean>

type PendingSave = {
  workspace: string
  workspaceEpoch: number
  documentEpoch: number
  fileName: string
  text: string
  mode: 'manual' | 'auto'
  save: (fileName: string, content: string) => Promise<boolean>
  allowConflictContent?: string
}

type QueuedSave = PendingSave & {
  waiters: Array<(saved: boolean) => void>
}

export type ExternalContentConflict = {
  workspace: string
  fileName: string
  externalContent: string
}

interface UseEditorDraftPersistenceOptions {
  workspace: string
  fileName: string | null
  content: string
  editor: Editor | null
  editorContainerRef: RefObject<HTMLDivElement | null>
  onSave: (fileName: string, content: string) => Promise<boolean>
  saveSignal: number
  autoSaveEnabled: boolean
  autoSaveDelayMs?: number
  applyExternalContent: (fileName: string | null, content: string, clearHistory: boolean) => void
  onExternalConflict?: (conflict: { fileName: string; localContent: string; externalContent: string }) => void
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
}

interface EditorDraftPersistence {
  saveStatus: SaveStatus | null
  externalConflict: ExternalContentConflict | null
  externalConflictSaving: boolean
  handleSave: () => Promise<void>
  loadExternalVersion: () => void
  keepLocalVersion: () => Promise<void>
}

const DEFAULT_AUTO_SAVE_DELAY_MS = 1500

function documentSaveKey(workspace: string, fileName: string): string {
  return `${workspace}\u0000${fileName}`
}

function normalizeAutoSaveDelayMs(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return DEFAULT_AUTO_SAVE_DELAY_MS
  }
  return Math.floor(value)
}

/** Owns the lifecycle and workspace/document guards for editor draft writes. */
export function useEditorDraftPersistence({
  workspace,
  fileName,
  content,
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
  const autoSaveTimer = useRef<number | null>(null)
  const saveStatusClearTimer = useRef<number | null>(null)
  const saveInFlightRef = useRef(false)
  const pendingSavesRef = useRef<QueuedSave[]>([])
  const scheduledSaveRef = useRef<PendingSave | null>(null)
  const workspaceRef = useRef(workspace)
  const workspaceEpochRef = useRef(0)
  const documentEpochsRef = useRef<Map<string, number>>(new Map())
  const lastSyncedFileRef = useRef<string | null>(null)
  const lastSyncedWorkspaceRef = useRef(workspace)
  const lastSyncedContentRef = useRef('')
  const localSaveContentRef = useRef<{ workspace: string; fileName: string; content: string } | null>(null)
  const dirtyRef = useRef(false)
  const externalConflictRef = useRef<ExternalContentConflict | null>(null)
  const fileNameRef = useRef<string | null>(fileName)
  const onSaveRef = useRef(onSave)
  const autoSaveEnabledRef = useRef(autoSaveEnabled)
  const autoSaveDelayMsRef = useRef(normalizeAutoSaveDelayMs(autoSaveDelayMs))
  const queueEditorSaveRef = useRef<(save: PendingSave) => Promise<boolean>>(async () => false)
  const lastSaveSignalRef = useRef(saveSignal)
  const filePositionsRef = useRef<Map<string, { scrollTop: number }>>(new Map())
  const resolvedAutoSaveDelayMs = normalizeAutoSaveDelayMs(autoSaveDelayMs)

  const currentDocumentEpoch = useCallback((targetWorkspace: string, targetFile: string) => (
    documentEpochsRef.current.get(documentSaveKey(targetWorkspace, targetFile)) ?? 0
  ), [])

  const cancelPendingDocumentSaves = useCallback((targetWorkspace: string, targetFile: string) => {
    const key = documentSaveKey(targetWorkspace, targetFile)
    documentEpochsRef.current.set(key, (documentEpochsRef.current.get(key) ?? 0) + 1)

    const scheduled = scheduledSaveRef.current
    if (scheduled?.workspace === targetWorkspace && scheduled.fileName === targetFile) {
      if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
      autoSaveTimer.current = null
      scheduledSaveRef.current = null
    }

    const retained: QueuedSave[] = []
    for (const pending of pendingSavesRef.current) {
      if (pending.workspace === targetWorkspace && pending.fileName === targetFile) {
        pending.waiters.forEach((resolve) => resolve(false))
      } else {
        retained.push(pending)
      }
    }
    pendingSavesRef.current = retained

    const localSave = localSaveContentRef.current
    if (localSave?.workspace === targetWorkspace && localSave.fileName === targetFile) {
      localSaveContentRef.current = null
    }
  }, [])

  const isPendingSaveCurrent = useCallback((save: PendingSave) => {
    if (save.workspace !== workspaceRef.current || save.workspaceEpoch !== workspaceEpochRef.current) return false
    if (save.documentEpoch !== currentDocumentEpoch(save.workspace, save.fileName)) return false
    const conflict = externalConflictRef.current
    const sameConflictDocument = conflict?.workspace === save.workspace && conflict.fileName === save.fileName
    if (sameConflictDocument) {
      return save.allowConflictContent !== undefined && save.allowConflictContent === conflict.externalContent
    }
    return save.allowConflictContent === undefined
  }, [currentDocumentEpoch])

  const updateExternalConflict = useCallback((next: ExternalContentConflict | null) => {
    externalConflictRef.current = next
    setExternalConflict(next)
  }, [])

  // A queued write is scoped to the workspace in which it was created. Switching
  // books invalidates every not-yet-started write instead of crossing API scope.
  useLayoutEffect(() => {
    if (workspaceRef.current === workspace) return
    workspaceRef.current = workspace
    workspaceEpochRef.current += 1
    if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
    autoSaveTimer.current = null
    scheduledSaveRef.current = null
    for (const pending of pendingSavesRef.current) {
      pending.waiters.forEach((resolve) => resolve(false))
    }
    pendingSavesRef.current = []
    localSaveContentRef.current = null
    externalConflictRef.current = null
    setExternalConflict(null)
    setExternalConflictSaving(false)
  }, [workspace])

  useEffect(() => {
    fileNameRef.current = fileName
    onSaveRef.current = onSave
  }, [fileName, onSave])

  useEffect(() => {
    autoSaveEnabledRef.current = autoSaveEnabled
    if (!autoSaveEnabled && autoSaveTimer.current) {
      window.clearTimeout(autoSaveTimer.current)
      autoSaveTimer.current = null
      scheduledSaveRef.current = null
    }
  }, [autoSaveEnabled])

  useEffect(() => {
    autoSaveDelayMsRef.current = resolvedAutoSaveDelayMs
  }, [resolvedAutoSaveDelayMs])

  useEffect(() => {
    const scheduled = scheduledSaveRef.current
    if (!scheduled || (scheduled.workspace === workspace && scheduled.fileName === fileName)) return
    if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
    autoSaveTimer.current = null
    scheduledSaveRef.current = null
    if (isPendingSaveCurrent(scheduled)) void queueEditorSaveRef.current(scheduled)
  }, [fileName, isPendingSaveCurrent, workspace])

  // Keep parent content and local drafts in sync without resetting the cursor
  // after a successful local save echo.
  useLayoutEffect(() => {
    if (!editor || editor.isDestroyed) return

    const previousFile = lastSyncedFileRef.current
    const previousWorkspace = lastSyncedWorkspaceRef.current
    const fileChanged = previousFile !== fileName || previousWorkspace !== workspace
    const contentChanged = lastSyncedContentRef.current !== content
    if (!fileChanged && !contentChanged) return

    const localSave = localSaveContentRef.current
    if (!fileChanged && contentChanged && localSave?.workspace === workspace && localSave.fileName === fileName && localSave.content === content) {
      lastSyncedContentRef.current = content
      localSaveContentRef.current = null
      if (readEditorText(editor, fileName) === content) dirtyRef.current = false
      updateExternalConflict(null)
      return
    }

    if (!fileChanged && contentChanged && dirtyRef.current) {
      const targetFile = fileName || ''
      cancelPendingDocumentSaves(workspace, targetFile)
      const conflict = { workspace, fileName: targetFile, externalContent: content }
      updateExternalConflict(conflict)
      setExternalConflictSaving(false)
      onExternalConflict?.({
        fileName: conflict.fileName,
        localContent: readEditorText(editor, fileName),
        externalContent: content,
      })
      return
    }

    // Parent navigation normally awaits the registered flush handler. Keep this
    // fallback for direct prop changes from other editor embedders.
    if (fileChanged && previousFile && previousWorkspace === workspace && dirtyRef.current) {
      if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
      autoSaveTimer.current = null
      scheduledSaveRef.current = null
      const text = readEditorText(editor, previousFile)
      const activeSave = localSaveContentRef.current
      const alreadySaving = activeSave?.workspace === previousWorkspace
        && activeSave.fileName === previousFile
        && activeSave.content === text
      const pending: PendingSave = {
        workspace: previousWorkspace,
        workspaceEpoch: workspaceEpochRef.current,
        documentEpoch: currentDocumentEpoch(previousWorkspace, previousFile),
        fileName: previousFile,
        text,
        mode: 'manual',
        save: onSaveRef.current,
      }
      if (!alreadySaving) void queueEditorSaveRef.current(pending)
    }

    const scrollEl = editorContainerRef.current
    if (fileChanged && previousFile) {
      filePositionsRef.current.set(documentSaveKey(previousWorkspace, previousFile), {
        scrollTop: scrollEl?.scrollTop ?? 0,
      })
    }
    if (fileChanged && scrollEl) scrollEl.style.visibility = 'hidden'

    lastSyncedFileRef.current = fileName
    lastSyncedWorkspaceRef.current = workspace
    lastSyncedContentRef.current = content
    localSaveContentRef.current = null
    dirtyRef.current = false
    updateExternalConflict(null)
    applyExternalContent(fileName, content, fileChanged ? previousFile !== null : contentChanged)

    if (fileChanged && scrollEl) {
      const saved = fileName ? filePositionsRef.current.get(documentSaveKey(workspace, fileName)) : null
      requestAnimationFrame(() => {
        scrollEl.scrollTop = saved ? saved.scrollTop : 0
        scrollEl.style.visibility = ''
      })
    }
  }, [applyExternalContent, cancelPendingDocumentSaves, content, editor, editorContainerRef, fileName, onExternalConflict, updateExternalConflict, workspace])

  const clearSaveStatusTimer = useCallback(() => {
    if (!saveStatusClearTimer.current) return
    window.clearTimeout(saveStatusClearTimer.current)
    saveStatusClearTimer.current = null
  }, [])

  const scheduleSaveStatusClear = useCallback((status: SaveStatus, delay: number) => {
    clearSaveStatusTimer()
    saveStatusClearTimer.current = window.setTimeout(() => {
      setSaveStatus((current) => current === status ? null : current)
      saveStatusClearTimer.current = null
    }, delay)
  }, [clearSaveStatusTimer])

  useEffect(() => clearSaveStatusTimer, [clearSaveStatusTimer])

  const persistEditorContent = useCallback(async (pending: PendingSave): Promise<boolean> => {
    const { workspace: targetWorkspace, fileName: targetFile, text, mode, save } = pending
    const activeDocument = workspaceRef.current === targetWorkspace && fileNameRef.current === targetFile
    if (activeDocument) {
      clearSaveStatusTimer()
      setSaveStatus(mode === 'auto' ? 'auto-saving' : 'manual-saving')
    }
    let ok = false
    try {
      ok = await save(targetFile, text)
    } catch (error) {
      console.error('编辑器保存任务失败', { workspace: targetWorkspace, path: targetFile, error })
    }
    const stillCurrent = workspaceRef.current === targetWorkspace
      && fileNameRef.current === targetFile
      && isPendingSaveCurrent(pending)
    const localSave = localSaveContentRef.current
    const matchesLocalSave = localSave?.workspace === targetWorkspace && localSave.fileName === targetFile && localSave.content === text
    if (stillCurrent) {
      if (ok) {
        lastSyncedContentRef.current = text
        if (matchesLocalSave) localSaveContentRef.current = null
        if (editor && !editor.isDestroyed && readEditorText(editor, targetFile) === text) dirtyRef.current = false
      } else if (matchesLocalSave) {
        localSaveContentRef.current = null
      }
    }
    const nextStatus: SaveStatus = ok ? (mode === 'auto' ? 'auto-saved' : 'manual-saved') : 'error'
    if (stillCurrent) setSaveStatus(nextStatus)
    if (mode === 'manual' && !ok && stillCurrent) toast.error(t('editor.saveFailed'))
    if (stillCurrent) scheduleSaveStatusClear(nextStatus, mode === 'auto' ? 1400 : 2000)
    return ok
  }, [clearSaveStatusTimer, editor, isPendingSaveCurrent, scheduleSaveStatusClear, t])

  const queueEditorSave = useCallback((save: PendingSave): Promise<boolean> => new Promise((resolve) => {
    if (!isPendingSaveCurrent(save)) {
      resolve(false)
      return
    }
    if (workspaceRef.current === save.workspace && fileNameRef.current === save.fileName) {
      localSaveContentRef.current = { workspace: save.workspace, fileName: save.fileName, content: save.text }
    }
    if (saveInFlightRef.current) {
      const existingIndex = pendingSavesRef.current.findIndex((pending) => (
        pending.workspace === save.workspace && pending.fileName === save.fileName && pending.save === save.save
      ))
      const existing = existingIndex >= 0 ? pendingSavesRef.current[existingIndex] : null
      const pending: QueuedSave = {
        ...(existing ?? save),
        ...save,
        mode: save.mode === 'manual' || existing?.mode === 'manual' ? 'manual' : 'auto',
        waiters: [...(existing?.waiters ?? []), resolve],
      }
      if (existingIndex >= 0) pendingSavesRef.current[existingIndex] = pending
      else pendingSavesRef.current.push(pending)
      if (workspaceRef.current === save.workspace && fileNameRef.current === save.fileName) {
        setSaveStatus(pending.mode === 'auto' ? 'auto-saving' : 'manual-saving')
      }
      return
    }

    const first: QueuedSave = { ...save, waiters: [resolve] }
    saveInFlightRef.current = true
    void (async () => {
      let nextSave: QueuedSave | undefined = first
      try {
        while (nextSave) {
          const ok = isPendingSaveCurrent(nextSave) ? await persistEditorContent(nextSave) : false
          nextSave.waiters.forEach((complete) => complete(ok))
          nextSave = pendingSavesRef.current.shift()
        }
      } finally {
        saveInFlightRef.current = false
      }
    })()
  }), [isPendingSaveCurrent, persistEditorContent])

  useEffect(() => {
    queueEditorSaveRef.current = queueEditorSave
  }, [queueEditorSave])

  const saveEditorContent = useCallback(async (mode: 'manual' | 'auto') => {
    if (!editor || !fileName) return
    if (externalConflictRef.current?.workspace === workspace && externalConflictRef.current.fileName === fileName) {
      if (mode === 'manual') toast.error(t('editor.externalConflict.saveBlocked'))
      return
    }
    await queueEditorSave({
      workspace,
      workspaceEpoch: workspaceEpochRef.current,
      documentEpoch: currentDocumentEpoch(workspace, fileName),
      fileName,
      text: readEditorText(editor, fileName),
      mode,
      save: onSave,
    })
  }, [currentDocumentEpoch, editor, fileName, onSave, queueEditorSave, t, workspace])

  const flushCurrentDraft = useCallback<EditorFlushHandler>(async () => {
    const targetWorkspace = workspaceRef.current
    const targetFile = fileNameRef.current
    if (!editor || editor.isDestroyed || !targetFile || !dirtyRef.current) return true
    if (externalConflictRef.current?.workspace === targetWorkspace && externalConflictRef.current.fileName === targetFile) {
      toast.error(t('editor.externalConflict.saveBlocked'))
      return false
    }
    if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
    autoSaveTimer.current = null
    scheduledSaveRef.current = null
    return queueEditorSave({
      workspace: targetWorkspace,
      workspaceEpoch: workspaceEpochRef.current,
      documentEpoch: currentDocumentEpoch(targetWorkspace, targetFile),
      fileName: targetFile,
      text: readEditorText(editor, targetFile),
      mode: 'manual',
      save: onSaveRef.current,
    })
  }, [currentDocumentEpoch, editor, queueEditorSave, t])

  useLayoutEffect(() => {
    onFlushHandlerChange?.(flushCurrentDraft)
    return () => onFlushHandlerChange?.(null)
  }, [flushCurrentDraft, onFlushHandlerChange])

  // Navigation awaits flushCurrentDraft. This is the last-resort guard for an
  // owning component disappearing before it can run that boundary.
  useEffect(() => () => {
    const targetWorkspace = workspaceRef.current
    const targetFile = fileNameRef.current
    if (!editor || editor.isDestroyed || !targetFile || !dirtyRef.current) return
    if (externalConflictRef.current?.workspace === targetWorkspace && externalConflictRef.current.fileName === targetFile) return
    if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
    autoSaveTimer.current = null
    scheduledSaveRef.current = null
    const text = readEditorText(editor, targetFile)
    const activeSave = localSaveContentRef.current
    if (activeSave?.workspace === targetWorkspace && activeSave.fileName === targetFile && activeSave.content === text) return
    dirtyRef.current = false
    void onSaveRef.current(targetFile, text).then((saved) => {
      if (!saved) console.error('编辑器卸载时保存草稿失败', { workspace: targetWorkspace, path: targetFile })
    }).catch((error) => {
      console.error('编辑器卸载时保存草稿异常', { workspace: targetWorkspace, path: targetFile, error })
    })
  }, [editor])

  const handleSave = useCallback(async () => {
    if (autoSaveTimer.current) {
      window.clearTimeout(autoSaveTimer.current)
      autoSaveTimer.current = null
    }
    scheduledSaveRef.current = null
    await saveEditorContent('manual')
  }, [saveEditorContent])

  useEffect(() => {
    if (saveSignal === lastSaveSignalRef.current) return
    lastSaveSignalRef.current = saveSignal
    void handleSave()
  }, [handleSave, saveSignal])

  // External content uses emitUpdate: false, so only local edits reach this path.
  useEffect(() => {
    if (!editor) return

    const handleUpdate = () => {
      const targetFile = fileNameRef.current
      if (!targetFile) return
      const targetWorkspace = workspaceRef.current
      dirtyRef.current = true
      clearSaveStatusTimer()
      setSaveStatus('dirty')
      if (externalConflictRef.current?.workspace === targetWorkspace && externalConflictRef.current.fileName === targetFile) {
        if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
        autoSaveTimer.current = null
        scheduledSaveRef.current = null
        return
      }
      if (!autoSaveEnabledRef.current) {
        if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
        autoSaveTimer.current = null
        scheduledSaveRef.current = null
        return
      }
      if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
      const text = isTxtFile(targetFile)
        ? normalizeEditorText(editor.getText({ blockSeparator: '\n' }))
        : normalizeEditorText(editor.getMarkdown())
      const scheduled: PendingSave = {
        workspace: targetWorkspace,
        workspaceEpoch: workspaceEpochRef.current,
        documentEpoch: currentDocumentEpoch(targetWorkspace, targetFile),
        fileName: targetFile,
        text,
        mode: 'auto',
        save: onSaveRef.current,
      }
      scheduledSaveRef.current = scheduled
      autoSaveTimer.current = window.setTimeout(() => {
        autoSaveTimer.current = null
        if (scheduledSaveRef.current !== scheduled) return
        scheduledSaveRef.current = null
        void queueEditorSaveRef.current(scheduled)
      }, autoSaveDelayMsRef.current)
    }

    editor.on('update', handleUpdate)
    return () => {
      editor.off('update', handleUpdate)
      if (autoSaveTimer.current) window.clearTimeout(autoSaveTimer.current)
    }
  }, [clearSaveStatusTimer, currentDocumentEpoch, editor])

  const loadExternalVersion = useCallback(() => {
    if (!externalConflict || !editor || editor.isDestroyed || externalConflict.workspace !== workspace || externalConflict.fileName !== fileName) return
    lastSyncedContentRef.current = externalConflict.externalContent
    localSaveContentRef.current = null
    dirtyRef.current = false
    updateExternalConflict(null)
    setExternalConflictSaving(false)
    setSaveStatus(null)
    applyExternalContent(fileName, externalConflict.externalContent, true)
  }, [applyExternalContent, editor, externalConflict, fileName, updateExternalConflict, workspace])

  const keepLocalVersion = useCallback(async () => {
    if (!externalConflict || !editor || editor.isDestroyed || externalConflictSaving) return
    if (externalConflict.workspace !== workspace || externalConflict.fileName !== fileName || !fileName) return
    const conflict = externalConflict
    const documentEpoch = currentDocumentEpoch(workspace, fileName)
    const text = readEditorText(editor, fileName)
    setExternalConflictSaving(true)
    const saved = await queueEditorSave({
      workspace,
      workspaceEpoch: workspaceEpochRef.current,
      documentEpoch,
      fileName,
      text,
      mode: 'manual',
      save: onSave,
      allowConflictContent: conflict.externalContent,
    })
    const activeConflict = externalConflictRef.current
    const sameConflict = activeConflict?.workspace === conflict.workspace
      && activeConflict.fileName === conflict.fileName
      && activeConflict.externalContent === conflict.externalContent
      && currentDocumentEpoch(workspace, fileName) === documentEpoch
    if (saved && sameConflict) {
      lastSyncedContentRef.current = text
      localSaveContentRef.current = null
      dirtyRef.current = false
      updateExternalConflict(null)
      setSaveStatus('manual-saved')
    }
    setExternalConflictSaving(false)
  }, [currentDocumentEpoch, editor, externalConflict, externalConflictSaving, fileName, onSave, queueEditorSave, updateExternalConflict, workspace])

  return {
    saveStatus,
    externalConflict,
    externalConflictSaving,
    handleSave,
    loadExternalVersion,
    keepLocalVersion,
  }
}
