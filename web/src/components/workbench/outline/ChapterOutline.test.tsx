import type { ReactElement } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChapterSummary, DocumentPreview } from '@/lib/api'
import { ChapterOutline } from './ChapterOutline'

function makeChapter(overrides: Partial<ChapterSummary>): ChapterSummary {
  return {
    path: 'chapters/ch1.md',
    file_name: 'ch1.md',
    display_title: '第 1 章 起点',
    index: 1,
    words: 1200,
    status: 'draft',
    confirmed: false,
    updated_at: '2026-07-25T00:00:00Z',
    volume: '',
    volume_path: '',
    ...overrides,
  }
}

function makePlan(overrides: Partial<DocumentPreview>): DocumentPreview {
  return {
    path: 'setting/chapter-plans/group-2.md',
    title: '第 6-10 章细纲',
    excerpt: '主角在城门口遇到旧识。',
    words: 300,
    updated_at: '2026-07-25T00:00:00Z',
    ...overrides,
  }
}

const chapters = [
  makeChapter({ path: 'chapters/ch1.md', display_title: '第 1 章 起点', index: 1 }),
  makeChapter({ path: 'chapters/ch2.md', display_title: '第 2 章 转折', index: 2 }),
]

const chapterPlans = [
  makePlan({ path: 'setting/chapter-plans/group-1.md', title: '第 1-5 章细纲' }),
  makePlan({}),
]

function renderOutline(props: Partial<Parameters<typeof ChapterOutline>[0]> = {}) {
  const onSelectFile = vi.fn()
  const result = render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 720, itemHeight: 72 }}>
      <ChapterOutline
        projectId="project-demo"
        tree={[]}
        chapters={chapters}
        chapterPlans={chapterPlans}
        selectedFile={null}
        onSelectFile={onSelectFile}
        onRequestBookSettingCreate={vi.fn()}
        onSetChapterConfirmed={vi.fn()}
        {...props}
      />
    </VirtuosoMockContext.Provider>,
  )
  return { onSelectFile, ...result }
}

