import type { ReactNode, RefObject } from 'react'
import type { Editor } from '@tiptap/react'
import { EditorContent } from '@tiptap/react'

import { EditorSearchBar } from './EditorSearchBar'
import type { EditorSettings } from './EditorSettingsPanel'
import { SelectionToolbar } from './SelectionToolbar'

interface EditorSurfaceProps {
  containerRef: RefObject<HTMLDivElement | null>
  editor: Editor | null
  settings: EditorSettings
  themeStyle: {
    background: string
    color: string
    accent: string
    dialogueHighlight: string
  }
  nativeIndent: boolean
  search: {
    open: boolean
    inputRef: RefObject<HTMLInputElement | null>
    query: string
    matchIndex: number
    matchCount: number
    onQueryChange: (query: string) => void
    onNavigate: (direction: 1 | -1) => void
    onClose: () => void
  }
  showSelectionToolbar: boolean
  selectionToolbarMode?: 'quote' | 'comment'
  onSelectionAction: () => void
  reviewAnnotations?: ReactNode
}

/** 编辑器的纯展示层；文档同步、保存和冲突状态由 MarkdownEditor 管理。 */
export function EditorSurface({
  containerRef,
  editor,
  settings,
  themeStyle,
  nativeIndent,
  search,
  showSelectionToolbar,
  selectionToolbarMode = 'quote',
  onSelectionAction,
  reviewAnnotations,
}: EditorSurfaceProps) {
  return (
    <div
      ref={containerRef}
      className="relative flex-1 overflow-y-auto px-4 py-6 md:px-10 md:py-8"
      style={{
        background: themeStyle.background,
        ['--nova-editor-color' as string]: themeStyle.color,
        ['--nova-editor-accent' as string]: themeStyle.accent,
        ['--nova-editor-line-height' as string]: String(settings.lineHeight),
        ['--nova-editor-dialogue-highlight' as string]: settings.dialogueHighlightColor || themeStyle.dialogueHighlight,
      }}
    >
      {search.open && (
        <EditorSearchBar
          inputRef={search.inputRef}
          query={search.query}
          matchIndex={search.matchIndex}
          matchCount={search.matchCount}
          onQueryChange={search.onQueryChange}
          onNavigate={search.onNavigate}
          onClose={search.onClose}
        />
      )}
      <EditorContent editor={editor} className={`editor-content editor-theme-${settings.theme}${nativeIndent ? ' native-indent' : ''}`} />
      {reviewAnnotations}
      {editor && showSelectionToolbar && (
        <SelectionToolbar editor={editor} mode={selectionToolbarMode} onAction={onSelectionAction} />
      )}
    </div>
  )
}
