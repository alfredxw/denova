import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from './client'
import type { LoreClassificationApplyRequest, LoreClassificationPreview, LoreClassificationPreviewRequest, LoreImagesGenerateRequest, LoreItem, LoreItemImageGenerateRequest, LoreItemInput, LoreTypeApplyResult, SSEEvent } from './types'

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

export async function updateLoreItem(id: string, item: Partial<LoreItemInput>, baseRevision?: string): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...item, base_revision: baseRevision } : item),
  })
}

export async function deleteLoreItem(id: string): Promise<void> {
  await requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function previewLoreClassification(input: LoreClassificationPreviewRequest = {}): Promise<LoreClassificationPreview> {
  return requestJSON('/api/lore/classification/preview', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function applyLoreClassification(input: LoreClassificationApplyRequest): Promise<LoreTypeApplyResult> {
  return requestJSON('/api/lore/classification/apply', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function generateLoreItemImage(id: string, input: LoreItemImageGenerateRequest = {}): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}/image/generate`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function clearLoreItemImage(id: string): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}/image`, { method: 'DELETE' })
}

export async function streamLoreImagesGenerate(input: LoreImagesGenerateRequest, signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI('/api/lore/images/generate/stream', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
    signal,
  })
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) {
    throw new Error('No response stream')
  }
  return parseSSEStream(res.body)
}

export async function abortLoreImagesGenerate(): Promise<void> {
  await requestJSON('/api/lore/images/generate/abort', { method: 'POST' })
}
