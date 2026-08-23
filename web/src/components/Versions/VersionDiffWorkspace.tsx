import { useMemo } from 'react'
import {
  ChevronsDownUp,
  ChevronsUpDown,
  Columns2,
  FileCode2,
  PanelLeft,
  PanelRightClose,
  PanelRightOpen,
  RotateCcw,
  Rows3,
  Undo2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { VersionDiff, VersionEntry, VersionFileDiff } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { CodeDiffSurface, type CodeDiffSurfaceFile } from '@/features/diff/CodeDiffSurface'
import { lineDiffStats } from '@/features/changes/diff-stats'
import type { DiffFileDocument, DiffFileNavigationItem, DiffLayout } from '@/features/diff/types'
import { useMultiFileDiffNavigation } from '@/features/diff/use-multi-file-diff-navigation'
import { formatBytes, formatTime, sourceText } from './version-panel-utils'

export type VersionDiffMode = DiffLayout

interface VersionDiffDocument extends DiffFileDocument {
  status: VersionFileDiff['status']
  text: boolean
}

interface VersionDiffWorkspaceProps {
  currentSelection: boolean
  targetVersion: VersionEntry | null
  summary: VersionDiff | null
  summaryRevision: number
  mode: VersionDiffMode
  isMobile: boolean
  loading: boolean
  restoring: boolean
  onModeChange: (mode: VersionDiffMode) => void
  onOpenSidebar: () => void
  onRestoreVersion: () => void
  onRestoreFile: (path: string) => void
}

/** Version-specific controls around the same continuous multi-file Diff used by Review. */
export function VersionDiffWorkspace({
  currentSelection,
  targetVersion,
  summary,
  summaryRevision,
  mode,
  isMobile,
  loading,
  restoring,
  onModeChange,
  onOpenSidebar,
  onRestoreVersion,
  onRestoreFile,
}: VersionDiffWorkspaceProps) {
  const { t } = useTranslation()
  const files = useMemo(() => versionDiffDocuments(summary, summaryRevision), [summary, summaryRevision])
  const surfaceFiles = useMemo<CodeDiffSurfaceFile[]>(() => files.map((file) => ({
    ...file,
    unavailableContent: file.binary
      ? t('versions.fileBinary')
      : !file.text
        ? t('versions.diffReadFailed')
        : undefined,
  })), [files, t])
  const paths = useMemo(() => files.map((file) => file.path), [files])
  const navigatorFiles = useMemo<DiffFileNavigationItem[]>(() => files.map((file) => ({
    path: file.path,
    kind: file.status === 'added' ? 'added' : file.status === 'deleted' ? 'deleted' : 'modified',
  })), [files])
  const navigation = useMultiFileDiffNavigation({
    identity: `${targetVersion?.id ?? 'empty'}:${summary?.comparison ?? 'none'}`,
    paths,
  })
  const diffTotals = useMemo(() => files.reduce((totals, file) => {
    const stats = lineDiffStats(file.before_content, file.after_content)
    totals.additions += stats.additions
    totals.deletions += stats.deletions
    return totals
  }, { additions: 0, deletions: 0 }), [files])
  const title = currentSelection
    ? t('versions.currentChanges')
    : targetVersion?.message || t('versions.emptyMessage')
  const description = currentSelection
    ? t('versions.currentComparedWithLatest')
    : targetVersion
      ? `${formatTime(targetVersion.created_at)} · ${targetVersion.id.slice(0, 8)}`
      : t('versions.firstVersionHint')
  const CollapseIcon = navigation.allDiffsCollapsed ? ChevronsUpDown : ChevronsDownUp
  const NavigatorIcon = navigation.navigatorVisible ? PanelRightClose : PanelRightOpen

  return (
    <main className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <header className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)]">
        <div className="flex min-h-12 flex-wrap items-center gap-1.5 px-2.5 py-1.5 sm:px-3">
          {isMobile && (
            <Button type="button" variant="outline" size="icon-sm" onClick={onOpenSidebar} aria-label={t('versions.openHistorySidebar')}>
              <PanelLeft />
            </Button>
          )}
          <div className="min-w-40 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              <h2 className="truncate text-sm font-semibold">{title}</h2>
              {targetVersion && <Badge variant="secondary" className="max-w-32 truncate text-[10px]">{sourceText(targetVersion.source, t)}</Badge>}
            </div>
            <p className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{description}</p>
          </div>
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--nova-text-muted)]">
            {t('changes.filesChanged', { count: files.length })}
          </span>
          <span className="font-mono text-[10px] tabular-nums text-[var(--nova-success)]">+{diffTotals.additions}</span>
          <span className="font-mono text-[10px] tabular-nums text-[var(--nova-danger)]">−{diffTotals.deletions}</span>
          <Badge variant="outline" className="hidden text-[10px] lg:inline-flex">{t('versions.diffReadOnly')}</Badge>

          <div role="group" aria-label={t('versions.diffLayout')} className="flex h-7 items-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-bg)] p-0.5">
            <button
              type="button"
              data-version-layout="unified"
              aria-pressed={mode === 'unified'}
              onClick={() => onModeChange('unified')}
              className={`flex h-6 items-center gap-1 rounded px-2 text-[10px] ${mode === 'unified' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]'}`}
            >
              <Rows3 className="size-3" />{t('versions.diffModeUnified')}
            </button>
            <button
              type="button"
              data-version-layout="split"
              aria-pressed={mode === 'split'}
              onClick={() => onModeChange('split')}
              className={`flex h-6 items-center gap-1 rounded px-2 text-[10px] ${mode === 'split' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]'}`}
            >
              <Columns2 className="size-3" />{t('versions.diffModeSplit')}
            </button>
          </div>
          <Button
            type="button"
            size="icon-xs"
            variant="ghost"
            disabled={files.length === 0}
            onClick={navigation.toggleAllDiffs}
            aria-label={t(navigation.allDiffsCollapsed ? 'changes.expandAllDiffs' : 'changes.collapseAllDiffs')}
          >
            <CollapseIcon />
          </Button>
          <Button
            type="button"
            size="icon-xs"
            variant="ghost"
            onClick={() => navigation.setNavigatorVisible((visible) => !visible)}
            aria-pressed={navigation.navigatorVisible}
            aria-label={t(navigation.navigatorVisible ? 'changes.hideFileNavigator' : 'changes.showFileNavigator')}
          >
            <NavigatorIcon />
          </Button>
          {targetVersion && !currentSelection && (
            <Button type="button" variant="outline" size="sm" onClick={onRestoreVersion} disabled={restoring} aria-label={t('versions.restoreVersion')} className="border-destructive/50 px-2 text-destructive hover:bg-destructive/10 hover:text-destructive sm:px-3">
              <RotateCcw />
              <span className="hidden sm:inline">{t('versions.restoreVersion')}</span>
            </Button>
          )}
        </div>
      </header>

      <CodeDiffSurface
        files={loading ? [] : surfaceFiles}
        navigatorFiles={loading ? [] : navigatorFiles}
        navigation={navigation}
        layout={mode}
        ariaLabel={t('versions.diffLayout')}
        empty={versionDiffEmptyState({ loading, targetVersion, t })}
        renderHeaderAction={(file) => targetVersion ? (
          <Button type="button" size="icon-xs" variant="ghost" disabled={restoring} onClick={() => onRestoreFile(file.path)} aria-label={t('versions.restoreFile')}>
            <Undo2 />
          </Button>
        ) : null}
      />

      {targetVersion && (
        <footer className="flex h-8 shrink-0 items-center justify-between gap-3 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[10px] text-[var(--nova-text-faint)]">
          <span className="truncate">{t('versions.filesBytes', { files: targetVersion.file_count, bytes: formatBytes(targetVersion.total_bytes) })}</span>
          <span className="shrink-0">{t('versions.diffReadOnly')}</span>
        </footer>
      )}
    </main>
  )
}

