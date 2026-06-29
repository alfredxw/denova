import { useEffect, useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { PointerEvent } from 'react'
import type { Editor } from '@tiptap/react'
import { useEditor, EditorContent } from '@tiptap/react'
import { Extension, Node, mergeAttributes } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import { CharacterCount } from '@tiptap/extension-character-count'
import Image from '@tiptap/extension-image'
import { Markdown } from '@tiptap/markdown'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey, TextSelection as PmTextSelection } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import { BookOpen, Check, ChevronDown, ChevronUp, ImagePlus, MessageSquareQuote, Palette, Rows3, Save, Search, Settings, X } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'

import type { ChapterIllustration, TextSelection as QuoteSelection } from '@/lib/api'
import type { ChapterSummary } from '@/lib/api'
import { workspaceAssetURL } from '@/lib/api'
import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'
import { isEditableTarget } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { formatLocaleNumber } from '@/i18n'

interface MarkdownEditorProps {
  fileName: string | null
  content: string
  onSave: (content: string) => Promise<boolean>
  onQuoteSelection?: (sel: QuoteSelection) => void
  saveSignal?: number
  autoSaveEnabled?: boolean
  autoSaveDelayMs?: number
  chapterSummary?: ChapterSummary
  searchIntent?: EditorSearchIntent | null
  onGenerateIllustration?: (chapterPath: string) => void
  generateIllustrationDisabled?: boolean
  illustrationInsertSignal?: { illustration: ChapterIllustration; nonce: number } | null
}

export interface EditorSearchIntent {
  query: string
  line: number
  nonce: number
}

type EditorTheme = 'ide' | 'paper' | 'sepia'
type SaveStatus = 'dirty' | 'auto-saving' | 'auto-saved' | 'manual-saving' | 'manual-saved' | 'error'
type PendingSave = { text: string; mode: 'manual' | 'auto' }

interface EditorSettings {
  lineHeight: number
  theme: EditorTheme
  dialogueHighlightColor: string
}

interface SearchState {
  query: string
  index: number
}

interface SearchMatch {
  from: number
  to: number
}

const searchPluginKey = new PluginKey<DecorationSet>('nova-search-highlight')
const dialogueHighlightPluginKey = new PluginKey<DecorationSet>('nova-editor-dialogue-highlight')
const DEFAULT_DIALOGUE_HIGHLIGHT_COLOR = ''
const COLOR_VALUE_PATTERN = /^#[0-9a-fA-F]{6}$/
const DEFAULT_PICKER_COLOR = '#ffd166'

const DEFAULT_SETTINGS: EditorSettings = {
  lineHeight: 1.9,
  theme: 'ide',
  dialogueHighlightColor: DEFAULT_DIALOGUE_HIGHLIGHT_COLOR,
}

const DEFAULT_AUTO_SAVE_DELAY_MS = 1500

const THEME_STYLES: Record<EditorTheme, { labelKey: string; background: string; color: string; accent: string; dialogueHighlight: string }> = {
  ide: {
    labelKey: 'editor.theme.ide',
    background: 'var(--nova-editor-ide-bg)',
    color: 'var(--nova-editor-ide-color)',
    accent: 'var(--nova-editor-ide-accent)',
    dialogueHighlight: 'var(--nova-dialogue-highlight)',
  },
  paper: {
    labelKey: 'editor.theme.paper',
    background: '#f5efe4',
    color: '#252525',
    accent: '#dfd3c2',
    dialogueHighlight: '#8a3f13',
  },
  sepia: {
    labelKey: 'editor.theme.sepia',
    background: '#efe3cc',
    color: '#2f271f',
    accent: '#d8c6a6',
    dialogueHighlight: '#75451f',
  },
}

const SAVE_STATUS_META: Record<SaveStatus, { labelKey: string; ariaLabelKey: string; className: string; dotClassName?: string; subtle?: boolean }> = {
  dirty: {
    labelKey: 'editor.status.dirty',
    ariaLabelKey: 'editor.status.dirtyAria',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'bg-[var(--nova-text-faint)] opacity-60',
    subtle: true,
  },
  'auto-saving': {
    labelKey: 'editor.status.autoSaving',
    ariaLabelKey: 'editor.status.autoSavingAria',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'animate-pulse bg-[var(--nova-text-muted)] opacity-70',
    subtle: true,
  },
  'auto-saved': {
    labelKey: 'editor.status.autoSaved',
    ariaLabelKey: 'editor.status.autoSavedAria',
    className: 'text-[var(--nova-text-faint)]',
    subtle: true,
  },
  'manual-saving': {
    labelKey: 'editor.status.manualSaving',
    ariaLabelKey: 'editor.status.manualSavingAria',
    className: 'text-[var(--nova-text-muted)]',
  },
  'manual-saved': {
    labelKey: 'editor.status.manualSaved',
    ariaLabelKey: 'editor.status.manualSavedAria',
    className: 'text-[var(--nova-accent-green)]',
  },
  error: {
    labelKey: 'editor.status.error',
    ariaLabelKey: 'editor.status.errorAria',
    className: 'text-[var(--nova-danger)]',
  },
}

