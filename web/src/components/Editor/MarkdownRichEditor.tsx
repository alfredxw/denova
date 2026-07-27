import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { TableKit } from '@tiptap/extension-table'
import { Markdown } from '@tiptap/markdown'

import { isSaveShortcut } from '@/lib/keyboard'
import { cn } from '@/lib/utils'
import type { DocumentReviewController, DocumentReviewNavigationIntent } from '@/features/document-review/controller'
import { sameDocumentReviewTarget, type DocumentReviewTarget } from '@/features/document-review/types'
import {
  createSearchHighlightExtension,
  findSearchMatches,
  searchPluginKey,
  selectSearchMatch,
} from './editorDecorations'
import type { SearchState } from './editorDecorations'
import { SelectionToolbar } from './SelectionToolbar'
import { DocumentReviewAnnotations, type DocumentReviewAnnotationsHandle } from './DocumentReviewAnnotations'
import type { DocumentReviewSnapshot } from './documentReviewAnchors'
import { createDocumentReviewExtension, type DocumentReviewDecorationState, type DocumentReviewPortalTarget } from './documentReviewDecorations'
import {
  createIndentedHardBreakExtension,
  createWorkspaceImageExtension,
  normalizeEditorText,
  placeEditorCaretAtClick,
  replaceEditorDocument,
} from './editorDocument'

export interface MarkdownRichEditorReview {
  target: DocumentReviewTarget
  resourceLabel: string
  controller: DocumentReviewController
  prepareSnapshot: () => Promise<DocumentReviewSnapshot>
  navigationIntent?: DocumentReviewNavigationIntent | null
}

interface MarkdownRichEditorProps {
  /**
   * 初始 Markdown 内容。挂载后文档由编辑器自持；value 变化只在确属外部更新时回灌，
   * 自己输入经 onChange 回传的相同内容不会写回文档（避免光标跳动）。
   */
  value: string
  onChange: (markdown: string) => void
  /** 外部搜索关键词（如目录搜索框）：全量高亮并定位到首个匹配；空串清除高亮。 */
  highlightQuery?: string
  /** Cmd/Ctrl+S 触发，对齐原 textarea 编辑器的保存快捷键行为。 */
  onSaveShortcut?: () => void
  review?: MarkdownRichEditorReview
  'aria-label'?: string
  className?: string
}

/**
 * 轻量可嵌入的所见即所得 Markdown 编辑器。
 *
 * 与章节编辑器（MarkdownEditor）共用 TipTap 扩展和搜索高亮装饰，但不耦合文件
 * 持久化/冲突处理；内容由父组件以 value/onChange 持有（如设置面板草稿态）。
 * 切换编辑对象时父组件应通过 key 重建实例，以隔离撤销历史。
 */
