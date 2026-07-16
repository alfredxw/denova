import { Bot, Check, ChevronDown, ChevronsDownUp, ChevronsUpDown, Columns2, PanelRightClose, PanelRightOpen, RefreshCw, RotateCcw, RotateCw, Rows3, Undo2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ReviewThread, WorkspaceChangeGroupSummary } from '../types'
import type { ReviewDiffLayout } from './monaco/review-editor-adapter'
import { ReviewUtilityTab } from './ReviewUtilityTab'

interface ReviewToolbarProps {
  thread: ReviewThread
  selectedGroup: WorkspaceChangeGroupSummary | null
  selectedScopeID: string
  fileCount: number
  layout: ReviewDiffLayout
  busy: boolean
  refreshing: boolean
  allDiffsCollapsed: boolean
  navigatorVisible: boolean
  agentVisible: boolean
  onLayoutChange: (layout: ReviewDiffLayout) => void
  onScopeChange: (scopeID: string) => void
  onReview: (decision: 'accept' | 'reject') => void
  onHistory: (action: 'undo' | 'redo') => void
  onRefresh: () => void
  onToggleAllDiffs: () => void
  onToggleNavigator: () => void
  onToggleAgent?: () => void
  onClose: () => void
}

export function ReviewToolbar({ thread, selectedGroup, selectedScopeID, fileCount, layout, busy, refreshing, allDiffsCollapsed, navigatorVisible, agentVisible, onLayoutChange, onScopeChange, onReview, onHistory, onRefresh, onToggleAllDiffs, onToggleNavigator, onToggleAgent, onClose }: ReviewToolbarProps) {
  const { t } = useTranslation()
  const canReview = selectedGroup?.apply_state === 'applied' && (selectedGroup.pending_edit_count ?? 0) > 0
  const CollapseIcon = allDiffsCollapsed ? ChevronsUpDown : ChevronsDownUp
  const NavigatorIcon = navigatorVisible ? PanelRightClose : PanelRightOpen
  const selectedGroupIndex = thread.groups.findIndex((group) => group.id === selectedScopeID)
  const selectedScopeLabel = selectedScopeID === 'thread'
    ? t('changes.scope.cumulative')
    : selectedGroupIndex >= 0
      ? groupLabel(t, thread.groups[selectedGroupIndex], selectedGroupIndex)
      : t('changes.scope.cumulative')
  const historicalGroups = thread.groups.map((group, index) => ({ group, index })).reverse()
  return (
    <header className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs text-[var(--nova-text-muted)]">
      <ReviewUtilityTab onClose={onClose} />
      <div className="flex min-h-10 flex-wrap items-center gap-2 px-3 py-1.5">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px]">
            {t('changes.filesChanged', { count: fileCount })}
          </span>
          {thread.groups.length > 0 && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button type="button" size="xs" variant="outline" disabled={busy} className="min-w-0 max-w-56 justify-between font-normal">
                  <span className="truncate">{selectedScopeLabel}</span>
                  <ChevronDown data-icon="inline-end" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="min-w-56">
                <DropdownMenuGroup>
                  <DropdownMenuCheckboxItem checked={selectedScopeID === 'thread'} onSelect={() => onScopeChange('thread')}>
                    {t('changes.scope.cumulative')}
                  </DropdownMenuCheckboxItem>
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  {historicalGroups.map(({ group, index }) => (
                    <DropdownMenuCheckboxItem key={group.id} checked={selectedScopeID === group.id} onSelect={() => onScopeChange(group.id)}>
                      <span className="min-w-0 flex-1 truncate">{groupLabel(t, group, index)}</span>
                    </DropdownMenuCheckboxItem>
                  ))}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
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

        <Button
          type="button"
          size="icon-xs"
          variant="ghost"
          disabled={fileCount === 0}
          onClick={onToggleAllDiffs}
          aria-label={t(allDiffsCollapsed ? 'changes.expandAllDiffs' : 'changes.collapseAllDiffs')}
          title={t(allDiffsCollapsed ? 'changes.expandAllDiffs' : 'changes.collapseAllDiffs')}
        >
          <CollapseIcon />
        </Button>
        <Button
          type="button"
          size="icon-xs"
          variant="ghost"
          onClick={onToggleNavigator}
          aria-pressed={navigatorVisible}
          aria-label={t(navigatorVisible ? 'changes.hideFileNavigator' : 'changes.showFileNavigator')}
          title={t(navigatorVisible ? 'changes.hideFileNavigator' : 'changes.showFileNavigator')}
        >
          <NavigatorIcon />
        </Button>
        <Button type="button" size="icon-xs" variant="ghost" disabled={busy || refreshing} onClick={onRefresh} aria-label={t('changes.refresh')}>
          <RefreshCw className={refreshing ? 'animate-spin' : ''} />
        </Button>
        {onToggleAgent && (
          <Button
            type="button"
            size="icon-xs"
            variant="ghost"
            onClick={onToggleAgent}
            aria-pressed={agentVisible}
            aria-label={t(agentVisible ? 'router.hideAgent' : 'router.showAgent')}
            title={t(agentVisible ? 'router.hideAgent' : 'router.showAgent')}
            className={agentVisible ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : undefined}
          >
            <Bot />
          </Button>
        )}
      </div>

      {selectedGroup && (
        <div className="flex min-h-9 flex-wrap items-center justify-end gap-1.5 border-t border-[var(--nova-border-soft)] px-3 py-1">
          <span className="mr-auto text-[10px] text-[var(--nova-text-faint)]">
            {t('changes.scope.group', { count: selectedGroup.paths?.length ?? 0 })} · {t(`changes.applyState.${selectedGroup.apply_state}`, { defaultValue: selectedGroup.apply_state })}
          </span>
          <Button type="button" size="xs" variant="ghost" disabled={busy || selectedGroup.can_undo !== true} onClick={() => onHistory('undo')}>
            <Undo2 />{t('changes.undo')}
          </Button>
          <Button type="button" size="xs" variant="ghost" disabled={busy || selectedGroup.can_redo !== true} onClick={() => onHistory('redo')}>
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

function groupLabel(t: ReturnType<typeof useTranslation>['t'], group: WorkspaceChangeGroupSummary, index: number): string {
  return `${t('changes.editNumber', { number: index + 1 })} · ${t(`changes.status.${group.review_status}`, { defaultValue: group.review_status })}`
}
