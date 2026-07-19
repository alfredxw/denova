import {
  preserveAutosaveConflict,
  type AutosaveConflictSnapshot,
  type PreservedAutosaveConflict,
} from '@/lib/api-client/autosave-conflicts'
import { rebaseJSONValueWithConflicts, rebaseTextWithConflicts } from '@/lib/three-way-rebase'

export const AUTOSAVE_CONFLICT_PRESERVED_EVENT = 'nova:autosave-conflict-preserved'

interface RebaseWithRecoveryOptions<T> {
  resource: string
  scope: string
  id: string
  baseline: AutosaveConflictSnapshot & { value: T }
  local: AutosaveConflictSnapshot & { value: T }
  external: AutosaveConflictSnapshot & { value: T }
}

export interface AutosaveConflictPreservedDetail extends PreservedAutosaveConflict {
  resource: string
  scope: string
  resourceID: string
}

/** Three-way text merge that durably archives true overlaps before returning. */
export async function rebaseTextWithRecovery(options: RebaseWithRecoveryOptions<string>): Promise<string> {
  const result = rebaseTextWithConflicts(options.baseline.value, options.local.value, options.external.value)
  await preserveIfNeeded(options, result.value, result.conflicts.map(conflict => conflict.path))
  return result.value
}

/** Three-way structured merge that durably archives every overlapping leaf before returning. */
export async function rebaseJSONWithRecovery<T>(options: RebaseWithRecoveryOptions<T>): Promise<T> {
  const result = rebaseJSONValueWithConflicts(options.baseline.value, options.local.value, options.external.value)
  await preserveIfNeeded(options, result.value, result.conflicts.map(conflict => conflict.path))
  return result.value
}

async function preserveIfNeeded<T>(
  options: RebaseWithRecoveryOptions<T>,
  merged: T,
  conflictPaths: string[][],
): Promise<void> {
  if (conflictPaths.length === 0) return
  const saved = await preserveAutosaveConflict({
    resource: options.resource,
    scope: options.scope,
    id: options.id,
    base: options.baseline,
    local: options.local,
    external: options.external,
    merged: { revision: options.external.revision, value: merged },
    strategy: 'merge_non_overlap_prefer_local',
    conflict_paths: conflictPaths,
  })
  window.dispatchEvent(new CustomEvent<AutosaveConflictPreservedDetail>(AUTOSAVE_CONFLICT_PRESERVED_EVENT, {
    detail: {
      ...saved,
      resource: options.resource,
      scope: options.scope,
      resourceID: options.id,
    },
  }))
}
