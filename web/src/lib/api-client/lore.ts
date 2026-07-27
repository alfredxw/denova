import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from './client'
import type { LoreClassificationApplyRequest, LoreClassificationPreview, LoreClassificationPreviewRequest, LoreImagesGenerateRequest, LoreItem, LoreItemImageGenerateRequest, LoreItemInput, LoreTypeApplyResult, SSEEvent } from './types'

const WORKSPACE_HEADER = 'X-Denova-Workspace'

export async function getLoreItems(workspace: string): Promise<LoreItem[]> {
  const data = await requestJSON<{ items: LoreItem[] }>('/api/lore/items', { headers: loreHeaders(workspace) })
  return data.items || []
}

export async function createLoreItem(workspace: string, item: Partial<LoreItemInput>): Promise<LoreItem> {
  return requestJSON('/api/lore/items', {
    method: 'POST',
    headers: loreHeaders(workspace, true),
    body: JSON.stringify(item),
  })
}

export async function updateLoreItem(workspace: string, id: string, item: Partial<LoreItemInput>, baseRevision?: string): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: loreHeaders(workspace, true),
    body: JSON.stringify(baseRevision ? { ...item, base_revision: baseRevision } : item),
  })
}

export async function deleteLoreItem(workspace: string, id: string): Promise<void> {
  await requestJSON(`/api/lore/items/${encodeURIComponent(id)}`, { method: 'DELETE', headers: loreHeaders(workspace) })
}

export async function previewLoreClassification(workspace: string, input: LoreClassificationPreviewRequest = {}): Promise<LoreClassificationPreview> {
  return requestJSON('/api/lore/classification/preview', {
    method: 'POST',
    headers: loreHeaders(workspace, true),
    body: JSON.stringify(input),
  })
}

export async function applyLoreClassification(workspace: string, input: LoreClassificationApplyRequest): Promise<LoreTypeApplyResult> {
  return requestJSON('/api/lore/classification/apply', {
    method: 'POST',
    headers: loreHeaders(workspace, true),
    body: JSON.stringify(input),
  })
}

export async function generateLoreItemImage(workspace: string, id: string, input: LoreItemImageGenerateRequest = {}): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}/image/generate`, {
    method: 'POST',
    headers: loreHeaders(workspace, true),
    body: JSON.stringify(input),
  })
}

export async function clearLoreItemImage(workspace: string, id: string): Promise<LoreItem> {
  return requestJSON(`/api/lore/items/${encodeURIComponent(id)}/image`, { method: 'DELETE', headers: loreHeaders(workspace) })
}

export async function streamLoreImagesGenerate(workspace: string, input: LoreImagesGenerateRequest, signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI('/api/lore/images/generate/stream', {
    method: 'POST',
    headers: loreHeaders(workspace, true),
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

export async function abortLoreImagesGenerate(workspace: string): Promise<void> {
  await requestJSON('/api/lore/images/generate/abort', { method: 'POST', headers: loreHeaders(workspace) })
}

function loreHeaders(workspace: string, includeJSON = false): HeadersInit {
  return {
    ...(includeJSON ? jsonHeaders : {}),
    [WORKSPACE_HEADER]: encodeURIComponent(workspace),
  }
}