/** 检测文本是否已自带缩进（首个非空行以全角/半角空格开头） */
function hasNativeIndent(text: string): boolean {
  const lines = text.split('\n')
  for (const line of lines) {
    if (!line.trim()) continue
    return /^[\s\u3000]{2,}/.test(line)
  }
  return false
}

/** 判断文件是否为纯文本（.txt）格式 */
function isTxtFile(name: string | null): boolean {
  return !!name && name.toLowerCase().endsWith('.txt')
}

function isMarkdownFile(name: string | null): boolean {
  return !!name && /\.(md|markdown)$/i.test(name)
}

function createWorkspaceImageExtension() {
  return Image.extend({
    renderHTML({ HTMLAttributes }) {
      const src = resolveWorkspaceImageSrc(HTMLAttributes.src)
      return ['img', mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, { src })]
    },
  }).configure({
    inline: false,
    allowBase64: true,
  })
}

function resolveWorkspaceImageSrc(src: unknown) {
  if (typeof src !== 'string' || src.trim() === '') return src
  const value = src.trim()
  if (/^(https?:|data:|blob:|\/)/i.test(value)) return value
  if (value.startsWith('assets/')) return workspaceAssetURL(value)
  return value
}

function normalizeAutoSaveDelayMs(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return DEFAULT_AUTO_SAVE_DELAY_MS
  }
  return Math.floor(value)
}

