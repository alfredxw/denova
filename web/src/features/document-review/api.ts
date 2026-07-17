import { jsonHeaders, requestJSON } from '@/lib/api-client/client'
import type {
  CreateDocumentCommentRequest,
  DocumentReviewComment,
  DocumentReviewMutationResult,
  DocumentReviewThread,
} from './types'

const WORKSPACE_HEADER = 'X-Denova-Workspace'

interface ReviewEnvelope {
  workspace?: string
  review_thread?: DocumentReviewThread
  comment?: DocumentReviewComment
}

export async function getDocumentReview(workspace: string): Promise<DocumentReviewThread> {
  const data = await requestJSON<ReviewEnvelope>('/api/workspace/document-review', {
    headers: documentReviewHeaders(workspace),
  })
  return normalizeThread(data.review_thread)
}

export async function createDocumentComment(workspace: string, request: CreateDocumentCommentRequest): Promise<DocumentReviewMutationResult> {
  const data = await requestJSON<ReviewEnvelope>('/api/workspace/document-comments', {
    method: 'POST',
    headers: documentReviewHeaders(workspace, true),
    body: JSON.stringify(request),
  })
  return normalizeMutation(data)
}

export async function updateDocumentComment(workspace: string, id: string, body: string): Promise<DocumentReviewMutationResult> {
  const data = await requestJSON<ReviewEnvelope>(`/api/workspace/document-comments/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: documentReviewHeaders(workspace, true),
    body: JSON.stringify({ body }),
  })
  return normalizeMutation(data)
}

export async function deleteDocumentComment(workspace: string, id: string): Promise<DocumentReviewMutationResult> {
  const data = await requestJSON<ReviewEnvelope>(`/api/workspace/document-comments/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: documentReviewHeaders(workspace),
  })
  return normalizeMutation(data)
}

function normalizeMutation(data: ReviewEnvelope): DocumentReviewMutationResult {
  if (!data.comment?.id) throw new Error('Invalid document review comment response')
  return {
    workspace: data.workspace || '',
    reviewThread: normalizeThread(data.review_thread),
    comment: data.comment,
  }
}

function normalizeThread(thread: DocumentReviewThread | undefined): DocumentReviewThread {
  return {
    ...(thread || { id: '' }),
    id: thread?.id || '',
    comments: Array.isArray(thread?.comments) ? thread.comments.filter((comment) => !comment.deleted) : [],
  }
}

function documentReviewHeaders(workspace: string, includeJSON = false): HeadersInit {
  return {
    ...(includeJSON ? jsonHeaders : {}),
    [WORKSPACE_HEADER]: encodeURIComponent(workspace),
  }
}
