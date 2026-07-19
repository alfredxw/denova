import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { getAutomations, updateAutomation } from '@/lib/api'
import type { AutomationTask, AutomationTaskUpdate } from '@/lib/api'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'
import { automationTaskKey } from './automation-catalog'
import {
  automationTaskDraftSignature,
  automationTaskUpdate,
  cloneAutomationTask,
  normalizeAutomationTaskShape,
} from './automation-task-draft'

type AutomationAutosaveDraft = AutomationTask & { id: string }

interface AutomationAutosaveOptions {
  activeId: string
  creating: boolean
  draft: AutomationTask
  tasks: AutomationTask[]
  workspace: string
  onSaved: (saved: AutomationTask, submitted: AutomationTask, submittedIsCurrent: boolean) => void
  onError: (error: unknown) => void
}

/** Owns the serialized autosave lane for an existing automation task. */
export function useAutomationAutosave({
  activeId,
  creating,
  draft,
  tasks,
  workspace,
  onSaved,
  onError,
}: AutomationAutosaveOptions) {
  const draftRef = useRef(draft)
  const onSavedRef = useRef(onSaved)
  const onErrorRef = useRef(onError)
  draftRef.current = draft
  onSavedRef.current = onSaved
  onErrorRef.current = onError

  const autosaveDraft = useMemo<AutomationAutosaveDraft | null>(() => {
    if (!activeId || creating) return null
    return { ...draft, id: activeId }
  }, [activeId, creating, draft])
  const taskScopeKey = useMemo(() => {
    if (!activeId) return 'automation:inactive'
    const target = draft.target
    return [
      'automation',
      activeId,
      target?.kind || draft.scope || 'unknown',
      target?.workspace_id || target?.workspace || '',
    ].join('\u0000')
  }, [activeId, draft.scope, draft.target])

  const autosave = useResourceAutosave<AutomationAutosaveDraft, AutomationTaskUpdate, AutomationTask>({
    draft: autosaveDraft,
    active: Boolean(activeId) && !creating,
    // The task owns its lane. Opening another book must not reset a global or
    // explicitly targeted task's pending definition save.
    scopeKey: taskScopeKey,
    makePayload: automationTaskUpdate,
    baselineFromSaved: (saved, submitted) => ({ ...submitted, ...saved, id: automationTaskKey(saved) || submitted.id }),
    signature: automationTaskDraftSignature,
    getRevision: (value) => value.revision,
    save: updateAutomation,
    resolveConflict: async ({ error, baseline, draft: submitted }) => {
      if (!isRevisionConflict(error) || !baseline) return null
      const latestTask = (await getAutomations())
        .find((task) => automationTaskKey(task) === submitted.id)
      if (!latestTask) return null
      const latest = { ...cloneAutomationTask(latestTask, workspace), id: submitted.id }
      const rebased = await rebaseJSONWithRecovery({
        resource: 'automation',
        scope: latest.target?.kind === 'workspace' ? latest.target.workspace || workspace : 'user',
        id: submitted.id,
        baseline: { revision: baseline.revision, value: baseline },
        local: { revision: submitted.revision, value: submitted },
        external: { revision: latest.revision, value: latest },
      })
      return {
        payload: automationTaskUpdate(rebased),
        baseRevision: latest.revision,
      }
    },
    onSaved: (saved, _mode, submitted) => {
      const normalized = normalizeAutomationTaskShape(saved, workspace)
      const submittedIsCurrent = automationTaskDraftSignature(draftRef.current) === automationTaskDraftSignature(submitted)
      onSavedRef.current(normalized, submitted, submittedIsCurrent)
    },
    onAutoSaveError: (cause) => onErrorRef.current(cause),
  })

  useEffect(() => {
    if (!activeId || creating) {
      autosave.resetBaseline(null)
      return
    }
    const baseline = tasks.find((task) => automationTaskKey(task) === activeId)
    if (baseline) autosave.resetBaseline({ ...cloneAutomationTask(baseline, workspace), id: activeId })
  }, [activeId, autosave.resetBaseline, creating, tasks, workspace])

  const flush = useCallback(async () => {
    try {
      await (autosave.flushPending() ?? autosave.saveNow('auto'))
      return true
    } catch (cause) {
      onErrorRef.current(cause)
      return false
    }
  }, [autosave.flushPending, autosave.saveNow])

  return {
    status: autosave.status,
    error: autosave.error,
    retry: autosave.retry,
    flush,
  }
}
