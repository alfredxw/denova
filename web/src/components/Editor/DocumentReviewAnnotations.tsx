import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { MessageSquarePlus } from 'lucide-react'
import type { Editor } from '@tiptap/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { InlineCommentThread } from '@/components/review/InlineCommentThread'
import type { CreateDocumentCommentRequest, DocumentReviewComment } from '@/features/document-review/types'
import { createDocumentReviewAnchor, textBlockRangeAtPosition, topLevelWidgetPosition, type DocumentReviewSnapshot, type EditorReviewRange } from './documentReviewAnchors'
import { documentReviewPluginKey, type DocumentReviewDecoration, type DocumentReviewDecorationState, type DocumentReviewPortalTarget } from './documentReviewDecorations'

export interface DocumentReviewAnnotationsHandle {
  startSelectionComment: () => void
}

interface DocumentReviewAnnotationsProps {
  enabled: boolean
  editor: Editor
  fileName: string
  snapshot: DocumentReviewSnapshot | null
  containerRef: RefObject<HTMLDivElement | null>
  comments: DocumentReviewComment[]
  decorationStateRef: { current: DocumentReviewDecorationState }
  portalTargets: DocumentReviewPortalTarget[]
  onCreate: (request: CreateDocumentCommentRequest) => Promise<DocumentReviewComment>
  onUpdate: (comment: DocumentReviewComment, body: string) => Promise<DocumentReviewComment>
  onDelete: (comment: DocumentReviewComment) => Promise<DocumentReviewComment>
}

interface DraftComment extends EditorReviewRange {
  key: string
  body: string
  submitting: boolean
}

interface AnnotationGroup {
  key: string
  range: EditorReviewRange
  comments: DocumentReviewComment[]
  quote: string
  outdated: boolean
  draft?: DraftComment
}

