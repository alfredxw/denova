import { useEffect, useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { Editor } from '@tiptap/react'
import { useEditor, EditorContent } from '@tiptap/react'
import { Extension, Node } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import { CharacterCount } from '@tiptap/extension-character-count'
import { Markdown } from '@tiptap/markdown'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey, TextSelection as PmTextSelection } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import { BookOpen, Check, ChevronDown, ChevronUp, MessageSquareQuote, Palette, Rows3, Save, Search, Settings, Type, X } from 'lucide-react'
import { toast } from 'sonner'

import type { TextSelection as QuoteSelection } from '@/lib/api'
import type { ChapterSummary, WorkspaceSummary } from '@/lib/api'
import { isEditableTarget } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'

interface MarkdownEditorProps {
  fileName: string | null
  content: string
  onSave: (content: string) => Promise<boolean>
  onQuoteSelection?: (sel: QuoteSelection) => void
  saveSignal?: number
  chapterSummary?: ChapterSummary
  workspaceSummary?: WorkspaceSummary | null
  searchIntent?: EditorSearchIntent | null
}

export interface EditorSearchIntent {
  query: string
  line: number
  nonce: number
}

type EditorTheme = 'ide' | 'paper' | 'sepia'
type SaveStatus = 'dirty' | 'auto-saving' | 'auto-saved' | 'manual-saving' | 'manual-saved' | 'error'

