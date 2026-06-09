import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ChapterDiffView } from './chapter-diff-view'

vi.mock('@monaco-editor/react', () => ({
  DiffEditor: ({ original, modified, language, theme, options }: {
    original: string
    modified: string
    language: string
    theme: string
    options: { renderSideBySide?: boolean }
  }) => (
    <div
      data-testid="diff-editor"
      data-language={language}
      data-theme={theme}
      data-side-by-side={String(options.renderSideBySide)}
    >
      <span>{original}</span>
      <span>{modified}</span>
    </div>
  ),
}))

describe('ChapterDiffView', () => {
  it('renders Monaco diff editor with markdown defaults', () => {
    render(<ChapterDiffView original="旧章节" modified="新章节" />)

    const editor = screen.getByTestId('diff-editor')
    expect(editor).toHaveAttribute('data-language', 'markdown')
    expect(editor).toHaveAttribute('data-theme', 'vs-dark')
    expect(editor).toHaveAttribute('data-side-by-side', 'true')
    expect(screen.getByText('旧章节')).toBeInTheDocument()
    expect(screen.getByText('新章节')).toBeInTheDocument()
  })
})
