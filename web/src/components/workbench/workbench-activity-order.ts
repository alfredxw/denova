import type { ReactNode } from 'react'

export type ActivityItemID = 'writing' | 'story' | 'lore' | 'teller' | 'versions' | 'books' | 'agentchat' | 'skills' | 'agents' | 'automations' | 'trajectory'
export type ActivityOrderScope = 'ide' | 'interactive'

export interface ActivityItem {
  id: ActivityItemID
  label: string
  onClick: () => void
  active: boolean
  icon: ReactNode
}

const LEGACY_ACTIVITY_ORDER_STORAGE_KEY = 'nova.activity.order.v1'
const LEGACY_SCOPED_ACTIVITY_ORDER_STORAGE_KEYS: Record<ActivityOrderScope, string> = {
  ide: 'nova.activity.order.ide.v1',
  interactive: 'nova.activity.order.interactive.v1',
}
const ACTIVITY_ORDER_STORAGE_KEYS: Record<ActivityOrderScope, string> = {
  ide: 'nova.activity.order.ide.v2',
  interactive: 'nova.activity.order.interactive.v2',
}
const DEFAULT_IDE_ACTIVITY_ORDER: ActivityItemID[] = ['writing', 'agentchat', 'trajectory', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']
const DEFAULT_INTERACTIVE_ACTIVITY_ORDER: ActivityItemID[] = ['story', 'agentchat', 'trajectory', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations']

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

export function defaultActivityOrderForScope(scope: ActivityOrderScope) {
  return scope === 'interactive' ? DEFAULT_INTERACTIVE_ACTIVITY_ORDER : DEFAULT_IDE_ACTIVITY_ORDER
}

export function readStoredActivityOrders(): Record<ActivityOrderScope, ActivityItemID[]> {
  return {
    ide: readStoredActivityOrder('ide'),
    interactive: readStoredActivityOrder('interactive'),
  }
}

export function storeActivityOrder(scope: ActivityOrderScope, order: ActivityItemID[]) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ACTIVITY_ORDER_STORAGE_KEYS[scope], JSON.stringify(order))
  cleanupLegacyActivityOrderStorage()
}

export function cleanupLegacyActivityOrderStorage() {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(LEGACY_ACTIVITY_ORDER_STORAGE_KEY)
  window.localStorage.removeItem(LEGACY_SCOPED_ACTIVITY_ORDER_STORAGE_KEYS.ide)
  window.localStorage.removeItem(LEGACY_SCOPED_ACTIVITY_ORDER_STORAGE_KEYS.interactive)
}

export function isActivityItemID(value: string): value is ActivityItemID {
  return DEFAULT_IDE_ACTIVITY_ORDER.includes(value as ActivityItemID) || DEFAULT_INTERACTIVE_ACTIVITY_ORDER.includes(value as ActivityItemID)
}

function readStoredActivityOrder(scope: ActivityOrderScope): ActivityItemID[] {
  const defaultOrder = defaultActivityOrderForScope(scope)
  if (typeof window === 'undefined') return defaultOrder
  try {
    const raw = window.localStorage.getItem(ACTIVITY_ORDER_STORAGE_KEYS[scope])
    if (!raw) return defaultOrder
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return defaultOrder
    const validIDs = new Set(defaultOrder)
    const stored = parsed.filter((id): id is ActivityItemID => validIDs.has(id))
    return insertMissingActivityItems(stored, defaultOrder)
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
