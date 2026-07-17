export type DocumentReviewAnchorKind = 'text-range' | 'text-block'

export interface DocumentReviewAnchor {
  kind: DocumentReviewAnchorKind
  encoding: 'utf8-bytes-v1'
  revision: string
  /** UTF-8 byte offsets in the canonical Markdown file. */
  start: number
  end: number
  quote: string
  prefix?: string
  suffix?: string
  display_quote: string
  /** TipTap positions are same-revision UI hints, not canonical anchors. */
  editor_from?: number
  editor_to?: number
}

export interface DocumentReviewComment {
  id: string
  thread_id: string
  path: string
  body: string
  anchor: DocumentReviewAnchor
  created_at: string
  updated_at: string
  deleted?: boolean
}

export interface DocumentReviewThread {
  id: string
  created_at?: string
  updated_at?: string
  comments: DocumentReviewComment[]
}

export interface CreateDocumentCommentRequest {
  path: string
  body: string
  anchor: DocumentReviewAnchor
}

export interface DocumentReviewMutationResult {
  workspace: string
  reviewThread: DocumentReviewThread
  comment: DocumentReviewComment
}