describe('ChapterOutline', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('当前细纲只在滚动目录展示一次，单击直接打开全文', async () => {
    const user = userEvent.setup()
    const { onSelectFile } = renderOutline()

    const navigation = screen.getByRole('navigation', { name: '作品大纲' })
    const latestPlan = screen.getByRole('button', { name: '第 6-10 章细纲' })
    expect(screen.getByText('当前细纲')).toBeInTheDocument()
    expect(navigation).toContainElement(latestPlan)
    expect(screen.getByTestId('book-settings-header-frame')).not.toContainElement(latestPlan)
    expect(latestPlan).toHaveClass('px-2', 'py-1', '!text-sm', '!leading-normal')
    expect(latestPlan).not.toHaveClass('px-3', 'py-2', 'text-xs')
    expect(screen.getAllByText('第 6-10 章细纲')).toHaveLength(1)
    expect(screen.queryByText('主角在城门口遇到旧识。')).not.toBeInTheDocument()

    await user.click(latestPlan)
    expect(onSelectFile).toHaveBeenCalledWith('setting/chapter-plans/group-2.md')
  })

  it('可取消固定书籍设定区域，当前细纲始终留在滚动目录中', async () => {
    const user = userEvent.setup()
    renderOutline()

    const navigation = screen.getByRole('navigation', { name: '作品大纲' })
    const latestPlan = screen.getByRole('button', { name: '第 6-10 章细纲' })
    expect(navigation).toContainElement(latestPlan)
    expect(screen.getByTestId('book-settings-header-frame')).not.toContainElement(latestPlan)
    const pinnedFrameClass = screen.getByTestId('book-settings-header-frame').className
    expect(screen.getByRole('button', { name: '取消固定顶部区域' }).querySelector('.lucide-pin-off')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '取消固定顶部区域' })).not.toHaveClass('bg-[var(--nova-active)]')

    await user.click(screen.getByRole('button', { name: '取消固定顶部区域' }))

    expect(navigation).toContainElement(screen.getByRole('button', { name: '第 6-10 章细纲' }))
    expect(screen.getByRole('button', { name: '固定顶部区域' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: '固定顶部区域' }).querySelector('.lucide-pin')).toBeInTheDocument()
    expect(screen.getByTestId('book-settings-header-frame')).toHaveClass(...pinnedFrameClass.split(' '))
    expect(window.localStorage.getItem('nova.outline.book-settings-header-pinned')).toBe('false')
  })

  it('下方只展示历史细纲，不重复最新细纲', async () => {
    const user = userEvent.setup()
    renderOutline()

    const history = screen.getByRole('button', { name: '历史细纲 1 组' })
    expect(history).toHaveAttribute('aria-expanded', 'false')
    await user.click(history)
    expect(screen.getByRole('button', { name: '第 1-5 章细纲' })).toBeInTheDocument()
    expect(screen.getAllByText('第 6-10 章细纲')).toHaveLength(1)
  })

  it('空数据时保留清晰空状态，不展示无效的最新章入口', () => {
    renderOutline({ chapters: [], chapterPlans: [] })

    expect(screen.getByText('还没有细纲')).toBeInTheDocument()
    expect(screen.getByText('chapters/ 下还没有章节')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '跳到最新章' })).not.toBeInTheDocument()
  })

  it('章节切换来自面板外部时自动滚动定位到已打开章节', async () => {
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    renderOutline({ selectedFile: 'chapters/ch2.md' })

    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'auto' })
    })
  })

  it('面板内点击选中的章节不再触发自动定位', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    const { rerender, ...rest } = renderOutline()

    await user.click(screen.getByText('第 2 章 转折'))
    expect(rest.onSelectFile).toHaveBeenCalledWith('chapters/ch2.md')

    rerenderOutline(rerender, { selectedFile: 'chapters/ch2.md' })
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(scrollIntoView).not.toHaveBeenCalled()
  })

  it('卷折叠后仍在固定区提供最新章入口，点击只定位且不覆盖卷标题', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    const { onSelectFile } = renderOutline()

    await user.click(screen.getByRole('button', { name: '未分卷 2 章' }))
    expect(screen.queryByText('第 2 章 转折')).not.toBeInTheDocument()
    expect(document.querySelector('.sticky')).not.toBeInTheDocument()
    const locateLatest = screen.getByRole('button', { name: '跳到最新章' })
    expect(locateLatest).not.toHaveTextContent('最新章')
    expect(screen.getByTestId('book-settings-header-frame')).toContainElement(locateLatest)
    await user.hover(locateLatest)
    expect(await screen.findByRole('tooltip')).toHaveTextContent('跳到最新章')
    await user.click(locateLatest)

    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'smooth' })
    })
    expect(screen.getByText('第 2 章 转折')).toBeInTheDocument()
    expect(onSelectFile).not.toHaveBeenCalled()
  })

  it('下滑超过阈值后启用固定区回顶动作，点击平滑回顶', async () => {
    renderOutline()
    const scrollContainer = screen.getByRole('navigation', { name: '作品大纲' })
    const scrollTo = vi.fn()
    Object.assign(scrollContainer, { scrollTo })
    Object.defineProperty(scrollContainer, 'scrollTop', { configurable: true, writable: true, value: 0 })

    const backToTop = screen.getByRole('button', { name: '回到顶部' })
    expect(backToTop).toBeDisabled()

    scrollContainer.scrollTop = 400
    fireEvent.scroll(scrollContainer)
    await waitFor(() => {
      expect(backToTop).toBeEnabled()
    })

    fireEvent.click(backToTop)
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
  })

  it('响应编辑器发出的目录定位请求，并展开目标章节所在卷', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    const { rerender } = renderOutline()

    await user.click(screen.getByRole('button', { name: '未分卷 2 章' }))
    rerenderOutline(rerender, { revealRequest: { path: 'chapters/ch2.md', nonce: 1 } })

    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'smooth' })
    })
    expect(screen.getByText('第 2 章 转折')).toBeInTheDocument()
  })

  it('章节文件可从右键菜单引用到 Chat', async () => {
    const user = userEvent.setup()
    const onReferenceFile = vi.fn()
    renderOutline({ onReferenceFile })

    await user.pointer({ keys: '[MouseRight]', target: screen.getByText('第 2 章 转折') })
    await user.click(await screen.findByRole('menuitem', { name: '引用到 Chat' }))

    expect(onReferenceFile).toHaveBeenCalledWith('chapters/ch2.md')
  })

  it('章节文件可通过更多菜单重命名和确认删除', async () => {
    const user = userEvent.setup()
    const onRenameItem = vi.fn(async () => undefined)
    const onDeleteItem = vi.fn(async () => undefined)
    renderOutline({ onRenameItem, onDeleteItem })
    const chapterRow = screen.getByText('第 2 章 转折').closest('[data-chapter-path]')?.parentElement
    if (!chapterRow) throw new Error('chapter action row is not rendered')

    await user.click(within(chapterRow).getByRole('button', { name: '更多操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '重命名文件' }))
    const renameInput = screen.getByLabelText('新名称')
    await user.clear(renameInput)
    await user.type(renameInput, 'chapter-02.md')
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: '确认' }))
    await waitFor(() => expect(onRenameItem).toHaveBeenCalledWith('chapters/ch2.md', 'chapter-02.md'))

    await user.click(within(chapterRow).getByRole('button', { name: '更多操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '删除' }))
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '删除' }))
    await waitFor(() => expect(onDeleteItem).toHaveBeenCalledWith('chapters/ch2.md'))
  })

  it('长篇作品只挂载视口附近的章节条目', async () => {
    const longBook = Array.from({ length: 300 }, (_, index) => makeChapter({
      path: `chapters/ch${index + 1}.md`,
      display_title: `第 ${index + 1} 章`,
      index: index + 1,
    }))
    renderOutline({ chapters: longBook, chapterPlans: [] })

    await waitFor(() => expect(document.querySelectorAll('[data-chapter-path]').length).toBeGreaterThan(0))
    expect(document.querySelectorAll('[data-chapter-path]').length).toBeLessThan(80)
  })

  it('切换选中章节不会重渲染无关的可见章节', async () => {
    let unrelatedTitleReads = 0
    const unrelatedChapter = makeChapter({ path: 'chapters/ch3.md', index: 3 })
    Object.defineProperty(unrelatedChapter, 'display_title', {
      configurable: true,
      enumerable: true,
      get: () => {
        unrelatedTitleReads += 1
        return '第 3 章 无关章节'
      },
    })
    const testChapters = [...chapters, unrelatedChapter]
    const { rerender } = renderOutline({ chapters: testChapters, selectedFile: 'chapters/ch1.md' })
    await screen.findByText('第 3 章 无关章节')
    const initialReads = unrelatedTitleReads

    rerenderOutline(rerender, { chapters: testChapters, selectedFile: 'chapters/ch2.md' })

    await waitFor(() => expect(screen.getByText('第 2 章 转折').closest('[data-chapter-path]')).toHaveClass('is-active'))
    expect(unrelatedTitleReads).toBe(initialReads)
  })
})

function rerenderOutline(rerender: (ui: ReactElement) => void, props: Partial<Parameters<typeof ChapterOutline>[0]>) {
  rerender(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 720, itemHeight: 72 }}>
      <ChapterOutline
        projectId="project-demo"
        tree={[]}
        chapters={chapters}
        chapterPlans={chapterPlans}
        selectedFile={null}
        onSelectFile={vi.fn()}
        onRequestBookSettingCreate={vi.fn()}
        onSetChapterConfirmed={vi.fn()}
        {...props}
      />
    </VirtuosoMockContext.Provider>,
  )
}
