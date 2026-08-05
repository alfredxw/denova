export const LORE_UPDATED_EVENT = 'nova:lore-updated'

export interface LoreUpdatedDetail {
  /** Stable resource scope. Consumers must ignore events from other projects. */
  projectId: string
  ids?: string[]
  source?: string
}

export function notifyLoreUpdated(detail: LoreUpdatedDetail) {
  if (!detail.projectId) return
  window.dispatchEvent(
    new CustomEvent<LoreUpdatedDetail>(LORE_UPDATED_EVENT, { detail }),
  )
}
