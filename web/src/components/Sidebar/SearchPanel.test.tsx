import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { replaceWorkspace, searchWorkspace } from '@/lib/api'
import { SearchPanel } from './SearchPanel'

vi.mock('@/lib/api', () => ({
  replaceWorkspace: vi.fn(),
  searchWorkspace: vi.fn(),
}))

describe('SearchPanel（跨正文/规划/资料搜索）', () => {
  beforeEach(() => {
    vi.mocked(replaceWorkspace).mockReset()
    vi.mocked(searchWorkspace).mockReset()
    vi.mocked(searchWorkspace).mockResolvedValue([])
  })

  it('一次搜索返回并展示正文、规划与资料结果', async () => {
    const user = userEvent.setup()
    vi.mocked(searchWorkspace).mockResolvedValue([
      { path: 'chapters/ch01.md', line: 3, column: 0, preview: '灯塔 矗立在悬崖上', match_text: '灯塔' },
      { path: 'plans/outline.md', line: 1, column: 0, preview: '规划：灯塔 是主线目标', match_text: '灯塔' },
      { path: 'lore/places.md', line: 2, column: 0, preview: '资料：旧灯塔 的传说', match_text: '灯塔' },
    ])

    render(
      <SearchPanel
        workspace="/books/demo"
        onSelectResult={vi.fn()}
      />,
    )

    await user.type(screen.getByPlaceholderText('搜索当前书籍...'), '灯塔')

    await waitFor(() => {
      expect(searchWorkspace).toHaveBeenCalledWith('灯塔', 100, { regex: false })
    })
    expect(await screen.findByText('找到 3 条结果')).toBeInTheDocument()
    expect(screen.getByText('chapters/ch01.md')).toBeInTheDocument()
    expect(screen.getByText('plans/outline.md')).toBeInTheDocument()
    expect(screen.getByText('lore/places.md')).toBeInTheDocument()
    expect(screen.getAllByText('灯塔')).toHaveLength(3)
  })

  it('点击结果时把结果与查询词交给外层', async () => {
    const user = userEvent.setup()
    const handleSelect = vi.fn()
    const chapterResult = { path: 'chapters/ch01.md', line: 3, column: 0, preview: '灯塔 矗立在悬崖上', match_text: '灯塔' }
    vi.mocked(searchWorkspace).mockResolvedValue([chapterResult])

    render(
      <SearchPanel
        workspace="/books/demo"
        onSelectResult={handleSelect}
      />,
    )

    await user.type(screen.getByPlaceholderText('搜索当前书籍...'), '灯塔')
    await user.click(await screen.findByRole('button', { name: /灯塔/ }))

    expect(handleSelect).toHaveBeenCalledWith(chapterResult, '灯塔')
  })

  it('用 initialQuery 恢复搜索词并回传查询词', async () => {
    const onQueryChange = vi.fn()

    render(
      <SearchPanel
        workspace="/books/demo"
        onSelectResult={vi.fn()}
        initialQuery="灯塔"
        onQueryChange={onQueryChange}
      />,
    )

    expect(screen.getByPlaceholderText('搜索当前书籍...')).toHaveValue('灯塔')
    await waitFor(() => expect(onQueryChange).toHaveBeenCalledWith('灯塔'))
  })

  it('切换项目时恢复新项目保存的搜索词', async () => {
    const onQueryChange = vi.fn()
    const view = render(
      <SearchPanel
        workspace="/books/demo"
        onSelectResult={vi.fn()}
        initialQuery="灯塔"
        onQueryChange={onQueryChange}
      />,
    )

    await userEvent.setup().type(screen.getByPlaceholderText('搜索当前书籍...'), '续写')
    view.rerender(
      <SearchPanel
        workspace="/books/other"
        onSelectResult={vi.fn()}
        initialQuery="星空"
        onQueryChange={onQueryChange}
      />,
    )

    await waitFor(() => {
      expect(screen.getByPlaceholderText('搜索当前书籍...')).toHaveValue('星空')
    })
    expect(onQueryChange).toHaveBeenCalledWith('星空')
  })

  it('切换项目后清空上一项目的搜索词', async () => {
    const onQueryChange = vi.fn()
    const view = render(
      <SearchPanel
        workspace="/books/demo"
        onSelectResult={vi.fn()}
        initialQuery="灯塔"
        onQueryChange={onQueryChange}
      />,
    )

    await userEvent.setup().type(screen.getByPlaceholderText('搜索当前书籍...'), '续写')
    view.rerender(
      <SearchPanel
        workspace="/books/other"
        onSelectResult={vi.fn()}
        initialQuery=""
        onQueryChange={onQueryChange}
      />,
    )

    await waitFor(() => {
      expect(screen.getByPlaceholderText('搜索当前书籍...')).toHaveValue('')
    })
    expect(onQueryChange).toHaveBeenCalledWith('')
  })
})
