import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { BarChart3, RefreshCw } from 'lucide-react'
import { ResourceWorkspace } from '@/components/layout/resource-workspace'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle, EmptyDescription } from '@/components/ui/empty'
import { LoadingState } from '@/components/common/LoadingState'
import { getStatistics } from './api'
import { WritingStatistics } from './WritingStatistics'
import { ModelStatistics } from './ModelStatistics'
import { GameStatistics } from './GameStatistics'

// Intent: creators review saved output and model consumption without leaving
// their active work. Hierarchy: one dominant trend, then quiet distributions.
// Palette/surfaces: existing neutral workbench tokens; red/green encode changes.
// Depth: existing Card borders. Type: existing Inter, 14px labels and 24px values.
// Spacing: 4px rhythm, 16px control groups, 24px between chart panels.
export function StatisticsPage({ visible }: { visible: boolean }) {
  const { t } = useTranslation()
  const [project, setProject] = useState('all')
  const [days, setDays] = useState('7')
  const [tab, setTab] = useState('writing')
  const query = useQuery({
    queryKey: ['statistics', project, days],
    queryFn: ({ signal }) => getStatistics(project === 'all' ? '' : project, Number(days), signal),
    enabled: visible,
    refetchInterval: visible ? 30_000 : false,
    placeholderData: keepPreviousData,
  })

  // Retained routes keep filter state, but hidden responsive charts have no
  // measurable viewport and must release their ResizeObservers.
  if (!visible) return null

  return (
    <ResourceWorkspace title={t('statistics.title')} className="h-full min-h-0" mainClassName="min-h-0 min-w-0">
      <div className="h-full overflow-auto" data-testid="statistics-page">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 p-4 sm:p-6">
          <header className="flex flex-wrap items-center justify-between gap-3">
            <h1 className="hidden text-xl font-semibold tracking-tight sm:block">{t('statistics.title')}</h1>
            <div className="flex flex-wrap items-center gap-2">
              <Select value={project} onValueChange={setProject}>
                <SelectTrigger aria-label={t('statistics.project')} className="w-40 max-w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent><SelectGroup>
                  <SelectItem value="all">{t('statistics.allProjects')}</SelectItem>
                  {query.data?.projects.map(item => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}
                </SelectGroup></SelectContent>
              </Select>
              <ToggleGroup type="single" size="sm" variant="outline" spacing={0} value={days} onValueChange={value => { if (value) setDays(value) }} aria-label={t('statistics.period')}>
                {['1', '7', '30'].map(value => <ToggleGroupItem key={value} value={value}>{t(`statistics.period.${value}`)}</ToggleGroupItem>)}
              </ToggleGroup>
              <Button variant="ghost" size="icon-sm" onClick={() => void query.refetch()} disabled={query.isFetching} aria-label={t('statistics.refresh')}>
                <RefreshCw />
              </Button>
            </div>
          </header>
          {query.isPending ? <LoadingState label={t('statistics.loading')} variant="panel" /> : query.isError ? (
            <Empty><EmptyHeader><EmptyMedia variant="icon"><BarChart3 /></EmptyMedia><EmptyTitle>{t('statistics.loadFailed')}</EmptyTitle></EmptyHeader>
              <Button variant="outline" onClick={() => void query.refetch()}>{t('statistics.retry')}</Button>
            </Empty>
          ) : query.data ? (
            <Tabs value={tab} onValueChange={setTab} aria-busy={query.isFetching} data-pending={query.isPlaceholderData || undefined} className="data-[pending]:opacity-50">
              <TabsList variant="line">
                <TabsTrigger value="writing">{t('statistics.writing')}</TabsTrigger>
                <TabsTrigger value="game">{t('statistics.game')}</TabsTrigger>
                <TabsTrigger value="models">{t('statistics.models')}</TabsTrigger>
              </TabsList>
              <TabsContent value="writing" className="pt-4"><WritingStatistics data={query.data.writing} /></TabsContent>
              <TabsContent value="game" className="pt-4"><GameStatistics data={query.data.game} /></TabsContent>
              <TabsContent value="models" className="pt-4"><ModelStatistics data={query.data.usage} /></TabsContent>
            </Tabs>
          ) : <Empty><EmptyHeader><EmptyTitle>{t('statistics.empty')}</EmptyTitle><EmptyDescription>{t('statistics.newRecords')}</EmptyDescription></EmptyHeader></Empty>}
          {!!query.data?.issues && <p role="status" className="text-sm text-muted-foreground">{t('statistics.partial', { count: query.data.issues })}</p>}
        </div>
      </div>
    </ResourceWorkspace>
  )
}
