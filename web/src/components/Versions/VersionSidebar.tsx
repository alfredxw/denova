import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Clock3, FileClock, RefreshCw, Search, ShieldCheck } from 'lucide-react'
import type { VersionEntry, VersionStatus } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { formatTime, sourceText, workspaceName } from './version-panel-utils'

export const CURRENT_WORKSPACE_SELECTION = '__workspace__'

interface VersionSidebarProps {
  workspace: string
  status: VersionStatus | null
  versions: VersionEntry[]
  selectedKey: string
  search: string
  error: string
  historyLoading: boolean
  statusLoading: boolean
  operationLoading: boolean
  refreshing: boolean
  canLoadMore: boolean
  onSearchChange: (value: string) => void
  onSelectCurrent: () => void
  onSelectVersion: (version: VersionEntry) => void
  onRefresh: () => void
  onCreate: () => void
  onLoadMore: () => void
}

/** Compact local navigation for the workspace baseline and version history. */
export function VersionSidebar({
  workspace,
  status,
  versions,
  selectedKey,
  search,
  error,
  historyLoading,
  statusLoading,
  operationLoading,
  refreshing,
  canLoadMore,
  onSearchChange,
  onSelectCurrent,
  onSelectVersion,
  onRefresh,
  onCreate,
  onLoadMore,
}: VersionSidebarProps) {
  const { t } = useTranslation()
  const filteredGroups = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase()
    const filtered = keyword
      ? versions.filter(version => `${version.message} ${version.id} ${sourceText(version.source, t)}`.toLocaleLowerCase().includes(keyword))
      : versions
    return filtered.reduce<Array<{ date: string; versions: VersionEntry[] }>>((groups, version) => {
      const date = version.created_at.slice(0, 10) || t('versions.unknownDate')
      const group = groups.at(-1)
      if (group?.date === date) group.versions.push(version)
      else groups.push({ date, versions: [version] })
      return groups
    }, [])
  }, [search, t, versions])

  return (
    <aside className="flex h-full min-h-0 w-full flex-col bg-background text-foreground">
      <div className="shrink-0 space-y-3 border-b px-3 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-semibold">{t('versions.title')}</div>
            <div className="truncate text-[11px] text-muted-foreground">{workspaceName(workspace)}</div>
          </div>
          <Button type="button" variant="ghost" size="icon-sm" aria-label={t('versions.refresh')} onClick={onRefresh} disabled={operationLoading || refreshing}>
            <RefreshCw className={refreshing ? 'animate-spin' : ''} />
          </Button>
          <Button type="button" size="sm" onClick={onCreate} disabled={operationLoading || !status}>
            {operationLoading ? <Spinner /> : <ShieldCheck />}
            {t('versions.saveCurrent')}
          </Button>
        </div>

        <button
          type="button"
          className={`flex w-full min-w-0 items-center gap-2 rounded-lg border px-2.5 py-2 text-left transition-colors hover:bg-muted ${selectedKey === CURRENT_WORKSPACE_SELECTION ? 'border-foreground/15 bg-muted' : 'border-border bg-background'}`}
          onClick={onSelectCurrent}
        >
          <span className={`size-2 shrink-0 rounded-full ${status?.clean ? 'bg-emerald-500' : 'bg-amber-500'}`} />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-xs font-medium">{t('versions.currentChanges')}</span>
            <span className="block truncate text-[10px] text-muted-foreground">
              {statusLoading ? t('versions.loadingStatus') : status?.has_versions ? t('versions.currentComparedWithLatest') : t('versions.firstVersionHint')}
            </span>
          </span>
          <Badge variant="secondary" className="h-5 px-1.5 text-[10px] tabular-nums">{status?.changes.length ?? 0}</Badge>
        </button>

        <InputGroup>
          <InputGroupAddon><Search /></InputGroupAddon>
          <InputGroupInput value={search} onChange={event => onSearchChange(event.target.value)} placeholder={t('versions.historySearchPlaceholder')} aria-label={t('versions.historySearchPlaceholder')} />
        </InputGroup>
        {error && <InlineErrorNotice message={error} />}
      </div>

      <section className="flex min-h-[180px] flex-1 flex-col">
        <div className="flex h-9 shrink-0 items-center gap-2 px-3 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
          <Clock3 className="size-3.5" />
          <span>{t('versions.history')}</span>
          <span className="ml-auto tabular-nums">{versions.length}</span>
        </div>
        <ScrollArea className="min-h-0 flex-1 px-2">
          {historyLoading && versions.length === 0 ? (
            <div className="space-y-2 px-1 py-2">
              {Array.from({ length: 5 }, (_, index) => <Skeleton key={index} className="h-12 w-full" />)}
            </div>
          ) : filteredGroups.length === 0 ? (
            <Empty className="min-h-40 border-0 px-4 py-6">
              <EmptyHeader>
                <EmptyMedia variant="icon"><FileClock /></EmptyMedia>
                <EmptyTitle>{search ? t('versions.noSearchResults') : t('versions.historyEmpty')}</EmptyTitle>
                <EmptyDescription>{search ? t('versions.tryAnotherSearch') : t('versions.firstVersionHint')}</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className="space-y-3 pb-2">
              {filteredGroups.map(group => (
                <div key={group.date}>
                  <div className="sticky top-0 z-10 bg-background/95 px-1 py-1 text-[10px] font-medium text-muted-foreground backdrop-blur">{group.date}</div>
                  <div className="space-y-0.5">
                    {group.versions.map(version => {
                      const selected = selectedKey === version.id
                      return (
                        <button
                          type="button"
                          key={version.id}
                          className={`group flex w-full min-w-0 items-start gap-2 rounded-lg border px-2 py-2 text-left transition-colors hover:bg-muted ${selected ? 'border-ring/50 bg-muted text-foreground' : 'border-transparent text-muted-foreground'}`}
                          onClick={() => onSelectVersion(version)}
                        >
                          <span className={`mt-1.5 size-1.5 shrink-0 rounded-full ${selected ? 'bg-foreground' : 'bg-border group-hover:bg-muted-foreground'}`} />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-xs font-medium text-foreground">{version.message || t('versions.emptyMessage')}</span>
                            <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[10px]">
                              <span className="truncate">{formatTime(version.created_at)}</span>
                              <span aria-hidden="true">·</span>
                              <span className="truncate">{sourceText(version.source, t)}</span>
                            </span>
                          </span>
                          <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">{version.changed_paths?.length ?? 0}</span>
                        </button>
                      )
                    })}
                  </div>
                </div>
              ))}
              {canLoadMore && !search && (
                <Button type="button" variant="ghost" size="sm" className="w-full text-muted-foreground" onClick={onLoadMore} disabled={historyLoading}>
                  {historyLoading && <Spinner />}
                  {t('versions.loadMoreHistory')}
                </Button>
              )}
            </div>
          )}
        </ScrollArea>
      </section>
    </aside>
  )
}
