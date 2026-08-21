import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import { setConfiguredLocale } from '@/i18n'
import type { BookRecord } from '@/lib/api'
import { BookSwitcher } from './BookSwitcher'

const books: BookRecord[] = [
  { project_id: 'book-current', name: '目录名', path: '/books/current', author: '', cover_updated_at: 'cover-1', last_opened_at: '2026-07-18T12:00:00Z' },
  { project_id: 'book-poem', name: '命定之诗', path: '/books/poem', author: '', cover_updated_at: 'cover-2', last_opened_at: '2026-07-17T12:00:00Z' },
  { project_id: 'book-moon', name: '枫江月', path: '/books/moon', author: '未名', last_opened_at: '' },
]

describe('BookSwitcher', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
  })

  it('opens from the current title and places the current book first with its cover metadata', async () => {
    const user = userEvent.setup()
    renderSwitcher()

    await user.click(screen.getByRole('button', { name: '切换书籍，当前：示例书' }))

    expect(screen.getByRole('menu', { name: '切换书籍' })).toBeInTheDocument()
    const items = screen.getAllByRole('menuitem')
    expect(within(items[0]).getByText('示例书')).toBeInTheDocument()
    expect(within(items[0]).getByText('10 章')).toBeInTheDocument()
    expect(items[0]).toHaveAttribute('aria-current', 'page')
    expect(items[0].querySelector('img')).toHaveAttribute('src', expect.stringContaining('cover-1'))
    expect(screen.queryByRole('img', { name: '示例书' })).not.toBeInTheDocument()
  })

  it('keeps aggregate stats out of the global trigger', () => {
    renderSwitcher({ currentWordCount: 12000 })

    const trigger = screen.getByRole('button', { name: '切换书籍，当前：示例书' })
    expect(trigger).toHaveTextContent('示例书')
    expect(trigger).not.toHaveTextContent('10 章')
    expect(trigger).not.toHaveTextContent('12,000 字')
  })

  it('keeps switcher chrome unselectable and aligns the chevron to the far edge', () => {
    const longTitle = '一部标题很长但仍应保留右侧展开符号的示例书'
    renderSwitcher({ currentBookName: longTitle })

    const trigger = screen.getByRole('button', { name: `切换书籍，当前：${longTitle}` })
    expect(trigger).toHaveClass('select-none')
    expect(within(trigger).getByText(longTitle)).toHaveClass('truncate')
    expect(trigger.querySelector('svg:last-child')).toHaveClass('ml-auto')
  })

  it('keeps the trigger and empty state usable without any book data', async () => {
    const user = userEvent.setup()
    renderSwitcher({ books: [], currentBookName: '', workspace: '' })

    const trigger = screen.getByRole('button', { name: '切换书籍，当前：' })
    expect(trigger.querySelector('svg:last-child')).toHaveClass('ml-auto')
    await user.click(trigger)
    expect(screen.getByText('书架中暂无书籍')).toBeInTheDocument()
  })

  it('switches without invoking book management and closes after a successful switch', async () => {
    const user = userEvent.setup()
    const onSwitchBook = vi.fn().mockResolvedValue(true)
    const onManageBooks = vi.fn()
    renderSwitcher({ onSwitchBook, onManageBooks })

    await user.click(screen.getByRole('button', { name: '切换书籍，当前：示例书' }))
    await user.click(screen.getByRole('menuitem', { name: /命定之诗/ }))

    expect(onSwitchBook).toHaveBeenCalledWith('/books/poem')
    expect(onManageBooks).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('menu', { name: '切换书籍' })).not.toBeInTheDocument())
  })

  it('preserves the shared manual order instead of pinning the current book first', async () => {
    const user = userEvent.setup()
    renderSwitcher({ books: [books[1], books[0], books[2]] })

    await user.click(screen.getByRole('button', { name: '切换书籍，当前：示例书' }))

    const items = screen.getAllByRole('menuitem')
    expect(within(items[0]).getByText('命定之诗')).toBeInTheDocument()
    expect(within(items[1]).getByText('示例书')).toBeInTheDocument()
    expect(items[1]).toHaveAttribute('aria-current', 'page')
  })

  it('opens the full bookshelf from the footer action', async () => {
    const user = userEvent.setup()
    const onManageBooks = vi.fn()
    renderSwitcher({ onManageBooks })

    await user.click(screen.getByRole('button', { name: '切换书籍，当前：示例书' }))
    await user.click(screen.getByRole('menuitem', { name: '管理书架' }))

    expect(onManageBooks).toHaveBeenCalledOnce()
  })
})

function renderSwitcher(overrides: Partial<ComponentProps<typeof BookSwitcher>> = {}) {
  return render(
    <BookSwitcher
      books={books}
      currentBookName="示例书"
      currentChapterCount={10}
      workspace="/books/current"
      onSwitchBook={vi.fn().mockResolvedValue(true)}
      onManageBooks={vi.fn()}
      {...overrides}
    />,
  )
}
