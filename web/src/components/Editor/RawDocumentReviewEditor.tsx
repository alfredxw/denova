import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Loader2, MessageSquare, MessageSquarePlus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { SearchHighlightTextarea } from '@/components/common/SearchHighlightTextarea'
import { InlineCommentThread } from '@/components/review/InlineCommentThread'
import { Button } from '@/components/ui/button'
import type { DocumentReviewComment } from '@/features/document-review/types'
import { sameDocumentReviewTarget } from '@/features/document-review/types'
import { isSaveShortcut } from '@/lib/keyboard'
import { cn } from '@/lib/utils'
import type { MarkdownRichEditorReview } from './MarkdownRichEditor'
import {
  createRawDocumentReviewAnchor,
  documentReviewAnchorKey,
  resolveRawDocumentReviewAnchor,
} from './documentReviewAnchors'

interface RawDocumentReviewEditorProps {
  value: string
  onChange: (value: string) => void
  onSaveShortcut?: () => void
  highlightQuery?: string
  review: MarkdownRichEditorReview
  'aria-label'?: string
  className?: string
}

interface RawSelection {
  start: number
  end: number
}

interface RawCommentDraft {
  anchor: DocumentReviewComment['anchor']
  body: string
  submitting: boolean
}

interface RawCommentGroup {
  key: string
  comments: DocumentReviewComment[]
  quote: string
  range: { start: number; end: number } | null
}

