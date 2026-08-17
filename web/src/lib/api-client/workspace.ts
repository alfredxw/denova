import { jsonHeaders, requestJSON } from './client'
import { projectAPIPath } from './project-scope'
import type {
  CharacterCardImportResult,
  CharacterCardPreview,
  WorkspaceReplaceResult,
  WorkspaceSearchResult,
} from './types'

export const MISSING_WORKSPACE_REVISION = 'missing'

export async function switchWorkspace(path: string): Promise<{ workspace: string; message: string }> {
  return requestJSON('/api/workspace/switch', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  })
}

export async function getCurrentWorkspace(): Promise<{ workspace: string; project_id: string; has_state: boolean }> {
  return requestJSON('/api/workspace/current')
}

export async function searchWorkspace(projectId: string, query: string, limit = 100, options: { regex?: boolean } = {}): Promise<WorkspaceSearchResult[]> {
  const params = new URLSearchParams({ q: query, limit: String(limit) })
  if (options.regex) params.set('regex', '1')
  const data = await requestJSON<{ results: WorkspaceSearchResult[] }>(`${projectAPIPath(projectId, 'workspace/search')}?${params.toString()}`)
  return Array.isArray(data.results) ? data.results : []
}

export async function replaceWorkspace(projectId: string, req: { query: string; replacement: string; regex: boolean }): Promise<WorkspaceReplaceResult> {
  return requestJSON(projectAPIPath(projectId, 'workspace/replace'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
}

export async function previewCharacterCard(file: File): Promise<CharacterCardPreview> {
  const form = new FormData()
  form.append('file', file)
  return requestJSON('/api/imports/character-card/preview', {
    method: 'POST',
    body: form,
  })
}

export async function importCharacterCard(
  file: File,
  options: { targetMode: 'current' | 'new_book'; projectId?: string; bookTitle?: string; userCharacterName?: string; loreClassification?: 'heuristic' | 'semantic' },
): Promise<CharacterCardImportResult> {
  const form = new FormData()
  form.append('file', file)
  if (options.bookTitle) form.append('book_title', options.bookTitle)
  if (options.userCharacterName) form.append('user_character_name', options.userCharacterName)
  if (options.loreClassification) form.append('lore_classification', options.loreClassification)
  const path = options.targetMode === 'new_book'
    ? '/api/books/import-character-card'
    : projectAPIPath(options.projectId || '', 'book/import-character-card')
  return requestJSON(path, {
    method: 'POST',
    body: form,
  })
}
