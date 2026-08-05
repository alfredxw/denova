import { useCallback, useState } from 'react'
import type { ResourceDirectorySection } from '@/components/resource-directory/types'
import type { PresetResourceKind } from '../../preset-ownership'

const PRESET_DIRECTORY_ORDER_STORAGE_KEY = 'nova.preset-directory-order'
const PRESET_DIRECTORY_ORDER_VERSION = 1
const PRESET_RESOURCE_KINDS: PresetResourceKind[] = ['teller', 'image', 'director', 'event', 'rule', 'actor-state']

export type PresetDirectoryOrder = Partial<Record<PresetResourceKind, string[]>>

/** 用户级前端偏好：全局预设使用一份排序，不随 Project 切换。 */
export function usePresetDirectoryOrder() {
  const [order, setOrder] = useState<PresetDirectoryOrder>(readPresetDirectoryOrder)

  const reorderItems = useCallback((kind: PresetResourceKind, visibleItemIds: string[], allItemIds: string[]) => {
    setOrder((currentOrder) => {
      const nextOrder: PresetDirectoryOrder = {
        ...currentOrder,
        [kind]: mergeVisiblePresetDirectoryOrder(allItemIds, currentOrder[kind], visibleItemIds),
      }
      writePresetDirectoryOrder(nextOrder)
      return nextOrder
    })
  }, [])

  return { order, reorderItems }
}

/** 已存储的 id 先按用户顺序排列，新增或未识别条目继续保持数据源顺序。 */
export function applyPresetDirectoryOrder(sections: ResourceDirectorySection[], order: PresetDirectoryOrder): ResourceDirectorySection[] {
  return sections.map((section) => ({
    ...section,
    items: orderKnownItems(section.items, order[section.id as PresetResourceKind]),
  }))
}

/** 在写作/游戏模式过滤后拖拽时，保留当前不可见条目在完整列表中的位置。 */
export function mergeVisiblePresetDirectoryOrder(allItemIds: string[], previousOrder: string[] | undefined, visibleItemIds: string[]): string[] {
  const canonical = orderKnownIDs(allItemIds, previousOrder)
  const known = new Set(canonical)
  const visible = uniqueStrings(visibleItemIds).filter((id) => known.has(id))
  const visibleSet = new Set(visible)
  let visibleIndex = 0
  return canonical.map((id) => visibleSet.has(id) ? visible[visibleIndex++] : id)
}

function orderKnownItems<T extends { id: string }>(items: T[], storedOrder: string[] | undefined): T[] {
  const itemByID = new Map(items.map((item) => [item.id, item]))
  return orderKnownIDs(items.map((item) => item.id), storedOrder)
    .map((id) => itemByID.get(id))
    .filter((item): item is T => Boolean(item))
}

function orderKnownIDs(ids: string[], storedOrder: string[] | undefined): string[] {
  const known = new Set(ids)
  const ordered = uniqueStrings(storedOrder || []).filter((id) => known.has(id))
  const orderedSet = new Set(ordered)
  return [...ordered, ...ids.filter((id) => !orderedSet.has(id))]
}

function readPresetDirectoryOrder(): PresetDirectoryOrder {
  if (typeof window === 'undefined') return {}
  try {
    const raw = window.localStorage.getItem(PRESET_DIRECTORY_ORDER_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as { version?: unknown; sections?: unknown }
    if (parsed.version !== PRESET_DIRECTORY_ORDER_VERSION || !parsed.sections || typeof parsed.sections !== 'object') return {}
    const sections = parsed.sections as Record<string, unknown>
    return Object.fromEntries(PRESET_RESOURCE_KINDS.flatMap((kind) => {
      const value = sections[kind]
      return Array.isArray(value) ? [[kind, uniqueStrings(value)]] : []
    })) as PresetDirectoryOrder
  } catch (error) {
    console.warn('[use-preset-directory-order] failed to read preset directory order', { error })
    return {}
  }
}

function writePresetDirectoryOrder(order: PresetDirectoryOrder) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(PRESET_DIRECTORY_ORDER_STORAGE_KEY, JSON.stringify({
      version: PRESET_DIRECTORY_ORDER_VERSION,
      sections: order,
    }))
  } catch (error) {
    console.warn('[use-preset-directory-order] failed to save preset directory order', { error })
  }
}

function uniqueStrings(values: unknown[]): string[] {
  return Array.from(new Set(values.filter((value): value is string => typeof value === 'string' && value.length > 0)))
}
