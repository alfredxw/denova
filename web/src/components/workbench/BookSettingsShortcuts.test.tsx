import { render, screen, within } from '@testing-library/react'
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
        headerPinned
        onSelectFile={vi.fn()}
        onToggleHeaderPinned={vi.fn()}
      />,
    )

    expect(screen.getByRole('button', { name: '大纲' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '规则' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '进度' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^灵感尚未创建/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^状态尚未创建/ })).toBeInTheDocument()
    expect(screen.getByTestId('book-setting-shortcuts')).toHaveClass('grid-cols-[repeat(auto-fill,minmax(4rem,1fr))]')
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
        headerPinned
        onSelectFile={vi.fn()}
        onToggleHeaderPinned={vi.fn()}
      />,
    )

    expect(screen.getAllByRole('button').filter((button) => ['灵感', '规则'].includes(button.textContent || '')).map((button) => button.textContent)).toEqual(['灵感', '规则'])
  })

  it('把旧版未自定义的默认三项迁移为新的默认五项', () => {
    window.localStorage.setItem('nova.outline.pinned-settings:/books/demo', JSON.stringify(['setting/outline.md', 'CREATOR.md', 'setting/progress.md']))
    render(<BookSettingsShortcuts workspace="/books/demo" tree={[]} chapterPlans={[]} selectedFile={null} headerPinned onSelectFile={vi.fn()} onToggleHeaderPinned={vi.fn()} />)

    expect(screen.getByRole('button', { name: /^灵感尚未创建/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^状态尚未创建/ })).toBeInTheDocument()
  })

  it('缺失的设定文件不打开空 Tab，并提示通过创作 Agent 创建', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    const onRequestCreate = vi.fn()
    render(
      <BookSettingsShortcuts
        workspace="/books/demo"
        tree={[{ name: 'CREATOR.md', type: 'file' }]}
        chapterPlans={[]}
        selectedFile={null}
        headerPinned
        onSelectFile={onSelectFile}
        onToggleHeaderPinned={vi.fn()}
        onRequestCreate={onRequestCreate}
      />,
    )

    const missingOutlineShortcut = screen.getByRole('button', { name: /大纲尚未创建/ })
    expect(missingOutlineShortcut).toHaveAttribute('data-book-setting-state', 'missing')
    expect(missingOutlineShortcut).toHaveClass('border-dashed')
    expect(missingOutlineShortcut.querySelector('svg')).toBeInTheDocument()

    await user.click(missingOutlineShortcut)
    expect(onSelectFile).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent('大纲还没有创建')
    expect(screen.getByRole('status')).toHaveTextContent('setting/outline.md')
    expect(screen.getByRole('status')).toHaveTextContent('创作 Agent')
    await user.click(screen.getByRole('button', { name: '和创作 Agent 对话' }))
    expect(onRequestCreate).toHaveBeenCalledWith(expect.objectContaining({ path: 'setting/outline.md', title: '大纲' }))

    await user.click(screen.getByRole('button', { name: '规则' }))
    expect(onSelectFile).toHaveBeenCalledWith('CREATOR.md')
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('在管理列表中以虚线与状态图标区分尚未创建的文件', async () => {
    const user = userEvent.setup()
    render(
      <BookSettingsShortcuts
        workspace="/books/demo"
        tree={[{ name: 'CREATOR.md', type: 'file' }]}
        chapterPlans={[]}
        selectedFile={null}
        headerPinned
        onSelectFile={vi.fn()}
        onToggleHeaderPinned={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '管理' }))
    const missingOutlineRow = screen.getAllByRole('button', { name: /大纲尚未创建/ }).find((button) => button.classList.contains('min-w-0'))
    if (!missingOutlineRow) throw new Error('missing outline row is not rendered')
    expect(missingOutlineRow).toHaveAttribute('data-book-setting-state', 'missing')
    expect(missingOutlineRow.closest('div.flex.items-center.gap-1.rounded-md.border')).toHaveClass('border-dashed')
    expect(missingOutlineRow.querySelector('svg.lucide-circle-dashed')).toBeInTheDocument()
  })

  it('当前细纲可选 Pin，并始终指向最新一份细纲', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    const firstPlans = [
      { path: 'setting/chapter-plans/group-1.md', title: '第一组细纲', excerpt: '', words: 100, updated_at: '' },
      { path: 'setting/chapter-plans/group-2.md', title: '第二组细纲', excerpt: '', words: 100, updated_at: '' },
    ]
    const { rerender } = render(
      <BookSettingsShortcuts
        workspace="/books/demo"
        tree={[]}
        chapterPlans={firstPlans}
        selectedFile={null}
        headerPinned
        onSelectFile={onSelectFile}
        onToggleHeaderPinned={vi.fn()}
      />,
    )

    expect(within(screen.getByTestId('book-setting-shortcuts')).queryByRole('button', { name: '当前细纲' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '管理' }))
    expect(screen.getByText('setting/chapter-plans/group-2.md')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Pin 当前细纲' }))
    expect(within(screen.getByTestId('book-setting-shortcuts')).getByRole('button', { name: '当前细纲' })).toBeInTheDocument()
    expect(JSON.parse(window.localStorage.getItem('nova.outline.pinned-settings:/books/demo') || '{}').paths).toContain('@current-chapter-plan')

    const latestPlans = [
      ...firstPlans,
      { path: 'setting/chapter-plans/group-3.md', title: '第三组细纲', excerpt: '', words: 100, updated_at: '' },
    ]
    rerender(
      <BookSettingsShortcuts
        workspace="/books/demo"
        tree={[]}
        chapterPlans={latestPlans}
        selectedFile={null}
        headerPinned
        onSelectFile={onSelectFile}
        onToggleHeaderPinned={vi.fn()}
      />,
    )
    await user.click(within(screen.getByTestId('book-setting-shortcuts')).getByRole('button', { name: '当前细纲' }))

    expect(onSelectFile).toHaveBeenLastCalledWith('setting/chapter-plans/group-3.md')
  })

  it('已 Pin 的设定文件隐藏更多按钮，文件操作保留在右键菜单', async () => {
    const user = userEvent.setup()
    const onReferenceFile = vi.fn()
    render(
      <BookSettingsShortcuts
        workspace="/books/demo"
        tree={[{ name: 'CREATOR.md', type: 'file' }]}
        chapterPlans={[]}
        selectedFile={null}
        headerPinned
        onSelectFile={vi.fn()}
        onToggleHeaderPinned={vi.fn()}
        onReferenceFile={onReferenceFile}
      />,
    )
    const shortcut = screen.getByRole('button', { name: '规则' })
    expect(shortcut).toHaveClass('text-center')
    expect(shortcut).not.toHaveClass('pr-7')
    expect(shortcut.parentElement).not.toBeNull()
    expect(within(shortcut.parentElement as HTMLElement).queryByRole('button', { name: '更多操作' })).not.toBeInTheDocument()

    await user.pointer({ keys: '[MouseRight]', target: shortcut })
    await user.click(await screen.findByRole('menuitem', { name: '引用到 Chat' }))

    expect(onReferenceFile).toHaveBeenCalledWith('CREATOR.md')
  })
})
