import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { AutosaveStatus } from '@/components/forms/autosave-status'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { rebaseTextWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'
import { rebaseText } from '@/lib/three-way-rebase'
import { readProjectFile, saveProjectFile, type ProjectFileDocument } from './api'

interface ProjectFileDraft {
  id: string
  content: string
  updated_at?: string
}

interface ProjectFileEditorOptions {
  projectId: string
  selectedPath: string | null
  autoSaveEnabled: boolean
  autoSaveDelayMs: number
  onSaved?: (path: string) => void | Promise<void>
}

interface ProjectFileEditorState {
  document: ProjectFileDocument | null
  draft: string
  loading: boolean
  error: string | null
  dirty: boolean
  status: AutosaveStatus
  autoSaveError: string | null
  setDraft: (value: string) => void
  flush: (force?: boolean) => Promise<boolean>
  retry: () => Promise<unknown>
  reload: () => Promise<void>
}

/** Serializes edits, CAS conflicts, and external rebases for one selected project file. */
export function useProjectFileEditor({
  projectId,
  selectedPath,
  autoSaveEnabled,
  autoSaveDelayMs,
  onSaved,
}: ProjectFileEditorOptions): ProjectFileEditorState {
  const [document, setDocument] = useState<ProjectFileDocument | null>(null)
  const [draft, setDraftState] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const documentRef = useRef(document)
  const draftRef = useRef(draft)
  const readVersionRef = useRef(0)
  documentRef.current = document
  draftRef.current = draft

  const autosaveDraft = useMemo<ProjectFileDraft | null>(() => (
    document?.kind === 'text' && document.editable
      ? { id: document.path, content: draft, updated_at: document.revision }
      : null
  ), [document, draft])

  const autosave = useResourceAutosave<ProjectFileDraft, ProjectFileDraft, ProjectFileDraft>({
    draft: autosaveDraft,
    active: Boolean(autoSaveEnabled && autosaveDraft && !loading),
    scopeKey: `${projectId}\u0000${selectedPath || ''}`,
    delayMs: autoSaveDelayMs,
    makePayload: (value) => value,
    baselineFromSaved: (saved) => saved,
    signature: projectFileDraftSignature,
    save: async (path, payload, baseRevision) => {
      const saved = await saveProjectFile(projectId, path, payload.content, baseRevision || '')
      return { ...payload, updated_at: saved.revision }
    },
    resolveConflict: async ({ error: saveError, baseline, draft: submitted, baseRevision }) => {
      if (!isRevisionConflict(saveError)) return null
      const latest = await readProjectFile(projectId, submitted.id)
      if (latest.kind !== 'text' || !latest.editable) return null
      const content = await rebaseTextWithRecovery({
        resource: 'project_file',
        scope: projectId,
        id: submitted.id,
        baseline: {
          revision: baseline?.updated_at || baseRevision,
          value: baseline?.content ?? latest.content ?? '',
        },
        local: { revision: baseRevision, value: submitted.content },
        external: { revision: latest.revision, value: latest.content ?? '' },
      })
      return {
        payload: { ...submitted, content, updated_at: latest.revision },
        baseRevision: latest.revision,
      }
    },
    onSaved: (saved, _mode, submitted) => {
      setDocument((current) => (
        current?.path === saved.id
          ? { ...current, content: saved.content, revision: saved.updated_at || current.revision }
          : current
      ))
      setDraftState((current) => (
        current === submitted.content ? saved.content : rebaseText(submitted.content, current, saved.content)
      ))
      setError(null)
      void onSaved?.(saved.id)
    },
    onAutoSaveError: (cause) => {
      console.error('[features/files/use-project-file-editor.ts] autosave failed', {
        projectId,
        path: selectedPath,
        cause,
      })
    },
  })

  const baselineDraft = useMemo<ProjectFileDraft | null>(() => (
    document?.kind === 'text' && document.editable
      ? { id: document.path, content: document.content ?? '', updated_at: document.revision }
      : null
  ), [document])

  useEffect(() => {
    autosave.resetBaseline(baselineDraft)
  }, [autosave.resetBaseline, baselineDraft])

  useEffect(() => {
    const version = ++readVersionRef.current
    if (!selectedPath) {
      setDocument(null)
      setDraftState('')
      setLoading(false)
      setError(null)
      return
    }
    setLoading(true)
    setError(null)
    setDocument(null)
    setDraftState('')
    void readProjectFile(projectId, selectedPath)
      .then((next) => {
        if (readVersionRef.current !== version) return
        setDocument(next)
        setDraftState(next.content ?? '')
      })
      .catch((cause) => {
        if (readVersionRef.current !== version) return
        setError(cause instanceof Error ? cause.message : String(cause))
      })
      .finally(() => {
        if (readVersionRef.current === version) setLoading(false)
      })
  }, [projectId, selectedPath])

  const reload = useCallback(async () => {
    const currentDocument = documentRef.current
    if (!selectedPath) return
    const version = ++readVersionRef.current
    try {
      const latest = await readProjectFile(projectId, selectedPath)
      if (readVersionRef.current !== version) return
      if (!currentDocument || currentDocument.path !== selectedPath) {
        setDocument(latest)
        setDraftState(latest.content ?? '')
        setError(null)
        return
      }
      if (latest.revision === currentDocument.revision) return
      if (latest.kind !== 'text' || currentDocument.kind !== 'text') {
        setDocument(latest)
        setDraftState(latest.content ?? '')
        setError(null)
        return
      }
      const merged = await rebaseTextWithRecovery({
        resource: 'project_file',
        scope: projectId,
        id: selectedPath,
        baseline: { revision: currentDocument.revision, value: currentDocument.content ?? '' },
        local: { revision: currentDocument.revision, value: draftRef.current },
        external: { revision: latest.revision, value: latest.content ?? '' },
      })
      if (readVersionRef.current !== version) return
      setDocument(latest)
      setDraftState(merged)
      setError(null)
    } catch (cause) {
      if (readVersionRef.current === version) setError(cause instanceof Error ? cause.message : String(cause))
      throw cause
    }
  }, [projectId, selectedPath])

  const dirty = Boolean(document?.kind === 'text' && draft !== (document.content ?? ''))
  const dirtyRef = useRef(dirty)
  const autosaveStatusRef = useRef(autosave.status)
  dirtyRef.current = dirty
  autosaveStatusRef.current = autosave.status
  const flush = useCallback(async (force = false) => {
    if (!documentRef.current || documentRef.current.kind !== 'text' || !documentRef.current.editable) return true
    try {
      const pending = autosave.flushPending()
      if (pending) await pending
      else if (force || dirtyRef.current || autosaveStatusRef.current === 'error') await autosave.saveNow('manual')
      setError(null)
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
      return false
    }
  }, [autosave.flushPending, autosave.saveNow])

  const status: AutosaveStatus = dirty && autosave.status === 'saved' ? 'pending' : autosave.status
  return {
    document,
    draft,
    loading,
    error,
    dirty,
    status,
    autoSaveError: autosave.error,
    setDraft: setDraftState,
    flush,
    retry: autosave.retry,
    reload,
  }
}

function projectFileDraftSignature(value: Partial<ProjectFileDraft>) {
  return value.content ?? ''
}
