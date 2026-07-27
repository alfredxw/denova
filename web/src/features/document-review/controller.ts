import type {
  CreateDocumentCommentRequest,
  DocumentReviewComment,
} from './types'

/** Shared mutation surface used by every reviewable text editor. */
export interface DocumentReviewController {
  comments: DocumentReviewComment[]
  onCreate: (
    request: CreateDocumentCommentRequest,
  ) => Promise<DocumentReviewComment>
  onUpdate: (
    comment: DocumentReviewComment,
    body: string,
  ) => Promise<DocumentReviewComment>
  onDelete: (comment: DocumentReviewComment) => Promise<DocumentReviewComment>
}

/** A nonce makes repeated navigation to the same comment observable. */
export interface DocumentReviewNavigationIntent {
  commentID: string
  nonce: number
}
