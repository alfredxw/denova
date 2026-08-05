import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from './client'
import type { LoreClassificationApplyRequest, LoreClassificationPreview, LoreClassificationPreviewRequest, LoreImagesGenerateRequest, LoreItem, LoreItemImageGenerateRequest, LoreTypeApplyResult, SSEEvent } from './types'

const WORKSPACE_HEADER = 'X-Denova-Workspace'

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
