import { useId, useMemo, useState } from 'react'
import { BookOpen, Check, ChevronDown, LibraryBig, Loader2 } from 'lucide-react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { BookCoverThumbnail } from '@/components/Home/BookCoverThumbnail'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatDateTime } from '@/i18n'
import type { BookRecord } from '@/lib/api'

interface BookSwitcherProps {
  books: BookRecord[]
  currentBookName: string
  currentChapterCount?: number
  workspace: string
  compact?: boolean
  onSwitchBook: (path: string) => Promise<boolean>
  onManageBooks: () => void
}

/** 顶栏书籍上下文入口：只切换工作区，不改变用户当前所在的功能或创作模式。 */
export function BookSwitcher({
  books,
  currentBookName,
  currentChapterCount,
  workspace,
  compact = false,
  onSwitchBook,
  onManageBooks,
}: BookSwitcherProps) {
  const { t } = useTranslation()
  const menuLabelID = useId()
  const [open, setOpen] = useState(false)
  const [switchingPath, setSwitchingPath] = useState('')
  const orderedBooks = useMemo(
    () => booksForSwitcher(books, workspace, currentBookName),
    [books, currentBookName, workspace],
  )

  const selectBook = async (book: BookRecord) => {
    if (book.path === workspace) {
      setOpen(false)
      return
    }
    setSwitchingPath(book.path)
    try {
      if (await onSwitchBook(book.path)) setOpen(false)
    } catch (error) {
      console.error('[BookSwitcher.tsx] 切换书籍失败', { from: workspace, to: book.path, error })
    } finally {
      setSwitchingPath('')
    }
  }

  const triggerLabel = t('workbench.bookSwitcher.trigger', { title: currentBookName })

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={triggerLabel}
          title={triggerLabel}
          className={`flex min-w-0 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] outline-none transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:ring-2 focus-visible:ring-[var(--nova-field-focus-border)] data-[state=open]:border-[var(--nova-field-focus-border)] data-[state=open]:bg-[var(--nova-active)] data-[state=open]:text-[var(--nova-text)] ${compact ? 'h-8 max-w-[34vw] px-2 text-[11px]' : 'h-7 max-w-[min(16rem,28vw)] px-2.5 text-[11px]'}`}
        >
          <BookOpen className={`h-3.5 w-3.5 shrink-0 ${compact ? 'hidden' : ''}`} />
          <span className="min-w-0 truncate font-medium">{currentBookName}</span>
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={6}
        collisionPadding={8}
        aria-labelledby={menuLabelID}
        className="max-h-[min(32rem,calc(100vh-3.5rem))] w-[22rem] max-w-[calc(100vw-1rem)] gap-0 overflow-hidden border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)] ring-0"
      >
        <div id={menuLabelID} className="shrink-0 px-3 pb-2 pt-3 text-[11px] font-medium text-[var(--nova-text-faint)]">
          {t('workbench.bookSwitcher.title')}
        </div>
        <DropdownMenuGroup className="max-h-[min(21rem,calc(100vh-10rem))] overflow-y-auto px-1.5 pb-1.5">
          {orderedBooks.length > 0 ? orderedBooks.map((book) => {
            const current = book.path === workspace
            const loading = book.path === switchingPath
            return (
              <DropdownMenuItem
                key={book.path}
                aria-current={current ? 'page' : undefined}
                disabled={Boolean(switchingPath)}
                className={`min-h-14 gap-2.5 border-b border-[var(--nova-border)] px-2 py-1.5 last:border-b-0 focus:bg-[var(--nova-hover)] focus:text-[var(--nova-text)] ${current ? 'bg-[var(--nova-active)]' : ''}`}
                onSelect={(event) => {
                  event.preventDefault()
                  void selectBook(book)
                }}
              >
                <BookCoverThumbnail
                  book={book}
                  version={book.cover_updated_at}
                  decorative
                  className="h-11 w-8 shrink-0 rounded-[4px]"
                  iconClassName="h-3.5 w-3.5 text-[var(--nova-text-faint)]"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-xs font-medium text-[var(--nova-text)]">{book.name}</span>
                  <span className="mt-0.5 block truncate text-[10px] text-[var(--nova-text-faint)]">
                    {bookDetail(book, current, currentChapterCount, t)}
                  </span>
                </span>
                {loading ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-muted)]" />
                ) : current ? (
                  <Check className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
                ) : null}
              </DropdownMenuItem>
            )
          }) : (
            <div className="px-3 py-5 text-center text-[11px] text-[var(--nova-text-faint)]">
              {t('workbench.bookSwitcher.empty')}
            </div>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator className="mx-0 my-0 bg-[var(--nova-border)]" />
        <DropdownMenuItem
          className="m-1.5 min-h-9 gap-2 px-2 text-xs text-[var(--nova-text-muted)] focus:bg-[var(--nova-hover)] focus:text-[var(--nova-text)]"
          onSelect={() => {
            setOpen(false)
            onManageBooks()
          }}
        >
          <LibraryBig className="h-3.5 w-3.5" />
          {t('workbench.bookSwitcher.manage')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function booksForSwitcher(books: BookRecord[], workspace: string, currentBookName: string): BookRecord[] {
  if (!workspace) return books
  const currentIndex = books.findIndex((book) => book.path === workspace)
  if (currentIndex === -1) {
    return [{ name: currentBookName, path: workspace, author: '', last_opened_at: '' }, ...books]
  }
  return books.map((book, index) => (
    index === currentIndex ? { ...book, name: currentBookName || book.name } : book
  ))
}

function bookDetail(
  book: BookRecord,
  current: boolean,
  currentChapterCount: number | undefined,
  t: TFunction,
) {
  if (current) {
    return typeof currentChapterCount === 'number'
      ? t('workbench.bookSwitcher.chapterCount', { count: currentChapterCount })
      : t('workbench.bookSwitcher.current')
  }
  const lastOpened = formatDateTime(book.last_opened_at)
  if (lastOpened) return t('workbench.bookSwitcher.lastOpened', { time: lastOpened })
  if (book.author.trim()) return book.author.trim()
  return t('workbench.bookSwitcher.book')
}
