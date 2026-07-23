import { preserveAutosaveConflict, type PreservedAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import {
  AUTOSAVE_CONFLICT_PRESERVED_EVENT,
  type AutosaveConflictPreservedDetail,
} from '@/lib/autosave/rebase-with-recovery'

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

/** Persists both sides of an editor overlap before a local-preferred result can be written. */
export async function preserveEditorConflict({
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