/** Source-mode Markdown editor with the same durable review protocol as TipTap. */
export function RawDocumentReviewEditor({
  value,
  onChange,
  onSaveShortcut,
  highlightQuery,
  review,
  className,
  'aria-label': ariaLabel,
}: RawDocumentReviewEditorProps) {
  const { t } = useTranslation()
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const lastNavigationNonceRef = useRef<number | null>(null)
  const preparationRequestRef = useRef(0)
  const [selection, setSelection] = useState<RawSelection | null>(null)
  const [draft, setDraft] = useState<RawCommentDraft | null>(null)
  const [preparing, setPreparing] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)
  const comments = useMemo(
    () => review.controller.comments.filter((comment) =>
      sameDocumentReviewTarget(comment.target, review.target)),
    [review.controller.comments, review.target],
  )
  const groups = useMemo(() => groupRawComments(value, comments), [comments, value])

  const syncSelection = useCallback(() => {
    const textarea = textareaRef.current
    if (!textarea || textarea.selectionStart === textarea.selectionEnd) {
      setSelection(null)
      return
    }
    const start = Math.min(textarea.selectionStart, textarea.selectionEnd)
    const end = Math.max(textarea.selectionStart, textarea.selectionEnd)
    setSelection(value.slice(start, end).trim()
      ? { start, end }
      : null)
  }, [value])

  const revealRange = useCallback((range: { start: number; end: number } | null) => {
    const textarea = textareaRef.current
    if (!textarea || !range) return
    textarea.focus({ preventScroll: true })
    textarea.setSelectionRange(range.start, range.end)
    setSelection(range)
  }, [])

  const startSelectionComment = useCallback(async () => {
    const textarea = textareaRef.current
    if (!textarea || preparing) return
    const start = Math.min(textarea.selectionStart, textarea.selectionEnd)
    const end = Math.max(textarea.selectionStart, textarea.selectionEnd)
    const frozenSelection = { content: value, start, end }
    if (start === end || !value.slice(start, end).trim()) return

    const request = ++preparationRequestRef.current
    setPreparing(true)
    try {
      const snapshot = await review.prepareSnapshot()
      if (request !== preparationRequestRef.current) return
      const anchor = createRawDocumentReviewAnchor(snapshot, frozenSelection)
      console.debug('[RawDocumentReviewEditor] prepared source comment anchor', {
        target: review.target,
        revision: anchor.revision,
        byteStart: anchor.start,
        byteEnd: anchor.end,
      })
      setDraft({ anchor, body: '', submitting: false })
      setPanelOpen(true)
      setSelection(null)
    } catch (error) {
      if (request !== preparationRequestRef.current) return
      console.error('[RawDocumentReviewEditor] failed to prepare source comment anchor', {
        target: review.target,
        error,
      })
      toast.error(t('editor.review.prepareFailed'))
    } finally {
      if (request === preparationRequestRef.current) setPreparing(false)
    }
  }, [preparing, review, t, value])

  useEffect(() => {
    const intent = review.navigationIntent
    if (!intent || lastNavigationNonceRef.current === intent.nonce) return
    const comment = comments.find((item) => item.id === intent.commentID)
    if (!comment) return
    lastNavigationNonceRef.current = intent.nonce
    setPanelOpen(true)
    const range = resolveRawDocumentReviewAnchor(value, comment.anchor)
    window.requestAnimationFrame(() => revealRange(range))
  }, [comments, revealRange, review.navigationIntent, value])

  useEffect(() => {
    preparationRequestRef.current += 1
    lastNavigationNonceRef.current = null
    setSelection(null)
    setDraft(null)
    setPreparing(false)
    setPanelOpen(false)
    return () => {
      preparationRequestRef.current += 1
    }
  }, [review.target.kind, review.target.id, review.target.kind === 'lore_item' ? review.target.field : ''])

  const submitDraft = async () => {
    if (!draft?.body.trim()) return
    setDraft((current) => current ? { ...current, submitting: true } : current)
    try {
      await review.controller.onCreate({
        target: review.target,
        body: draft.body.trim(),
        anchor: draft.anchor,
      })
      setDraft(null)
    } catch (error) {
      console.error('[RawDocumentReviewEditor] failed to create source comment', {
        target: review.target,
        error,
      })
      toast.error(t('editor.review.createFailed'))
      setDraft((current) => current ? { ...current, submitting: false } : current)
    }
  }

  const draftKey = draft ? documentReviewAnchorKey(draft.anchor) : ''
  const draftComments = draftKey
    ? groups.find((group) => group.key === draftKey)?.comments ?? []
    : []

  return (
    <div className={cn('relative flex min-h-0 min-w-0 flex-col', className)}>
      <SearchHighlightTextarea
        ref={textareaRef}
        containerClassName="min-h-0 flex-1"
        autoResize={false}
        spellCheck={false}
        value={value}
        highlightQuery={highlightQuery}
        onChange={(event) => {
          setSelection(null)
          onChange(event.target.value)
        }}
        onSelect={syncSelection}
        onKeyDown={(event) => {
          if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === 'l') {
            event.preventDefault()
            void startSelectionComment()
            return
          }
          if (!isSaveShortcut(event)) return
          event.preventDefault()
          onSaveShortcut?.()
        }}
        aria-label={ariaLabel}
        className="mx-auto h-full min-h-0 min-w-0 max-w-[880px] resize-none rounded-none border-0 bg-transparent px-6 py-8 font-mono text-xs leading-6 text-[var(--nova-text)] shadow-none focus-visible:ring-0 md:px-10 md:py-10"
      />

      {!panelOpen && (
        <div className="pointer-events-none absolute bottom-3 right-3 z-30 flex items-center gap-1.5">
          {comments.length > 0 ? (
            <Button
              type="button"
              size="xs"
              variant="outline"
              className="pointer-events-auto bg-[var(--nova-surface)] text-[var(--nova-text-muted)] shadow-sm"
              aria-label={t(comments.length === 1 ? 'changes.commentCount.one' : 'changes.commentCount.other', { count: comments.length })}
              onClick={() => setPanelOpen(true)}
            >
              <MessageSquare />
              {comments.length}
            </Button>
          ) : null}
          {selection ? (
            <Button
              type="button"
              size="xs"
              variant="outline"
              className="pointer-events-auto bg-[var(--nova-surface)] text-[var(--nova-text)] shadow-sm"
              disabled={preparing}
              onPointerDown={(event) => event.preventDefault()}
              onClick={() => void startSelectionComment()}
            >
              {preparing ? <Loader2 className="animate-spin" /> : <MessageSquarePlus />}
              {t('editor.review.addComment')}
            </Button>
          ) : null}
        </div>
      )}

      {panelOpen ? (
        <aside
          aria-label={t('changes.comments')}
          className="absolute inset-y-2 right-2 z-40 flex w-[min(22rem,calc(100%-1rem))] flex-col overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] shadow-[var(--nova-shadow)]"
        >
          <div className="flex h-9 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-2.5">
            <MessageSquare className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
            <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text)]">
              {t('changes.comments')}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={t('common.close')}
              onClick={() => {
                setPanelOpen(false)
                setDraft(null)
              }}
            >
              <X />
            </Button>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto py-1">
            {draft ? (
              <InlineCommentThread
                comments={draftComments}
                quote={draft.anchor.display_quote}
                anchorLabel={rawAnchorLabel(value, resolveRawDocumentReviewAnchor(value, draft.anchor), t)}
                draft={{
                  body: draft.body,
                  submitting: draft.submitting,
                  onChange: (body) => setDraft((current) => current ? { ...current, body } : current),
                  onSubmit: () => void submitDraft(),
                  onCancel: () => {
                    setDraft(null)
                    if (comments.length === 0) setPanelOpen(false)
                  },
                }}
                onUpdate={(comment, body) => updateComment(review, comment, body, t)}
                onDelete={(comment) => deleteComment(review, comment, t)}
              />
            ) : null}
            {groups.filter((group) => group.key !== draftKey).map((group) => (
              <InlineCommentThread
                key={group.key}
                comments={group.comments}
                quote={group.quote}
                anchorLabel={rawAnchorLabel(value, group.range, t)}
                onUpdate={(comment, body) => updateComment(review, comment, body, t)}
                onDelete={(comment) => deleteComment(review, comment, t)}
              />
            ))}
          </div>
        </aside>
      ) : null}
    </div>
  )
}

