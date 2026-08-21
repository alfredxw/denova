import { jsonHeaders, requestJSON } from './client'
import type { LoreClassificationApplyRequest, LoreClassificationPreview, LoreClassificationPreviewRequest, LoreItem, LoreItemImageGenerateRequest, LoreTypeApplyResult } from './types'
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
