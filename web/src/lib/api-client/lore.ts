import { jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from './client'
import type { ChatMessage, LoreAgentResult, LoreItem, LoreItemInput, LoreVersion, SSEEvent } from './types'

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

export async function runLoreAgent(instruction: string, references: string[] = []): Promise<LoreAgentResult> {
  return requestJSON('/api/lore/agent', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ instruction, references }),
  })
}

export async function runLoreAgentStream(instruction: string, references: string[] = []): Promise<ReadableStream<SSEEvent>> {
  const res = await fetch('/api/lore/agent/stream', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ instruction, references }),
  })
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function getLoreAgentMessages(): Promise<ChatMessage[]> {
  return requestJSON('/api/lore/agent/messages')
}

export async function clearLoreAgentSession(): Promise<void> {
  await requestJSON('/api/lore/agent/clear', { method: 'POST' })
}

export async function getLoreVersions(): Promise<LoreVersion[]> {
  const data = await requestJSON<{ versions: LoreVersion[] }>('/api/lore/versions')
  return data.versions || []
}

export async function createLoreVersion(message: string): Promise<LoreVersion> {
  return requestJSON('/api/lore/versions', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ message }),
  })
}

export async function restoreLoreVersion(id: string): Promise<LoreItem[]> {
  const data = await requestJSON<{ items: LoreItem[] }>(`/api/lore/versions/${encodeURIComponent(id)}/restore`, {
    method: 'POST',
  })
  return data.items || []
}
