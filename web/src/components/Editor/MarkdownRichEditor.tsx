import { useEffect, useMemo, useRef } from 'react'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { TableKit } from '@tiptap/extension-table'
import { Markdown } from '@tiptap/markdown'

import { isSaveShortcut } from '@/lib/keyboard'
import { cn } from '@/lib/utils'
import {
  createSearchHighlightExtension,
  findSearchMatches,
  searchPluginKey,
  selectSearchMatch,
} from './editorDecorations'
import type { SearchState } from './editorDecorations'
import {
  createIndentedHardBreakExtension,
  createWorkspaceImageExtension,
  normalizeEditorText,
  placeEditorCaretAtClick,
  replaceEditorDocument,
} from './editorDocument'

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
  className,
  'aria-label': ariaLabel,
}: MarkdownRichEditorProps) {
  const searchStateRef = useRef<SearchState>({ query: '', index: 0, useRegex: false })
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

  const editor = useEditor({
    extensions: [
      StarterKit.configure({ hardBreak: false }),
      createIndentedHardBreakExtension(),
      searchExtension,
      workspaceImageExtension,
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
        if (!isSaveShortcut(event)) return false
        event.preventDefault()
        event.stopPropagation()
        onSaveShortcutRef.current?.()
        return true
      },
      handleClick: placeEditorCaretAtClick,
    },
    onUpdate: ({ editor: instance }) => {
      const markdown = normalizeEditorText(instance.getMarkdown())
      lastEmittedRef.current = markdown
      onChangeRef.current(markdown)
    },
  })

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

  return (
    <EditorContent
      editor={editor}
      className={cn('nova-rich-markdown chat-agent-message min-w-0 text-[var(--nova-text)]', className)}
    />
  )
}
