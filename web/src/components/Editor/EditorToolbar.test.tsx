import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { EditorToolbar } from './EditorToolbar'

describe('EditorToolbar chapter outline navigation', () => {
  it('shows a chapter-only reveal action and forwards the canonical chapter path', async () => {
    const user = userEvent.setup()
    const onRevealChapter = vi.fn()
    const { rerender } = render(
      <EditorToolbar
        fileName="chapters/chapter-01.md"
        displayTitle="第一章"
        chapterPath="chapters/chapter-01.md"
        chapterWords={2206}
        updatedAt="2026-07-11 22:00"
        currentLine={49}
        saveStatus={null}
        onSave={vi.fn()}
        settingsOpen={false}
        onSettingsOpenChange={vi.fn()}
        settings={{ lineHeight: 1.9, theme: 'ide', dialogueHighlightColor: '' }}
        onSettingsChange={vi.fn()}
        onRevealChapter={onRevealChapter}
        generateIllustrationDisabled={false}
      />,
    )

    expect(screen.getByText('2,206 字')).toBeInTheDocument()
    expect(screen.getByText('更新：07/11 22:00')).toBeInTheDocument()
    expect(screen.getByText('行 49')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '在作品目录中显示当前章节' }))
    expect(onRevealChapter).toHaveBeenCalledWith('chapters/chapter-01.md')

    rerender(
      <EditorToolbar
        fileName="setting/outline.md"
        saveStatus={null}
        onSave={vi.fn()}
        settingsOpen={false}
        onSettingsOpenChange={vi.fn()}
        settings={{ lineHeight: 1.9, theme: 'ide', dialogueHighlightColor: '' }}
        onSettingsChange={vi.fn()}
        onRevealChapter={onRevealChapter}
        generateIllustrationDisabled={false}
      />,
    )

    expect(screen.queryByRole('button', { name: '在作品目录中显示当前章节' })).not.toBeInTheDocument()
  })
})
