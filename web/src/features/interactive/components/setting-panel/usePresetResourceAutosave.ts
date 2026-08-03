import type { ResourceAutosaveOptions, ResourceConflictContext, ResourceConflictResolution } from '@/hooks/use-resource-autosave'
import { useResourceAutosave } from '@/hooks/use-resource-autosave'
import { rebaseJSONWithRecovery } from '@/lib/autosave/rebase-with-recovery'
import { isRevisionConflict } from '@/lib/revision-conflict'

type RevisionedPreset = { id: string; revision?: string; updated_at?: string }

/** Presets use the exact persisted-content revision for every CAS operation. */
export function presetResourceRevision(value: { revision?: string }): string | undefined {
  return value.revision
}

export function usePresetResourceAutosave<
  Draft extends RevisionedPreset,
  Payload,
  Saved extends { revision?: string; updated_at?: string },
>(options: ResourceAutosaveOptions<Draft, Payload, Saved>) {
  return useResourceAutosave({
    ...options,
    getRevision: options.getRevision ?? presetResourceRevision,
  })
}

interface PresetConflictIdentity {
  resource: string
  scope: string
}

/** Creates the shared reload/rebase policy used by every revisioned preset kind. */
export function createPresetConflictResolver<
  Draft extends RevisionedPreset,
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
    const latestRevision = presetResourceRevision(latest)
    if (!latestRevision) throw new Error(`Preset ${draft.id} has no content revision`)
    const rebased = await rebaseJSONWithRecovery({
      ...identity,
      id: draft.id,
      baseline: {
        revision: (baseline && presetResourceRevision(baseline)) || baseRevision || latestRevision,
        value: baseline ?? latest,
      },
      local: {
        revision: presetResourceRevision(draft) || baseRevision,
        value: draft,
      },
      external: {
        revision: latestRevision,
        value: latest,
      },
    })
    return {
      payload: makePayload(rebased),
      baseRevision: latestRevision,
    }
  }
}
export type { ResourceSaveMode as PresetResourceSaveMode } from '@/hooks/use-resource-autosave'