/** Renders durable comments into ProseMirror widget hosts without changing Markdown. */
export const DocumentReviewAnnotations = forwardRef<DocumentReviewAnnotationsHandle, DocumentReviewAnnotationsProps>(function DocumentReviewAnnotations({
  enabled,
  editor,
  fileName,
  snapshot,
  containerRef,
  comments,
  decorationStateRef,
  portalTargets,
  onCreate,
  onUpdate,
  onDelete,
}, ref) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<DraftComment | null>(null)
  const [quickAction, setQuickAction] = useState<{ top: number; left: number; range: EditorReviewRange } | null>(null)
  const quickActionButtonRef = useRef<HTMLButtonElement>(null)
  const decorationLayoutCacheRef = useRef<{ key: string; decorations: DocumentReviewDecoration[] }>({ key: '', decorations: [] })

  const startDraft = useCallback((range: EditorReviewRange) => {
    setDraft({ ...range, key: `draft:${range.from}:${range.to}`, body: '', submitting: false })
    setQuickAction(null)
  }, [])

  const startSelectionComment = useCallback(() => {
    const { from, to } = editor.state.selection
    if (from === to) return
    const displayQuote = editor.state.doc.textBetween(from, to, '\n').trim()
    if (!displayQuote) return
    startDraft({ from, to, widgetPos: topLevelWidgetPosition(editor.state.doc, to), kind: 'text-range', displayQuote })
  }, [editor, startDraft])

  useImperativeHandle(ref, () => ({ startSelectionComment }), [startSelectionComment])

  useEffect(() => {
    if (!enabled) {
      setDraft(null)
      setQuickAction(null)
    }
  }, [enabled])

  useEffect(() => {
    setDraft(null)
    setQuickAction(null)
  }, [fileName])

  const groups = useMemo(() => buildGroups(editor, snapshot, comments, draft), [comments, draft, editor, snapshot])
  const decorationLayoutKey = JSON.stringify(groups.map((group) => [
    group.key,
    group.outdated ? null : group.range.from,
    group.outdated ? null : group.range.to,
    group.range.widgetPos,
    group.outdated,
  ]))
  if (decorationLayoutCacheRef.current.key !== decorationLayoutKey) {
    decorationLayoutCacheRef.current = {
      key: decorationLayoutKey,
      decorations: groups.map((group) => ({
        key: group.key,
        from: group.outdated ? undefined : group.range.from,
        to: group.outdated ? undefined : group.range.to,
        widgetPos: group.range.widgetPos,
        outdated: group.outdated,
      })),
    }
  }
  const reviewDecorations = decorationLayoutCacheRef.current.decorations

  useEffect(() => {
    decorationStateRef.current = {
      enabled,
      decorations: reviewDecorations,
    }
    if (!editor.isDestroyed) editor.view.dispatch(editor.state.tr.setMeta(documentReviewPluginKey, true))
  }, [decorationStateRef, editor, enabled, reviewDecorations])

  useEffect(() => {
    if (!enabled || draft) return
    const container = containerRef.current
    if (!container) return
    const onPointerMove = (event: PointerEvent) => {
      const target = event.target as HTMLElement | null
      if (target?.closest('[data-document-review-quick-action]')) return
      if (target?.closest('.nova-review-comment-thread')) {
        setQuickAction(null)
        return
      }
      if (!target || !editor.view.dom.contains(target)) {
        setQuickAction(null)
        return
      }
      const position = editor.view.posAtCoords({ left: event.clientX, top: event.clientY })?.pos
      if (position === undefined) {
        setQuickAction(null)
        return
      }
      const range = textBlockRangeAtPosition(editor.state.doc, position)
      if (!range) {
        setQuickAction(null)
        return
      }
      const containerRect = container.getBoundingClientRect()
      const editorRect = editor.view.dom.getBoundingClientRect()
      const top = event.clientY - containerRect.top + container.scrollTop - 13
      const left = Math.min(container.clientWidth - 30, editorRect.right - containerRect.left + 8)
      setQuickAction((current) => {
        if (current?.range.from === range.from && current.range.to === range.to) {
          // Pointer movement is continuous; keep its coordinates outside React's render loop.
          if (quickActionButtonRef.current) {
            quickActionButtonRef.current.style.top = `${top}px`
            quickActionButtonRef.current.style.left = `${left}px`
          }
          return current
        }
        return { top, left, range }
      })
    }
    container.addEventListener('pointermove', onPointerMove)
    return () => container.removeEventListener('pointermove', onPointerMove)
  }, [containerRef, draft, editor, enabled])

  const submitDraft = async () => {
    if (!draft || !snapshot || !draft.body.trim()) return
    setDraft((current) => current ? { ...current, submitting: true } : current)
    try {
      const anchor = createDocumentReviewAnchor(editor, snapshot, draft)
      await onCreate({ path: fileName, body: draft.body.trim(), anchor })
      setDraft(null)
    } catch (error) {
      console.error('创建正文审阅评论失败', { fileName, error })
      toast.error(t('editor.review.createFailed'))
      setDraft((current) => current ? { ...current, submitting: false } : current)
    }
  }

  const targets = new Map(portalTargets.map((target) => [target.key, target.element]))
  if (!enabled) return null

  return (
    <>
      {quickAction && (
        <button
          ref={quickActionButtonRef}
          type="button"
          data-document-review-quick-action
          className="absolute z-20 flex h-6 w-6 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] shadow-md transition-colors hover:border-[var(--nova-border-strong)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          style={{ top: quickAction.top, left: quickAction.left }}
          aria-label={t('editor.review.commentCurrentLine')}
          title={t('editor.review.commentCurrentLine')}
          onPointerDown={(event) => event.preventDefault()}
          onClick={() => startDraft(quickAction.range)}
        >
          <MessageSquarePlus className="h-3.5 w-3.5" />
        </button>
      )}
      {groups.map((group) => {
        const target = targets.get(group.key)
        if (!target) return null
        return createPortal(
          <InlineCommentThread
            comments={group.comments}
            quote={group.quote}
            anchorLabel={group.outdated ? t('editor.review.outdated') : t('editor.review.comment')}
            draft={group.draft ? {
              body: group.draft.body,
              submitting: group.draft.submitting,
              onChange: (body) => setDraft((current) => current ? { ...current, body } : current),
              onSubmit: () => { void submitDraft() },
              onCancel: () => setDraft(null),
            } : undefined}
            onUpdate={async (comment, body) => {
              try { await onUpdate(comment, body) } catch (error) {
                console.error('更新正文审阅评论失败', { fileName, commentID: comment.id, error })
                toast.error(t('editor.review.updateFailed'))
                throw error
              }
            }}
            onDelete={async (comment) => {
              try { await onDelete(comment) } catch (error) {
                console.error('删除正文审阅评论失败', { fileName, commentID: comment.id, error })
                toast.error(t('editor.review.deleteFailed'))
                throw error
              }
            }}
          />,
          target,
          group.key,
        )
      })}
    </>
  )
})

function buildGroups(editor: Editor, snapshot: DocumentReviewSnapshot | null, comments: DocumentReviewComment[], draft: DraftComment | null): AnnotationGroup[] {
  const groups = new Map<string, AnnotationGroup>()
  for (const comment of comments) {
    const sameRevision = Boolean(snapshot?.revision) && comment.anchor.revision === snapshot?.revision
    const from = comment.anchor.editor_from || 0
    const to = comment.anchor.editor_to || 0
    const displayQuote = comment.anchor.display_quote || comment.anchor.quote
    const visibleQuote = from > 0 && to > from && to <= editor.state.doc.content.size
      ? editor.state.doc.textBetween(from, to, '\n').trim()
      : ''
    const validRange = sameRevision && Boolean(displayQuote.trim()) && visibleQuote === displayQuote.trim()
    const key = `comment:${comment.anchor.revision}:${comment.anchor.start}:${comment.anchor.end}`
    const existing = groups.get(key)
    if (existing) {
      existing.comments.push(comment)
      continue
    }
    groups.set(key, {
      key,
      comments: [comment],
      quote: displayQuote,
      outdated: !validRange,
      range: {
        from: validRange ? from : 0,
        to: validRange ? to : 0,
        widgetPos: validRange ? topLevelWidgetPosition(editor.state.doc, to) : editor.state.doc.content.size,
        kind: comment.anchor.kind,
        displayQuote,
      },
    })
  }
  if (draft) {
    groups.set(draft.key, { key: draft.key, range: draft, comments: [], quote: draft.displayQuote, outdated: false, draft })
  }
  return [...groups.values()].sort((left, right) => left.range.widgetPos - right.range.widgetPos || left.key.localeCompare(right.key))
}
