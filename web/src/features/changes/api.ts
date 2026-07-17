import { jsonHeaders, requestJSON } from '@/lib/api-client/client'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThread,
  ReviewWorkspaceChangeRequest,
  WorkspaceChangeComment,
  WorkspaceChangeGroup,
  WorkspaceChangeGroupSummary,
  WorkspaceChangeMutationResult,
} from './types'

export interface ListWorkspaceChangeGroupsOptions {
  status?: string
  path?: string
  runID?: string
  sessionID?: string
  reviewThreadID?: string
}

const WORKSPACE_HEADER = 'X-Denova-Workspace'

export async function listWorkspaceChangeGroups(workspace: string, options: ListWorkspaceChangeGroupsOptions = {}): Promise<WorkspaceChangeGroupSummary[]> {
  const params = new URLSearchParams()
  if (options.status) params.set('status', options.status)
  if (options.path) params.set('path', options.path)
  if (options.runID) params.set('run_id', options.runID)
  if (options.sessionID) params.set('session_id', options.sessionID)
  if (options.reviewThreadID) params.set('review_thread_id', options.reviewThreadID)
  const suffix = params.size ? `?${params.toString()}` : ''
  const data = await requestJSON<{ workspace?: string; groups?: WorkspaceChangeGroupSummary[] } | WorkspaceChangeGroupSummary[]>(`/api/workspace/change-groups${suffix}`, {
    headers: workspaceChangeHeaders(workspace),
  })
  return Array.isArray(data) ? data : (Array.isArray(data.groups) ? data.groups : [])
}

export async function getWorkspaceChangeGroup(workspace: string, id: string): Promise<WorkspaceChangeGroup> {
  const data = await requestJSON<{ workspace?: string; group?: WorkspaceChangeGroup } | WorkspaceChangeGroup>(`/api/workspace/change-groups/${encodeURIComponent(id)}`, {
    headers: workspaceChangeHeaders(workspace),
  })
  if ('group' in data && data.group) return data.group
  return data as WorkspaceChangeGroup
}

export async function getWorkspaceChangeReviewThread(workspace: string, id: string): Promise<ReviewThread> {
  const data = await requestJSON<{ workspace?: string; review_thread?: ReviewThread; thread?: ReviewThread } | ReviewThread>(`/api/workspace/change-review-threads/${encodeURIComponent(id)}`, {
    headers: workspaceChangeHeaders(workspace),
  })
  const thread = 'review_thread' in data && data.review_thread
    ? data.review_thread
    : 'thread' in data && data.thread
      ? data.thread
      : data as ReviewThread
  return normalizeReviewThread(thread)
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

export function reviewWorkspaceChangeGroup(workspace: string, id: string, request: ReviewWorkspaceChangeRequest): Promise<WorkspaceChangeMutationResult> {
  return requestJSON(`/api/workspace/change-groups/${encodeURIComponent(id)}/review`, {
    method: 'POST',
    headers: workspaceChangeHeaders(workspace, true),
    body: JSON.stringify(request),
  })
}

export function undoWorkspaceChangeGroup(workspace: string, id: string): Promise<WorkspaceChangeMutationResult> {
  return requestJSON(`/api/workspace/change-groups/${encodeURIComponent(id)}/undo`, {
    method: 'POST',
    headers: workspaceChangeHeaders(workspace, true),
  })
}

export function redoWorkspaceChangeGroup(workspace: string, id: string): Promise<WorkspaceChangeMutationResult> {
  return requestJSON(`/api/workspace/change-groups/${encodeURIComponent(id)}/redo`, {
    method: 'POST',
    headers: workspaceChangeHeaders(workspace, true),
  })
}

export async function createWorkspaceChangeComment(workspace: string, request: CreateWorkspaceChangeCommentRequest): Promise<WorkspaceChangeComment> {
  const data = await requestJSON<{ workspace?: string; comment?: WorkspaceChangeComment } | WorkspaceChangeComment>('/api/workspace/change-comments', {
    method: 'POST',
    headers: workspaceChangeHeaders(workspace, true),
    body: JSON.stringify(request),
  })
  if ('comment' in data && data.comment) return { ...data.comment, workspace: data.workspace }
  return data as WorkspaceChangeComment
}

export async function updateWorkspaceChangeComment(workspace: string, id: string, body: string): Promise<WorkspaceChangeComment> {
  const data = await requestJSON<{ workspace?: string; comment?: WorkspaceChangeComment } | WorkspaceChangeComment>(`/api/workspace/change-comments/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: workspaceChangeHeaders(workspace, true),
    body: JSON.stringify({ body }),
  })
  if ('comment' in data && data.comment) return { ...data.comment, workspace: data.workspace }
  return data as WorkspaceChangeComment
}

export async function deleteWorkspaceChangeComment(workspace: string, id: string): Promise<WorkspaceChangeComment> {
  const data = await requestJSON<{ workspace?: string; comment?: WorkspaceChangeComment } | WorkspaceChangeComment>(`/api/workspace/change-comments/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: workspaceChangeHeaders(workspace),
  })
  if ('comment' in data && data.comment) return { ...data.comment, workspace: data.workspace }
  return data as WorkspaceChangeComment
}

export async function resolveWorkspaceChangeComment(workspace: string, id: string, resolved: boolean): Promise<WorkspaceChangeComment> {
  const data = await requestJSON<{ workspace?: string; comment?: WorkspaceChangeComment } | WorkspaceChangeComment>(`/api/workspace/change-comments/${encodeURIComponent(id)}/resolve`, {
    method: 'POST',
    headers: workspaceChangeHeaders(workspace, true),
    body: JSON.stringify({ resolved }),
  })
  if ('comment' in data && data.comment) return { ...data.comment, workspace: data.workspace }
  return data as WorkspaceChangeComment
}

function workspaceChangeHeaders(workspace: string, includeJSON = false): HeadersInit {
  return {
    ...(includeJSON ? jsonHeaders : {}),
    [WORKSPACE_HEADER]: encodeURIComponent(workspace),
  }
}
