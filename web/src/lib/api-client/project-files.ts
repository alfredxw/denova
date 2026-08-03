import { jsonHeaders, requestJSON } from '@/lib/api-client/client'

export type ProjectFileEntryType = 'file' | 'dir'
export type ProjectFileDocumentKind = 'text' | 'image' | 'binary'

export interface ProjectFileEntry {
  name: string
  path: string
  type: ProjectFileEntryType
  ignored?: boolean
  symlink?: boolean
}

export type ProjectDirectoryChildrenState = 'complete' | 'partial'

export interface ProjectDirectoryPage {
  path: string
  revision: string
  entries: ProjectFileEntry[]
  children_state: ProjectDirectoryChildrenState
  continuation?: string
}

export interface ProjectFileTreeResolveTarget {
  id?: string
  path: string
  cursor?: string
}

export interface ProjectFileTreeResolveRequest {
  targets: ProjectFileTreeResolveTarget[]
  include_ignored?: boolean
  follow_single_child_directories?: boolean
  entry_budget?: number
}

export interface ProjectFileTreeResolveResult {
  id?: string
  path: string
  ok: boolean
  directories: ProjectDirectoryPage[]
  code?: string
  error?: string
}

export interface ProjectFileTreeResolveResponse {
  project_id: string
  results: ProjectFileTreeResolveResult[]
}

export interface ProjectFileDocument {
  project_id: string
  path: string
  content?: string
  revision: string
  kind: ProjectFileDocumentKind
  mime_type: string
  size: number
  editable: boolean
}

export interface ProjectFileSaveResult {
  project_id: string
  path: string
  revision: string
  changed: boolean
  message?: string
}

export type ProjectFileOperation =
  | { id?: string; kind: 'create'; path: string; type: ProjectFileEntryType; content?: string }
  | { id?: string; kind: 'delete'; path: string }
  | { id?: string; kind: 'rename'; path: string; new_name: string }
  | { id?: string; kind: 'copy' | 'move'; path: string; to: string }

export interface ProjectFileOperationResult {
  id?: string
  kind: ProjectFileOperation['kind']
  ok: boolean
  path?: string
  code?: string
  error?: string
}

function projectFilesURL(projectId: string) {
  return `/api/projects/${encodeURIComponent(projectId)}/files`
}

export function resolveProjectFileTree(
  projectId: string,
  request: ProjectFileTreeResolveRequest,
): Promise<ProjectFileTreeResolveResponse> {
  return requestJSON<ProjectFileTreeResolveResponse>(`${projectFilesURL(projectId)}/resolve`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(request),
  }).then((response) => ({
    ...response,
    results: (response.results ?? []).map((result) => ({
      ...result,
      directories: (result.directories ?? []).map((directory) => ({
        ...directory,
        entries: directory.entries ?? [],
      })),
    })),
  }))
}

export function readProjectFile(projectId: string, path: string): Promise<ProjectFileDocument> {
  const params = new URLSearchParams({ path })
  return requestJSON<ProjectFileDocument>(`${projectFilesURL(projectId)}/file?${params.toString()}`)
}

export function saveProjectFile(
  projectId: string,
  path: string,
  content: string,
  baseRevision: string,
): Promise<ProjectFileSaveResult> {
  return requestJSON<ProjectFileSaveResult>(`${projectFilesURL(projectId)}/file`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({ path, content, base_revision: baseRevision }),
  })
}

export function applyProjectFileOperations(
  projectId: string,
  operations: ProjectFileOperation[],
): Promise<ProjectFileOperationResult[]> {
  return requestJSON<{ results?: ProjectFileOperationResult[] }>(`${projectFilesURL(projectId)}/operations`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ operations }),
  }).then((response) => response.results ?? [])
}

export function projectFileAssetURL(projectId: string, path: string, revision = '') {
  const params = new URLSearchParams({ path })
  if (revision) params.set('revision', revision)
  return `${projectFilesURL(projectId)}/asset?${params.toString()}`
}
