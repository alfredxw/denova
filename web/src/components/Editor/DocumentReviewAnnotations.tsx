import { forwardRef, useCallback, useEffect, useImperativeHandle, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { Loader2, MessageSquarePlus } from 'lucide-react'
import type { Editor } from '@tiptap/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { InlineCommentThread } from '@/components/review/InlineCommentThread'
import type { CreateDocumentCommentRequest, DocumentReviewAnchor, DocumentReviewComment } from '@/features/document-review/types'
import {
  captureDocumentReviewSelection,
  commentWidgetPosition,
  createDocumentReviewAnchor,
  type DocumentReviewSnapshot,
  type EditorReviewRange,
} from './documentReviewAnchors'
import { documentReviewKeysFromElement, documentReviewPluginKey, type DocumentReviewDecoration, type DocumentReviewDecorationState, type DocumentReviewPortalTarget } from './documentReviewDecorations'
import { documentReviewRangeAtCoordinates } from './documentReviewHover'

export interface DocumentReviewAnnotationsHandle {
  startSelectionComment: () => void
}

interface DocumentReviewAnnotationsProps {
  editor: Editor
  fileName: string
  containerRef: RefObject<HTMLDivElement | null>
  comments: DocumentReviewComment[]
  decorationStateRef: { current: DocumentReviewDecorationState }
  portalTargets: DocumentReviewPortalTarget[]
  onPrepareSnapshot: () => Promise<DocumentReviewSnapshot>
  onCreate: (request: CreateDocumentCommentRequest) => Promise<DocumentReviewComment>
  onUpdate: (comment: DocumentReviewComment, body: string) => Promise<DocumentReviewComment>
  onDelete: (comment: DocumentReviewComment) => Promise<DocumentReviewComment>
}

interface DraftComment extends EditorReviewRange {
  key: string
  body: string
  submitting: boolean
  anchor: DocumentReviewAnchor
}

interface AnnotationGroup {
  key: string
  range: EditorReviewRange
  comments: DocumentReviewComment[]
  quote: string
  outdated: boolean
  showWidget: boolean
  draft?: DraftComment
}

/** Renders durable comments into ProseMirror widget hosts without changing Markdown. */
export const DocumentReviewAnnotations = forwardRef<DocumentReviewAnnotationsHandle, DocumentReviewAnnotationsProps>(function DocumentReviewAnnotations({
  editor,
  fileName,
  containerRef,
  comments,
  decorationStateRef,
  portalTargets,
  onPrepareSnapshot,
  onCreate,
  onUpdate,
  onDelete,
}, ref) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<DraftComment | null>(null)
  const [expandedKeys, setExpandedKeys] = useState<ReadonlySet<string>>(() => new Set())
  const [preparing, setPreparing] = useState(false)
  const [quickAction, setQuickAction] = useState<{ top: number; left: number; range: EditorReviewRange } | null>(null)
  const preparationRequestRef = useRef(0)
  const decorationLayoutCacheRef = useRef<{ key: string; decorations: DocumentReviewDecoration[] }>({ key: '', decorations: [] })

  const startDraft = useCallback(async (range: EditorReviewRange) => {
    if (preparing) return
    const request = ++preparationRequestRef.current
    console.debug('[DocumentReviewAnnotations.startDraft] preparing document comment anchor', {
      fileName,
      kind: range.kind,
      editorFrom: range.from,
      editorTo: range.to,
    })
    setPreparing(true)
    setQuickAction(null)
    try {
      const selection = captureDocumentReviewSelection(editor, range)
      const snapshot = await onPrepareSnapshot()
      if (request !== preparationRequestRef.current) return
      const anchor = createDocumentReviewAnchor(editor, snapshot, selection)
      console.debug('[DocumentReviewAnnotations.startDraft] document comment anchor prepared', {
        fileName,
        revision: anchor.revision,
        byteStart: anchor.start,
        byteEnd: anchor.end,
      })
      // 草稿与创建后的评论共用同一分组 key：提交时 widget 宿主与 Portal 原地复用，避免闪一下
      setDraft({ ...range, anchor, key: documentCommentAnchorKey(anchor), body: '', submitting: false })
    } catch (error) {
      if (request !== preparationRequestRef.current) return
      console.error('准备正文审阅评论失败:', error instanceof Error ? error.message : String(error), { fileName, error })
      toast.error(t('editor.review.prepareFailed'))
    } finally {
      if (request === preparationRequestRef.current) setPreparing(false)
    }
  }, [editor, fileName, onPrepareSnapshot, preparing, t])

  const startSelectionComment = useCallback(() => {
    const { from, to } = editor.state.selection
    if (from === to) return
    const displayQuote = editor.state.doc.textBetween(from, to, '\n').trim()
    if (!displayQuote) return
    // 弹层打开后折叠选区，让浮动评论按钮立即消失；评论范围由审阅高亮继续标记
    editor.chain().setTextSelection(to).run()
    void startDraft({ from, to, widgetPos: commentWidgetPosition(editor.state.doc, to), kind: 'text-range', displayQuote })
  }, [editor, startDraft])

  useImperativeHandle(ref, () => ({ startSelectionComment }), [startSelectionComment])

  useEffect(() => {
    preparationRequestRef.current += 1
    setDraft(null)
    setExpandedKeys(new Set())
    setPreparing(false)
    setQuickAction(null)
    return () => {
      preparationRequestRef.current += 1
    }
  }, [fileName])

  const toggleExpandedComments = useCallback((keys: readonly string[]) => {
    setExpandedKeys((current) => {
      const next = new Set(current)
      const uniqueKeys = [...new Set(keys)]
      if (!uniqueKeys.length) return current
      if (uniqueKeys.every((key) => next.has(key))) {
        for (const key of uniqueKeys) next.delete(key)
      } else {
        for (const key of uniqueKeys) next.add(key)
      }
      return next
    })
  }, [])
  const groups = useMemo(() => buildGroups(editor, comments, draft, expandedKeys), [comments, draft, editor, expandedKeys])
  const decorationLayoutKey = JSON.stringify(groups.map((group) => [
    group.key,
    group.outdated ? null : group.range.from,
    group.outdated ? null : group.range.to,
    group.range.widgetPos,
    group.outdated,
    group.showWidget,
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
        showWidget: group.showWidget,
      })),
    }
  }
  const reviewDecorations = decorationLayoutCacheRef.current.decorations

  useEffect(() => {
    decorationStateRef.current = {
      enabled: true,
      decorations: reviewDecorations,
      onHighlightClick: toggleExpandedComments,
      expandLabel: t('editor.review.expandComment'),
      collapseLabel: t('editor.review.collapseComment'),
    }
    if (!editor.isDestroyed) editor.view.dispatch(editor.state.tr.setMeta(documentReviewPluginKey, true))
  }, [decorationStateRef, editor, reviewDecorations, t, toggleExpandedComments])

  useEffect(() => {
    return () => {
      decorationStateRef.current = { enabled: false, decorations: [] }
      if (!editor.isDestroyed) editor.view.dispatch(editor.state.tr.setMeta(documentReviewPluginKey, true))
    }
  }, [decorationStateRef, editor])

  useEffect(() => {
    const availableKeys = new Set(groups.map((group) => group.key))
    setExpandedKeys((current) => {
      const next = new Set([...current].filter((key) => availableKeys.has(key)))
      return next.size === current.size ? current : next
    })
  }, [groups])

  useLayoutEffect(() => {
    if (!portalTargets.length) return
    const align = () => alignCommentThreads(editor, portalTargets)
    align()
    const resizeObserver = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(align)
    resizeObserver?.observe(editor.view.dom)
    return () => resizeObserver?.disconnect()
  }, [editor, portalTargets])

  useEffect(() => {
    if (draft || preparing) return
    const container = containerRef.current
    if (!container) return
    const onPointerMove = (event: PointerEvent) => {
      const target = event.target as HTMLElement | null
      if (target?.closest('[data-document-review-quick-action]')) return
      if (target?.closest('.nova-review-comment-thread')) {
        setQuickAction(null)
        return
      }
      if (!target) {
        setQuickAction(null)
        return
      }
      const range = documentReviewRangeAtCoordinates(editor, event.clientX, event.clientY)
      if (!range) {
        setQuickAction(null)
        return
      }
      const containerRect = container.getBoundingClientRect()
      const editorRect = editor.view.dom.getBoundingClientRect()
      const line = editor.view.coordsAtPos(range.from)
      const top = (line.top + line.bottom) / 2 - containerRect.top + container.scrollTop - 12
      const left = Math.max(4, Math.min(container.clientWidth - 32, editorRect.right - containerRect.left + 8))
      setQuickAction((current) => {
        if (current?.range.from === range.from
          && current.range.to === range.to
          && Math.abs(current.top - top) < 0.5
          && Math.abs(current.left - left) < 0.5) return current
        return { top, left, range }
      })
    }
    const clearQuickAction = () => setQuickAction(null)
    const clearOutsideContainer = (event: PointerEvent) => {
      const target = event.target
      if (!(target instanceof Node) || !container.contains(target)) setQuickAction(null)
    }
    container.addEventListener('pointermove', onPointerMove)
    container.addEventListener('pointerleave', clearQuickAction)
    container.addEventListener('scroll', clearQuickAction, { passive: true })
    document.addEventListener('pointermove', clearOutsideContainer, true)
    const resizeObserver = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(clearQuickAction)
    resizeObserver?.observe(container)
    return () => {
      container.removeEventListener('pointermove', onPointerMove)
      container.removeEventListener('pointerleave', clearQuickAction)
      container.removeEventListener('scroll', clearQuickAction)
      document.removeEventListener('pointermove', clearOutsideContainer, true)
      resizeObserver?.disconnect()
    }
  }, [containerRef, draft, editor, preparing])

  const submitDraft = async () => {
    if (!draft || !draft.body.trim()) return
    setDraft((current) => current ? { ...current, submitting: true } : current)
    try {
      const created = await onCreate({ path: fileName, body: draft.body.trim(), anchor: draft.anchor })
      setExpandedKeys((current) => new Set(current).add(documentCommentGroupKey(created)))
      setDraft(null)
    } catch (error) {
      console.error('创建正文审阅评论失败', { fileName, error })
      toast.error(t('editor.review.createFailed'))
      setDraft((current) => current ? { ...current, submitting: false } : current)
    }
  }

  const targets = new Map(portalTargets.map((target) => [target.key, target.element]))

  return (
    <>
      {quickAction && (
        <button
          type="button"
          data-document-review-quick-action
          className="absolute z-20 flex h-6 w-7 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] shadow-md transition-colors hover:border-[var(--nova-border-strong)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:cursor-wait disabled:opacity-70"
          style={{ top: quickAction.top, left: quickAction.left }}
          aria-label={t('editor.review.commentCurrentLine')}
          title={t('editor.review.commentCurrentLine')}
          disabled={preparing}
          onPointerDown={(event) => event.preventDefault()}
          onClick={() => { void startDraft(quickAction.range) }}
        >
          {preparing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <MessageSquarePlus className="h-3.5 w-3.5" />}
        </button>
      )}
      {groups.map((group) => {
        if (!group.showWidget) return null
        const target = targets.get(group.key)
        if (!target) return null
        return createPortal(
          <div className="nova-document-review-thread-anchor">
            <InlineCommentThread
              comments={group.comments}
              quote={group.outdated ? group.quote : undefined}
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
              onCollapse={!group.outdated && !group.draft ? () => {
                setExpandedKeys((current) => {
                  if (!current.has(group.key)) return current
                  const next = new Set(current)
                  next.delete(group.key)
                  return next
                })
              } : undefined}
            />
          </div>,
          target,
          group.key,
        )
      })}
    </>
  )
})