function groupRawComments(value: string, comments: DocumentReviewComment[]): RawCommentGroup[] {
  const groups = new Map<string, RawCommentGroup>()
  for (const comment of comments) {
    const key = documentReviewAnchorKey(comment.anchor)
    const current = groups.get(key)
    if (current) {
      current.comments.push(comment)
      continue
    }
    groups.set(key, {
      key,
      comments: [comment],
      quote: comment.anchor.display_quote || comment.anchor.quote,
      range: resolveRawDocumentReviewAnchor(value, comment.anchor),
    })
  }
  return [...groups.values()].sort((left, right) =>
    (left.range?.start ?? Number.MAX_SAFE_INTEGER) - (right.range?.start ?? Number.MAX_SAFE_INTEGER)
    || left.key.localeCompare(right.key))
}

function rawAnchorLabel(
  value: string,
  range: { start: number; end: number } | null,
  t: (key: string) => string,
): string {
  if (!range) return t('editor.review.outdated')
  const line = value.slice(0, range.start).split(/\r\n|\r|\n/).length
  return `${t('editor.review.comment')} · L${line}`
}

async function updateComment(
  review: MarkdownRichEditorReview,
  comment: DocumentReviewComment,
  body: string,
  t: (key: string) => string,
) {
  try {
    await review.controller.onUpdate(comment, body)
  } catch (error) {
    console.error('[RawDocumentReviewEditor] failed to update source comment', {
      target: review.target,
      commentID: comment.id,
      error,
    })
    toast.error(t('editor.review.updateFailed'))
    throw error
  }
}

async function deleteComment(
  review: MarkdownRichEditorReview,
  comment: DocumentReviewComment,
  t: (key: string) => string,
) {
  try {
    await review.controller.onDelete(comment)
  } catch (error) {
    console.error('[RawDocumentReviewEditor] failed to delete source comment', {
      target: review.target,
      commentID: comment.id,
      error,
    })
    toast.error(t('editor.review.deleteFailed'))
    throw error
  }
}
