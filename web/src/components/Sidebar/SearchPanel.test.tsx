import type { Key, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { WorkspaceSearchResult } from '@/lib/api'
import { SearchPanel } from './SearchPanel'

const api = vi.hoisted(() => ({
  searchWorkspace: vi.fn(),
  replaceWorkspace: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api')>(),
  ...api,
}))

// SearchPanel owns the grouping logic; the library only needs a deterministic DOM adapter here.
vi.mock('react-virtuoso', () => ({
  GroupedVirtuoso: ({
    groupCounts = [],
    data = [],
    groupContent,
    itemContent,
    computeItemKey,
  }: {
    groupCounts?: number[]
    data?: WorkspaceSearchResult[]
    groupContent?: (groupIndex: number) => ReactNode
    itemContent?: (index: number, groupIndex: number, item: WorkspaceSearchResult) => ReactNode
    computeItemKey?: (index: number, item?: WorkspaceSearchResult) => Key
  }) => {
    let itemOffset = 0
    return (
      <div data-testid="grouped-search-results">
        {groupCounts.map((count, groupIndex) => {
          const groupOffset = itemOffset
          itemOffset += count
          return (
            <section key={computeItemKey?.(groupIndex, undefined) ?? groupIndex}>
              {groupContent?.(groupIndex)}
              {Array.from({ length: count }, (_, localIndex) => {
                const index = groupOffset + localIndex
                const item = data[index]
                return (
                  <div key={computeItemKey?.(index, item) ?? index}>
                    {itemContent?.(index, groupIndex, item)}
                  </div>
                )
              })}
            </section>
          )
        })}
      </div>
    )
  },
}))

vi.mock('@/components/ui/dropdown-menu', () => ({
  DropdownMenu: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DropdownMenuCheckboxItem: ({
    children,
    checked,
    onCheckedChange,
  }: {
    children: ReactNode
    checked: boolean
    onCheckedChange: (checked: boolean) => void
  }) => (
    <button type="button" role="menuitemcheckbox" aria-checked={checked} onClick={() => onCheckedChange(!checked)}>
      {children}
    </button>
  ),
  DropdownMenuItem: ({ children, onSelect }: { children: ReactNode; onSelect: () => void }) => (
    <button type="button" role="menuitem" onClick={onSelect}>{children}</button>
  ),
}))

describe('SearchPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.searchWorkspace.mockResolvedValue([])
  })

  it('focuses the full-width query and keeps regex and replace in search options', async () => {
    const user = userEvent.setup()
    render(<SearchPanel projectId="book-a" onSelectResult={vi.fn()} />)

    const input = screen.getByPlaceholderText('搜索当前书籍...')
    expect(input).toHaveFocus()

    await user.click(screen.getByRole('button', { name: '搜索选项' }))
    await user.click(screen.getByRole('menuitemcheckbox', { name: '正则匹配' }))
    await user.type(input, 'alpha')

    await waitFor(() => expect(api.searchWorkspace).toHaveBeenLastCalledWith(
      'book-a',
      'alpha',
      100,
      { regex: true },
    ))

    await user.click(screen.getByRole('button', { name: '搜索选项' }))
    await user.click(screen.getByRole('menuitem', { name: '替换' }))
    expect(screen.getByPlaceholderText('替换为...')).toBeVisible()
  })

  it('shows a fair preview per file, expands from the full file row, and preserves selection', async () => {
    const user = userEvent.setup()
    const onSelectResult = vi.fn()
    api.searchWorkspace.mockResolvedValue([
      result('chapters/ch00001-第一章-起点.md', 1, 'alpha one'),
      result('chapters/ch00001-第一章-起点.md', 2, 'alpha two'),
      result('chapters/ch00001-第一章-起点.md', 3, 'alpha three'),
      result('chapters/ch00001-第一章-起点.md', 4, 'alpha four'),
      result('setting/lore.md', 8, 'alpha lore'),
    ])
    render(<SearchPanel projectId="book-a" onSelectResult={onSelectResult} />)

    await user.type(screen.getByPlaceholderText('搜索当前书籍...'), 'alpha')

    const firstPreview = await screen.findByText(exactElementText('alpha one'))
    expect(firstPreview).toBeVisible()
    expect(screen.getByText(exactElementText('alpha lore'))).toBeVisible()
    expect(screen.queryByText(exactElementText('alpha four'))).not.toBeInTheDocument()
    expect(screen.getByText('第一章 · 起点')).toBeVisible()
    expect(screen.getByTitle('chapters/ch00001-第一章-起点.md')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '展开 第一章 · 起点 的全部 4 条结果' }))
    const fourthResult = screen.getByText(exactElementText('alpha four')).closest('button')
    expect(fourthResult).not.toBeNull()
    if (!fourthResult) throw new Error('Expected the expanded search result to be clickable')
    await user.click(fourthResult)

    expect(onSelectResult).toHaveBeenCalledWith(expect.objectContaining({ line: 4 }), 'alpha')
    expect(fourthResult).toHaveAttribute('aria-current', 'true')
  })

  it('discloses when the backend result limit may have truncated the list', async () => {
    const user = userEvent.setup()
    api.searchWorkspace.mockResolvedValue(
      Array.from({ length: 100 }, (_, index) => result('chapters/long.md', index + 1, `alpha ${index + 1}`)),
    )
    render(<SearchPanel projectId="book-a" onSelectResult={vi.fn()} />)

    await user.type(screen.getByPlaceholderText('搜索当前书籍...'), 'alpha')

    expect(await screen.findByText('显示前 100 条结果')).toBeVisible()
  })
})

function result(path: string, line: number, preview: string): WorkspaceSearchResult {
  return {
    path,
    line,
    column: 1,
    preview,
    match_text: 'alpha',
  }
}

function exactElementText(text: string) {
  return (_content: string, element: Element | null) => element?.tagName === 'P' && element.textContent === text
}
