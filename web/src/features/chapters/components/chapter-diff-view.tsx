import { useEffect, useState } from 'react'
import { DiffEditor } from '@monaco-editor/react'

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
  const [compact, setCompact] = useState(false)

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return
    const query = window.matchMedia('(max-width: 760px)')
    const update = () => setCompact(query.matches)
    update()
    query.addEventListener('change', update)
    return () => query.removeEventListener('change', update)
  }, [])

  return (
    <div className={`h-full min-h-[360px] w-full overflow-hidden bg-[#050505] ${className}`}>
      <DiffEditor
        height="100%"
        theme="vs-dark"
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
