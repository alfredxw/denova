import { MessageSquareText, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export const MAX_REVIEW_FEEDBACK_COMMENT_COUNT = 256
export const MAX_REVIEW_FEEDBACK_CONTEXT_BYTES = 256 * 1024

export interface ReviewFeedbackSelection {
  source?: 'workspace_change' | 'document'
  reviewThreadId: string
  comments: ReviewFeedbackComment[]
}

export type ReviewFeedbackBatch = readonly ReviewFeedbackSelection[]

export interface ReviewFeedbackComment {
  id: string
  body: string
  path?: string
  group_id?: string
  change_set_id?: string
  edit_id?: string
  hunk_id?: string
  review_path?: string
  review_line?: number
  created_at?: string
  updated_at?: string
  anchor?: {
    kind?: string
    side?: string
    encoding?: string
    revision?: string
    start?: number
    end?: number
    quote?: string
    prefix?: string
    suffix?: string
    display_quote?: string
  }
}

interface ReviewFeedbackTrayProps {
  feedback: ReviewFeedbackBatch
  onRemove: (selection: ReviewFeedbackSelection, commentID: string) => void
}

/** Pending inline review comments selected for the next Agent turn. */
export function ReviewFeedbackTray({ feedback, onRemove }: ReviewFeedbackTrayProps) {
  const { t } = useTranslation()
  const selectedComments = feedback.flatMap((selection) => selection.comments.map((comment) => ({ selection, comment })))
  if (!selectedComments.length) return null
  const contextTooLarge = reviewFeedbackContextBytes(feedback) > MAX_REVIEW_FEEDBACK_CONTEXT_BYTES

  return (
    <div className="mb-2 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2" data-review-feedback-tray>
      <div className="mb-1.5 flex items-center gap-2 text-[11px] font-medium text-[var(--nova-text-muted)]">
        <MessageSquareText className="h-3.5 w-3.5" />
        <span>{t('changes.feedback.selected', { count: selectedComments.length })}</span>
      </div>
      {selectedComments.length > MAX_REVIEW_FEEDBACK_COMMENT_COUNT && (
        <p role="alert" className="mb-1.5 text-[10px] leading-4 text-[var(--nova-danger)]">
          {t('changes.feedback.tooMany', { maximum: MAX_REVIEW_FEEDBACK_COMMENT_COUNT })}
        </p>
      )}
      {contextTooLarge && (
        <p role="alert" className="mb-1.5 text-[10px] leading-4 text-[var(--nova-danger)]">
          {t('changes.feedback.tooLarge')}
        </p>
      )}
      <div className="flex max-h-24 flex-wrap gap-1.5 overflow-y-auto">
        {selectedComments.map(({ selection, comment }) => (
          <span
            key={`${selection.source || 'workspace_change'}:${selection.reviewThreadId}:${comment.id}`}
            className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-bg)] px-2 py-1 text-[11px] text-[var(--nova-text)]"
          >
            <span className="max-w-56 truncate" title={comment.body}>
              {t(selection.source === 'document' ? 'changes.feedback.source.document' : 'changes.feedback.source.diff')}
              {' · '}{comment.review_path || comment.path || comment.change_set_id || t('changes.comment')}
              {comment.review_line !== undefined ? ` · ${t('changes.feedback.line', { line: comment.review_line })}` : ''}
              {' — '}{comment.body}
            </span>
            <button
              type="button"
              onClick={() => onRemove(selection, comment.id)}
              className="rounded p-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              aria-label={t('changes.feedback.remove')}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
      </div>
    </div>
  )
}

/** Mirrors the trusted server payload with a small safety allowance for its prompt wrapper. */
export function reviewFeedbackContextBytes(feedback: ReviewFeedbackBatch): number {
  const payload = JSON.stringify(feedback.map((selection) => ({
    source: selection.source || 'workspace_change',
    review_thread_id: selection.reviewThreadId,
    comments: selection.comments.map((comment) => ({
      comment_id: comment.id,
      group_id: comment.group_id,
      ...(comment.change_set_id ? { change_set_id: comment.change_set_id } : {}),
      ...(comment.edit_id ? { edit_id: comment.edit_id } : {}),
      ...(comment.hunk_id ? { hunk_id: comment.hunk_id } : {}),
      ...(comment.review_path || comment.path ? { path: comment.review_path || comment.path } : {}),
      body: comment.body,
      anchor: compactAnchor(comment.anchor),
    })),
  }))).replace(/[<>&\u2028\u2029]/g, (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`)
  return new TextEncoder().encode(payload).length + 2 * 1024
}

export function reviewFeedbackCommentCount(feedback: ReviewFeedbackBatch): number {
  return feedback.reduce((count, selection) => count + selection.comments.length, 0)
}

function compactAnchor(anchor: ReviewFeedbackComment['anchor']): Record<string, string | number> {
  if (!anchor) return {}
  return Object.fromEntries(Object.entries(anchor).filter(([, value]) => value !== undefined && value !== '' && value !== 0))
}
