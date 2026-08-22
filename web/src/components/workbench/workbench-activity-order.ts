import type { ReactNode } from 'react'

export type ActivityItemID = 'writing' | 'story' | 'lore' | 'teller' | 'versions' | 'books' | 'agentchat' | 'skills' | 'agents' | 'automations' | 'trajectory'
export type ActivityOrderScope = 'workspace'

export interface ActivityItem {
  id: ActivityItemID
  label: string
  onClick: () => void
  active: boolean
  icon: ReactNode
}

const ACTIVITY_ORDER_STORAGE_KEY = 'nova.activity.order.workspace.v1'
const ACTIVITY_HIDDEN_STORAGE_KEY = 'nova.activity.hidden.workspace.v1'
const DEFAULT_ACTIVITY_ORDER: ActivityItemID[] = ['writing', 'story', 'agentchat', 'trajectory', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']

export function sortActivityItems(items: ActivityItem[], order: ActivityItemID[], defaultOrder: ActivityItemID[]) {
  const orderIndex = new Map<ActivityItemID, number>()
  order.forEach((id, index) => orderIndex.set(id, index))
  const defaultIndex = new Map<ActivityItemID, number>()
  defaultOrder.forEach((id, index) => defaultIndex.set(id, index))
  return [...items].sort((a, b) => {
    const aIndex = orderIndex.get(a.id) ?? defaultOrder.length + (defaultIndex.get(a.id) ?? 0)
    const bIndex = orderIndex.get(b.id) ?? defaultOrder.length + (defaultIndex.get(b.id) ?? 0)
    return aIndex - bIndex
  })
}

export function mergeVisibleActivityOrder(visibleIDs: ActivityItemID[], currentOrder: ActivityItemID[], defaultOrder: ActivityItemID[]) {
  const visibleSet = new Set(visibleIDs)
  const hiddenIDs = currentOrder.filter((id) => !visibleSet.has(id))
  const knownIDs = new Set([...visibleIDs, ...hiddenIDs])
  const missingIDs = defaultOrder.filter((id) => !knownIDs.has(id))
  return [...visibleIDs, ...hiddenIDs, ...missingIDs]
}

export function defaultActivityOrderForScope(_scope: ActivityOrderScope) {
  return DEFAULT_ACTIVITY_ORDER
}

export function readStoredActivityOrders(): Record<ActivityOrderScope, ActivityItemID[]> {
  return {
    workspace: readStoredActivityOrder(),
  }
}

export function storeActivityOrder(_scope: ActivityOrderScope, order: ActivityItemID[]) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ACTIVITY_ORDER_STORAGE_KEY, JSON.stringify(order))
}

export function readStoredHiddenActivityIDs(): ActivityItemID[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(ACTIVITY_HIDDEN_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const hiddenIDs = [...new Set(parsed.filter((id): id is ActivityItemID => typeof id === 'string' && isActivityItemID(id)))]
    // Corrupt or manually edited storage must not remove every primary destination.
    return hiddenIDs.length === DEFAULT_ACTIVITY_ORDER.length
      ? hiddenIDs.filter((id) => id !== DEFAULT_ACTIVITY_ORDER[0])
      : hiddenIDs
  } catch {
    return []
  }
}

export function storeHiddenActivityIDs(ids: ActivityItemID[]) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ACTIVITY_HIDDEN_STORAGE_KEY, JSON.stringify(ids))
}

export function isActivityItemID(value: string): value is ActivityItemID {
  return DEFAULT_ACTIVITY_ORDER.includes(value as ActivityItemID)
}

function readStoredActivityOrder(): ActivityItemID[] {
  const defaultOrder = DEFAULT_ACTIVITY_ORDER
  if (typeof window === 'undefined') return defaultOrder
  try {
    const raw = window.localStorage.getItem(ACTIVITY_ORDER_STORAGE_KEY)
    if (!raw) return defaultOrder
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return defaultOrder
    const validIDs = new Set(defaultOrder)
    const storedIDs = parsed.filter((id): id is ActivityItemID => validIDs.has(id))
    return insertMissingActivityItems(storedIDs, defaultOrder)
  } catch {
    return defaultOrder
  }
}

/** Preserve the user's relative order while placing newly introduced items by their defaults. */
function insertMissingActivityItems(stored: ActivityItemID[], defaultOrder: ActivityItemID[]) {
  const result = [...stored]
  for (const id of defaultOrder) {
    if (result.includes(id)) continue
    const defaultIndex = defaultOrder.indexOf(id)
    const precedingID = defaultOrder.slice(0, defaultIndex).reverse().find((candidate) => result.includes(candidate))
    if (precedingID) {
      result.splice(result.indexOf(precedingID) + 1, 0, id)
      continue
    }
    const followingID = defaultOrder.slice(defaultIndex + 1).find((candidate) => result.includes(candidate))
    if (followingID) result.splice(result.indexOf(followingID), 0, id)
    else result.push(id)
  }
  return result
}