/** TipTap 编辑器组件，支持 Markdown 和纯文本格式 */
export function MarkdownEditor({
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
}: MarkdownEditorProps) {
  const { t } = useTranslation()
  const [saveStatus, setSaveStatus] = useState<SaveStatus | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settings, setSettings] = useState<EditorSettings>(() => loadEditorSettings())
  const [nativeIndent, setNativeIndent] = useState(false)
  const [selectedCharacters, setSelectedCharacters] = useState(0)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchIndex, setSearchIndex] = useState(0)
  const [searchMatches, setSearchMatches] = useState<SearchMatch[]>([])
  const autoSaveTimer = useRef<number | null>(null)
  const saveStatusClearTimer = useRef<number | null>(null)
  const saveInFlightRef = useRef(false)
  const pendingSaveRef = useRef<PendingSave | null>(null)
  const lastSyncedFileRef = useRef<string | null>(null)
  const lastSyncedContentRef = useRef('')
  const fileNameRef = useRef<string | null>(fileName)
  const autoSaveEnabledRef = useRef(autoSaveEnabled)
  const autoSaveDelayMsRef = useRef(normalizeAutoSaveDelayMs(autoSaveDelayMs))
  const saveEditorContentRef = useRef<(mode: 'manual' | 'auto') => Promise<void>>(async () => {})
  const searchInputRef = useRef<HTMLInputElement>(null)
  const lastSaveSignalRef = useRef(saveSignal)
  const lastIllustrationInsertNonceRef = useRef<number | null>(null)
  const lastSearchIntentNonceRef = useRef<number | null>(null)
  const searchStateRef = useRef<SearchState>({ query: '', index: 0 })
  const searchExtension = useMemo(() => createSearchHighlightExtension(searchStateRef), [])
  const dialogueHighlightExtension = useMemo(() => createDialogueHighlightExtension(), [])
  const workspaceImageExtension = useMemo(() => createWorkspaceImageExtension(), [])
  const editorContainerRef = useRef<HTMLDivElement>(null)
  /** 每个文件的滚动位置缓存 */
  const filePositionsRef = useRef<Map<string, { scrollTop: number }>>(new Map())
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        hardBreak: false,
      }),
      /* 自定义 HardBreak：渲染为 <span class="nova-hard-break"><br></span>，
         配合 CSS ::after 伪元素在换行后添加 2em 缩进 */
      Node.create({
        name: 'hardBreak',
        inline: true,
        group: 'inline',
        selectable: false,
        linebreakReplacement: true,
        parseHTML() {
          return [{ tag: 'br' }]
        },
        renderHTML() {
          return ['span', { class: 'nova-hard-break' }, ['br']]
        },
        addKeyboardShortcuts() {
          return {
            'Shift-Enter': () => this.editor.commands.setHardBreak(),
          }
        },
        addCommands() {
          return {
            setHardBreak: () => ({ commands }) => {
              return commands.first([
                () => commands.exitCode(),
                () => commands.insertContent({ type: this.name }),
              ])
            },
          }
        },
      }),
      searchExtension,
      dialogueHighlightExtension,
      workspaceImageExtension,
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
  const resolvedAutoSaveDelayMs = normalizeAutoSaveDelayMs(autoSaveDelayMs)

  useEffect(() => {
    fileNameRef.current = fileName
  }, [fileName])

  useEffect(() => {
    autoSaveEnabledRef.current = autoSaveEnabled
    if (!autoSaveEnabled && autoSaveTimer.current) {
      window.clearTimeout(autoSaveTimer.current)
      autoSaveTimer.current = null
    }
  }, [autoSaveEnabled])

  useEffect(() => {
    autoSaveDelayMsRef.current = resolvedAutoSaveDelayMs
  }, [resolvedAutoSaveDelayMs])

  useEffect(() => {
    if (autoSaveTimer.current) {
      window.clearTimeout(autoSaveTimer.current)
      autoSaveTimer.current = null
    }
  }, [fileName])

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

  // 仅在切换文件或外部内容真正变化时同步，避免自动保存后重置光标。
  useLayoutEffect(() => {
    if (!editor) return

    const fileChanged = lastSyncedFileRef.current !== fileName
    const contentChanged = lastSyncedContentRef.current !== content
    if (!fileChanged && !contentChanged) return

    const scrollEl = editorContainerRef.current

    // 切换文件前：保存旧文件的滚动位置
    if (fileChanged && lastSyncedFileRef.current) {
      filePositionsRef.current.set(lastSyncedFileRef.current, {
        scrollTop: scrollEl?.scrollTop ?? 0,
      })
    }

    // 切换文件时先隐藏，防止闪烁
    if (fileChanged && scrollEl) {
      scrollEl.style.visibility = 'hidden'
    }

    lastSyncedFileRef.current = fileName
    lastSyncedContentRef.current = content
    setNativeIndent(hasNativeIndent(content))
    if (isTxtFile(fileName)) {
      // 纯文本：将换行转换为段落，以 HTML 方式写入编辑器
      const html = content.split('\n').map((line) => `<p>${line || '<br>'}</p>`).join('')
      editor.commands.setContent(html, { emitUpdate: false, contentType: 'html' })
    } else {
      editor.commands.setContent(content, { emitUpdate: false, contentType: 'markdown' })
    }
    updateCharacterStats(editor, setSelectedCharacters)
    updateSearch(searchStateRef.current.query, 0)

    // 切换文件后：等待 DOM 渲染完成再恢复位置并显示
    if (fileChanged && scrollEl) {
      const saved = fileName ? filePositionsRef.current.get(fileName) : null
      requestAnimationFrame(() => {
        scrollEl.scrollTop = saved ? saved.scrollTop : 0
        scrollEl.style.visibility = ''
      })
    }
  }, [content, editor, fileName, updateSearch])

  // 监听 TipTap 内容和选区变化，实时更新选区字数。
  useEffect(() => {
    if (!editor) return

    const updateStats = () => updateCharacterStats(editor, setSelectedCharacters)
    updateStats()
    editor.on('update', updateStats)
    editor.on('selectionUpdate', updateStats)
    return () => {
      editor.off('update', updateStats)
      editor.off('selectionUpdate', updateStats)
    }
  }, [editor])

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

  const clearSaveStatusTimer = useCallback(() => {
    if (saveStatusClearTimer.current) {
      window.clearTimeout(saveStatusClearTimer.current)
      saveStatusClearTimer.current = null
    }
  }, [])

  const scheduleSaveStatusClear = useCallback((status: SaveStatus, delay: number) => {
    clearSaveStatusTimer()
    saveStatusClearTimer.current = window.setTimeout(() => {
      setSaveStatus((current) => current === status ? null : current)
      saveStatusClearTimer.current = null
    }, delay)
  }, [clearSaveStatusTimer])

  useEffect(() => clearSaveStatusTimer, [clearSaveStatusTimer])

  const persistEditorContent = useCallback(async (text: string, mode: 'manual' | 'auto') => {
    clearSaveStatusTimer()
    setSaveStatus(mode === 'auto' ? 'auto-saving' : 'manual-saving')
    const ok = await onSave(text)
    const nextStatus: SaveStatus = ok ? (mode === 'auto' ? 'auto-saved' : 'manual-saved') : 'error'
    setSaveStatus(nextStatus)
    if (mode === 'manual') {
      if (!ok) toast.error(t('editor.saveFailed'))
    }
    scheduleSaveStatusClear(nextStatus, mode === 'auto' ? 1400 : 2000)
  }, [clearSaveStatusTimer, onSave, scheduleSaveStatusClear, t])

  const queueEditorSave = useCallback(async (text: string, mode: 'manual' | 'auto') => {
    lastSyncedContentRef.current = text
    if (saveInFlightRef.current) {
      const pendingMode = mode === 'manual' || pendingSaveRef.current?.mode === 'manual' ? 'manual' : 'auto'
      pendingSaveRef.current = { text, mode: pendingMode }
      setSaveStatus(pendingMode === 'auto' ? 'auto-saving' : 'manual-saving')
      return
    }

    saveInFlightRef.current = true
    let nextText = text
    let nextMode = mode
    try {
      for (;;) {
        pendingSaveRef.current = null
        await persistEditorContent(nextText, nextMode)
        const pending = pendingSaveRef.current as PendingSave | null
        if (!pending || pending.text === nextText) break
        nextText = pending.text
        nextMode = pending.mode
      }
    } finally {
      saveInFlightRef.current = false
    }
  }, [persistEditorContent])

  /** 保存当前编辑内容 */
  const saveEditorContent = useCallback(async (mode: 'manual' | 'auto') => {
    if (!editor || !fileName) return
    const text = isTxtFile(fileName)
      ? normalizeEditorText(editor.getText({ blockSeparator: '\n' }))
      : normalizeEditorText(editor.getMarkdown())
    await queueEditorSave(text, mode)
  }, [editor, fileName, queueEditorSave])

  useEffect(() => {
    saveEditorContentRef.current = saveEditorContent
  }, [saveEditorContent])

  /** 执行手动保存 */
  const handleSave = useCallback(async () => {
    if (autoSaveTimer.current) {
      window.clearTimeout(autoSaveTimer.current)
      autoSaveTimer.current = null
    }
    await saveEditorContent('manual')
  }, [saveEditorContent])

  useEffect(() => {
    if (saveSignal === lastSaveSignalRef.current) return
    lastSaveSignalRef.current = saveSignal
    void handleSave()
  }, [handleSave, saveSignal])

  // 用户修改后延迟自动保存；外部内容同步使用 emitUpdate: false，不会进入这里。
  useEffect(() => {
    if (!editor) return

    const handleUpdate = () => {
      if (!fileNameRef.current) return
      clearSaveStatusTimer()
      setSaveStatus('dirty')
      if (!autoSaveEnabledRef.current) {
        if (autoSaveTimer.current) {
          window.clearTimeout(autoSaveTimer.current)
          autoSaveTimer.current = null
        }
        return
      }
      if (autoSaveTimer.current) {
        window.clearTimeout(autoSaveTimer.current)
      }
      autoSaveTimer.current = window.setTimeout(() => {
        autoSaveTimer.current = null
        void saveEditorContentRef.current('auto')
      }, autoSaveDelayMsRef.current)
    }

    editor.on('update', handleUpdate)
    return () => {
      editor.off('update', handleUpdate)
      if (autoSaveTimer.current) {
        window.clearTimeout(autoSaveTimer.current)
      }
    }
  }, [clearSaveStatusTimer, editor])

  // Ctrl+F / Cmd+F 打开文章内搜索，保存快捷键由工作台统一分发。
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      // 当焦点在 chat 输入框等 textarea/input 时，不拦截快捷键
      const inCurrentEditor = e.target instanceof globalThis.Node && editor?.view.dom.contains(e.target)
      if (isEditableTarget(e.target) && !inCurrentEditor) return

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'f') {
        e.preventDefault()
        setSearchOpen(true)
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [])

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

  // Cmd+Shift+L 快捷键：引用选区到 Chat
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const inCurrentEditor = e.target instanceof globalThis.Node && editor?.view.dom.contains(e.target)
      if (isEditableTarget(e.target) && !inCurrentEditor) return

      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'l') {
        e.preventDefault()
        quoteCurrentSelection()
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [quoteCurrentSelection])

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

  const saveStatusMeta = saveStatus ? SAVE_STATUS_META[saveStatus] : null
  const saveStatusLabel = saveStatusMeta ? t(saveStatusMeta.labelKey) : ''
  const saveStatusAriaLabel = saveStatusMeta ? t(saveStatusMeta.ariaLabelKey) : ''

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* 编辑器工具栏 */}
      <div className="nova-editor-toolbar flex h-9 shrink-0 items-center justify-between gap-3 overflow-hidden border-b px-3">
        <div className="flex min-w-0 items-center gap-2 text-xs text-[var(--nova-text-muted)]">
          <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
          <span className="truncate font-medium text-[var(--nova-text)]">{chapterSummary?.display_title || fileName}</span>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {saveStatusMeta && (
            <span
              className={`inline-flex h-5 min-w-5 items-center justify-end gap-1 text-[11px] transition-colors ${saveStatusMeta.className}`}
              aria-live="polite"
              aria-label={saveStatusAriaLabel}
              title={saveStatusAriaLabel}
            >
              {saveStatus === 'auto-saved' ? (
                <Check className="h-3 w-3 opacity-45" />
              ) : saveStatusMeta.dotClassName ? (
                <span className={`h-1.5 w-1.5 rounded-full ${saveStatusMeta.dotClassName}`} />
              ) : null}
              <span className={saveStatusMeta.subtle ? 'sr-only' : ''}>{saveStatusLabel}</span>
            </span>
          )}
          {onGenerateIllustration && (
            <TooltipIconButton
              label={generateIllustrationDisabled ? t('editor.generateIllustrationDisabled') : t('editor.generateIllustration')}
              size="icon-xs"
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-45"
              disabled={generateIllustrationDisabled || !chapterSummary?.path}
              onClick={() => {
                if (chapterSummary?.path) onGenerateIllustration(chapterSummary.path)
              }}
            >
              <ImagePlus className="h-3.5 w-3.5" />
            </TooltipIconButton>
          )}
          <Button
            type="button"
            onClick={handleSave}
            size="xs"
            variant="ghost"
            className="flex items-center gap-1 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          >
            <Save className="w-3.5 h-3.5" />
            {t('editor.save')}
          </Button>
          <Popover open={settingsOpen} onOpenChange={setSettingsOpen}>
            <PopoverTrigger asChild>
              <Button
                type="button"
                size="xs"
                variant="ghost"
                className="flex items-center gap-1 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                aria-label={t('editor.settings')}
              >
                <Settings className="h-3.5 w-3.5" />
                {t('editor.settingsShort')}
              </Button>
            </PopoverTrigger>
            <PopoverContent
              align="end"
              side="bottom"
              className="nova-editor-settings-panel w-[340px] overflow-hidden rounded-lg border border-[var(--nova-border)] p-0 text-[var(--nova-text)]"
            >
              <EditorSettingsPanel
                settings={settings}
                onChange={setSettings}
                onClose={() => setSettingsOpen(false)}
              />
            </PopoverContent>
          </Popover>
        </div>
      </div>
      {/* 编辑器内容区 */}
      <div
        ref={editorContainerRef}
        className="relative flex-1 overflow-y-auto px-4 py-6 md:px-10 md:py-8"
        style={{
          background: themeStyle.background,
          ['--nova-editor-color' as string]: themeStyle.color,
          ['--nova-editor-accent' as string]: themeStyle.accent,
          ['--nova-editor-line-height' as string]: String(settings.lineHeight),
          ['--nova-editor-dialogue-highlight' as string]: settings.dialogueHighlightColor || themeStyle.dialogueHighlight,
        }}
      >
        {searchOpen && (
          <div className="sticky top-0 z-20 ml-auto mb-3 flex w-[360px] items-center gap-1 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-menu-bg)] p-1 shadow-xl backdrop-blur">
            <Search className="ml-2 h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
            <input
              ref={searchInputRef}
              value={searchQuery}
              onChange={(e) => updateSearch(e.target.value, 0)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  goToSearchMatch(e.shiftKey ? -1 : 1)
                }
                if (e.key === 'Escape') {
                  e.preventDefault()
                  closeSearch()
                }
              }}
              placeholder={t('editor.searchPlaceholder')}
              className="min-w-0 flex-1 bg-transparent px-1 py-1 text-xs text-[var(--nova-text)] outline-none placeholder:text-[var(--nova-text-faint)]"
            />
            <span className="w-14 text-center text-[11px] text-[var(--nova-text-muted)]">
              {searchMatches.length > 0 ? `${searchIndex + 1}/${searchMatches.length}` : '0/0'}
            </span>
            <TooltipIconButton
              label={t('editor.searchPrev')}
              size="icon-xs"
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={() => goToSearchMatch(-1)}
              disabled={searchMatches.length === 0}
            >
              <ChevronUp className="h-3.5 w-3.5" />
            </TooltipIconButton>
            <TooltipIconButton
              label={t('editor.searchNext')}
              size="icon-xs"
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={() => goToSearchMatch(1)}
              disabled={searchMatches.length === 0}
            >
              <ChevronDown className="h-3.5 w-3.5" />
            </TooltipIconButton>
            <TooltipIconButton
              label={t('editor.closeSearch')}
              size="icon-xs"
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={closeSearch}
            >
              <X className="h-3.5 w-3.5" />
            </TooltipIconButton>
          </div>
        )}
        <EditorContent editor={editor} className={`editor-content editor-theme-${settings.theme}${nativeIndent ? ' native-indent' : ''}`} />
        {/* 选区浮动工具条 */}
        {editor && selectedCharacters > 0 && onQuoteSelection && (
          <SelectionToolbar editor={editor} onQuote={quoteCurrentSelection} />
        )}
      </div>
      <div className="nova-editor-statusbar flex h-7 shrink-0 items-center gap-4 border-t px-3 text-[11px] text-[var(--nova-text-faint)]">
        {chapterSummary && <span>{t('editor.updatedAt', { time: chapterSummary.updated_at || t('editor.unknownTime') })}</span>}
        {selectedCharacters > 0 && (
          <span className="text-[var(--nova-text-muted)]">{t('editor.selectedWords', { count: formatNumber(selectedCharacters) })}</span>
        )}
      </div>
    </div>
  )
}

