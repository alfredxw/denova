import { useTranslation } from 'react-i18next'
import { formatDateTime, formatLocaleNumber } from '@/i18n'
import type { ChapterSummary, WorkspaceSummary } from '@/lib/api'

interface WritingStatusBarProps {
  summary: WorkspaceSummary | null
  currentChapter?: ChapterSummary
  editorLine?: number
  isStreaming: boolean
}

/** Writing-only progress and cursor context; global navigation stays free of document metadata. */
export function WritingStatusBar({ summary, currentChapter, editorLine, isStreaming }: WritingStatusBarProps) {
  const { t } = useTranslation()
  return (
    <div className="nova-writing-statusbar flex h-6 shrink-0 items-center gap-3 overflow-hidden border-t border-[var(--nova-border)] bg-[var(--nova-bg-deep)] px-3 text-[10px] text-[var(--nova-text-faint)]">
      {summary ? (
        <span className="shrink-0">
          {t('workbench.status.summary', {
            title: summary.title || t('workbench.untitled'),
            chapters: formatLocaleNumber(summary.chapter_count),
            words: formatLocaleNumber(summary.total_words),
          })}
        </span>
      ) : null}
      {currentChapter ? (
        <span className="min-w-0 truncate">
          {t('workbench.status.currentChapter', {
            title: currentChapter.display_title,
            words: formatLocaleNumber(currentChapter.words),
          })}
        </span>
      ) : null}
      {currentChapter ? (
        <span className="shrink-0">
          {t('editor.updatedAt', {
            time: currentChapter.updated_at ? formatDateTime(currentChapter.updated_at) : t('editor.unknownTime'),
          })}
          {editorLine !== undefined ? ` · ${t('editor.currentLine', { line: formatLocaleNumber(editorLine) })}` : null}
        </span>
      ) : null}
      {isStreaming ? (
        <span role="status" className={currentChapter ? 'shrink-0' : 'ml-auto shrink-0'}>
          {t('workbench.status.streaming')}
        </span>
      ) : null}
    </div>
  )
}
