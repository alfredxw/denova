import type { ResourceConflictContext, ResourceConflictResolution } from '@/hooks/use-resource-autosave'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'

export { useResourceAutosave as usePresetResourceAutosave }

interface PresetConflictIdentity {
  resource: string
  scope: string
}

/** Creates the shared reload/rebase policy used by every revisioned preset kind. */
export function createPresetConflictResolver<
  Draft extends { id: string; updated_at?: string },
  Payload,
>(
  load: () => Promise<Draft[]>,
  makePayload: (draft: Draft) => Payload,
  identity: PresetConflictIdentity,
) {
  return async ({
    error,
    baseline,
    draft,
    baseRevision,
  }: ResourceConflictContext<Draft, Payload>): Promise<ResourceConflictResolution<Payload> | null> => {
    if (!isRevisionConflict(error)) return null
    const latest = (await load()).find((item) => item.id === draft.id)
    if (!latest) throw new Error(`Preset ${draft.id} no longer exists`)
    const rebased = await rebaseJSONWithRecovery({
      ...identity,
      id: draft.id,
      baseline: {
        revision: baseline?.updated_at || baseRevision || latest.updated_at,
        value: baseline ?? latest,
      },
      local: {
        revision: draft.updated_at || baseRevision,
        value: draft,
      },
      external: {
        revision: latest.updated_at,
        value: latest,
      },
    })
    return {
      payload: makePayload(rebased),
      baseRevision: latest.updated_at,
    }
  }
}
export type { ResourceSaveMode as PresetResourceSaveMode } from '@/hooks/use-resource-autosave'