function buildGroups(editor: Editor, comments: DocumentReviewComment[], draft: DraftComment | null, expandedKeys: ReadonlySet<string>): AnnotationGroup[] {
  const groups = new Map<string, AnnotationGroup>()
  for (const comment of comments) {
    const key = documentCommentGroupKey(comment)
    const mappedRange = mappedCommentRange(editor, key)
    const from = mappedRange?.from ?? comment.anchor.editor_from ?? 0
    const to = mappedRange?.to ?? comment.anchor.editor_to ?? 0
    const displayQuote = comment.anchor.display_quote || comment.anchor.quote
    const visibleQuote = from > 0 && to > from && to <= editor.state.doc.content.size
      ? editor.state.doc.textBetween(from, to, '\n').trim()
      : ''
    const validRange = Boolean(displayQuote.trim()) && visibleQuote === displayQuote.trim()
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
      showWidget: !validRange || expandedKeys.has(key),
      range: {
        from: validRange ? from : 0,
        to: validRange ? to : 0,
        widgetPos: validRange ? commentWidgetPosition(editor.state.doc, to) : editor.state.doc.content.size,
        kind: comment.anchor.kind,
        displayQuote,
      },
    })
  }
  if (draft) {
    groups.set(draft.key, { key: draft.key, range: draft, comments: [], quote: draft.displayQuote, outdated: false, showWidget: true, draft })
  }
  return [...groups.values()].sort((left, right) => left.range.widgetPos - right.range.widgetPos || left.key.localeCompare(right.key))
}

