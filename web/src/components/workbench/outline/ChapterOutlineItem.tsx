import { memo, useState } from 'react'
import { BookOpen, CheckCircle2, Circle, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ChapterSummary } from '@/lib/api'
import { formatNumber } from '../workbench-utils'
import { OutlineFileActions } from './OutlineFileActions'

interface ChapterOutlineItemProps {
  chapter: ChapterSummary
  active: boolean
  onSelectFile: (path: string) => void | Promise<void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
}

/** One windowed outline row; the data path lets navigation find an already mounted target. */
export const ChapterOutlineItem = memo(function ChapterOutlineItem({
  chapter,
  active,
  onSelectFile,
  onSetChapterConfirmed,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
}: ChapterOutlineItemProps) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const handleSelect = () => {
    void onSelectFile(chapter.path)
  }
  const handleToggleConfirmed = async () => {
    if (saving || chapter.words === 0) return
    setSaving(true)
    try {
      await onSetChapterConfirmed(chapter.path, !chapter.confirmed)
    } catch (error) {
      console.error('Failed to update chapter confirmation state', error)
    } finally {
      setSaving(false)
    }
  }
  const ConfirmIcon = saving ? Loader2 : chapter.confirmed ? CheckCircle2 : Circle
  const toggleTitle = saving ? t('common.loading') : chapter.confirmed ? t('planning.markDraft') : t('planning.confirmChapter')
  return (
    <OutlineFileActions
      path={chapter.path}
      triggerPlacement="top"
      onReferenceFile={onReferenceFile}
      onRevealFile={onRevealFile}
      onRenameItem={onRenameItem}
      onDeleteItem={onDeleteItem}
    >
      <div
        className={`nova-nav-item relative w-full border text-left ${
          active
            ? 'is-active border-[var(--nova-border)]'
            : 'border-transparent bg-[var(--nova-surface)]'
        }`}
        data-chapter-path={chapter.path}
      >
        <button type="button" className="w-full px-3 py-2 text-left" onClick={handleSelect}>
          <div className="flex w-full min-w-0 items-center gap-2 pr-6 text-left">
            <BookOpen className={`h-3.5 w-3.5 shrink-0 ${active ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
            <span className="min-w-0 flex-1 truncate text-xs font-medium">{chapter.display_title}</span>
          </div>
          <div className="mt-1 flex items-center justify-between gap-2 overflow-hidden pr-6 text-[11px] text-[var(--nova-text-faint)]">
            <span className="min-w-0 truncate">{t('common.words', { count: formatNumber(chapter.words) })}</span>
            <span className="shrink-0 whitespace-nowrap rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">{chapter.status}</span>
          </div>
        </button>
        <button
          type="button"
          className={`absolute bottom-1.5 right-1.5 inline-flex h-5 w-5 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-faint)] hover:bg-[var(--nova-surface-2)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-40 ${saving ? 'opacity-70' : ''}`}
          disabled={chapter.words === 0}
          aria-label={toggleTitle}
          aria-busy={saving}
          aria-disabled={saving || chapter.words === 0}
          onClick={() => { void handleToggleConfirmed() }}
        >
          <ConfirmIcon className={`h-3.5 w-3.5 ${saving ? 'animate-spin' : ''}`} />
        </button>
      </div>
    </OutlineFileActions>
  )
})
