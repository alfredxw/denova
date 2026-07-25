import { useState } from 'react'
import type { KeyboardEvent } from 'react'
import { BookOpen, CheckCircle2, Circle, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ChapterSummary } from '@/lib/api'
import { formatNumber } from '../workbench-utils'

interface ChapterOutlineItemProps {
  chapter: ChapterSummary
  active: boolean
  onSelectFile: (path: string) => void | Promise<void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
}

/**
 * 目录栏里的单个章节条目。
 * 根节点带 data-chapter-path，供「定位当前章节」在滚动容器内查找目标元素。
 */
export function ChapterOutlineItem({
  chapter,
  active,
  onSelectFile,
  onSetChapterConfirmed,
}: ChapterOutlineItemProps) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  const handleSelect = () => {
    void onSelectFile(chapter.path)
  }
  const handleSelectKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    handleSelect()
  }
  const handleToggleConfirmed = async () => {
    if (saving || chapter.words === 0) return
    setSaving(true)
    try {
      await onSetChapterConfirmed(chapter.path, !chapter.confirmed)
    } catch (error) {
      console.error('更新章节确认状态失败', error)
    } finally {
      setSaving(false)
    }
  }
  const ConfirmIcon = saving ? Loader2 : chapter.confirmed ? CheckCircle2 : Circle
  const toggleTitle = saving ? t('common.loading') : chapter.confirmed ? t('planning.markDraft') : t('planning.confirmChapter')
  return (
    <div
      className={`nova-nav-item w-full border px-3 py-2 text-left ${
        active
          ? 'is-active border-[var(--nova-border)]'
          : 'border-transparent bg-[var(--nova-surface)]'
      }`}
      role="button"
      tabIndex={0}
      data-chapter-path={chapter.path}
      onClick={handleSelect}
      onKeyDown={handleSelectKeyDown}
    >
      <div className="flex w-full min-w-0 items-center gap-2 text-left">
        <BookOpen className={`h-3.5 w-3.5 shrink-0 ${active ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
        <span className="min-w-0 flex-1 truncate text-xs font-medium">{chapter.display_title}</span>
      </div>
      <div className="mt-1 flex items-center justify-between text-[11px] text-[var(--nova-text-faint)]">
        <span>{t('common.words', { count: formatNumber(chapter.words) })}</span>
        <div className="flex items-center gap-1.5">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 text-[var(--nova-text-muted)]">{chapter.status}</span>
          <button
            type="button"
            className={`inline-flex h-5 w-5 items-center justify-center rounded-[var(--nova-radius)] text-[var(--nova-text-faint)] hover:bg-[var(--nova-surface-2)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-40 ${saving ? 'opacity-70' : ''}`}
            disabled={chapter.words === 0}
            title={toggleTitle}
            aria-label={toggleTitle}
            aria-busy={saving}
            aria-disabled={saving || chapter.words === 0}
            onClick={(event) => {
              event.stopPropagation()
              void handleToggleConfirmed()
            }}
          >
            <ConfirmIcon className={`h-3.5 w-3.5 ${saving ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>
    </div>
  )
}
