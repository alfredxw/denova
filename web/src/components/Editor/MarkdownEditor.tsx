import { useEffect, useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
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
  normalizeEditorText,
  readEditorText,
  resetEditorStateHistory,
  updateCharacterStats,
} from './editorDocument'
import {
  clampIndex,
  createDialogueHighlightExtension,
  createSearchHighlightExtension,
  findSearchMatches,
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

interface MarkdownEditorProps {
  /** Canonical workspace identity. Save tasks never cross this boundary. */
  workspace?: string
  fileName: string | null
  content: string
  onSave: (fileName: string, content: string) => Promise<boolean>
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
  const [reviewMode, setReviewMode] = useState(false)
  const [reviewSwitching, setReviewSwitching] = useState(false)
  const [reviewSnapshot, setReviewSnapshot] = useState<DocumentReviewSnapshot | null>(null)
  const [reviewPortalTargets, setReviewPortalTargets] = useState<DocumentReviewPortalTarget[]>([])
  const searchInputRef = useRef<HTMLInputElement>(null)
  const lastIllustrationInsertNonceRef = useRef<number | null>(null)
  const lastSearchIntentNonceRef = useRef<number | null>(null)
  const searchStateRef = useRef<SearchState>({ query: '', index: 0 })
  const searchExtension = useMemo(() => createSearchHighlightExtension(searchStateRef), [])
  const dialogueHighlightExtension = useMemo(() => createDialogueHighlightExtension(), [])
  const workspaceImageExtension = useMemo(() => createWorkspaceImageExtension(), [])
  const editorContainerRef = useRef<HTMLDivElement>(null)
  const reviewAnnotationsRef = useRef<DocumentReviewAnnotationsHandle>(null)
  const reviewDocumentKeyRef = useRef(`${workspace}\u0000${fileName || ''}`)
  const reviewModeRef = useRef(false)
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
  })

  const themeStyle = THEME_STYLES[settings.theme]

  const updateSearch = useCallback((query: string, nextIndex = 0) => {
    if (!editor) return
    const matches = findSearchMatches(editor, query)
    const normalizedIndex = matches.length === 0 ? 0 : clampIndex(nextIndex, matches.length)
    setSearchQuery(query)
    searchStateRef.current = { query, index: normalizedIndex }
    setSearchMatches(matches)
    setSearchIndex(normalizedIndex)
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    if (matches.length > 0) {
      selectSearchMatch(editor, matches[normalizedIndex])
    }
  }, [editor])

  const applyExternalContent = useCallback((nextFile: string | null, nextContent: string, clearHistory: boolean) => {
    if (!editor || editor.isDestroyed) return
    const chain = editor.chain().setMeta('addToHistory', false)
    if (isTxtFile(nextFile)) {
      const html = nextContent.split('\n').map((line) => `<p>${line || '<br>'}</p>`).join('')
      chain.setContent(html, { emitUpdate: false, contentType: 'html' }).run()
    } else {
      chain.setContent(nextContent, { emitUpdate: false, contentType: 'markdown' }).run()
    }
    if (clearHistory) resetEditorStateHistory(editor)
    setNativeIndent(hasNativeIndent(nextContent))
    updateCharacterStats(editor, setSelectedCharacters)
    onLineChange?.(getLineNumber(editor.state.doc, editor.state.selection.head))
    updateSearch(searchStateRef.current.query, 0)
  }, [editor, onLineChange, updateSearch])

  const exitReviewMode = useCallback((reason = 'author') => {
    if (reviewModeRef.current) console.debug('[editor-review] exit review mode', { reason })
    reviewModeRef.current = false
    reviewDecorationStateRef.current = { ...reviewDecorationStateRef.current, enabled: false }
    setReviewMode(false)
    setReviewSnapshot(null)
    if (editor && !editor.isDestroyed) {
      editor.view.dom.removeAttribute('aria-readonly')
      if (typeof editor.setEditable === 'function') editor.setEditable(true)
    }
  }, [editor])

  // This must precede draft persistence's layout effects. Otherwise the
  // Review-mode transaction filter would reject a legitimate file/content
  // replacement before the later passive effect could exit Review mode.
  useLayoutEffect(() => {
    const nextDocumentKey = `${workspace}\u0000${fileName || ''}`
    if (reviewDocumentKeyRef.current !== nextDocumentKey) {
      reviewDocumentKeyRef.current = nextDocumentKey
      exitReviewMode('document_changed')
      return
    }
    if (reviewModeRef.current && reviewSnapshot && normalizeEditorText(content) !== normalizeEditorText(reviewSnapshot.content)) {
      exitReviewMode('content_changed')
    }
  }, [content, exitReviewMode, fileName, reviewSnapshot, workspace])

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

  const enterReviewMode = useCallback(async () => {
    if (!editor || editor.isDestroyed || !fileName || !documentReview || !isMarkdownFile(fileName) || reviewSwitching) return
    setReviewSwitching(true)
    try {
      if (!(await flushCurrentDraft())) return
      const document = await readFile(fileName)
      if (!document.revision || (workspace && document.workspace !== workspace)) {
        throw new Error('The canonical document snapshot is unavailable')
      }
      if (normalizeEditorText(document.content) !== readEditorText(editor, fileName)) {
        throw new Error('The editor and workspace snapshots differ')
      }
      setReviewSnapshot({ content: document.content, revision: document.revision })
      reviewDecorationStateRef.current = { ...reviewDecorationStateRef.current, enabled: true }
      editor.view.dom.setAttribute('aria-readonly', 'true')
      reviewModeRef.current = true
      setReviewMode(true)
      // Selection must remain editable at the view level so ProseMirror keeps
      // browser text selections in sync. The review plugin rejects doc changes.
      if (typeof editor.setEditable === 'function') editor.setEditable(true)
    } catch (error) {
      console.error('进入正文审阅模式失败', { workspace, fileName, error })
      toast.error(t('editor.review.enterFailed'))
    } finally {
      setReviewSwitching(false)
    }
  }, [documentReview, editor, fileName, flushCurrentDraft, reviewSwitching, t, workspace])

  useEffect(() => {
    if (reviewMode && !documentReview) exitReviewMode('review_unavailable')
  }, [documentReview, exitReviewMode, reviewMode])

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

    const matches = findSearchMatches(editor, searchIntent.query)
    const targetIndex = searchIntent.line > 0
      ? matches.findIndex((match) => getLineNumber(editor.state.doc, match.from) === searchIntent.line)
      : -1
    setSearchOpen(true)
    updateSearch(searchIntent.query, targetIndex >= 0 ? targetIndex : 0)
  }, [editor, searchIntent, updateSearch])

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

  // Cmd+Shift+L：编辑模式引用到 Chat，审阅模式创建评论。
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const inCurrentEditor = Boolean(
        editor && !editor.isDestroyed && e.target instanceof globalThis.Node && editor.view.dom.contains(e.target),
      )
      if (isEditableTarget(e.target) && !inCurrentEditor) return

      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'l') {
        e.preventDefault()
        if (reviewMode) commentCurrentSelection()
        else quoteCurrentSelection()
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [commentCurrentSelection, quoteCurrentSelection, reviewMode])

  /** 跳转到下一处搜索结果。 */
  const goToSearchMatch = useCallback((direction: 1 | -1) => {
    if (!editor || searchMatches.length === 0) return
    const nextIndex = clampIndex(searchIndex + direction, searchMatches.length)
    searchStateRef.current = { query: searchQuery, index: nextIndex }
    setSearchIndex(nextIndex)
    editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    selectSearchMatch(editor, searchMatches[nextIndex])
  }, [editor, searchIndex, searchMatches, searchQuery])

  /** 关闭搜索栏并清除高亮。 */
  const closeSearch = useCallback(() => {
    if (editor) {
      searchStateRef.current = { query: '', index: 0 }
      editor.view.dispatch(editor.state.tr.setMeta(searchPluginKey, true))
    }
    setSearchOpen(false)
    setSearchQuery('')
    setSearchIndex(0)
    setSearchMatches([])
    editor?.commands.focus()
  }, [editor])

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
        reviewAvailable={Boolean(documentReview && isMarkdownFile(fileName))}
        reviewMode={reviewMode}
        reviewSwitching={reviewSwitching}
        onReviewModeChange={(nextReviewMode) => {
          if (nextReviewMode) void enterReviewMode()
          else exitReviewMode('author')
        }}
      />
      {externalConflict?.workspace === workspace && externalConflict.fileName === fileName && (
        <div role="alert" className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <TriangleAlert className="h-4 w-4 shrink-0 text-[var(--nova-warning)]" />
          <div className="min-w-[180px] flex-1">
            <div className="font-medium text-[var(--nova-text)]">{t('editor.externalConflict.title')}</div>
            <div className="mt-0.5 text-[var(--nova-text-faint)]">{t('editor.externalConflict.description')}</div>
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
          onQueryChange: (query) => updateSearch(query, 0),
          onNavigate: goToSearchMatch,
          onClose: closeSearch,
        }}
        showSelectionToolbar={selectedCharacters > 0 && (reviewMode || Boolean(onQuoteSelection))}
        selectionToolbarMode={reviewMode ? 'comment' : 'quote'}
        onSelectionAction={reviewMode ? commentCurrentSelection : quoteCurrentSelection}
        reviewMode={reviewMode}
        reviewAnnotations={editor && fileName && documentReview ? (
          <DocumentReviewAnnotations
            ref={reviewAnnotationsRef}
            enabled={reviewMode}
            editor={editor}
            fileName={fileName}
            snapshot={reviewSnapshot}
            containerRef={editorContainerRef}
            comments={documentReview.comments.filter((comment) => comment.path === fileName)}
            decorationStateRef={reviewDecorationStateRef}
            portalTargets={reviewPortalTargets}
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
