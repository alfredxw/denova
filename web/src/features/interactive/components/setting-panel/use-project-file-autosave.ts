import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { readProjectFile, saveProjectFile } from '@/lib/api'
import { rebaseTextWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'

export interface ProjectFileDraft {
  id: string
  content: string
  project_id: string
  updated_at?: string
}

interface ProjectFileAutosaveOptions {
  projectId: string
  path: string
  content: string
  revision: string
  fileProjectId: string
  active: boolean
  onSaved?: (saved: ProjectFileDraft, submitted: ProjectFileDraft) => void
  onAutoSaveError?: (error: unknown) => void
}

/** Revision-aware autosave for a configuration file owned by one explicit Project. */
export function useProjectFileAutosave({
  projectId,
  path,
  content,
  revision,
  fileProjectId,
  active,
  onSaved,
  onAutoSaveError,
}: ProjectFileAutosaveOptions) {
  return useResourceAutosave<ProjectFileDraft, ProjectFileDraft, ProjectFileDraft>({
    draft: projectId && fileProjectId === projectId && revision
      ? { id: path, content, project_id: projectId, updated_at: revision }
      : null,
    active,
    scopeKey: projectId,
    makePayload: (file) => file,
    baselineFromSaved: (saved) => saved,
    signature: projectFileSignature,
    save: async (_id, file, baseRevision) => {
      const saved = await saveProjectFile(file.project_id, path, file.content, baseRevision || '')
      return { ...file, project_id: saved.project_id, updated_at: saved.revision || '' }
    },
    resolveConflict: async ({ error, baseline, draft: submitted, baseRevision }) => {
      if (!isRevisionConflict(error)) return null
      const latest = await readProjectFile(submitted.project_id, path)
      const latestContent = latest.content || ''
      const content = await rebaseTextWithRecovery({
        resource: 'project_file',
        scope: submitted.project_id,
        id: submitted.id,
        baseline: {
          revision: baseline?.updated_at || baseRevision || latest.revision,
          value: baseline?.content ?? latestContent,
        },
        local: {
          revision: submitted.updated_at || baseRevision,
          value: submitted.content,
        },
        external: {
          revision: latest.revision,
          value: latestContent,
        },
      })
      return {
        payload: {
          ...submitted,
          content,
          project_id: latest.project_id,
          updated_at: latest.revision || '',
        },
        baseRevision: latest.revision || '',
      }
    },
    onSaved: (saved, _mode, submitted) => onSaved?.(saved, submitted),
    onAutoSaveError,
  })
}

function projectFileSignature(file: Partial<ProjectFileDraft>) {
  return JSON.stringify({ content: file.content || '', project_id: file.project_id || '' })
}
