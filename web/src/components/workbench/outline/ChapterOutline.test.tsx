import type { ReactElement } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
    <ChapterOutline
      workspace="demo"
      tree={[]}
      chapters={chapters}
      chapterPlans={chapterPlans}
      selectedFile={null}
      onSelectFile={onSelectFile}
      onRequestBookSettingCreate={vi.fn()}
      onSetChapterConfirmed={vi.fn()}
      {...props}
    />,
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

  it('固定区只展示一次最新细纲，单击直接打开全文', async () => {
    const user = userEvent.setup()
    const { onSelectFile } = renderOutline()

    const navigation = screen.getByRole('navigation', { name: '作品大纲' })
    const latestPlan = screen.getByRole('button', { name: '第 6-10 章细纲' })
    expect(navigation).not.toContainElement(latestPlan)
    expect(screen.getAllByText('第 6-10 章细纲')).toHaveLength(1)
    expect(screen.queryByText('主角在城门口遇到旧识。')).not.toBeInTheDocument()

    await user.click(latestPlan)
    expect(onSelectFile).toHaveBeenCalledWith('setting/chapter-plans/group-2.md')
  })

  it('可取消固定顶部区域，并持久化到滚动导航中', async () => {
    const user = userEvent.setup()
    renderOutline()

    await user.click(screen.getByRole('button', { name: '取消固定顶部区域' }))

    const navigation = screen.getByRole('navigation', { name: '作品大纲' })
    expect(navigation).toContainElement(screen.getByRole('button', { name: '第 6-10 章细纲' }))
    expect(screen.getByRole('button', { name: '固定顶部区域' })).toHaveAttribute('aria-pressed', 'false')
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

  it('最新章不可见时提供明确入口，点击只定位不打开文件', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    const { onSelectFile } = renderOutline()

    await user.click(screen.getByRole('button', { name: '未分卷 2 章' }))
    const locateLatest = await screen.findByRole('button', { name: '跳到最新章' })
    await user.click(locateLatest)

    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'smooth' })
    })
    expect(onSelectFile).not.toHaveBeenCalled()
  })

  it('下滑超过阈值后出现回顶浮标，点击平滑回顶', async () => {
    renderOutline()
    const scrollContainer = screen.getByRole('navigation', { name: '作品大纲' })
    const scrollTo = vi.fn()
    Object.assign(scrollContainer, { scrollTo })
    Object.defineProperty(scrollContainer, 'scrollTop', { configurable: true, writable: true, value: 0 })

    expect(screen.getByTitle('回到顶部')).toHaveClass('opacity-0')

    scrollContainer.scrollTop = 400
    fireEvent.scroll(scrollContainer)
    await waitFor(() => {
      expect(screen.getByTitle('回到顶部')).toHaveClass('opacity-80')
    })

    fireEvent.click(screen.getByTitle('回到顶部'))
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
  })
})

function rerenderOutline(rerender: (ui: ReactElement) => void, props: Partial<Parameters<typeof ChapterOutline>[0]>) {
  rerender(
    <ChapterOutline
      workspace="demo"
      tree={[]}
      chapters={chapters}
      chapterPlans={chapterPlans}
      selectedFile={null}
      onSelectFile={vi.fn()}
      onRequestBookSettingCreate={vi.fn()}
      onSetChapterConfirmed={vi.fn()}
      {...props}
    />,
  )
}
