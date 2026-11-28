import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EditorContent, useEditor, type Editor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { TableKit } from '@tiptap/extension-table'
import { Markdown } from '@tiptap/markdown'
import type { Extensions } from '@tiptap/core'
import { createMarkdownParser } from './markdownParser'

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
import { createInlineEditDecorationExtension } from './inline-edit/editorInlineEditDecorations'
import { InlineEditPopover } from './inline-edit/InlineEditPopover'
import { useTipTapInlineEdit } from './inline-edit/useTipTapInlineEdit'
import type { InlineEditMode } from '@/lib/api-client/inline-edit'
import type { LoreItem } from '@/lib/api'
import { DocumentReviewAnnotations, type DocumentReviewAnnotationsHandle } from './DocumentReviewAnnotations'
import type { DocumentReviewSnapshot } from './documentReviewAnchors'
import { richMarkdownReviewAdapter, sourceMarkdownReviewAdapter } from './documentReviewAdapter'
import { createDocumentReviewExtension, type DocumentReviewDecorationState, type DocumentReviewPortalTarget } from './documentReviewDecorations'
import {
  createIndentedHardBreakExtension,
  createWorkspaceImageExtension,
  insertPastedWorkspaceMarkdownImage,
  normalizeEditorText,
  placeEditorCaretAtClick,
  replaceEditorDocument,
  resetEditorStateHistory,
} from './editorDocument'
import {
  createMarkdownSourceDocument,
  RawMarkdown,
  readMarkdownSourceDocument,
} from './markdownSourceDocument'
import { projectFileAssetURL } from '@/lib/api-client/project-files'

export type MarkdownContentMode = 'rich' | 'source'

export interface MarkdownContentEditorReview {
  target: DocumentReviewTarget
  resourceLabel: string
  controller: DocumentReviewController
  prepareSnapshot: () => Promise<DocumentReviewSnapshot>
  navigationIntent?: DocumentReviewNavigationIntent | null
}

export interface MarkdownContentEditorInlineEdit {
  mode: InlineEditMode
  loreItems?: readonly LoreItem[]
}

export interface MarkdownContentEditorProps {
  /** Stable owner for relative assets embedded in the Markdown. */
  projectId: string
  /** The canonical Markdown draft shared by both visual and source representations. */
  value: string
  onChange: (markdown: string) => void
  mode: MarkdownContentMode
  /** External library search query; all matches are decorated in either mode. */
  highlightQuery?: string
  onSaveShortcut?: () => void
  autoFocus?: boolean
  review?: MarkdownContentEditorReview
  inlineEdit?: MarkdownContentEditorInlineEdit
  /** Domain extensions must stay stable for the lifetime of this editor. */
  extensions?: Extensions
  readOnly?: boolean
  'aria-label'?: string
  className?: string
}

/**
 * Controlled TipTap Markdown editor with visual and literal-source representations.
 * Persistence and review anchors always use the shared Markdown value; mode is
 * only a document/UI representation and never a second draft owner.
 */
