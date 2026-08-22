import { useTranslation } from 'react-i18next'
import { Columns2, FileCode2, List, MoreHorizontal, PanelLeft, RotateCcw, Undo2 } from 'lucide-react'
import type { VersionDiff, VersionEntry } from '@/lib/api'
import { ChapterDiffView } from '@/features/chapters/components/chapter-diff-view'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { formatBytes, formatTime, sourceText } from './version-panel-utils'

export type VersionDiffMode = 'split' | 'unified'

interface VersionDiffWorkspaceProps {
  currentSelection: boolean
  targetVersion: VersionEntry | null
  summary: VersionDiff | null
  diff: VersionDiff | null
  selectedPath: string
  mode: VersionDiffMode
  isMobile: boolean
  loading: boolean
  restoring: boolean
  onModeChange: (mode: VersionDiffMode) => void
  onOpenSidebar: () => void
  onRestoreVersion: () => void
  onRestoreFile: () => void
}

/** Persistent read-only diff surface for either workspace changes or one historical commit. */
export function VersionDiffWorkspace({
  currentSelection,
  targetVersion,
  summary,
  diff,
  selectedPath,
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
  const title = currentSelection
    ? t('versions.currentChanges')
    : targetVersion?.message || t('versions.emptyMessage')
  const description = currentSelection
    ? t('versions.currentComparedWithLatest')
    : targetVersion
      ? `${formatTime(targetVersion.created_at)} · ${targetVersion.id.slice(0, 8)}`
      : t('versions.firstVersionHint')
  const originalLabel = currentSelection
    ? t('versions.diffSnapshot')
    : summary?.base_version?.message || t('versions.emptyBase')
  const modifiedLabel = currentSelection
    ? t('versions.diffWorkspace')
    : targetVersion?.message || t('versions.diffSelectedVersion')

  return (
    <main className="flex h-full min-h-0 min-w-0 flex-col bg-background text-foreground">
      <header className="flex min-h-16 shrink-0 items-center gap-3 border-b px-3 py-2 sm:px-4">
        {isMobile && (
          <Button type="button" variant="outline" size="icon-sm" onClick={onOpenSidebar} aria-label={t('versions.openHistorySidebar')}>
            <PanelLeft />
          </Button>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <h2 className="truncate text-sm font-semibold">{title}</h2>
            {targetVersion && <Badge variant="secondary" className="max-w-32 truncate text-[10px]">{sourceText(targetVersion.source, t)}</Badge>}
          </div>
          <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{description}</p>
        </div>
        {targetVersion && !currentSelection && (
          <Button type="button" variant="outline" size="sm" onClick={onRestoreVersion} disabled={restoring} className="hidden border-destructive/50 text-destructive hover:bg-destructive/10 hover:text-destructive sm:inline-flex">
            <RotateCcw />
            {t('versions.restoreVersion')}
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="icon-sm" aria-label={t('versions.moreActions')} disabled={!targetVersion}>
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuGroup>
              <DropdownMenuItem onSelect={onRestoreFile} disabled={!selectedPath || restoring}>
                <Undo2 />
                {t('versions.restoreFile')}
              </DropdownMenuItem>
              {!currentSelection && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onSelect={onRestoreVersion} disabled={restoring}>
                    <RotateCcw />
                    {t('versions.restoreVersion')}
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </header>

      <div className="flex h-10 shrink-0 min-w-0 items-center gap-2 border-b bg-muted/30 px-3 sm:px-4">
        <FileCode2 className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{selectedPath || t('versions.selectFile')}</span>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          spacing={0}
          value={isMobile ? 'unified' : mode}
          onValueChange={value => value && onModeChange(value as VersionDiffMode)}
          aria-label={t('versions.diffLayout')}
        >
          <ToggleGroupItem value="unified" aria-label={t('versions.diffModeUnified')} title={t('versions.diffModeUnified')}><List /></ToggleGroupItem>
          <ToggleGroupItem value="split" aria-label={t('versions.diffModeSplit')} title={t('versions.diffModeSplit')} disabled={isMobile}><Columns2 /></ToggleGroupItem>
        </ToggleGroup>
        <Badge variant="outline" className="hidden text-[10px] sm:inline-flex">{t('versions.diffReadOnly')}</Badge>
      </div>

      {selectedPath && (
        <div className="grid h-8 shrink-0 grid-cols-2 border-b bg-muted/20 text-[10px] text-muted-foreground max-md:hidden">
          <div className="flex min-w-0 items-center gap-2 border-r px-4">
            <span className="size-1.5 shrink-0 rounded-full bg-amber-500" />
            <span className="truncate">{originalLabel}</span>
          </div>
          <div className="flex min-w-0 items-center gap-2 px-4">
            <span className="size-1.5 shrink-0 rounded-full bg-emerald-500" />
            <span className="truncate">{modifiedLabel}</span>
          </div>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-hidden">
        {!targetVersion ? (
          <Empty className="h-full rounded-none border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon"><FileCode2 /></EmptyMedia>
              <EmptyTitle>{t('versions.historyEmpty')}</EmptyTitle>
              <EmptyDescription>{t('versions.firstVersionHint')}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : loading ? (
          <div className="h-full space-y-3 p-5">
            <Skeleton className="h-5 w-2/5" />
            <Skeleton className="h-[calc(100%-2rem)] w-full" />
          </div>
        ) : !selectedPath ? (
          <Empty className="h-full rounded-none border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon"><FileCode2 /></EmptyMedia>
              <EmptyTitle>{t('versions.noComparableFiles')}</EmptyTitle>
              <EmptyDescription>{t('versions.selectAnotherVersion')}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : diff?.binary ? (
          <Empty className="h-full rounded-none border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon"><FileCode2 /></EmptyMedia>
              <EmptyTitle>{t('versions.binaryFileTitle')}</EmptyTitle>
              <EmptyDescription>{t('versions.fileBinary')}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : diff?.text ? (
          <ChapterDiffView
            original={diff.original || ''}
            modified={diff.modified || ''}
            language={languageForPath(selectedPath)}
            sideBySide={mode === 'split' && !isMobile}
          />
        ) : (
          <Empty className="h-full rounded-none border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon"><FileCode2 /></EmptyMedia>
              <EmptyTitle>{t('versions.diffUnavailable')}</EmptyTitle>
              <EmptyDescription>{t('versions.diffReadFailed')}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </div>

      {targetVersion && (
        <footer className="flex h-8 shrink-0 items-center justify-between gap-3 border-t bg-muted/20 px-3 text-[10px] text-muted-foreground sm:px-4">
          <span className="truncate">{t('versions.filesBytes', { files: targetVersion.file_count, bytes: formatBytes(targetVersion.total_bytes) })}</span>
          <span className="shrink-0">{t('versions.diffReadOnly')}</span>
        </footer>
      )}
    </main>
  )
}

function languageForPath(path: string) {
  const extension = path.split('.').pop()?.toLowerCase()
  if (extension === 'json') return 'json'
  if (extension === 'yaml' || extension === 'yml') return 'yaml'
  if (extension === 'ts' || extension === 'tsx') return 'typescript'
  if (extension === 'js' || extension === 'jsx') return 'javascript'
  return 'markdown'
}
