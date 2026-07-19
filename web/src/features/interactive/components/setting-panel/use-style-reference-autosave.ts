import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { rebaseTextWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'
import { readStyleReferenceFile, updateStyleReferenceFile } from '../../api'
import type { StyleReferenceFileDocument } from '../../types'

interface StyleReferenceAutosaveDraft {
  id: string
  updated_at?: string
  path: string
  content: string
}

interface StyleReferenceAutosaveSaved extends StyleReferenceAutosaveDraft {
  document: StyleReferenceFileDocument
}

interface StyleReferenceAutosaveOptions {
  document: StyleReferenceFileDocument | null
  content: string
  active: boolean
  onSaved: (document: StyleReferenceFileDocument, submittedContent: string) => void
  onError: (message: string) => void
}

function styleReferenceSignature(value: Partial<StyleReferenceAutosaveDraft>) {
  return `${value.path || ''}\u0000${value.content || ''}`
}

/** Revision-aware autosave controller for an already-created shared style reference. */
export function useStyleReferenceAutosave({ document, content, active, onSaved, onError }: StyleReferenceAutosaveOptions) {
  const onErrorRef = useRef(onError)
  onErrorRef.current = onError
  const hasDocument = Boolean(document)
  const draft = useMemo<StyleReferenceAutosaveDraft | null>(() => document ? {
    id: document.reference.display_path,
    updated_at: document.revision,
    path: document.reference.display_path,
    content,
  } : null, [content, document])

  const autosave = useResourceAutosave<StyleReferenceAutosaveDraft, StyleReferenceAutosaveDraft, StyleReferenceAutosaveSaved>({
    draft,
    active: active && Boolean(document),
    scopeKey: document?.reference.display_path || '',
    valid: Boolean(content.trim()),
    makePayload: (value) => value,
    baselineFromSaved: (saved, submitted) => ({
      ...submitted,
      content: saved.document.content,
      updated_at: saved.document.revision,
    }),
    signature: styleReferenceSignature,
    save: async (_id, payload, baseRevision) => {
      const saved = await updateStyleReferenceFile({
        path: payload.path,
        content: payload.content,
        base_revision: baseRevision || payload.updated_at || '',
      })
      return {
        ...payload,
        document: saved,
        updated_at: saved.revision,
      }
    },
    resolveConflict: async ({ error, baseline, draft: submitted, baseRevision }) => {
      if (!isRevisionConflict(error)) return null
      const latest = await readStyleReferenceFile(submitted.path)
      const rebasedContent = await rebaseTextWithRecovery({
        resource: 'style_reference',
        scope: 'user',
        id: submitted.path,
        baseline: {
          revision: baseline?.updated_at || baseRevision || latest.revision,
          value: baseline?.content ?? latest.content,
        },
        local: {
          revision: submitted.updated_at || baseRevision,
          value: submitted.content,
        },
        external: {
          revision: latest.revision,
          value: latest.content,
        },
      })
      return {
        payload: {
          ...submitted,
          content: rebasedContent,
          updated_at: latest.revision,
        },
        baseRevision: latest.revision,
      }
    },
    onSaved: (saved, _mode, submitted) => onSaved(saved.document, submitted.content),
  })

  useEffect(() => {
    autosave.resetBaseline(document ? {
      id: document.reference.display_path,
      updated_at: document.revision,
      path: document.reference.display_path,
      content: document.content,
    } : null)
  }, [autosave.resetBaseline, document])

  const flush = useCallback(async (force = false) => {
    if (!hasDocument) return true
    try {
      const pending = autosave.flushPending()
      if (pending) {
        await pending
      } else if (force || autosave.status === 'error') {
        await autosave.saveNow('manual')
      }
      return true
    } catch (error) {
      onErrorRef.current(error instanceof Error ? error.message : String(error))
      return false
    }
  }, [autosave.flushPending, autosave.saveNow, autosave.status, hasDocument])

  return {
    status: autosave.status,
    error: autosave.error,
    retry: autosave.retry,
    flush,
  }
}
