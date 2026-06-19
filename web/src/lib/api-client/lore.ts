import { jsonHeaders, requestJSON } from './client'
import type { LoreItem, LoreItemInput } from './types'

export async function getLoreItems(): Promise<LoreItem[]> {
  const data = await requestJSON<{ items: LoreItem[] }>('/api/lore/items')
  return data.items || []
}

export async function createLoreItem(item: Partial<LoreItemInput>): Promise<LoreItem> {
  return requestJSON('/api/lore/items', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(item),
  })
}

export async function updateLoreItem(id: string, item: Partial<LoreItemInput>): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(item),
  })
}

export async function deleteLoreItem(id: string): Promise<void> {
  await requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
