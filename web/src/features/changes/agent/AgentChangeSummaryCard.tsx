import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ChevronDown, ChevronUp, FileDiff, Loader2, RefreshCw, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { getWorkspaceChangeGroup, undoWorkspaceChangeGroup } from '../api'
import { invalidateWorkspaceChangeQueries, workspaceChangeKeys } from '../use-change-review'
import type { WorkspaceChangeGroup, WorkspaceChangeGroupSummary, WorkspaceChangeSet } from '../types'
import { logWorkspaceChangeError, workspaceChangeErrorMessage } from '../errors'
import { lineDiffStats } from '../diff-stats'

interface AgentChangeSummaryCardProps {
  workspace: string
  summary: WorkspaceChangeGroupSummary
  disabled?: boolean
  onReview: (reviewThreadID: string, groupID: string) => void
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
}

interface FileChangeSummary {
  path: string
  additions: number
  deletions: number
}

/** Codex-style, durable summary for one Agent run. */
export function AgentChangeSummaryCard({ workspace, summary, disabled = false, onReview, onWorkspaceChanged }: AgentChangeSummaryCardProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [expanded, setExpanded] = useState(false)
  const groupQuery = useQuery({
    queryKey: workspaceChangeKeys.detail(workspace, summary.id),
    queryFn: () => getWorkspaceChangeGroup(workspace, summary.id),
    enabled: Boolean(workspace && summary.id),
    staleTime: 10_000,
  })
  const files = useMemo(() => summarizeGroupFiles(groupQuery.data), [groupQuery.data])
  const totals = useMemo(() => files.reduce(
    (result, file) => ({ additions: result.additions + file.additions, deletions: result.deletions + file.deletions }),
    { additions: 0, deletions: 0 },
  ), [files])
  const visibleFiles = expanded ? files : files.slice(0, 3)
  const reviewThreadID = summary.review_thread_id || groupQuery.data?.review_thread_id || summary.id
  useEffect(() => {
    if (groupQuery.isError) logWorkspaceChangeError('Agent 变更摘要加载失败', groupQuery.error)
  }, [groupQuery.error, groupQuery.isError])
  const undoMutation = useMutation({
    mutationFn: () => undoWorkspaceChangeGroup(workspace, summary.id),
    onSuccess: async (result) => {
      await invalidateWorkspaceChangeQueries(queryClient, workspace)
      if (result.workspace !== workspace) return
      const paths = Array.from(new Set([
        ...(result.affected_paths ?? []),
        ...(result.paths ?? []),
        ...(result.path ? [result.path] : []),
      ]))
      if (paths.length) await onWorkspaceChanged?.(paths)
      toast.success(t('changes.undoSuccess'))
    },
    onError: (error) => {
      logWorkspaceChangeError('Agent 变更摘要撤销失败', error)
      toast.error(t('changes.operationFailed'), { description: workspaceChangeErrorMessage(t, error) })
    },
  })

  const fileCount = files.length || summary.paths?.length || summary.change_set_count || 0

  return (
    <section
      data-change-summary-card={summary.id}
      className="overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-xs text-[var(--nova-text)] shadow-[0_12px_34px_rgba(0,0,0,0.18)]"
      aria-label={t('changes.summary.title', { count: fileCount })}
    >
      <header className="flex min-h-16 items-center gap-3 px-3 py-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--nova-bg)] text-[var(--nova-text-muted)]">
          {groupQuery.isLoading
            ? <Loader2 className="h-4 w-4 animate-spin" />
            : groupQuery.isError
              ? <AlertTriangle className="h-4 w-4 text-[var(--nova-warning)]" />
              : <FileDiff className="h-4 w-4" />}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold">{t('changes.summary.title', { count: fileCount })}</div>
          {groupQuery.isError ? (
            <div className="mt-0.5 text-[11px] text-[var(--nova-warning)]">{t('changes.loadFailed')}</div>
          ) : (
            <div className="mt-0.5 flex gap-2 font-mono text-xs">
              <span className="text-[var(--nova-success)]">+{totals.additions}</span>
              <span className="text-[var(--nova-danger)]">−{totals.deletions}</span>
            </div>
          )}
        </div>
        {groupQuery.isError && (
          <Button type="button" size="icon-xs" variant="ghost" onClick={() => void groupQuery.refetch()} aria-label={t('changes.retry')}>
            <RefreshCw />
          </Button>
        )}
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={!canUndoAgentChange(summary, disabled) || undoMutation.isPending}
          onClick={() => undoMutation.mutate()}
          className="shrink-0"
        >
          {undoMutation.isPending ? <Loader2 className="animate-spin" /> : <RotateCcw />}
          {t('changes.undo')}
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={() => onReview(reviewThreadID, summary.id)} className="shrink-0">
          {t('changes.review')}
        </Button>
      </header>

      {visibleFiles.length > 0 && (
        <div className="border-t border-[var(--nova-border)]">
          {visibleFiles.map((file) => (
            <button
              key={file.path}
              type="button"
              onClick={() => onReview(reviewThreadID, summary.id)}
              className="flex w-full items-center gap-3 border-b border-[var(--nova-border-soft)] px-3 py-2 text-left last:border-b-0 hover:bg-[var(--nova-hover)]"
            >
              <span className="min-w-0 flex-1 truncate">{file.path}</span>
              <span className="shrink-0 font-mono text-[var(--nova-success)]">+{file.additions}</span>
              <span className="shrink-0 font-mono text-[var(--nova-danger)]">−{file.deletions}</span>
            </button>
          ))}
          {files.length > 3 && (
            <button
              type="button"
              onClick={() => setExpanded((value) => !value)}
              className="flex w-full items-center gap-2 border-t border-[var(--nova-border)] px-3 py-2 text-left text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            >
              {expanded ? t('changes.summary.showLess') : t('changes.summary.showMore', { count: files.length - 3 })}
              {expanded ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
            </button>
          )}
        </div>
      )}
    </section>
  )
}

export function canUndoAgentChange(summary: WorkspaceChangeGroupSummary, disabled: boolean): boolean {
  return !disabled && summary.can_undo === true
}

export function summarizeGroupFiles(group?: WorkspaceChangeGroup | null): FileChangeSummary[] {
  if (!group) return []
  const byPath = new Map<string, WorkspaceChangeSet[]>()
  for (const changeSet of group.change_sets) {
    if (changeSet.origin === 'review' || changeSet.origin === 'undo' || changeSet.origin === 'redo') continue
    const list = byPath.get(changeSet.path) ?? []
    list.push(changeSet)
    byPath.set(changeSet.path, list)
  }
  return Array.from(byPath.entries())
    .map(([path, changeSets]) => {
      changeSets.sort((left, right) => (left.sequence ?? 0) - (right.sequence ?? 0))
      const before = changeSets[0]?.before_content ?? ''
      const after = changeSets[changeSets.length - 1]?.after_content ?? ''
      return { path, ...lineDiffStats(before, after) }
    })
    .sort((left, right) => left.path.localeCompare(right.path))
}
