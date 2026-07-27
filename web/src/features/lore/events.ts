export const LORE_UPDATED_EVENT = 'nova:lore-updated'

export interface LoreUpdatedDetail {
  ids?: string[]
  /** Legacy event payload retained while older import flows are still mounted. */
  item_ids?: string[]
  source?: string
}

export function notifyLoreUpdated(detail: LoreUpdatedDetail = {}) {
  window.dispatchEvent(
    new CustomEvent<LoreUpdatedDetail>(LORE_UPDATED_EVENT, { detail }),
  )
}
