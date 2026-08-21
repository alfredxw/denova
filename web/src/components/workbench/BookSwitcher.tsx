import { useId, useMemo, useState } from 'react'
import { BookOpen, Check, LibraryBig, Loader2 } from 'lucide-react'
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
import { formatDateTime, formatLocaleNumber } from '@/i18n'
import type { BookRecord } from '@/lib/api'
import {
  WorkbenchContextSwitcherTrigger,
  WORKBENCH_CONTEXT_MENU_CLASS,
  WORKBENCH_CONTEXT_MENU_GROUP_CLASS,
  WORKBENCH_CONTEXT_MENU_ITEM_CLASS,
} from './WorkbenchContextSwitcher'

interface BookSwitcherProps {
  books: BookRecord[]
  currentBookName: string
  currentChapterCount?: number
  currentWordCount?: number
  workspace: string
  compact?: boolean
  iconOnly?: boolean
  onSwitchBook: (path: string) => Promise<boolean>
  onManageBooks: () => void
}

/** Switches the active book context without changing the current feature or creation mode. */
export function BookSwitcher({
  books,
  currentBookName,
  currentChapterCount,
  currentWordCount,
  workspace,
  compact = false,
  iconOnly = false,
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
      console.error('[BookSwitcher.tsx] Failed to switch books', { from: workspace, to: book.path, error })
    } finally {
      setSwitchingPath('')
    }
  }

  const triggerLabel = t('workbench.bookSwitcher.trigger', { title: currentBookName })

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <WorkbenchContextSwitcherTrigger
          aria-label={triggerLabel}
          icon={BookOpen}
          label={currentBookName}
          compact={compact}
          iconOnly={iconOnly}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={6}
        collisionPadding={8}
        aria-labelledby={menuLabelID}
        className={WORKBENCH_CONTEXT_MENU_CLASS}
      >
        <div id={menuLabelID} className="shrink-0 px-3 pb-2 pt-3 text-[11px] font-medium text-[var(--nova-text-faint)]">
          {t('workbench.bookSwitcher.title')}
        </div>
        <DropdownMenuGroup className={WORKBENCH_CONTEXT_MENU_GROUP_CLASS}>
          {orderedBooks.length > 0 ? orderedBooks.map((book) => {
            const current = book.path === workspace
            const loading = book.path === switchingPath
            return (
              <DropdownMenuItem
                key={book.path}
                aria-current={current ? 'page' : undefined}
                disabled={Boolean(switchingPath)}
                className={`${WORKBENCH_CONTEXT_MENU_ITEM_CLASS} ${current ? 'bg-[var(--nova-active)]' : ''}`}
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
                    {bookDetail(book, current, currentChapterCount, currentWordCount, t)}
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
    return [{ project_id: '', name: currentBookName, path: workspace, author: '', last_opened_at: '' }, ...books]
  }
  return books.map((book, index) => (
    index === currentIndex ? { ...book, name: currentBookName || book.name } : book
  ))
}

function bookDetail(
  book: BookRecord,
  current: boolean,
  currentChapterCount: number | undefined,
  currentWordCount: number | undefined,
  t: TFunction,
) {
  if (current) {
    if (typeof currentChapterCount === 'number' && typeof currentWordCount === 'number') {
      return t('workbench.bookSwitcher.stats', {
        chapters: formatLocaleNumber(currentChapterCount),
        words: formatLocaleNumber(currentWordCount),
      })
    }
    return typeof currentChapterCount === 'number'
      ? t('workbench.bookSwitcher.chapterCount', { count: formatLocaleNumber(currentChapterCount) })
      : t('workbench.bookSwitcher.current')
  }
  const lastOpened = formatDateTime(book.last_opened_at)
  if (lastOpened) return t('workbench.bookSwitcher.lastOpened', { time: lastOpened })
  if (book.author.trim()) return book.author.trim()
  return t('workbench.bookSwitcher.book')
}