export function MarkdownRichEditor({
  value,
  onChange,
  highlightQuery,
  onSaveShortcut,
  review,
  className,
  'aria-label': ariaLabel,
}: MarkdownRichEditorProps) {
  const searchStateRef = useRef<SearchState>({ query: '', index: 0, useRegex: false })
  const [hasSelection, setHasSelection] = useState(false)
  const [reviewPortalTargets, setReviewPortalTargets] = useState<DocumentReviewPortalTarget[]>([])
  const containerRef = useRef<HTMLDivElement>(null)
  const reviewAnnotationsRef = useRef<DocumentReviewAnnotationsHandle>(null)
  const reviewDecorationStateRef = useRef<DocumentReviewDecorationState>({ enabled: false, decorations: [] })
  const lastReviewNavigationNonceRef = useRef<number | null>(null)
  const reviewRef = useRef(review)
  reviewRef.current = review
  // 记录编辑器最近发出/接收的内容：onChange 回灌的 value 不再写回文档，真正的外部变更才 setContent。
  const lastEmittedRef = useRef<string>(value)
  const onChangeRef = useRef(onChange)
  const onSaveShortcutRef = useRef(onSaveShortcut)
  useEffect(() => {
    onChangeRef.current = onChange
    onSaveShortcutRef.current = onSaveShortcut
  })

  const searchExtension = useMemo(() => createSearchHighlightExtension(searchStateRef), [])
  const workspaceImageExtension = useMemo(() => createWorkspaceImageExtension(), [])
  const updateReviewPortalTargets = useCallback((targets: DocumentReviewPortalTarget[]) => {
    setReviewPortalTargets((current) => sameReviewPortalTargets(current, targets) ? current : targets)
  }, [])
  const reviewExtension = useMemo(() => createDocumentReviewExtension(reviewDecorationStateRef, updateReviewPortalTargets), [updateReviewPortalTargets])

  const editor = useEditor({
    extensions: [
      StarterKit.configure({ hardBreak: false }),
      createIndentedHardBreakExtension(),
      searchExtension,
      workspaceImageExtension,
      reviewExtension,
      TableKit.configure({
        table: { resizable: false },
      }),
      Markdown.configure({
        markedOptions: { gfm: true, breaks: true },
      }),
    ],
    content: value,
    contentType: 'markdown',
    editorProps: {
      attributes: {
        role: 'textbox',
        'aria-multiline': 'true',
        ...(ariaLabel ? { 'aria-label': ariaLabel } : {}),
      },
      handleKeyDown: (_view, event) => {
        if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === 'l' && reviewRef.current) {
          event.preventDefault()
          event.stopPropagation()
          reviewAnnotationsRef.current?.startSelectionComment()
          return true
        }
        if (!isSaveShortcut(event)) return false
        event.preventDefault()
        event.stopPropagation()
        onSaveShortcutRef.current?.()
        return true
      },
      handleClick: (view, position, event) => {
        placeEditorCaretAtClick(view, position, event)
        // Keep review-highlight and future extension click handlers in the same event chain.
        return false
      },
    },
    onUpdate: ({ editor: instance }) => {
      const markdown = normalizeEditorText(instance.getMarkdown())
      lastEmittedRef.current = markdown
      onChangeRef.current(markdown)
    },
  })

  useEffect(() => {
    if (!editor) return
    const updateSelection = () => setHasSelection(editor.state.selection.from !== editor.state.selection.to)
    updateSelection()
    editor.on('selectionUpdate', updateSelection)
    editor.on('update', updateSelection)
    return () => {
      editor.off('selectionUpdate', updateSelection)
      editor.off('update', updateSelection)
    }
  }, [editor])

  useEffect(() => {
    const intent = review?.navigationIntent
    if (!intent || lastReviewNavigationNonceRef.current === intent.nonce) return
    const revealed = reviewAnnotationsRef.current?.revealComment(intent.commentID)
    if (revealed) lastReviewNavigationNonceRef.current = intent.nonce
  }, [review?.controller.comments, review?.navigationIntent])

  // 外部内容变更（Agent 写入、重新加载等）时回灌文档；自己输入产生的回灌跳过。
  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    if (value === lastEmittedRef.current) return
    const current = normalizeEditorText(editor.getMarkdown())
    lastEmittedRef.current = value
    if (value === current) return
    replaceEditorDocument(editor, value, {
      contentType: 'markdown',
      preserveSelection: true,
    })
  }, [editor, value])

  // 外部搜索词变化时刷新高亮并定位到首个匹配。
  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    const query = highlightQuery?.trim() || ''
    searchStateRef.current = { query, index: 0, useRegex: false }
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    if (!query) return
    const matches = findSearchMatches(editor, query)
    if (matches.length > 0) selectSearchMatch(editor, matches[0])
  }, [editor, highlightQuery])

  const reviewComments = review
    ? review.controller.comments.filter((comment) => sameDocumentReviewTarget(comment.target, review.target))
    : []

  return (
    <div ref={containerRef} className={cn('relative min-h-0 min-w-0', className)}>
      <EditorContent
        editor={editor}
        className="nova-rich-markdown chat-agent-message min-w-0 text-[var(--nova-text)]"
      />
      {editor && review ? (
        <DocumentReviewAnnotations
          ref={reviewAnnotationsRef}
          editor={editor}
          target={review.target}
          resourceLabel={review.resourceLabel}
          containerRef={containerRef}
          comments={reviewComments}
          decorationStateRef={reviewDecorationStateRef}
          portalTargets={reviewPortalTargets}
          onPrepareSnapshot={review.prepareSnapshot}
          onCreate={review.controller.onCreate}
          onUpdate={review.controller.onUpdate}
          onDelete={review.controller.onDelete}
        />
      ) : null}
      {editor && review && hasSelection ? (
        <SelectionToolbar editor={editor} mode="comment" onAction={() => reviewAnnotationsRef.current?.startSelectionComment()} />
      ) : null}
    </div>
  )
}

function sameReviewPortalTargets(current: DocumentReviewPortalTarget[], next: DocumentReviewPortalTarget[]): boolean {
  return current.length === next.length && current.every((target, index) => target.key === next[index]?.key && target.element === next[index]?.element)
}
