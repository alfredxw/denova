import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { EditorToolbar } from './EditorToolbar'

describe('EditorToolbar chapter outline navigation', () => {
  it('把高频操作收敛为带悬浮提示的纯图标按钮', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    const onSettingsOpenChange = vi.fn()
    render(
      <EditorToolbar
        fileName="chapters/chapter-01.md"
        displayTitle="第一章"
        chapterPath="chapters/chapter-01.md"
        saveStatus={null}
        onSave={onSave}
        settingsOpen={false}
        onSettingsOpenChange={onSettingsOpenChange}
        settings={{ lineHeight: 1.9, theme: 'ide', dialogueHighlightColor: '' }}
        onSettingsChange={vi.fn()}
        onRevealChapter={vi.fn()}
        generateIllustrationDisabled={false}
      />,
    )

    const revealButton = screen.getByRole('button', { name: '在作品目录中显示当前章节' })
    const saveButton = screen.getByRole('button', { name: '保存' })
    const settingsButton = screen.getByRole('button', { name: '编辑器设置' })
    expect(revealButton).not.toHaveTextContent('目录定位')
    expect(saveButton).not.toHaveTextContent('保存')
    expect(settingsButton).not.toHaveTextContent('设置')

    await user.hover(settingsButton)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('编辑器设置')
    await user.click(saveButton)
    await user.click(settingsButton)
    expect(onSave).toHaveBeenCalledOnce()
    expect(onSettingsOpenChange).toHaveBeenCalledWith(true)
  })

  it('shows a chapter-only reveal action and forwards the canonical chapter path', async () => {
    const user = userEvent.setup()
    const onRevealChapter = vi.fn()
    const { rerender } = render(
      <EditorToolbar
        fileName="chapters/chapter-01.md"
        displayTitle="第一章"
        chapterPath="chapters/chapter-01.md"
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