export function versionDiffDocuments(summary: VersionDiff | null, revisionToken: number): VersionDiffDocument[] {
  if (!summary?.files) return []
  const baseRevision = `${summary.base_version?.id ?? summary.version.id}:base:${revisionToken}`
  const revision = `${summary.comparison === 'workspace' ? 'workspace' : summary.version.id}:target:${revisionToken}`
  return summary.files.map((file) => ({
    path: file.path,
    status: file.status,
    before_content: file.original ?? '',
    after_content: file.modified ?? '',
    base_revision: baseRevision,
    revision,
    before_exists: file.missing_in_original !== true,
    after_exists: file.missing_in_modified !== true,
    binary: file.binary,
    text: file.text,
  }))
}

function versionDiffEmptyState({ loading, targetVersion, t }: {
  loading: boolean
  targetVersion: VersionEntry | null
  t: ReturnType<typeof useTranslation>['t']
}) {
  if (loading) {
    return (
      <div className="h-full space-y-3 p-5">
        <Skeleton className="h-5 w-2/5" />
        <Skeleton className="h-[calc(100%-2rem)] w-full" />
      </div>
    )
  }
  return (
    <Empty className="h-full rounded-none border-0">
      <EmptyHeader>
        <EmptyMedia variant="icon"><FileCode2 /></EmptyMedia>
        <EmptyTitle>{targetVersion ? t('versions.noComparableFiles') : t('versions.historyEmpty')}</EmptyTitle>
        <EmptyDescription>{targetVersion ? t('versions.selectAnotherVersion') : t('versions.firstVersionHint')}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
