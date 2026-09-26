import { jsonHeaders, requestJSON } from './client'
import type {
  LoreClassificationApplyRequest,
  LoreClassificationPreview,
  LoreClassificationPreviewRequest,
  LoreItem,
  LoreAsset,
  LoreMaterialMutation,
  LoreItemImageGenerateRequest,
  LoreItemSpeechGenerateRequest,
  LoreTypeApplyResult,
} from './types'
import { projectAPIPath } from './project-scope'

function lorePath(projectId: string, suffix: string): string {
  return projectAPIPath(projectId, `book/lore/${suffix.replace(/^\/+/, '')}`)
}

export async function previewLoreClassification(
  projectId: string,
  input: LoreClassificationPreviewRequest = {},
): Promise<LoreClassificationPreview> {
  return requestJSON(lorePath(projectId, 'classification/preview'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function applyLoreClassification(
  projectId: string,
  input: LoreClassificationApplyRequest,
): Promise<LoreTypeApplyResult> {
  return requestJSON(lorePath(projectId, 'classification/apply'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function generateLoreItemImage(
  projectId: string,
  id: string,
  input: LoreItemImageGenerateRequest = {},
): Promise<LoreItem> {
  return requestJSON(lorePath(projectId, `items/${encodeURIComponent(id)}/image/generate`), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export async function uploadLoreItemMaterial(
  projectId: string,
  id: string,
  file: File,
): Promise<LoreItem> {
  const form = new FormData()
  form.append('file', file, file.name)
  return requestJSON(lorePath(projectId, `items/${encodeURIComponent(id)}/materials/upload`), {
    method: 'POST',
    body: form,
  })
}

export function generateLoreItemSpeech(
  projectId: string,
  id: string,
  input: LoreItemSpeechGenerateRequest,
): Promise<LoreItem> {
  return requestJSON(lorePath(projectId, `items/${encodeURIComponent(id)}/speech/generate`), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
}

export function getLoreAssets(projectId: string): Promise<LoreAsset[]> {
  return requestJSON(lorePath(projectId, 'assets'))
}
export function mutateLoreMaterial(
  projectId: string,
  id: string,
  mutation: LoreMaterialMutation,
): Promise<LoreItem> {
  return requestJSON(lorePath(projectId, `items/${encodeURIComponent(id)}/materials`), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(mutation),
  })
}
