import { useEffect, useCallback, useMemo, useRef, useState } from 'react'
import { useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import { CharacterCount } from '@tiptap/extension-character-count'
import { TableKit } from '@tiptap/extension-table'
import { Markdown } from '@tiptap/markdown'
import { TriangleAlert } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'

import type { ChapterIllustration, TextSelection as QuoteSelection } from '@/lib/api'
import type { ChapterSummary } from '@/lib/api'
import { isEditableTarget } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { THEME_STYLES, loadEditorSettings } from './EditorSettingsPanel'
import type { EditorSettings } from './EditorSettingsPanel'
import { EditorSurface } from './EditorSurface'
import { EditorToolbar } from './EditorToolbar'
import {
  countTextCharacters,
  createIndentedHardBreakExtension,
  createWorkspaceImageExtension,
  getLineNumber,
  hasNativeIndent,
  isMarkdownFile,
  isTxtFile,
  placeEditorCaretAtClick,
  replaceEditorDocument,
  resetEditorStateHistory,
  updateCharacterStats,
} from './editorDocument'
import {
  clampIndex,
  createDialogueHighlightExtension,
  createSearchHighlightExtension,
  findSearchMatches,
  replaceAllSearchMatches,
  replaceCurrentSearchMatch,
  searchPluginKey,
  selectSearchMatch,
} from './editorDecorations'
import type { SearchMatch, SearchState } from './editorDecorations'
import { useEditorDraftPersistence, type EditorFlushHandler } from './useEditorDraftPersistence'
import { readFile } from '@/lib/api-client/workspace'
import type { CreateDocumentCommentRequest, DocumentReviewComment } from '@/features/document-review/types'
import { DocumentReviewAnnotations, type DocumentReviewAnnotationsHandle } from './DocumentReviewAnnotations'
import type { DocumentReviewSnapshot } from './documentReviewAnchors'
import { createDocumentReviewExtension, type DocumentReviewDecorationState, type DocumentReviewPortalTarget } from './documentReviewDecorations'

export type { EditorFlushHandler } from './useEditorDraftPersistence'

export interface DocumentReviewController {
  comments: DocumentReviewComment[]
  onCreate: (request: CreateDocumentCommentRequest) => Promise<DocumentReviewComment>
  onUpdate: (comment: DocumentReviewComment, body: string) => Promise<DocumentReviewComment>
  onDelete: (comment: DocumentReviewComment) => Promise<DocumentReviewComment>
}

export interface DocumentReviewNavigationIntent {
  commentID: string
  nonce: number
}

interface MarkdownEditorProps {
  /** Canonical workspace identity. Save tasks never cross this boundary. */
  workspace?: string
  fileName: string | null
  content: string
  revision?: string
  onSave: (fileName: string, content: string, baseRevision: string) => Promise<boolean | { revision?: string }>
  onQuoteSelection?: (sel: QuoteSelection) => void
  saveSignal?: number
  autoSaveEnabled?: boolean
  autoSaveDelayMs?: number
  chapterSummary?: ChapterSummary
  searchIntent?: EditorSearchIntent | null
  onGenerateIllustration?: (chapterPath: string) => void
  generateIllustrationDisabled?: boolean
  illustrationInsertSignal?: { illustration: ChapterIllustration; nonce: number } | null
  onLineChange?: (line: number) => void
  onExternalConflict?: (conflict: { fileName: string; localContent: string; externalContent: string }) => void
  /** Registers the navigation guard used by tabs, previews, and workspace switches. */
  onFlushHandlerChange?: (handler: EditorFlushHandler | null) => void
  documentReview?: DocumentReviewController
  documentReviewNavigationIntent?: DocumentReviewNavigationIntent | null
}

interface EditorSearchIntent {
  query: string
  line: number
  nonce: number
}

/** TipTap 编辑器组件，支持 Markdown 和纯文本格式 */
export function MarkdownEditor({
  workspace = '',
  fileName,
  content,
  revision = '',
  onSave,
  onQuoteSelection,
  saveSignal = 0,
  autoSaveEnabled = true,
  autoSaveDelayMs,
  chapterSummary,
  searchIntent,
  onGenerateIllustration,
  generateIllustrationDisabled = false,
  illustrationInsertSignal,
  onLineChange,
  onExternalConflict,
  onFlushHandlerChange,
  documentReview,
  documentReviewNavigationIntent,
}: MarkdownEditorProps) {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settings, setSettings] = useState<EditorSettings>(() => loadEditorSettings())
  const [nativeIndent, setNativeIndent] = useState(false)
  const [selectedCharacters, setSelectedCharacters] = useState(0)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchIndex, setSearchIndex] = useState(0)
  const [searchMatches, setSearchMatches] = useState<SearchMatch[]>([])
  const [useRegex, setUseRegex] = useState(false)
  const [replaceOpen, setReplaceOpen] = useState(false)
  const [replaceText, setReplaceText] = useState('')
  const [reviewPortalTargets, setReviewPortalTargets] = useState<DocumentReviewPortalTarget[]>([])
  const searchInputRef = useRef<HTMLInputElement>(null)
  const lastIllustrationInsertNonceRef = useRef<number | null>(null)
  const lastSearchIntentNonceRef = useRef<number | null>(null)
  const lastDocumentReviewNavigationNonceRef = useRef<number | null>(null)
  const searchStateRef = useRef<SearchState>({ query: '', index: 0, useRegex: false })
  const searchExtension = useMemo(() => createSearchHighlightExtension(searchStateRef), [])
  const dialogueHighlightExtension = useMemo(() => createDialogueHighlightExtension(), [])
  const workspaceImageExtension = useMemo(() => createWorkspaceImageExtension(), [])
  const editorContainerRef = useRef<HTMLDivElement>(null)
  const reviewAnnotationsRef = useRef<DocumentReviewAnnotationsHandle>(null)
  const reviewDecorationStateRef = useRef<DocumentReviewDecorationState>({ enabled: false, decorations: [] })
  const updateReviewPortalTargets = useCallback((targets: DocumentReviewPortalTarget[]) => {
    setReviewPortalTargets((current) => sameReviewPortalTargets(current, targets) ? current : targets)
  }, [])
  const reviewExtension = useMemo(() => createDocumentReviewExtension(reviewDecorationStateRef, updateReviewPortalTargets), [updateReviewPortalTargets])
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        hardBreak: false,
      }),
      createIndentedHardBreakExtension(),
      searchExtension,
      dialogueHighlightExtension,
      workspaceImageExtension,
      reviewExtension,
      TableKit.configure({
        table: {
          resizable: false,
        },
      }),
      Markdown.configure({
        markedOptions: {
          gfm: true,
          breaks: true,
        },
      }),
      CharacterCount.configure({
        textCounter: countTextCharacters,
      }),
      Placeholder.configure({
        placeholder: t('editor.placeholder'),
      }),
    ],
    content,
    contentType: 'markdown',
    editorProps: {
      handleClick: (view, position, event) => {
        placeEditorCaretAtClick(view, position, event)
        // Keep review-highlight and future extension click handlers in the same event chain.
        return false
      },
    },
  })

  useEffect(() => {
    if (!documentReviewNavigationIntent) return
    if (lastDocumentReviewNavigationNonceRef.current === documentReviewNavigationIntent.nonce) return
    const revealed = reviewAnnotationsRef.current?.revealComment(documentReviewNavigationIntent.commentID)
    if (revealed) lastDocumentReviewNavigationNonceRef.current = documentReviewNavigationIntent.nonce
  }, [documentReview?.comments, documentReviewNavigationIntent])

  const themeStyle = THEME_STYLES[settings.theme]

  const updateSearch = useCallback((query: string, nextIndex = 0) => {
    if (!editor) return
    const matches = findSearchMatches(editor, query, useRegex)
    const normalizedIndex = matches.length === 0 ? 0 : clampIndex(nextIndex, matches.length)
    setSearchQuery(query)
    searchStateRef.current = { query, index: normalizedIndex, useRegex }
    setSearchMatches(matches)
    setSearchIndex(normalizedIndex)
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    if (matches.length > 0) {
      selectSearchMatch(editor, matches[normalizedIndex])
    }
  }, [editor, useRegex])

  const applyExternalContent = useCallback((
    nextFile: string | null,
    nextContent: string,
    options: { resetHistory: boolean; preserveSelection: boolean },
  ) => {
    if (!editor || editor.isDestroyed) return
    if (isTxtFile(nextFile)) {
      const html = nextContent.split('\n').map((line) => `<p>${line || '<br>'}</p>`).join('')
      replaceEditorDocument(editor, html, {
        contentType: 'html',
        preserveSelection: options.preserveSelection,
      })
    } else {
      replaceEditorDocument(editor, nextContent, {
        contentType: 'markdown',
        preserveSelection: options.preserveSelection,
      })
    }
    if (options.resetHistory) resetEditorStateHistory(editor)
    setNativeIndent(hasNativeIndent(nextContent))
    updateCharacterStats(editor, setSelectedCharacters)
    onLineChange?.(getLineNumber(editor.state.doc, editor.state.selection.head))
    updateSearch(searchStateRef.current.query, 0)
  }, [editor, onLineChange, updateSearch])

  const {
    saveStatus,
    externalConflict,
    externalConflictSaving,
    handleSave,
    flushCurrentDraft,
    loadExternalVersion,
    keepLocalVersion,
  } = useEditorDraftPersistence({
    workspace,
    fileName,
    content,
    revision,
    editor,
    editorContainerRef,
    onSave,
    saveSignal,
    autoSaveEnabled,
    autoSaveDelayMs,
    applyExternalContent,
    onExternalConflict,
    onFlushHandlerChange,
  })

  const prepareDocumentReviewSnapshot = useCallback(async (): Promise<DocumentReviewSnapshot> => {
    if (!editor || editor.isDestroyed || !fileName || !documentReview || !isMarkdownFile(fileName)) {
      throw new Error('Document comments are unavailable')
    }
    if (!(await flushCurrentDraft())) throw new Error('The current draft could not be saved')
    const document = await readFile(fileName)
    if (!document.revision || (workspace && document.workspace !== workspace)) {
      throw new Error('The canonical document snapshot is unavailable')
    }
    // TipTap can insert equivalent blank lines or normalize Markdown markers.
    // The anchor builder validates the selected range against this canonical snapshot.
    return { content: document.content, revision: document.revision }
  }, [documentReview, editor, fileName, flushCurrentDraft, workspace])

  // 监听 TipTap 内容和选区变化，实时更新选区字数与光标行号。
  useEffect(() => {
    if (!editor) return

    const updateStats = () => {
      updateCharacterStats(editor, setSelectedCharacters)
      onLineChange?.(getLineNumber(editor.state.doc, editor.state.selection.head))
    }
    updateStats()
    editor.on('update', updateStats)
    editor.on('selectionUpdate', updateStats)
    return () => {
      editor.off('update', updateStats)
      editor.off('selectionUpdate', updateStats)
    }
  }, [editor, onLineChange])

  // 保存编辑器设置
  useEffect(() => {
    localStorage.setItem('nova.editor.settings', JSON.stringify(settings))
  }, [settings])

  useEffect(() => {
    if (searchOpen) {
      requestAnimationFrame(() => searchInputRef.current?.focus())
    }
  }, [searchOpen])

  useEffect(() => {
    if (searchOpen) {
      updateSearch(searchQuery, searchIndex)
    }
  }, [searchOpen, searchQuery, searchIndex, updateSearch])

  useEffect(() => {
    if (!editor || !searchIntent || !searchIntent.query.trim()) return
    if (lastSearchIntentNonceRef.current === searchIntent.nonce) return
    lastSearchIntentNonceRef.current = searchIntent.nonce

    const matches = findSearchMatches(editor, searchIntent.query, useRegex)
    const targetIndex = searchIntent.line > 0
      ? matches.findIndex((match) => getLineNumber(editor.state.doc, match.from) === searchIntent.line)
      : -1
    setSearchOpen(true)
    updateSearch(searchIntent.query, targetIndex >= 0 ? targetIndex : 0)
  }, [editor, searchIntent, updateSearch, useRegex])

  useEffect(() => {
    if (!editor || !illustrationInsertSignal) return
    if (lastIllustrationInsertNonceRef.current === illustrationInsertSignal.nonce) return
    lastIllustrationInsertNonceRef.current = illustrationInsertSignal.nonce
    if (!fileName || isTxtFile(fileName) || !isMarkdownFile(fileName)) {
      toast.error(t('editor.illustrationMarkdownOnly'))
      return
    }
    const { illustration } = illustrationInsertSignal
    const imagePath = illustration.image_path
    if (!imagePath) {
      toast.error(t('editor.illustrationInsertFailed'))
      return
    }
    const insertAt = Math.max(1, editor.state.selection.from || 1)
    const ok = editor
      .chain()
      .focus()
      .insertContentAt(insertAt, {
        type: 'image',
        attrs: {
          src: imagePath,
          alt: illustration.alt_text || t('chat.illustration.previewAlt'),
          title: illustration.alt_text || undefined,
        },
      })
      .run()
    if (!ok) {
      toast.error(t('editor.illustrationInsertFailed'))
      return
    }
  }, [editor, fileName, illustrationInsertSignal, t])

  // Ctrl+F / Cmd+F 打开文章内搜索，保存快捷键由工作台统一分发。
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      // 当焦点在 chat 输入框等 textarea/input 时，不拦截快捷键
      const inCurrentEditor = Boolean(
        editor && !editor.isDestroyed && e.target instanceof globalThis.Node && editor.view.dom.contains(e.target),
      )
      if (isEditableTarget(e.target) && !inCurrentEditor) return

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'f') {
        e.preventDefault()
        setSearchOpen(true)
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [editor])

  /** 引用当前选区到 Chat */
  const quoteCurrentSelection = useCallback(() => {
    if (!editor || !fileName || !onQuoteSelection) return
    const { from, to } = editor.state.selection
    if (from === to) return // 无选区
    const text = editor.state.doc.textBetween(from, to, '\n')
    if (!text.trim()) return
    // 计算行号
    const startLine = getLineNumber(editor.state.doc, from)
    const endLine = getLineNumber(editor.state.doc, to)
    onQuoteSelection({ fileName, startLine, endLine, content: text })
  }, [editor, fileName, onQuoteSelection])

  const commentCurrentSelection = useCallback(() => {
    reviewAnnotationsRef.current?.startSelectionComment()
  }, [])

  const documentCommentsAvailable = Boolean(documentReview && fileName && isMarkdownFile(fileName))

  // Cmd+Shift+L：正文支持评论时创建评论，其他文件保留原有的选区引用能力。
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const inCurrentEditor = Boolean(
        editor && !editor.isDestroyed && e.target instanceof globalThis.Node && editor.view.dom.contains(e.target),
      )
      if (isEditableTarget(e.target) && !inCurrentEditor) return

      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'l') {
        e.preventDefault()
        if (documentCommentsAvailable) commentCurrentSelection()
        else quoteCurrentSelection()
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [commentCurrentSelection, documentCommentsAvailable, editor, quoteCurrentSelection])

  /** 跳转到下一处搜索结果。 */
  const goToSearchMatch = useCallback((direction: 1 | -1) => {
    if (!editor || searchMatches.length === 0) return
    const nextIndex = clampIndex(searchIndex + direction, searchMatches.length)
    searchStateRef.current = { query: searchQuery, index: nextIndex, useRegex }
    setSearchIndex(nextIndex)
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    selectSearchMatch(editor, searchMatches[nextIndex])
  }, [editor, searchIndex, searchMatches, searchQuery, useRegex])

  /** 切换正则匹配模式并刷新搜索结果。 */
  const toggleRegex = useCallback(() => {
    setUseRegex((prev) => {
      const next = !prev
      searchStateRef.current = { ...searchStateRef.current, useRegex: next }
      return next
    })
  }, [])

  /** 切换替换栏展开状态。 */
  const toggleReplace = useCallback(() => {
    setReplaceOpen((prev) => !prev)
  }, [])

  /** 替换当前匹配项并刷新搜索结果。 */
  const handleReplace = useCallback(() => {
    if (!editor) return
    const replaced = replaceCurrentSearchMatch(editor, searchQuery, replaceText, useRegex, searchIndex)
    if (!replaced) return
    // 替换后文档变化，重新计算匹配并定位到下一处
    const matches = findSearchMatches(editor, searchQuery, useRegex)
    const nextIndex = matches.length === 0 ? 0 : clampIndex(searchIndex, matches.length)
    searchStateRef.current = { query: searchQuery, index: nextIndex, useRegex }
    setSearchMatches(matches)
    setSearchIndex(nextIndex)
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    if (matches.length > 0) {
      selectSearchMatch(editor, matches[nextIndex])
    }
  }, [editor, searchQuery, replaceText, useRegex, searchIndex])

  /** 批量替换所有匹配项。 */
  const handleReplaceAll = useCallback(() => {
    if (!editor) return
    const count = replaceAllSearchMatches(editor, searchQuery, replaceText, useRegex)
    if (count === 0) return
    // 替换完成后清空匹配高亮
    searchStateRef.current = { query: searchQuery, index: 0, useRegex }
    setSearchMatches([])
    setSearchIndex(0)
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    toast.success(t('editor.replaceAllDone', { count }))
  }, [editor, searchQuery, replaceText, useRegex, t])

  /** 关闭搜索栏并清除高亮。 */
  const closeSearch = useCallback(() => {
    if (editor) {
      searchStateRef.current = { query: '', index: 0, useRegex }
      editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    }
    setSearchOpen(false)
    setSearchQuery('')
    setSearchIndex(0)
    setSearchMatches([])
    setReplaceOpen(false)
    setReplaceText('')
    editor?.commands.focus()
  }, [editor, useRegex])

  // 未选中文件时显示占位
  if (!fileName) {
    return (
      <div className="flex-1 flex items-center justify-center text-gray-400 text-sm">
        {t('editor.noFile')}
      </div>
    )
  }

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <EditorToolbar
        fileName={fileName}
        displayTitle={chapterSummary?.display_title}
        chapterPath={chapterSummary?.path}
        saveStatus={saveStatus}
        onSave={handleSave}
        settingsOpen={settingsOpen}
        onSettingsOpenChange={setSettingsOpen}
        settings={settings}
        onSettingsChange={setSettings}
        onGenerateIllustration={onGenerateIllustration}
        generateIllustrationDisabled={generateIllustrationDisabled}
      />
      {externalConflict?.workspace === workspace && externalConflict.fileName === fileName && (
        <div role="alert" className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <TriangleAlert className="h-4 w-4 shrink-0 text-[var(--nova-warning)]" />
          <div className="min-w-[180px] flex-1">
            <div className="font-medium text-[var(--nova-text)]">{t('editor.externalConflict.title')}</div>
            <div className="mt-0.5 text-[var(--nova-text-faint)]">{t('editor.externalConflict.description')}</div>
            {externalConflict.recoveryID ? (
              <div className="mt-0.5 font-mono text-[10px] text-[var(--nova-text-faint)]">
                {t('editor.externalConflict.recovery', { id: externalConflict.recoveryID })}
              </div>
            ) : null}
          </div>
          <Button type="button" size="xs" variant="outline" disabled={externalConflictSaving} onClick={() => void keepLocalVersion()}>{t('editor.externalConflict.keepLocal')}</Button>
          <Button type="button" size="xs" disabled={externalConflictSaving} onClick={loadExternalVersion}>{t('editor.externalConflict.loadExternal')}</Button>
        </div>
      )}
      <EditorSurface
        containerRef={editorContainerRef}
        editor={editor}
        settings={settings}
        themeStyle={themeStyle}
        nativeIndent={nativeIndent}
        search={{
          open: searchOpen,
          inputRef: searchInputRef,
          query: searchQuery,
          matchIndex: searchIndex,
          matchCount: searchMatches.length,
          useRegex,
          replaceOpen,
          replaceText,
          onQueryChange: (query) => updateSearch(query, 0),
          onNavigate: goToSearchMatch,
          onClose: closeSearch,
          onToggleRegex: toggleRegex,
          onToggleReplace: toggleReplace,
          onReplaceChange: setReplaceText,
          onReplace: handleReplace,
          onReplaceAll: handleReplaceAll,
        }}
        showSelectionToolbar={selectedCharacters > 0 && (documentCommentsAvailable || Boolean(onQuoteSelection))}
        selectionToolbarMode={documentCommentsAvailable ? 'comment' : 'quote'}
        onSelectionAction={documentCommentsAvailable ? commentCurrentSelection : quoteCurrentSelection}
        reviewAnnotations={editor && fileName && documentReview && documentCommentsAvailable ? (
          <DocumentReviewAnnotations
            ref={reviewAnnotationsRef}
            editor={editor}
            fileName={fileName}
            containerRef={editorContainerRef}
            comments={documentReview.comments.filter((comment) => comment.path === fileName)}
            decorationStateRef={reviewDecorationStateRef}
            portalTargets={reviewPortalTargets}
            onPrepareSnapshot={prepareDocumentReviewSnapshot}
            onCreate={documentReview.onCreate}
            onUpdate={documentReview.onUpdate}
            onDelete={documentReview.onDelete}
          />
        ) : null}
      />
    </div>
  )
}

function sameReviewPortalTargets(current: DocumentReviewPortalTarget[], next: DocumentReviewPortalTarget[]): boolean {
  return current.length === next.length && current.every((target, index) => target.key === next[index]?.key && target.element === next[index]?.element)
}
