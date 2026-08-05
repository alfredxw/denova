import { jsonHeaders, requestJSON } from '@/lib/api-client/client'
import { assertProjectScope } from '@/lib/api-client/project-scope'
import type {
  CreateDocumentCommentRequest,
  DocumentReviewComment,
  DocumentReviewMutationResult,
  DocumentReviewThread,
} from './types'

interface ReviewEnvelope {
  project_id: string
  workspace?: string
  review_thread?: DocumentReviewThread
  comment?: DocumentReviewComment
}

export async function getDocumentReview(projectId: string): Promise<DocumentReviewThread> {
  const data = await requestJSON<ReviewEnvelope>(`${projectReviewURL(projectId)}/document-review`)
  assertProjectScope(projectId, data.project_id, 'Document review')
  return normalizeThread(data.review_thread)
}

export async function createDocumentComment(projectId: string, request: CreateDocumentCommentRequest): Promise<DocumentReviewMutationResult> {
  const data = await requestJSON<ReviewEnvelope>(`${projectReviewURL(projectId)}/document-comments`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(request),
  })
  return normalizeMutation(projectId, data)
}

export async function updateDocumentComment(projectId: string, id: string, body: string): Promise<DocumentReviewMutationResult> {
  const data = await requestJSON<ReviewEnvelope>(`${projectReviewURL(projectId)}/document-comments/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: jsonHeaders,
    body: JSON.stringify({ body }),
  })
  return normalizeMutation(projectId, data)
}

export async function deleteDocumentComment(projectId: string, id: string): Promise<DocumentReviewMutationResult> {
  const data = await requestJSON<ReviewEnvelope>(`${projectReviewURL(projectId)}/document-comments/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  return normalizeMutation(projectId, data)
}

function normalizeMutation(projectId: string, data: ReviewEnvelope): DocumentReviewMutationResult {
  assertProjectScope(projectId, data.project_id, 'Document review')
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

function projectReviewURL(projectId: string) {
  return `/api/projects/${encodeURIComponent(projectId)}/book`
}
