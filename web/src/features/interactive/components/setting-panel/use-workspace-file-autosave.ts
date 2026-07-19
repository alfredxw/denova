import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { readFile, saveFile } from '@/lib/api'
import { rebaseTextWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'

export interface WorkspaceFileDraft {
  id: string
  content: string
  workspace: string
  updated_at?: string
}

interface WorkspaceFileAutosaveOptions {
  path: string
  content: string
  revision: string
  fileWorkspace: string
  active: boolean
  scopeKey: string
  onSaved?: (saved: WorkspaceFileDraft, submitted: WorkspaceFileDraft) => void
  onAutoSaveError?: (error: unknown) => void
}

/** Revision-aware autosave for configuration files stored in a workspace. */
export function useWorkspaceFileAutosave({
  path,
  content,
  revision,
  fileWorkspace,
  active,
  scopeKey,
  onSaved,
  onAutoSaveError,
}: WorkspaceFileAutosaveOptions) {
  return useResourceAutosave<WorkspaceFileDraft, WorkspaceFileDraft, WorkspaceFileDraft>({
    draft: fileWorkspace && revision
      ? { id: path, content, workspace: fileWorkspace, updated_at: revision }
      : null,
    active,
    scopeKey,
    makePayload: (file) => file,
    baselineFromSaved: (saved) => saved,
    signature: workspaceFileSignature,
    save: async (_id, file, baseRevision) => {
      const saved = await saveFile(path, file.content, baseRevision || '', file.workspace)
      return { ...file, updated_at: saved.revision || '' }
    },
    resolveConflict: async ({ error, baseline, draft: submitted, baseRevision }) => {
      if (!isRevisionConflict(error)) return null
      const latest = await readFile(path)
      const content = await rebaseTextWithRecovery({
        resource: 'workspace_file',
        scope: latest.workspace || submitted.workspace || scopeKey,
        id: submitted.id,
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
          content,
          workspace: latest.workspace || submitted.workspace,
          updated_at: latest.revision || '',
        },
        baseRevision: latest.revision || '',
      }
    },
    onSaved: (saved, _mode, submitted) => onSaved?.(saved, submitted),
    onAutoSaveError,
  })
}

function workspaceFileSignature(file: Partial<WorkspaceFileDraft>) {
  return JSON.stringify({ content: file.content || '', workspace: file.workspace || '' })
}
