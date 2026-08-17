import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from './client'
import type { LoreClassificationApplyRequest, LoreClassificationPreview, LoreClassificationPreviewRequest, LoreImagesGenerateRequest, LoreItem, LoreItemImageGenerateRequest, LoreTypeApplyResult, SSEEvent } from './types'
import { projectAPIPath } from './project-scope'

function lorePath(projectId: string, suffix: string): string {
  return projectAPIPath(projectId, `book/lore/${suffix.replace(/^\/+/, '')}`)
}

export async function previewLoreClassification(projectId: string, input: LoreClassificationPreviewRequest = {}): Promise<LoreClassificationPreview> {
  return requestJSON(lorePath(projectId, 'classification/preview'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function applyLoreClassification(projectId: string, input: LoreClassificationApplyRequest): Promise<LoreTypeApplyResult> {
  return requestJSON(lorePath(projectId, 'classification/apply'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function generateLoreItemImage(projectId: string, id: string, input: LoreItemImageGenerateRequest = {}): Promise<LoreItem> {
  return requestJSON(lorePath(projectId, `items/${encodeURIComponent(id)}/image/generate`), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function uploadLoreItemImage(projectId: string, id: string, file: File): Promise<LoreItem> {
	const form = new FormData()
	form.append('file', file, file.name)
	return requestJSON(lorePath(projectId, `items/${encodeURIComponent(id)}/image/upload`), {
		method: 'POST',
		body: form,
	})
}

export async function clearLoreItemImage(projectId: string, id: string): Promise<LoreItem> {
  return requestJSON(lorePath(projectId, `items/${encodeURIComponent(id)}/image`), { method: 'DELETE' })
}

export async function streamLoreImagesGenerate(projectId: string, input: LoreImagesGenerateRequest, signal?: AbortSignal): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI(lorePath(projectId, 'images/generate/stream'), {
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

export async function abortLoreImagesGenerate(projectId: string): Promise<void> {
  await requestJSON(lorePath(projectId, 'images/generate/abort'), { method: 'POST' })
}
