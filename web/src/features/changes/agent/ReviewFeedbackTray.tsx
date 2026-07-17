import { MessageSquareText, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { WorkspaceChangeComment } from '../types'

export const MAX_REVIEW_FEEDBACK_COMMENT_COUNT = 256
export const MAX_REVIEW_FEEDBACK_CONTEXT_BYTES = 256 * 1024

export interface ReviewFeedbackSelection {
  reviewThreadId: string
  comments: WorkspaceChangeComment[]
}

interface ReviewFeedbackTrayProps {
  feedback: ReviewFeedbackSelection
  onRemove: (commentID: string) => void
}

/** Pending inline review comments selected for the next Agent turn. */
export function ReviewFeedbackTray({ feedback, onRemove }: ReviewFeedbackTrayProps) {
  const { t } = useTranslation()
  if (!feedback.comments.length) return null
  const contextTooLarge = reviewFeedbackContextBytes(feedback) > MAX_REVIEW_FEEDBACK_CONTEXT_BYTES

  return (
    <div className="mb-2 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2" data-review-feedback-tray>
      <div className="mb-1.5 flex items-center gap-2 text-[11px] font-medium text-[var(--nova-text-muted)]">
        <MessageSquareText className="h-3.5 w-3.5" />
        <span>{t('changes.feedback.selected', { count: feedback.comments.length })}</span>
      </div>
      {feedback.comments.length > MAX_REVIEW_FEEDBACK_COMMENT_COUNT && (
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
        {feedback.comments.map((comment) => (
          <span
            key={comment.id}
            className="inline-flex max-w-full items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-bg)] px-2 py-1 text-[11px] text-[var(--nova-text)]"
          >
            <span className="max-w-56 truncate" title={comment.body}>
              {comment.review_path || comment.change_set_id || t('changes.comment')}
              {comment.review_line !== undefined ? ` · ${t('changes.feedback.line', { line: comment.review_line })}` : ''}
              {' — '}{comment.body}
            </span>
            <button
              type="button"
              onClick={() => onRemove(comment.id)}
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
export function reviewFeedbackContextBytes(feedback: ReviewFeedbackSelection): number {
  const payload = JSON.stringify({
    review_thread_id: feedback.reviewThreadId,
    comments: feedback.comments.map((comment) => ({
      comment_id: comment.id,
      group_id: comment.group_id,
      ...(comment.change_set_id ? { change_set_id: comment.change_set_id } : {}),
      ...(comment.edit_id ? { edit_id: comment.edit_id } : {}),
      ...(comment.hunk_id ? { hunk_id: comment.hunk_id } : {}),
      ...(comment.review_path ? { path: comment.review_path } : {}),
      body: comment.body,
      anchor: compactAnchor(comment.anchor),
    })),
  }).replace(/[<>&\u2028\u2029]/g, (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`)
  return new TextEncoder().encode(payload).length + 2 * 1024
}

function compactAnchor(anchor: WorkspaceChangeComment['anchor']): Record<string, string | number> {
  if (!anchor) return {}
  return Object.fromEntries(Object.entries(anchor).filter(([, value]) => value !== undefined && value !== '' && value !== 0))
}