interface EditorSettings {
  fontSize: number
  lineHeight: number
  theme: EditorTheme
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

const DEFAULT_SETTINGS: EditorSettings = {
  fontSize: 18,
  lineHeight: 1.9,
  theme: 'ide',
}

const THEME_STYLES: Record<EditorTheme, { label: string; background: string; color: string; accent: string }> = {
  ide: {
    label: 'IDE 深色',
    background: '#1a1a1a',
    color: '#d7dbe2',
    accent: '#303238',
  },
  paper: {
    label: '纸张',
    background: '#f5efe4',
    color: '#252525',
    accent: '#dfd3c2',
  },
  sepia: {
    label: '护眼',
    background: '#efe3cc',
    color: '#2f271f',
    accent: '#d8c6a6',
  },
}

const SAVE_STATUS_META: Record<SaveStatus, { label: string; ariaLabel: string; className: string; dotClassName?: string; subtle?: boolean }> = {
  dirty: {
    label: '有改动',
    ariaLabel: '内容有改动，等待自动保存',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'bg-[var(--nova-text-faint)] opacity-60',
    subtle: true,
  },
  'auto-saving': {
    label: '同步中',
    ariaLabel: '正在自动保存',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'animate-pulse bg-[var(--nova-text-muted)] opacity-70',
    subtle: true,
  },
  'auto-saved': {
    label: '已同步',
    ariaLabel: '已自动保存',
    className: 'text-[var(--nova-text-faint)]',
    subtle: true,
  },
  'manual-saving': {
    label: '保存中…',
    ariaLabel: '正在保存',
    className: 'text-[var(--nova-text-muted)]',
  },
  'manual-saved': {
    label: '已保存',
    ariaLabel: '已保存',
    className: 'text-[var(--nova-accent-green)]',
  },
  error: {
    label: '保存失败',
    ariaLabel: '保存失败',
    className: 'text-[#ff6b6b]',
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

/** TipTap 编辑器组件，支持 Markdown 和纯文本格式 */
export function MarkdownEditor({ fileName, content, onSave, onQuoteSelection, saveSignal = 0, chapterSummary, workspaceSummary, searchIntent }: MarkdownEditorProps) {
  const [saveStatus, setSaveStatus] = useState<SaveStatus | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settings, setSettings] = useState<EditorSettings>(() => loadEditorSettings())
  const [nativeIndent, setNativeIndent] = useState(false)
  const [totalCharacters, setTotalCharacters] = useState(0)
  const [selectedCharacters, setSelectedCharacters] = useState(0)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchIndex, setSearchIndex] = useState(0)
  const [searchMatches, setSearchMatches] = useState<SearchMatch[]>([])
  const autoSaveTimer = useRef<number | null>(null)
  const saveStatusClearTimer = useRef<number | null>(null)
  const lastSyncedFileRef = useRef<string | null>(null)
  const lastSyncedContentRef = useRef('')
  const searchInputRef = useRef<HTMLInputElement>(null)
  const lastSaveSignalRef = useRef(saveSignal)
  const lastSearchIntentNonceRef = useRef<number | null>(null)
  const searchStateRef = useRef<SearchState>({ query: '', index: 0 })
  const searchExtension = useMemo(() => createSearchHighlightExtension(searchStateRef), [])
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
        placeholder: '选择一个文件开始编辑...',
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
    updateCharacterStats(editor, setTotalCharacters, setSelectedCharacters)
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

  // 监听 TipTap 内容和选区变化，实时更新全文/选区字数。
  useEffect(() => {
    if (!editor) return

    const updateStats = () => updateCharacterStats(editor, setTotalCharacters, setSelectedCharacters)
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

  /** 保存当前编辑内容 */
  const saveEditorContent = useCallback(async (mode: 'manual' | 'auto') => {
    if (!editor || !fileName) return
    const text = isTxtFile(fileName)
      ? normalizeEditorText(editor.getText({ blockSeparator: '\n' }))
      : normalizeEditorText(editor.getMarkdown())
    lastSyncedContentRef.current = text
    clearSaveStatusTimer()
    setSaveStatus(mode === 'auto' ? 'auto-saving' : 'manual-saving')
    const ok = await onSave(text)
    const nextStatus: SaveStatus = ok ? (mode === 'auto' ? 'auto-saved' : 'manual-saved') : 'error'
    setSaveStatus(nextStatus)
    if (mode === 'manual') {
      if (ok) toast.success('保存成功')
      else toast.error('保存失败')
    }
    scheduleSaveStatusClear(nextStatus, mode === 'auto' ? 1400 : 2000)
  }, [clearSaveStatusTimer, editor, fileName, onSave, scheduleSaveStatusClear])

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

  // 编辑后防抖自动保存
  useEffect(() => {
    if (!editor) return

    const handleUpdate = () => {
      if (!fileName) return
      clearSaveStatusTimer()
      setSaveStatus('dirty')
      if (autoSaveTimer.current) {
        window.clearTimeout(autoSaveTimer.current)
      }
      autoSaveTimer.current = window.setTimeout(() => {
        autoSaveTimer.current = null
        void saveEditorContent('auto')
      }, 1200)
    }

    editor.on('update', handleUpdate)
    return () => {
      editor.off('update', handleUpdate)
      if (autoSaveTimer.current) {
        window.clearTimeout(autoSaveTimer.current)
      }
    }
  }, [clearSaveStatusTimer, editor, fileName, saveEditorContent])

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
        选择左侧 Markdown 文件开始编辑
      </div>
    )
  }

  const saveStatusMeta = saveStatus ? SAVE_STATUS_META[saveStatus] : null

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
              aria-label={saveStatusMeta.ariaLabel}
              title={saveStatusMeta.ariaLabel}
            >
              {saveStatus === 'auto-saved' ? (
                <Check className="h-3 w-3 opacity-45" />
              ) : saveStatusMeta.dotClassName ? (
                <span className={`h-1.5 w-1.5 rounded-full ${saveStatusMeta.dotClassName}`} />
              ) : null}
              <span className={saveStatusMeta.subtle ? 'sr-only' : ''}>{saveStatusMeta.label}</span>
            </span>
          )}
          <Button
            type="button"
            onClick={handleSave}
            size="xs"
            variant="ghost"
            className="flex items-center gap-1 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          >
            <Save className="w-3.5 h-3.5" />
            保存
          </Button>
          <Popover open={settingsOpen} onOpenChange={setSettingsOpen}>
            <PopoverTrigger asChild>
              <Button
                type="button"
                size="xs"
                variant="ghost"
                className="flex items-center gap-1 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                aria-label="编辑器设置"
              >
                <Settings className="h-3.5 w-3.5" />
                设置
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
        className="relative flex-1 overflow-y-auto px-10 py-8"
        style={{
          background: themeStyle.background,
          ['--nova-editor-color' as string]: themeStyle.color,
          ['--nova-editor-accent' as string]: themeStyle.accent,
          ['--nova-editor-font-size' as string]: `${settings.fontSize}px`,
          ['--nova-editor-line-height' as string]: String(settings.lineHeight),
        }}
      >
        {searchOpen && (
          <div className="sticky top-0 z-20 ml-auto mb-3 flex w-[360px] items-center gap-1 rounded-lg border border-[#303238] bg-[#202124]/95 p-1 shadow-xl backdrop-blur">
            <Search className="ml-2 h-3.5 w-3.5 text-[#858b96]" />
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
              placeholder="搜索当前文章..."
              className="min-w-0 flex-1 bg-transparent px-1 py-1 text-xs text-[#d7dbe2] outline-none placeholder:text-[#6f7682]"
            />
            <span className="w-14 text-center text-[11px] text-[#858b96]">
              {searchMatches.length > 0 ? `${searchIndex + 1}/${searchMatches.length}` : '0/0'}
            </span>
            <TooltipIconButton
              label="上一处"
              size="icon-xs"
              className="text-[#858b96] hover:bg-[#303238] hover:text-[#d7dbe2]"
              onClick={() => goToSearchMatch(-1)}
              disabled={searchMatches.length === 0}
            >
              <ChevronUp className="h-3.5 w-3.5" />
            </TooltipIconButton>
            <TooltipIconButton
              label="下一处"
              size="icon-xs"
              className="text-[#858b96] hover:bg-[#303238] hover:text-[#d7dbe2]"
              onClick={() => goToSearchMatch(1)}
              disabled={searchMatches.length === 0}
            >
              <ChevronDown className="h-3.5 w-3.5" />
            </TooltipIconButton>
            <TooltipIconButton
              label="关闭搜索"
              size="icon-xs"
              className="text-[#858b96] hover:bg-[#303238] hover:text-[#d7dbe2]"
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
        <span>本章：{formatNumber(totalCharacters)} 字</span>
        {workspaceSummary && <span>全书：{formatNumber(workspaceSummary.total_words)} 字</span>}
        {chapterSummary && <span>更新：{chapterSummary.updated_at || '未知'}</span>}
        {selectedCharacters > 0 && (
          <span className="text-[var(--nova-text-muted)]">已选：{formatNumber(selectedCharacters)} 字</span>
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
  const patch = (partial: Partial<EditorSettings>) => onChange({ ...settings, ...partial })

  return (
    <div>
      <div className="border-b border-[var(--nova-border-soft)] px-3 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-[#3a3a3a] bg-[#202020] text-[#a3a3a3]">
              <Palette className="h-3.5 w-3.5" />
            </span>
            <div className="min-w-0">
              <div className="text-xs font-medium text-[var(--nova-text)]">编辑器设置</div>
              <div className="text-[11px] text-[var(--nova-text-faint)]">阅读密度与编辑器背景</div>
            </div>
          </div>
          <button type="button" className="rounded px-2 py-1 text-xs text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={onClose}>
            关闭
          </button>
        </div>
      </div>

      <div className="space-y-3 p-3">
        <label className="nova-editor-control block rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-3 text-xs">
            <span className="flex items-center gap-2 font-medium text-[var(--nova-text-muted)]">
              <Type className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
              字号
            </span>
            <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-0.5 font-mono text-[11px] text-[var(--nova-text)]">{settings.fontSize}px</span>
          </div>
          <input
            type="range"
            min="14"
            max="28"
            step="1"
            value={settings.fontSize}
            onChange={(e) => patch({ fontSize: Number(e.target.value) })}
            className="nova-editor-range w-full"
          />
        </label>

        <label className="nova-editor-control block rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-3">
          <div className="mb-2 flex items-center justify-between gap-3 text-xs">
            <span className="flex items-center gap-2 font-medium text-[var(--nova-text-muted)]">
              <Rows3 className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
              行间距
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

        <div>
          <div className="mb-2 flex items-center justify-between text-xs text-[var(--nova-text-muted)]">
            <span className="font-medium">背景主题</span>
            <span className="text-[11px] text-[var(--nova-text-faint)]">当前：{THEME_STYLES[settings.theme].label}</span>
          </div>
          <div className="grid gap-2">
            {(Object.keys(THEME_STYLES) as EditorTheme[]).map((theme) => (
              <button
                key={theme}
                type="button"
                className={`nova-editor-theme-option flex w-full items-center justify-between rounded-lg border px-2.5 py-2 text-left text-xs ${
                  settings.theme === theme
                    ? 'is-active border-[#4a4a4a] bg-[#2f2f2f] text-[var(--nova-text)]'
                    : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:border-[#3a3a3a] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
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
                    <span className="block font-medium">{THEME_STYLES[theme].label}</span>
                    <span className="mt-0.5 block text-[11px] text-[var(--nova-text-faint)]">正文 / 引用 / 代码块</span>
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
      fontSize: parsed.fontSize ?? DEFAULT_SETTINGS.fontSize,
      lineHeight: parsed.lineHeight ?? DEFAULT_SETTINGS.lineHeight,
      theme: parsed.theme && parsed.theme in THEME_STYLES ? parsed.theme : DEFAULT_SETTINGS.theme,
    }
  } catch {
    return DEFAULT_SETTINGS
  }
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
  setTotal: (value: number) => void,
  setSelected: (value: number) => void,
) {
  const storage = editor.storage.characterCount as { characters?: () => number } | undefined
  setTotal(storage?.characters?.() ?? countTextCharacters(editor.state.doc.textContent))

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
  return new Intl.NumberFormat('zh-CN').format(value)
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
      className="absolute z-30 flex items-center gap-1 rounded-md border border-[#303238] bg-[#25262a]/95 px-1.5 py-1 shadow-xl backdrop-blur"
      style={{ top: coords.top, left: coords.left }}
    >
      <button
        type="button"
        className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[#c5c9d1] hover:bg-[#4a4d54]/30 hover:text-white"
        onClick={onQuote}
        title="引用到 AI (⌘⇧L)"
      >
        <MessageSquareQuote className="h-3.5 w-3.5" />
        <span>引用到 AI</span>
      </button>
    </div>
  )
}
