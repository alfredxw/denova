import { DiffEditor } from '@monaco-editor/react'
import { useTheme } from 'next-themes'
import { useIsMobile } from '@/hooks/useIsMobile'

export type ChapterDiffViewProps = {
  original: string
  modified: string
  language?: string
  sideBySide?: boolean
  className?: string
}

/** 章节差异视图，基于 Monaco Diff Editor 只读展示版本差异。 */
export function ChapterDiffView({
  original,
  modified,
  language = 'markdown',
  sideBySide = true,
  className = '',
}: ChapterDiffViewProps) {
  const compact = useIsMobile()
  const { resolvedTheme } = useTheme()
  const monacoTheme = resolvedTheme === 'light' ? 'light' : 'vs-dark'

  return (
    <div className={`h-full min-h-[360px] w-full overflow-hidden bg-[var(--nova-bg)] ${className}`}>
      <DiffEditor
        height="100%"
        theme={monacoTheme}
        language={language}
        original={original}
        modified={modified}
        options={{
          readOnly: true,
          originalEditable: false,
          wordWrap: 'on',
          minimap: { enabled: false },
          renderSideBySide: sideBySide && !compact,
          scrollBeyondLastLine: false,
          automaticLayout: true,
          renderOverviewRuler: false,
          glyphMargin: false,
          folding: false,
          lineNumbersMinChars: 3,
          padding: { top: 14, bottom: 14 },
          scrollbar: {
            verticalScrollbarSize: 10,
            horizontalScrollbarSize: 10,
          },
        }}
      />
    </div>
  )
}
