import { jsonHeaders, requestJSON } from './client'
import { assertProjectScope } from './project-scope'
import type { LoreItem, LoreItemInput, WorkspaceSummary } from './types'

export interface ProjectBookFileNode {
  name: string
  type: 'file' | 'dir'
  children?: ProjectBookFileNode[]
}

export interface ProjectBookSnapshot {
  project_id: string
  workspace: string
  tree: ProjectBookFileNode[]
  summary: WorkspaceSummary
}

interface ProjectBookTreeEnvelope {
  project_id: string
  workspace: string
  tree: ProjectBookFileNode[]
}

interface ProjectBookSummaryEnvelope {
  project_id: string
  workspace: string
  summary: WorkspaceSummary
}

interface ProjectLoreItemsEnvelope {
  project_id: string
  items?: LoreItem[]
}

interface ProjectLoreItemEnvelope {
  project_id: string
  item?: LoreItem
}

function projectBookURL(projectId: string) {
  return `/api/projects/${encodeURIComponent(projectId)}/book`
}

export async function getProjectBookSnapshot(projectId: string): Promise<ProjectBookSnapshot> {
  const snapshot = await requestJSON<ProjectBookSnapshot>(projectBookURL(projectId))
  assertProjectScope(projectId, snapshot.project_id)
  return {
    ...snapshot,
    tree: Array.isArray(snapshot.tree) ? snapshot.tree : [],
    summary: {
      ...snapshot.summary,
      chapters: Array.isArray(snapshot.summary?.chapters) ? snapshot.summary.chapters : [],
      chapter_plans: Array.isArray(snapshot.summary?.chapter_plans) ? snapshot.summary.chapter_plans : [],
    },
  }
}

export async function getProjectBookTree(projectId: string): Promise<ProjectBookFileNode[]> {
  const response = await requestJSON<ProjectBookTreeEnvelope>(`${projectBookURL(projectId)}/tree`)
  assertProjectScope(projectId, response.project_id)
  return Array.isArray(response.tree) ? response.tree : []
}

export async function getProjectBookSummary(projectId: string): Promise<WorkspaceSummary> {
  const response = await requestJSON<ProjectBookSummaryEnvelope>(`${projectBookURL(projectId)}/summary`)
  assertProjectScope(projectId, response.project_id)
  return {
    ...response.summary,
    chapters: Array.isArray(response.summary?.chapters) ? response.summary.chapters : [],
    chapter_plans: Array.isArray(response.summary?.chapter_plans) ? response.summary.chapter_plans : [],
  }
}

export async function setProjectChapterConfirmed(projectId: string, path: string, confirmed: boolean) {
  const response = await requestJSON<{ project_id: string; path: string; confirmed: boolean; message: string }>(
    `${projectBookURL(projectId)}/chapter-status`,
    {
      method: 'PATCH',
      headers: jsonHeaders,
      body: JSON.stringify({ path, confirmed }),
    },
  )
  assertProjectScope(projectId, response.project_id)
  return response
}

function projectLoreURL(projectId: string) {
  return `${projectBookURL(projectId)}/lore/items`
}

export async function getProjectLoreItems(projectId: string): Promise<LoreItem[]> {
  const data = await requestJSON<ProjectLoreItemsEnvelope>(projectLoreURL(projectId))
  assertProjectScope(projectId, data.project_id)
  return data.items ?? []
}

export async function createProjectLoreItem(projectId: string, item: Partial<LoreItemInput>): Promise<LoreItem> {
  const response = await requestJSON<ProjectLoreItemEnvelope>(projectLoreURL(projectId), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(item),
  })
  assertProjectScope(projectId, response.project_id)
  if (!response.item) throw new Error('Invalid Project Lore item response')
  return response.item
}

export async function updateProjectLoreItem(
  projectId: string,
  id: string,
  item: LoreItemInput,
  baseRevision?: string,
): Promise<LoreItem> {
  const response = await requestJSON<ProjectLoreItemEnvelope>(`${projectLoreURL(projectId)}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(baseRevision ? { ...item, base_revision: baseRevision } : item),
  })
  assertProjectScope(projectId, response.project_id)
  if (!response.item) throw new Error('Invalid Project Lore item response')
  return response.item
}

export async function deleteProjectLoreItem(projectId: string, id: string): Promise<void> {
  const response = await requestJSON<{ project_id: string }>(
    `${projectLoreURL(projectId)}/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  assertProjectScope(projectId, response.project_id)
}