function documentCommentGroupKey(comment: DocumentReviewComment): string {
  return documentCommentAnchorKey(comment.anchor)
}

function documentCommentAnchorKey(anchor: DocumentReviewAnchor): string {
  return `comment:${anchor.revision}:${anchor.start}:${anchor.end}`
}

function mappedCommentRange(editor: Editor, key: string): { from: number; to: number } | null {
  const decorations = documentReviewPluginKey.getState(editor.state)?.find(
    0,
    editor.state.doc.content.size,
    (spec) => Array.isArray(spec.documentReviewKeys) && spec.documentReviewKeys.includes(key) && spec.kind === 'highlight',
  )
  const highlights = decorations?.filter((decoration) => decoration.to > decoration.from) ?? []
  if (!highlights.length) return null
  return {
    from: Math.min(...highlights.map((highlight) => highlight.from)),
    to: Math.max(...highlights.map((highlight) => highlight.to)),
  }
}

function alignCommentThreads(editor: Editor, targets: DocumentReviewPortalTarget[]): void {
  const highlights = Array.from(editor.view.dom.querySelectorAll<HTMLElement>('[data-document-review-keys]'))
  const highlightByKey = new Map<string, HTMLElement>()
  for (const highlight of highlights) {
    for (const key of documentReviewKeysFromElement(highlight)) {
      if (!highlightByKey.has(key)) highlightByKey.set(key, highlight)
    }
  }
  for (const target of targets) {
    const highlight = highlightByKey.get(target.key)
    if (!highlight) {
      target.element.style.setProperty('--nova-document-review-anchor-offset', '0px')
      continue
    }
    const highlightRect = highlight.getClientRects()[0] ?? highlight.getBoundingClientRect()
    const targetRect = target.element.getBoundingClientRect()
    const minimumThreadWidth = Math.min(288, targetRect.width)
    const maximumOffset = Math.max(0, targetRect.width - minimumThreadWidth)
    const offset = Math.max(0, Math.min(maximumOffset, highlightRect.left - targetRect.left))
    target.element.style.setProperty('--nova-document-review-anchor-offset', `${offset}px`)
  }
}
