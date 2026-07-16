import { Check, Columns2, RefreshCw, RotateCcw, RotateCw, Rows3, Undo2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { ReviewThread, WorkspaceChangeGroupSummary } from '../types'
import type { ReviewDiffLayout } from './monaco/review-editor-adapter'
import { ReviewUtilityTab } from './ReviewUtilityTab'

interface ReviewToolbarProps {
  thread: ReviewThread
  selectedGroup: WorkspaceChangeGroupSummary | null
  layout: ReviewDiffLayout
  busy: boolean
  refreshing: boolean
  actionScopeAvailable: boolean
  onLayoutChange: (layout: ReviewDiffLayout) => void
  onGroupChange: (groupID: string) => void
  onReview: (decision: 'accept' | 'reject') => void
  onHistory: (action: 'undo' | 'redo') => void
  onRefresh: () => void
  onClose: () => void
}

export function ReviewToolbar({ thread, selectedGroup, layout, busy, refreshing, actionScopeAvailable, onLayoutChange, onGroupChange, onReview, onHistory, onRefresh, onClose }: ReviewToolbarProps) {
  const { t } = useTranslation()
  const canReview = actionScopeAvailable && selectedGroup?.apply_state === 'applied' && (selectedGroup.pending_edit_count ?? 0) > 0
  return (
    <header className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs text-[var(--nova-text-muted)]">
      <ReviewUtilityTab onClose={onClose} disabled={busy} />
      <div className="flex min-h-10 flex-wrap items-center gap-2 px-3 py-1.5">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px]">
            {t('changes.filesChanged', { count: thread.files.length })}
          </span>
          {thread.groups.length > 0 && (
            <label className="flex min-w-0 items-center gap-1.5">
              <span className="sr-only">{t('changes.noSelection')}</span>
              <select
                value={selectedGroup?.id ?? ''}
                disabled={busy}
                onChange={(event) => onGroupChange(event.target.value)}
                className="h-7 max-w-48 min-w-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-bg)] px-2 text-[11px] text-[var(--nova-text)] outline-none focus:border-[var(--nova-accent-blue)] disabled:opacity-50"
              >
                {thread.groups.map((group, index) => (
                  <option key={group.id} value={group.id}>
                    {t('changes.editNumber', { number: index + 1 })} · {t(`changes.status.${group.review_status}`, { defaultValue: group.review_status })}
                  </option>
                ))}
              </select>
            </label>
          )}
        </div>

        <div role="group" aria-label={t('changes.viewDiff')} className="flex h-7 items-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-bg)] p-0.5">
          <button
            type="button"
            data-review-layout="unified"
            aria-pressed={layout === 'unified'}
            onClick={() => onLayoutChange('unified')}
            className={`flex h-6 items-center gap-1 rounded px-2 text-[10px] ${layout === 'unified' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]'}`}
          >
            <Rows3 className="h-3 w-3" />{t('changes.diff.unified')}
          </button>
          <button
            type="button"
            data-review-layout="split"
            aria-pressed={layout === 'split'}
            onClick={() => onLayoutChange('split')}
            className={`flex h-6 items-center gap-1 rounded px-2 text-[10px] ${layout === 'split' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]'}`}
          >
            <Columns2 className="h-3 w-3" />{t('changes.diff.split')}
          </button>
        </div>

        <Button type="button" size="icon-xs" variant="ghost" disabled={busy || refreshing} onClick={onRefresh} aria-label={t('changes.refresh')}>
          <RefreshCw className={refreshing ? 'animate-spin' : ''} />
        </Button>
      </div>

      {selectedGroup && (
        <div className="flex min-h-9 flex-wrap items-center justify-end gap-1.5 border-t border-[var(--nova-border-soft)] px-3 py-1">
          <span className={`mr-auto text-[10px] ${actionScopeAvailable ? 'text-[var(--nova-text-faint)]' : 'text-[var(--nova-warning)]'}`}>
            {actionScopeAvailable
              ? `${t('changes.scope.group', { count: selectedGroup.paths?.length ?? 0 })} · ${t(`changes.applyState.${selectedGroup.apply_state}`, { defaultValue: selectedGroup.apply_state })}`
              : t('changes.scope.mismatch')}
          </span>
          <Button type="button" size="xs" variant="ghost" disabled={busy || !actionScopeAvailable || selectedGroup.can_undo !== true} onClick={() => onHistory('undo')}>
            <Undo2 />{t('changes.undo')}
          </Button>
          <Button type="button" size="xs" variant="ghost" disabled={busy || !actionScopeAvailable || selectedGroup.can_redo !== true} onClick={() => onHistory('redo')}>
            <RotateCw />{t('changes.redo')}
          </Button>
          {canReview && (
            <>
              <Button type="button" size="xs" variant="outline" disabled={busy} className="border-[var(--nova-success)]/40 text-[var(--nova-success)]" onClick={() => onReview('accept')}>
                <Check />{t('changes.acceptAll')}
              </Button>
              <Button type="button" size="xs" variant="outline" disabled={busy} className="border-[var(--nova-danger-border)] text-[var(--nova-danger)]" onClick={() => onReview('reject')}>
                <RotateCcw />{t('changes.rejectAll')}
              </Button>
            </>
          )}
        </div>
      )}
    </header>
  )
}