export function MarkdownContentEditor({
  projectId,
  value,
  onChange,
  mode,
  highlightQuery,
  onSaveShortcut,
  autoFocus = false,
  review,
  inlineEdit: inlineEditConfig,
  extensions = [],
  readOnly = false,
  className,
  'aria-label': ariaLabel,
}: MarkdownContentEditorProps) {
  const { t } = useTranslation()
  const initialModeRef = useRef(mode)
  const renderedModeRef = useRef(mode)
  const modeRef = useRef(mode)
  modeRef.current = mode
  const searchStateRef = useRef<SearchState>({ query: '', index: 0, useRegex: false })
  const [reviewPortalTargets, setReviewPortalTargets] = useState<DocumentReviewPortalTarget[]>([])
  const containerRef = useRef<HTMLDivElement>(null)
  const reviewAnnotationsRef = useRef<DocumentReviewAnnotationsHandle>(null)
  const reviewDecorationStateRef = useRef<DocumentReviewDecorationState>({ enabled: false, decorations: [] })
  const lastReviewNavigationNonceRef = useRef<number | null>(null)
  const reviewRef = useRef(review)
  reviewRef.current = review
  // Echoed onChange values never rewrite the ProseMirror document; external
  // updates and representation switches still replace it explicitly.
  const lastEmittedRef = useRef(value)
  const onChangeRef = useRef(onChange)
  const onSaveShortcutRef = useRef(onSaveShortcut)
  useEffect(() => {
    onChangeRef.current = onChange
    onSaveShortcutRef.current = onSaveShortcut
  })

  const searchExtension = useMemo(() => createSearchHighlightExtension(searchStateRef), [])
  const workspaceImageExtension = useMemo(
    () => createWorkspaceImageExtension((path) => projectFileAssetURL(projectId, path)),
    [projectId],
  )
  const updateReviewPortalTargets = useCallback((targets: DocumentReviewPortalTarget[]) => {
    setReviewPortalTargets((current) => sameReviewPortalTargets(current, targets) ? current : targets)
  }, [])
  const reviewExtension = useMemo(
    () => createDocumentReviewExtension(reviewDecorationStateRef, updateReviewPortalTargets),
    [updateReviewPortalTargets],
  )
  const inlineEditExtension = useMemo(() => createInlineEditDecorationExtension(), [])
  const openInlineEditRef = useRef<() => boolean>(() => false)

  const editor = useEditor({
    editable: !readOnly,
    autofocus: autoFocus ? 'end' : false,
    extensions: [
      StarterKit.configure({ hardBreak: false, trailingNode: { notAfter: ['rawMarkdown'] } }),
      RawMarkdown.configure({ isSourceMode: () => modeRef.current === 'source' }),
      createIndentedHardBreakExtension(),
      searchExtension,
      inlineEditExtension,
      workspaceImageExtension,
      reviewExtension,
      TableKit.configure({ table: { resizable: false } }),
      ...extensions,
      Markdown.configure({ marked: createMarkdownParser(), markedOptions: { gfm: true, breaks: true } }),
    ],
    content: initialModeRef.current === 'source'
      ? createMarkdownSourceDocument(value)
      : value,
    contentType: initialModeRef.current === 'source' ? 'json' : 'markdown',
    editorProps: {
      attributes: {
        role: readOnly ? 'document' : 'textbox',
        'aria-multiline': 'true',
        ...(ariaLabel ? { 'aria-label': ariaLabel } : {}),
      },
      handlePaste: (view, event) => (
        modeRef.current === 'rich' && insertPastedWorkspaceMarkdownImage(view, event)
      ),
      handleKeyDown: (_view, event) => {
        if (!event.isComposing && (event.metaKey || event.ctrlKey) && !event.shiftKey && !event.altKey && event.key.toLowerCase() === 'j') {
          if (openInlineEditRef.current()) {
            event.preventDefault()
            event.stopPropagation()
            return true
          }
        }
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
        return false
      },
    },
    onUpdate: ({ editor: instance }) => {
      const markdown = readModeContent(instance, modeRef.current)
      lastEmittedRef.current = markdown
      onChangeRef.current(markdown)
    },
  })

  useEffect(() => {
    const intent = review?.navigationIntent
    if (!intent || lastReviewNavigationNonceRef.current === intent.nonce) return
    const revealed = reviewAnnotationsRef.current?.revealComment(intent.commentID)
    if (revealed) lastReviewNavigationNonceRef.current = intent.nonce
  }, [review?.controller.comments, review?.navigationIntent])

  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    if (ariaLabel) editor.view.dom.setAttribute('aria-label', ariaLabel)
    else editor.view.dom.removeAttribute('aria-label')
  }, [ariaLabel, editor])

  // A representation switch is a schema-valid whole-document replacement. It
  // intentionally clears undo history so source edits cannot undo rich-node steps.
  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    const previousMode = renderedModeRef.current
    const modeChanged = previousMode !== mode
    if (!modeChanged && value === lastEmittedRef.current) return
    const current = readModeContent(editor, previousMode)
    lastEmittedRef.current = value
    renderedModeRef.current = mode
    if (!modeChanged && value === current) return
    replaceEditorDocument(
      editor,
      mode === 'source' ? createMarkdownSourceDocument(value) : value,
      { contentType: mode === 'source' ? 'json' : 'markdown', preserveSelection: !modeChanged },
    )
    if (modeChanged) resetEditorStateHistory(editor)
  }, [editor, mode, value])

  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    const query = highlightQuery?.trim() || ''
    searchStateRef.current = { query, index: 0, useRegex: false }
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    if (!query) return
    const matches = findSearchMatches(editor, query)
    if (matches.length > 0) selectSearchMatch(editor, matches[0])
  }, [editor, highlightQuery, mode])

  const inlineEditEnabled = Boolean(inlineEditConfig && projectId && !readOnly && mode === 'rich')
  const inlineEdit = useTipTapInlineEdit({
    editor,
    containerRef,
    enabled: inlineEditEnabled,
    isMarkdown: true,
    resetKey: `${projectId}:${mode}:${String(readOnly)}`,
    loreItems: inlineEditConfig?.loreItems ?? [],
  })
  openInlineEditRef.current = inlineEdit.openFromCurrentSelection

  const selectionLeadingAction = useMemo(() => inlineEditEnabled ? ({
    label: t('editor.inlineEdit.action'),
    icon: <Sparkles className="h-3.5 w-3.5 text-amber-500 dark:text-amber-400" />,
    onClick: () => { inlineEdit.openFromCurrentSelection() },
  }) : null, [inlineEdit.openFromCurrentSelection, inlineEditEnabled, t])

  const reviewComments = review
    ? review.controller.comments.filter((comment) => sameDocumentReviewTarget(comment.target, review.target))
    : []
  const anchorAdapter = mode === 'source' ? sourceMarkdownReviewAdapter : richMarkdownReviewAdapter

  return (
    <div ref={containerRef} className={cn('relative min-h-0 min-w-0', className)}>
      <EditorContent
        editor={editor}
        data-content-mode={mode}
        className="nova-markdown-editor nova-rich-markdown chat-agent-message min-w-0 text-[var(--nova-text)]"
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
          anchorAdapter={anchorAdapter}
          enableBlockComments={mode === 'rich'}
          onPrepareSnapshot={review.prepareSnapshot}
          onCreate={review.controller.onCreate}
          onUpdate={review.controller.onUpdate}
          onDelete={review.controller.onDelete}
        />
      ) : null}
      {editor && inlineEdit.session && inlineEditConfig ? (
        <InlineEditPopover
          projectId={projectId}
          mode={inlineEditConfig.mode}
          selection={inlineEdit.session.selection}
          before={inlineEdit.session.before}
          after={inlineEdit.session.after}
          matchedLoreIds={inlineEdit.session.matchedLoreIds}
          anchor={inlineEdit.session.anchor}
          containerRef={containerRef}
          onApply={inlineEdit.apply}
          onClose={inlineEdit.close}
        />
      ) : null}
      {editor && !inlineEdit.isOpen && (review || selectionLeadingAction) ? (
        <SelectionToolbar
          editor={editor}
          mode="comment"
          leadingAction={selectionLeadingAction}
          onAction={review ? () => reviewAnnotationsRef.current?.startSelectionComment() : undefined}
        />
      ) : null}
    </div>
  )
}

function readModeContent(editor: Editor, mode: MarkdownContentMode): string {
  if (mode === 'source') return readMarkdownSourceDocument(editor)?.content ?? ''
  return normalizeEditorText(editor.getMarkdown())
}

function sameReviewPortalTargets(current: DocumentReviewPortalTarget[], next: DocumentReviewPortalTarget[]): boolean {
  return current.length === next.length
    && current.every((target, index) => target.key === next[index]?.key && target.element === next[index]?.element)
}