function EditorSettingsPanel({
  settings,
  onChange,
  onClose,
}: {
  settings: EditorSettings
  onChange: (settings: EditorSettings) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const patch = (partial: Partial<EditorSettings>) => onChange({ ...settings, ...partial })

  return (
    <div>
      <div className="border-b border-[var(--nova-border-soft)] px-3 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
              <Palette className="h-3.5 w-3.5" />
            </span>
            <div className="min-w-0">
              <div className="text-xs font-medium text-[var(--nova-text)]">{t('editor.settings')}</div>
              <div className="text-[11px] text-[var(--nova-text-faint)]">{t('editor.settingsDescription')}</div>
            </div>
          </div>
          <button type="button" className="rounded px-2 py-1 text-xs text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </div>

      <div className="space-y-3 p-3">
        <label className="nova-editor-control block rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-3 text-xs">
            <span className="flex items-center gap-2 font-medium text-[var(--nova-text-muted)]">
              <Rows3 className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
              {t('editor.lineHeight')}
            </span>
            <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 font-mono text-[11px] text-[var(--nova-text)]">{settings.lineHeight.toFixed(1)}</span>
          </div>
          <input
            type="range"
            min="1.4"
            max="2.6"
            step="0.1"
            value={settings.lineHeight}
            onChange={(e) => patch({ lineHeight: Number(e.target.value) })}
            className="nova-editor-range w-full"
          />
        </label>

        <div className="nova-editor-control block rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-3 text-xs">
            <span className="flex items-center gap-2 font-medium text-[var(--nova-text-muted)]">
              <MessageSquareQuote className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
              {t('editor.dialogueHighlightColor')}
            </span>
          </div>
          <DialogueHighlightColorPicker
            value={settings.dialogueHighlightColor}
            defaultColor={THEME_STYLES[settings.theme].dialogueHighlight}
            onChange={(dialogueHighlightColor) => patch({ dialogueHighlightColor })}
            onReset={() => patch({ dialogueHighlightColor: DEFAULT_DIALOGUE_HIGHLIGHT_COLOR })}
          />
        </div>

        <div>
          <div className="mb-2 flex items-center justify-between text-xs text-[var(--nova-text-muted)]">
            <span className="font-medium">{t('editor.backgroundTheme')}</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">{t('editor.currentTheme', { theme: t(THEME_STYLES[settings.theme].labelKey) })}</span>
          </div>
          <div className="grid gap-2">
            {(Object.keys(THEME_STYLES) as EditorTheme[]).map((theme) => (
              <button
                key={theme}
                type="button"
                className={`nova-editor-theme-option flex w-full items-center justify-between rounded-lg border px-2.5 py-2 text-left text-xs ${
                  settings.theme === theme
                    ? 'is-active border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]'
                    : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                }`}
                onClick={() => patch({ theme })}
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    className="flex h-9 w-12 shrink-0 items-center justify-center rounded-md border border-black/15 text-[10px]"
                    style={{
                      background: THEME_STYLES[theme].background,
                      color: THEME_STYLES[theme].color,
                    }}
                  >
                    Aa
                  </span>
                  <span className="min-w-0">
                    <span className="block font-medium">{t(THEME_STYLES[theme].labelKey)}</span>
                    <span className="mt-0.5 block text-[11px] text-[var(--nova-text-faint)]">{t('editor.themePreview')}</span>
                  </span>
                </span>
                {settings.theme === theme && <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-accent-green)]" />}
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function loadEditorSettings(): EditorSettings {
  try {
    const raw = localStorage.getItem('nova.editor.settings')
    if (!raw) return DEFAULT_SETTINGS
    const parsed = JSON.parse(raw) as Partial<EditorSettings>
    return {
      lineHeight: parsed.lineHeight ?? DEFAULT_SETTINGS.lineHeight,
      theme: parsed.theme && parsed.theme in THEME_STYLES ? parsed.theme : DEFAULT_SETTINGS.theme,
      dialogueHighlightColor: normalizeColorValue(parsed.dialogueHighlightColor) ?? DEFAULT_SETTINGS.dialogueHighlightColor,
    }
  } catch {
    return DEFAULT_SETTINGS
  }
}

function normalizeColorValue(value: unknown): string | null {
  if (typeof value !== 'string') return null
  if (value === DEFAULT_DIALOGUE_HIGHLIGHT_COLOR) return DEFAULT_DIALOGUE_HIGHLIGHT_COLOR
  return COLOR_VALUE_PATTERN.test(value) ? value : null
}

function DialogueHighlightColorPicker({ value, defaultColor, onChange, onReset }: { value: string; defaultColor: string; onChange: (value: string) => void; onReset: () => void }) {
  const { t } = useTranslation()
  const color = normalizeColorValue(value) || normalizeColorValue(defaultColor) || DEFAULT_PICKER_COLOR
  const hsv = useMemo(() => hexToHsv(color), [color])
  const fieldRef = useRef<HTMLButtonElement>(null)
  const hueRef = useRef<HTMLButtonElement>(null)
  const hueColor = hsvToHex({ h: hsv.h, s: 1, v: 1 })

  const updateFieldColor = useCallback((clientX: number, clientY: number) => {
    const rect = fieldRef.current?.getBoundingClientRect()
    if (!rect) return
    const s = clampNumber((clientX - rect.left) / rect.width, 0, 1)
    const v = clampNumber(1 - ((clientY - rect.top) / rect.height), 0, 1)
    onChange(hsvToHex({ h: hsv.h, s, v }))
  }, [hsv.h, onChange])

  const updateHueColor = useCallback((clientX: number) => {
    const rect = hueRef.current?.getBoundingClientRect()
    if (!rect) return
    const h = clampNumber((clientX - rect.left) / rect.width, 0, 1) * 360
    onChange(hsvToHex({ ...hsv, h }))
  }, [hsv, onChange])

  const handlePointerDrag = (update: (event: PointerEvent<HTMLButtonElement>) => void) => (event: PointerEvent<HTMLButtonElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId)
    update(event)
  }

  const handleHexInput = (raw: string) => {
    const next = raw.startsWith('#') ? raw : `#${raw}`
    if (COLOR_VALUE_PATTERN.test(next)) onChange(next)
  }

  return (
    <div className="space-y-2">
      <button
        ref={fieldRef}
        type="button"
        className="relative h-24 w-full overflow-hidden rounded-md border border-[var(--nova-border)]"
        aria-label={t('editor.dialogueHighlightField')}
        onPointerDown={handlePointerDrag((event) => updateFieldColor(event.clientX, event.clientY))}
        onPointerMove={(event) => { if (event.buttons === 1) updateFieldColor(event.clientX, event.clientY) }}
        style={{
          background: `linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, ${hueColor})`,
        }}
      >
        <span
          className="absolute h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgba(0,0,0,0.55)]"
          style={{ left: `${hsv.s * 100}%`, top: `${(1 - hsv.v) * 100}%` }}
        />
      </button>
      <button
        ref={hueRef}
        type="button"
        className="relative h-5 w-full rounded-md border border-[var(--nova-border)]"
        aria-label={t('editor.dialogueHighlightHue')}
        onPointerDown={handlePointerDrag((event) => updateHueColor(event.clientX))}
        onPointerMove={(event) => { if (event.buttons === 1) updateHueColor(event.clientX) }}
        style={{ background: 'linear-gradient(to right, #ef4444, #eab308, #22c55e, #06b6d4, #6366f1, #d946ef, #ef4444)' }}
      >
        <span
          className="absolute top-1/2 h-6 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full border border-white bg-[var(--nova-surface)] shadow-[0_0_0_1px_rgba(0,0,0,0.45)]"
          style={{ left: `${(hsv.h / 360) * 100}%` }}
        />
      </button>
      <div className="flex items-center gap-2">
        <span className="h-7 w-7 shrink-0 rounded-md border border-[var(--nova-border)]" style={{ background: color }} />
        <input
          value={color}
          onChange={(event) => handleHexInput(event.target.value)}
          aria-label={t('editor.dialogueHighlightHex')}
          className="min-w-0 flex-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 font-mono text-[11px] text-[var(--nova-text)] outline-none focus:border-[var(--nova-field-focus-border)]"
        />
        <button
          type="button"
          className="shrink-0 rounded px-2 py-1 text-[11px] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={onReset}
        >
          {t('editor.dialogueHighlightReset')}
        </button>
      </div>
    </div>
  )
}

function clampNumber(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function hexToHsv(hex: string) {
  const normalized = normalizeColorValue(hex) || DEFAULT_PICKER_COLOR
  const r = parseInt(normalized.slice(1, 3), 16) / 255
  const g = parseInt(normalized.slice(3, 5), 16) / 255
  const b = parseInt(normalized.slice(5, 7), 16) / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const delta = max - min
  let h = 0
  if (delta !== 0) {
    if (max === r) h = 60 * (((g - b) / delta) % 6)
    else if (max === g) h = 60 * ((b - r) / delta + 2)
    else h = 60 * ((r - g) / delta + 4)
  }
  if (h < 0) h += 360
  return { h, s: max === 0 ? 0 : delta / max, v: max }
}

function hsvToHex({ h, s, v }: { h: number; s: number; v: number }) {
  const chroma = v * s
  const x = chroma * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = v - chroma
  let r = 0
  let g = 0
  let b = 0
  if (h < 60) [r, g, b] = [chroma, x, 0]
  else if (h < 120) [r, g, b] = [x, chroma, 0]
  else if (h < 180) [r, g, b] = [0, chroma, x]
  else if (h < 240) [r, g, b] = [0, x, chroma]
  else if (h < 300) [r, g, b] = [x, 0, chroma]
  else [r, g, b] = [chroma, 0, x]
  return `#${[r, g, b].map((channel) => Math.round((channel + m) * 255).toString(16).padStart(2, '0')).join('')}`
}

function normalizeEditorText(text: string): string {
  return text
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((line) => line.trimEnd())
    .join('\n')
    .replace(/\n{4,}/g, '\n\n\n')
    .trimEnd()
    .concat('\n')
}

function updateCharacterStats(
  editor: NonNullable<ReturnType<typeof useEditor>>,
  setSelected: (value: number) => void,
) {
  const { from, to, empty } = editor.state.selection
  if (empty) {
    setSelected(0)
    return
  }
  setSelected(countTextCharacters(editor.state.doc.textBetween(from, to, '\n')))
}

function countTextCharacters(text: string) {
  return Array.from(text.replace(/\s/g, '')).length
}

function formatNumber(value: number) {
  return formatLocaleNumber(value)
}

/** 创建编辑器搜索高亮扩展，使用 ProseMirror Decoration 标记匹配项。 */
function createSearchHighlightExtension(searchStateRef: { current: SearchState }) {
  return Extension.create({
    name: 'novaSearchHighlight',

    addProseMirrorPlugins() {
      return [
        new Plugin<DecorationSet>({
          key: searchPluginKey,
          state: {
            init: (_, state) => createSearchDecorations(state.doc, searchStateRef.current),
            apply: (tr, previousDecorations, _oldState, newState) => {
              if (tr.docChanged || tr.getMeta(searchPluginKey)) {
                return createSearchDecorations(newState.doc, searchStateRef.current)
              }
              return previousDecorations.map(tr.mapping, tr.doc)
            },
          },
          props: {
            decorations: (state) => searchPluginKey.getState(state) ?? DecorationSet.empty,
          },
        }),
      ]
    },
  })
}

/** 创建编辑器对白高亮扩展，不改变正文内容，仅用 Decoration 标记可视样式。 */
function createDialogueHighlightExtension() {
  return Extension.create({
    name: 'novaEditorDialogueHighlight',

    addProseMirrorPlugins() {
      return [
        new Plugin<DecorationSet>({
          key: dialogueHighlightPluginKey,
          state: {
            init: (_, state) => createDialogueDecorations(state.doc),
            apply: (tr, previousDecorations, _oldState, newState) => {
              if (tr.docChanged) return createDialogueDecorations(newState.doc)
              return previousDecorations.map(tr.mapping, tr.doc)
            },
          },
          props: {
            decorations: (state) => dialogueHighlightPluginKey.getState(state) ?? DecorationSet.empty,
          },
        }),
      ]
    },
  })
}

function createDialogueDecorations(doc: ProseMirrorNode) {
  const decorations: Decoration[] = []
  doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return
    for (const range of findDialogueHighlightRanges(node.text)) {
      decorations.push(Decoration.inline(pos + range.from, pos + range.to, { class: 'nova-editor-dialogue-highlight' }))
    }
  })
  return decorations.length > 0 ? DecorationSet.create(doc, decorations) : DecorationSet.empty
}

function createSearchDecorations(doc: ProseMirrorNode, searchState: SearchState) {
  const matches = findSearchMatchesInDoc(doc, searchState.query)
  if (matches.length === 0) return DecorationSet.empty

  const currentIndex = clampIndex(searchState.index, matches.length)
  const decorations = matches.map((match, index) =>
    Decoration.inline(match.from, match.to, {
      class: index === currentIndex ? 'nova-search-match nova-search-current' : 'nova-search-match',
    }),
  )
  return DecorationSet.create(doc, decorations)
}

function findSearchMatches(editor: Editor, query: string): SearchMatch[] {
  return findSearchMatchesInDoc(editor.state.doc, query)
}

function findSearchMatchesInDoc(doc: ProseMirrorNode, query: string): SearchMatch[] {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return []

  const matches: SearchMatch[] = []
  doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return

    const normalizedText = node.text.toLowerCase()
    let searchFrom = 0
    while (searchFrom < normalizedText.length) {
      const index = normalizedText.indexOf(normalizedQuery, searchFrom)
      if (index === -1) break
      matches.push({
        from: pos + index,
        to: pos + index + normalizedQuery.length,
      })
      searchFrom = index + normalizedQuery.length
    }
  })
  return matches
}

function selectSearchMatch(editor: Editor, match: SearchMatch) {
  const selection = PmTextSelection.create(editor.state.doc, match.from, match.to)
  editor.view.dispatch(editor.state.tr.setSelection(selection).scrollIntoView())
  // 额外使用 DOM scrollIntoView 确保 scroll-margin-top 生效（避免被 sticky 搜索栏遮挡）
  requestAnimationFrame(() => {
    const el = editor.view.dom.querySelector('.nova-search-current') as HTMLElement | null
    el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  })
}

function clampIndex(index: number, length: number) {
  return ((index % length) + length) % length
}

/** 计算文档中某位置对应的行号（从 1 开始） */
function getLineNumber(doc: ProseMirrorNode, pos: number): number {
  let line = 1
  doc.forEach((node, nodeOffset) => {
    if (nodeOffset + node.nodeSize <= pos) {
      line++
    }
  })
  return line
}

/** 选区浮动工具条，定位在光标（选区 head 端）旁边 */
function SelectionToolbar({ editor, onQuote }: { editor: Editor; onQuote: () => void }) {
  const { t } = useTranslation()
  const [coords, setCoords] = useState<{ top: number; left: number } | null>(null)
  const toolbarRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const updatePosition = () => {
      const { from, to, head } = editor.state.selection
      if (from === to) {
        setCoords(null)
        return
      }
      try {
        const headCoords = editor.view.coordsAtPos(head)
        const containerEl = editor.view.dom.closest('.relative') as HTMLElement | null
        if (!containerEl) { setCoords(null); return }
        const containerRect = containerEl.getBoundingClientRect()
        const scrollTop = containerEl.scrollTop
        const toolbarWidth = toolbarRef.current?.offsetWidth ?? 100
        // coordsAtPos 返回视口坐标，需加上 scrollTop 转换为容器内容区域坐标
        let top = headCoords.bottom - containerRect.top + scrollTop + 4
        let left = headCoords.left - containerRect.left
        // 防止溢出右侧
        const maxLeft = containerRect.width - toolbarWidth - 8
        if (left > maxLeft) left = maxLeft
        if (left < 4) left = 4
        // 如果下方空间不够（相对当前可见区域），改为显示在光标行上方
        const toolbarHeight = toolbarRef.current?.offsetHeight ?? 32
        const visibleBottom = scrollTop + containerRect.height
        if (top + toolbarHeight > visibleBottom) {
          top = headCoords.top - containerRect.top + scrollTop - toolbarHeight - 4
        }
        setCoords({ top: Math.max(scrollTop, top), left })
      } catch {
        setCoords(null)
      }
    }
    updatePosition()
    editor.on('selectionUpdate', updatePosition)
    return () => { editor.off('selectionUpdate', updatePosition) }
  }, [editor])

  if (!coords) return null

  return (
    <div
      ref={toolbarRef}
      className="absolute z-30 flex items-center gap-1 rounded-md border border-[var(--nova-border)] bg-[var(--nova-menu-bg)] px-1.5 py-1 shadow-xl backdrop-blur"
      style={{ top: coords.top, left: coords.left }}
    >
      <button
        type="button"
        className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-menu-item-hover-bg)] hover:text-[var(--nova-text)]"
        onClick={onQuote}
        title={t('editor.quoteSelectionShortcut')}
      >
        <MessageSquareQuote className="h-3.5 w-3.5" />
        <span>{t('editor.quoteSelection')}</span>
      </button>
    </div>
  )
}
