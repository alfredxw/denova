import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { BookSettingsShortcuts } from './BookSettingsShortcuts'

describe('BookSettingsShortcuts', () => {
  beforeEach(() => window.localStorage.clear())

  it('默认 Pin 五个自适应快捷入口，并可 Pin 动态发现的 Markdown 文件', async () => {
    const user = userEvent.setup()
    render(
      <BookSettingsShortcuts
        workspace="/books/demo"
        tree={[
          { name: 'CREATOR.md', type: 'file' },
          { name: 'setting', type: 'dir', children: [
            { name: 'outline.md', type: 'file' },
            { name: 'progress.md', type: 'file' },
            { name: '人物关系.md', type: 'file' },
          ] },
          { name: 'chapters', type: 'dir', children: [{ name: 'ch01.md', type: 'file' }] },
          { name: 'interactive', type: 'dir', children: [{ name: 'director.md', type: 'file' }] },
        ]}
        chapterPlans={[]}
        selectedFile={null}
        onSelectFile={vi.fn()}
      />,
    )

    expect(screen.getByRole('button', { name: '大纲' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '规则' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '进度' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '灵感' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '状态' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '人物关系' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '管理' }))
    expect(screen.getByText('setting/人物关系.md')).toBeInTheDocument()
    expect(screen.queryByText('chapters/ch01.md')).not.toBeInTheDocument()
    expect(screen.queryByText('interactive/director.md')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Pin 人物关系' }))

    expect(screen.getByRole('button', { name: '人物关系' })).toBeInTheDocument()
    expect(JSON.parse(window.localStorage.getItem('nova.outline.pinned-settings:/books/demo') || '{}').paths).toContain('setting/人物关系.md')
  })

  it('按工作区恢复用户的 Pin 顺序', () => {
    window.localStorage.setItem('nova.outline.pinned-settings:/books/demo', JSON.stringify(['ideas.md', 'CREATOR.md']))
    render(
      <BookSettingsShortcuts
        workspace="/books/demo"
        tree={[]}
        chapterPlans={[]}
        selectedFile={null}
        onSelectFile={vi.fn()}
      />,
    )

    expect(screen.getAllByRole('button').filter((button) => ['灵感', '规则'].includes(button.textContent || '')).map((button) => button.textContent)).toEqual(['灵感', '规则'])
  })

  it('把旧版未自定义的默认三项迁移为新的默认五项', () => {
    window.localStorage.setItem('nova.outline.pinned-settings:/books/demo', JSON.stringify(['setting/outline.md', 'CREATOR.md', 'setting/progress.md']))
    render(<BookSettingsShortcuts workspace="/books/demo" tree={[]} chapterPlans={[]} selectedFile={null} onSelectFile={vi.fn()} />)

    expect(screen.getByRole('button', { name: '灵感' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '状态' })).toBeInTheDocument()
  })
})
