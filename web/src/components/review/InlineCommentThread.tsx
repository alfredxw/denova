import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Check, MessageSquarePlus, Pencil, RefreshCw, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

const INLINE_COMMENT_EDITOR_CLASS = 'nova-review-inline-editor min-h-0 resize-none rounded-none border-0 bg-transparent px-0 py-0 text-xs leading-5 shadow-none focus:outline-none focus:ring-0 focus-visible:border-transparent focus-visible:outline-none focus-visible:ring-0'

export interface InlineReviewComment {
  id: string
  body: string
  resolved?: boolean
}

interface InlineCommentThreadProps<T extends InlineReviewComment> {
  comments?: T[]
  anchorLabel?: string
  quote?: string
  disabled?: boolean
  draft?: {
    body: string
    submitting: boolean
    onChange: (body: string) => void
    onSubmit: () => void
    onCancel: () => void
  }
  onUpdate?: (comment: T, body: string) => Promise<void> | void
  onResolve?: (comment: T, resolved: boolean) => Promise<void> | void
  onDelete?: (comment: T) => Promise<void> | void
  onEditingChange?: (editing: boolean) => void
}

/** Shared inline thread used by diff review and author document review. */
export function InlineCommentThread<T extends InlineReviewComment>({ comments = [], anchorLabel, quote, disabled = false, draft, onUpdate, onResolve, onDelete, onEditingChange }: InlineCommentThreadProps<T>) {
  const { t } = useTranslation()
  const [editingID, setEditingID] = useState('')
  const [editBody, setEditBody] = useState('')
  const [localBusy, setLocalBusy] = useState('')
  const editingRef = useRef(false)
  const editorRef = useRef<HTMLTextAreaElement | null>(null)
  const editingCallbackRef = useRef(onEditingChange)
  editingCallbackRef.current = onEditingChange
  const busy = disabled || Boolean(localBusy)
  const focusRequestKey = editingID || (draft ? 'draft' : '')

  useLayoutEffect(() => {
    if (focusRequestKey) editorRef.current?.focus({ preventScroll: true })
  }, [focusRequestKey])

  useEffect(() => () => {
    if (editingRef.current) editingCallbackRef.current?.(false)
  }, [])

  const setEditing = (id: string) => {
    const editing = Boolean(id)
    if (editing !== editingRef.current) editingCallbackRef.current?.(editing)
    editingRef.current = editing
    setEditingID(id)
  }

  const run = async (id: string, operation: () => Promise<void> | void) => {
    setLocalBusy(id)
    try {
      await operation()
      return true
    } catch {
      return false
    } finally {
      setLocalBusy('')
    }
  }

  return (
    <div className="nova-review-comment-thread mx-2 my-1 overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
      <div className="flex min-h-8 items-center gap-2 border-b border-[var(--nova-border)] px-3 py-1.5 text-[10px] text-[var(--nova-text-faint)]">
        <MessageSquarePlus className="h-3.5 w-3.5" />
        <span className="min-w-0 flex-1 truncate">{anchorLabel || t('changes.comments')}</span>
        {comments.length > 0 && <span>{comments.length}</span>}
      </div>
      {quote && (
        <blockquote className="mx-3 mt-2 line-clamp-3 border-l-2 border-[var(--nova-border-strong)] pl-2 text-[11px] leading-5 text-[var(--nova-text-muted)]" title={quote}>
          {quote}
        </blockquote>
      )}

      {comments.map((comment) => (
        <div key={comment.id} className={`border-b border-[var(--nova-border-soft)] px-3 py-2 last:border-b-0 ${comment.resolved ? 'opacity-60' : ''}`}>
          {editingID === comment.id ? (
            <>
              <Textarea ref={editorRef} value={editBody} disabled={busy} onChange={(event) => setEditBody(event.target.value)} minRows={1} maxRows={8} className={INLINE_COMMENT_EDITOR_CLASS} />
              <div className="mt-2 flex justify-end gap-1">
                <Button type="button" size="xs" variant="ghost" disabled={busy} onClick={() => setEditing('')}>{t('common.cancel')}</Button>
                <Button type="button" size="xs" disabled={busy || !editBody.trim()} onClick={() => void run(comment.id, () => onUpdate?.(comment, editBody.trim())).then((saved) => { if (saved) setEditing('') })}>
                  <Check />{t('common.save')}
                </Button>
              </div>
            </>
          ) : (
            <>
              <p className="whitespace-pre-wrap break-words leading-5">{comment.body}</p>
              <div className="mt-1.5 flex flex-wrap items-center justify-end gap-1">
                {onUpdate && <Button type="button" size="icon-xs" variant="ghost" disabled={busy} aria-label={t('changes.editComment')} onClick={() => { setEditing(comment.id); setEditBody(comment.body) }}><Pencil /></Button>}
                {onResolve && <Button type="button" size="xs" variant="ghost" disabled={busy} onClick={() => void run(comment.id, () => onResolve(comment, !comment.resolved))}>{comment.resolved ? <RefreshCw /> : <Check />}{t(comment.resolved ? 'changes.reopen' : 'changes.resolve')}</Button>}
                {onDelete && <Button type="button" size="icon-xs" variant="ghost" disabled={busy} className="text-[var(--nova-danger)]" aria-label={t('common.delete')} onClick={() => void run(comment.id, () => onDelete(comment))}><Trash2 /></Button>}
              </div>
            </>
          )}
        </div>
      ))}

      {draft && (
        <div className="px-3 py-2">
          <Textarea ref={editorRef} value={draft.body} disabled={disabled || draft.submitting} onChange={(event) => draft.onChange(event.target.value)} placeholder={t('changes.commentPlaceholder')} minRows={2} maxRows={8} className={INLINE_COMMENT_EDITOR_CLASS} />
          <div className="mt-2 flex justify-end gap-1.5">
            <Button type="button" size="xs" variant="ghost" disabled={disabled || draft.submitting} onClick={draft.onCancel}><X />{t('common.cancel')}</Button>
            <Button type="button" size="xs" disabled={disabled || draft.submitting || !draft.body.trim()} onClick={draft.onSubmit}><MessageSquarePlus />{t('changes.addComment')}</Button>
          </div>
        </div>
      )}
    </div>
  )
}
