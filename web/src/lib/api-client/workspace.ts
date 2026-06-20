import { jsonHeaders, requestJSON } from './client'
import type {
  CharacterCardImportResult,
  CharacterCardPreview,
  CopyMoveRequest,
  CreateFileRequest,
  FileOperationResult,
  RenameRequest,
  WorkspaceSearchResult,
  WorkspaceSummary,
} from './types'

export async function getStatus(): Promise<{ has_state: boolean; context: string }> {
  return requestJSON('/api/status')
}

export async function switchWorkspace(path: string): Promise<{ workspace: string; message: string }> {
  return requestJSON('/api/workspace/switch', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  })
}

export async function getCurrentWorkspace(): Promise<{ workspace: string; has_state: boolean }> {
  return requestJSON('/api/workspace/current')
}

export async function getWorkspaceSummary(): Promise<WorkspaceSummary> {
  const summary = await requestJSON<WorkspaceSummary>('/api/workspace/summary')
  return {
    ...summary,
    chapters: Array.isArray(summary.chapters) ? summary.chapters : [],
    chapter_plans: Array.isArray(summary.chapter_plans) ? summary.chapter_plans : [],
  }
}

export async function getWorkspaceTree(): Promise<unknown[]> {
  const data = await requestJSON<unknown[]>('/api/workspace/tree')
  return Array.isArray(data) ? data : []
}

export async function readFile(path: string): Promise<{ path: string; content: string }> {
  return requestJSON(`/api/workspace/file?path=${encodeURIComponent(path)}`)
}

export async function searchWorkspace(query: string, limit = 100): Promise<WorkspaceSearchResult[]> {
  const params = new URLSearchParams({ q: query, limit: String(limit) })
  const data = await requestJSON<{ results: WorkspaceSearchResult[] }>(`/api/workspace/search?${params.toString()}`)
  return Array.isArray(data.results) ? data.results : []
}

export async function saveFile(path: string, content: string): Promise<{ message: string }> {
  return requestJSON('/api/workspace/file', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path, content }),
  })
}

export async function createWorkspaceItem(req: CreateFileRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/create', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
}

export async function deleteWorkspaceItem(path: string): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/delete', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  })
}

export async function renameWorkspaceItem(req: RenameRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/rename', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
}

export async function copyWorkspaceItem(req: CopyMoveRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/copy', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
}

export async function moveWorkspaceItem(req: CopyMoveRequest): Promise<FileOperationResult> {
  return requestJSON('/api/workspace/move', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(req),
  })
}

export async function previewCharacterCard(file: File): Promise<CharacterCardPreview> {
  const form = new FormData()
  form.append('file', file)
  return requestJSON('/api/workspace/import-character-card/preview', {
    method: 'POST',
    body: form,
  })
}

export async function importCharacterCard(
  file: File,
  options: { targetMode?: 'current' | 'new_book'; bookTitle?: string; userCharacterName?: string } = {},
): Promise<CharacterCardImportResult> {
  const form = new FormData()
  form.append('file', file)
  if (options.targetMode) form.append('target_mode', options.targetMode)
  if (options.bookTitle) form.append('book_title', options.bookTitle)
  if (options.userCharacterName) form.append('user_character_name', options.userCharacterName)
  return requestJSON('/api/workspace/import-character-card', {
    method: 'POST',
    body: form,
  })
}
