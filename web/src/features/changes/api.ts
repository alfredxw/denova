import { jsonHeaders, requestJSON } from '@/lib/api-client/client'
import { assertProjectScope, projectAPIPath } from '@/lib/api-client/project-scope'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThread,
  ReviewWorkspaceChangeRequest,
  WorkspaceChangeComment,
  WorkspaceChangeGroup,
  WorkspaceChangeGroupSummary,
  WorkspaceChangeMutationResult,
} from './types'

export interface ListProjectChangeGroupsOptions {
  status?: string
  path?: string
  runID?: string
  sessionID?: string
  reviewThreadID?: string
}

export async function listProjectChangeGroups(projectId: string, options: ListProjectChangeGroupsOptions = {}): Promise<WorkspaceChangeGroupSummary[]> {
  const params = new URLSearchParams()
  if (options.status) params.set('status', options.status)
  if (options.path) params.set('path', options.path)
  if (options.runID) params.set('run_id', options.runID)
  if (options.sessionID) params.set('session_id', options.sessionID)
  if (options.reviewThreadID) params.set('review_thread_id', options.reviewThreadID)
  const suffix = params.size ? `?${params.toString()}` : ''
  const data = await requestJSON<{ project_id: string; workspace: string; groups?: WorkspaceChangeGroupSummary[] }>(`${projectAPIPath(projectId, 'changes/groups')}${suffix}`)
  assertProjectScope(projectId, data.project_id, 'Project change groups')
  return Array.isArray(data.groups) ? data.groups : []
}

export async function getProjectChangeGroup(projectId: string, id: string): Promise<WorkspaceChangeGroup> {
  const data = await requestJSON<{ project_id: string; workspace: string; group: WorkspaceChangeGroup }>(projectAPIPath(projectId, `changes/groups/${encodeURIComponent(id)}`))
  assertProjectScope(projectId, data.project_id, 'Project change group')
  return data.group
}

export async function getProjectChangeReviewThread(projectId: string, id: string): Promise<ReviewThread> {
  const data = await requestJSON<{ project_id: string; workspace: string; review_thread: ReviewThread }>(projectAPIPath(projectId, `changes/review-threads/${encodeURIComponent(id)}`))
  assertProjectScope(projectId, data.project_id, 'Project change review thread')
  return normalizeReviewThread(data.review_thread)
}

/** Keeps the client boundary stable when Go encodes an empty slice as null. */
function normalizeReviewThread(thread: ReviewThread): ReviewThread {
  if (!thread || typeof thread.id !== 'string' || !thread.id.trim()) {
    throw new Error('Invalid workspace change review thread response')
  }
  const groups = Array.isArray(thread.groups) ? thread.groups : []
  const files = Array.isArray(thread.files) ? thread.files : []
  return {
    ...thread,
    latest_group_id: thread.latest_group_id || groups[groups.length - 1]?.id || thread.id,
    groups,
    comments: Array.isArray(thread.comments) ? thread.comments : [],
    files: files.map((file) => ({
      ...file,
      before_content: file.before_content ?? '',
      after_content: file.after_content ?? '',
      group_ids: Array.isArray(file.group_ids) ? file.group_ids : [],
      change_set_ids: Array.isArray(file.change_set_ids) ? file.change_set_ids : [],
      pending_edit_ids: Array.isArray(file.pending_edit_ids) ? file.pending_edit_ids : [],
      continuity: file.continuity || 'continuous',
    })),
  }
}

export async function reviewProjectChangeGroup(projectId: string, id: string, request: ReviewWorkspaceChangeRequest): Promise<WorkspaceChangeMutationResult> {
  const result = await requestJSON<WorkspaceChangeMutationResult>(projectAPIPath(projectId, `changes/groups/${encodeURIComponent(id)}/review`), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(request),
  })
  assertProjectScope(projectId, result.project_id, 'Project change review')
  return result
}

export async function undoProjectChangeGroup(projectId: string, id: string): Promise<WorkspaceChangeMutationResult> {
  const result = await requestJSON<WorkspaceChangeMutationResult>(projectAPIPath(projectId, `changes/groups/${encodeURIComponent(id)}/undo`), {
    method: 'POST',
    headers: jsonHeaders,
  })
  assertProjectScope(projectId, result.project_id, 'Project change undo')
  return result
}

export async function redoProjectChangeGroup(projectId: string, id: string): Promise<WorkspaceChangeMutationResult> {
  const result = await requestJSON<WorkspaceChangeMutationResult>(projectAPIPath(projectId, `changes/groups/${encodeURIComponent(id)}/redo`), {
    method: 'POST',
    headers: jsonHeaders,
  })
  assertProjectScope(projectId, result.project_id, 'Project change redo')
  return result
}

export async function createProjectChangeComment(projectId: string, request: CreateWorkspaceChangeCommentRequest): Promise<WorkspaceChangeComment> {
  const data = await requestJSON<{ project_id: string; workspace: string; comment: WorkspaceChangeComment }>(projectAPIPath(projectId, 'changes/comments'), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(request),
  })
  assertProjectScope(projectId, data.project_id, 'Project change comment')
  return { ...data.comment, workspace: data.workspace }
}

export async function updateProjectChangeComment(projectId: string, id: string, body: string): Promise<WorkspaceChangeComment> {
  const data = await requestJSON<{ project_id: string; workspace: string; comment: WorkspaceChangeComment }>(projectAPIPath(projectId, `changes/comments/${encodeURIComponent(id)}`), {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ body }),
  })
  assertProjectScope(projectId, data.project_id, 'Project change comment')
  return { ...data.comment, workspace: data.workspace }
}

export async function deleteProjectChangeComment(projectId: string, id: string): Promise<WorkspaceChangeComment> {
  const data = await requestJSON<{ project_id: string; workspace: string; comment: WorkspaceChangeComment }>(projectAPIPath(projectId, `changes/comments/${encodeURIComponent(id)}`), {
    method: 'DELETE',
  })
  assertProjectScope(projectId, data.project_id, 'Project change comment')
  return { ...data.comment, workspace: data.workspace }
}
