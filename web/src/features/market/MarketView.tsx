import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Download,
  LayoutGrid,
  List,
  Package,
  RefreshCw,
  Search,
  Upload,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { ResourceWorkspace } from '@/components/layout/resource-workspace'
import { SidebarVisibilityToggle } from '@/components/layout/sidebar-visibility-toggle'
import {
  MarketSidebar,
  matchesCategory,
  type MarketSection,
  type MarketCategory,
} from './MarketSidebar'
import { MarketEntryDetail } from './MarketEntryDetail'
import { MarketEntryCard } from './MarketEntryCard'
import { useWorkspaceStore } from '@/stores/workspace-store'
import { cn } from '@/lib/utils'
import { getBooks } from '@/lib/api'
import { ImportDialog, type ImportDialogProps } from './ImportDialog'
import { ExportDialog } from './ExportDialog'
import { AcquiredResources } from './AcquiredResources'
import {
  exchange,
  getCatalog,
  localized,
  type Catalog,
  type Installation,
  type MarketEntry,
} from './api'

export function MarketView({
  visible,
  onSwitchProject,
}: {
  visible: boolean
  onSwitchProject: (path: string) => Promise<boolean>
}) {
  const { t, i18n } = useTranslation()
  const requestedInstallation = useWorkspaceStore(
    (state) => state.marketInstallationID,
  )
  const [tab, setTab] = useState<MarketSection>('discover')
  const [sidebarVisible, setSidebarVisible] = useState(true)
  const [catalog, setCatalog] = useState<Catalog>()
  const [installations, setInstallations] = useState<Installation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState<MarketCategory>('all')
  const [tag, setTag] = useState('all')
  const [sort, setSort] = useState('featured')
  const [view, setView] = useState('grid')
  const [page, setPage] = useState(1)
  const [detail, setDetail] = useState<MarketEntry>()
  const [importing, setImporting] =
    useState<Omit<ImportDialogProps, 'onClose' | 'onInstalled'>>()
  const [exporting, setExporting] = useState<{ installation?: Installation }>()
  useEffect(() => {
    if (visible && requestedInstallation) {
      setTab('acquired')
      setCategory('all')
      setDetail(undefined)
    }
  }, [visible, requestedInstallation])
  const refreshInstalled = useCallback(async () => {
    setInstallations(await exchange<Installation[]>('/installations'))
  }, [])
  const refresh = useCallback(
    async (force = false) => {
      setLoading(true)
      setError('')
      const results = await Promise.allSettled([
        getCatalog(force),
        refreshInstalled(),
      ])
      if (results[0].status === 'fulfilled') setCatalog(results[0].value)
      else setError('market.errors.catalogUnavailable')
      if (results[1].status === 'rejected')
        setError('market.errors.operationFailed')
      setLoading(false)
    },
    [refreshInstalled],
  )
  useEffect(() => {
    if (visible) void refresh()
  }, [visible, refresh])
  const tags = [
    ...new Set(catalog?.entries.flatMap((entry) => entry.tags) || []),
  ].sort()
  const filtered = useMemo(
    () =>
      (catalog?.entries || [])
        .filter(
          (entry) =>
            matchesCategory(entry, category) &&
            (tag === 'all' || entry.tags.includes(tag)) &&
            `${localized(entry.name, i18n.language)} ${localized(entry.description, i18n.language)} ${entry.author}`
              .toLocaleLowerCase()
              .includes(query.trim().toLocaleLowerCase()),
        )
        .sort((a, b) =>
          sort === 'featured' && a.featured !== b.featured
            ? Number(!!b.featured) - Number(!!a.featured)
            : b.updated_at.localeCompare(a.updated_at) ||
              a.id.localeCompare(b.id),
        ),
    [catalog, category, tag, query, sort, i18n.language],
  )
  useEffect(() => {
    setPage(1)
  }, [query, category, tag, sort])
  const openDomain = async (item: Installation) => {
    if (item.project_id) {
      const book = (await getBooks()).find(
        (book) => book.project_id === item.project_id,
      )
      if (!book) throw new Error(t('market.states.unavailable'))
      if (!(await onSwitchProject(book.path))) return
    }
    const kinds = item.bindings.map((b) => b.local.kind)
    useWorkspaceStore
      .getState()
      .setMode(
        kinds.some((k) => k.startsWith('extension.'))
          ? 'extensions'
          : kinds.includes('skill')
            ? 'skills'
            : kinds.some(
                  (kind) => kind === 'lore.item' || kind === 'game.opening',
                )
              ? 'lore'
              : kinds.includes('project.cover')
                ? 'books'
                : 'presets',
      )
  }
  const updates = installations.filter(
    (item) =>
      item.tracking === 'tracked' && item.remote_state === 'update_available',
  )
  const heading =
    tab === 'discover' && category !== 'all'
      ? t(`market.category.${category}`)
      : t(`market.${tab}`)
  return (
    <div className="h-full min-h-0" data-testid="resource-market">
      <FeaturePageShell
        icon={Package}
        title={t('market.title')}
        mobileHeader={detail ? 'hidden' : 'toolbar'}
        leadingContent={!detail &&
          <SidebarVisibilityToggle
            visible={sidebarVisible}
            onToggle={() => setSidebarVisible((value) => !value)}
          />
        }
        actions={!detail &&
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setExporting({})}
            >
              <Upload data-icon="inline-start" />
              {t('market.export.title')}
            </Button>
            <Button size="sm" onClick={() => setImporting({})}>
              <Download data-icon="inline-start" />
              {t('market.import.title')}
            </Button>
          </>
        }
      >
        <ResourceWorkspace
          title={t('market.title')}
          left={detail ? undefined : {
            id: 'market-navigation',
            title: t('market.navigation'),
            side: 'left',
            icon: <Package />,
            content: (
              <MarketSidebar
                section={tab}
                category={category}
                entries={catalog?.entries || []}
                acquiredCount={
                  installations.filter((item) => item.tracking === 'tracked')
                    .length
                }
                updateCount={updates.length}
                onSelect={(section, selected) => {
                  setTab(section)
                  setCategory(selected)
                  setTag('all')
                  setDetail(undefined)
                }}
              />
            ),
            desktopVisible: sidebarVisible,
            desktopClassName: 'min-h-0 border-r',
            mobileClassName: 'w-[min(90vw,320px)]',
          }}
          leftResize={{
            layoutKey: 'nova-market-navigation-layout',
            label: t('layout.resize.sidebar'),
            defaultSize: '216px',
            minSize: '184px',
            maxSize: '30%',
          }}
          className="flex-1"
          mainClassName="min-h-0 min-w-0"
        >
          <div hidden={!!detail} className="h-full min-h-0 overflow-auto">
            <div className="mx-auto flex max-w-[1600px] flex-col gap-4 p-4 lg:p-5">
              <div className="flex items-center gap-2 border-b pb-3">
                <h1 className="text-base font-semibold">{heading}</h1>
                <Badge variant="secondary" className="tabular-nums">
                  {t('market.results', {
                    count: tab === 'discover'
                      ? filtered.length
                      : tab === 'updates'
                        ? updates.length
                        : installations.filter(
                            (item) => item.tracking === 'tracked',
                          ).length,
                  })}
                </Badge>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="ml-auto"
                  disabled={loading}
                  aria-label={t('market.refresh')}
                  onClick={() => void refresh(true)}
                >
                  <RefreshCw className={loading ? 'animate-spin' : ''} />
                </Button>
              </div>
              {(error || catalog?.stale) && (
                <div
                  role="status"
                  className="rounded-lg border bg-muted px-4 py-3 text-sm"
                >
                  {error
                    ? t(error)
                    : t('market.stale', {
                        time: catalog?.fetched_at
                          ? new Date(catalog.fetched_at).toLocaleString(
                              i18n.language,
                            )
                          : '',
                      })}
                </div>
              )}
              {tab === 'discover' && (
                <>
                  <div className="flex flex-wrap items-center gap-2">
                    <InputGroup className="min-w-48 flex-1">
                      <InputGroupAddon>
                        <Search />
                      </InputGroupAddon>
                      <InputGroupInput
                        aria-label={t('market.search')}
                        placeholder={t('market.search')}
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                      />
                    </InputGroup>
                    <Select value={sort} onValueChange={setSort}>
                      <SelectTrigger aria-label={t('market.sort')}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="featured">
                            {t('market.featured')}
                          </SelectItem>
                          <SelectItem value="recent">
                            {t('market.recent')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <ToggleGroup
                      type="single"
                      variant="outline"
                      value={view}
                      onValueChange={(value) => {
                        if (value) setView(value)
                      }}
                    >
                      <ToggleGroupItem
                        value="grid"
                        aria-label={t('market.grid')}
                      >
                        <LayoutGrid />
                      </ToggleGroupItem>
                      <ToggleGroupItem
                        value="list"
                        aria-label={t('market.list')}
                      >
                        <List />
                      </ToggleGroupItem>
                    </ToggleGroup>
                  </div>
                  {tags.length > 0 && (
                    <div className="overflow-x-auto pb-1">
                      <ToggleGroup
                        type="single"
                        size="sm"
                        spacing={1}
                        value={tag}
                        aria-label={t('market.tags')}
                        className="w-max"
                        onValueChange={(value) => setTag(value || 'all')}
                      >
                        {['all', ...tags].map((value) => (
                          <ToggleGroupItem
                            key={value}
                            value={value}
                            className="rounded-full px-3 text-xs"
                          >
                            {value === 'all'
                              ? t('market.allTags')
                              : t(`market.tag.${value}`, {
                                  defaultValue: value,
                                })}
                          </ToggleGroupItem>
                        ))}
                      </ToggleGroup>
                    </div>
                  )}
                  {loading && !catalog ? (
                    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                      {[0, 1, 2].map((i) => (
                        <Skeleton key={i} className="h-48 rounded-xl" />
                      ))}
                    </div>
                  ) : filtered.length === 0 ? (
                    <Empty>
                      <EmptyHeader>
                        <EmptyMedia variant="icon">
                          <Package />
                        </EmptyMedia>
                        <EmptyTitle>{t('market.empty')}</EmptyTitle>
                        <EmptyDescription>
                          {t(
                            error
                              ? 'market.errors.catalogUnavailable'
                              : 'market.emptyHelp',
                          )}
                        </EmptyDescription>
                      </EmptyHeader>
                      <Button
                        variant="outline"
                        onClick={() => {
                          setQuery('')
                          setCategory('all')
                          setTag('all')
                        }}
                      >
                        {t('market.clear')}
                      </Button>
                    </Empty>
                  ) : (
                    <>
                      <div
                        className={cn(
                          'grid gap-3',
                          view === 'grid'
                            ? 'grid-cols-[repeat(auto-fill,minmax(min(100%,16rem),1fr))]'
                            : 'grid-cols-1',
                        )}
                      >
                        {filtered
                          .slice((page - 1) * 50, page * 50)
                          .map((entry) => (
                            <MarketEntryCard
                              key={entry.id}
                              entry={entry}
                              view={view}
                              onOpen={() => setDetail(entry)}
                              acquired={installations.some(
                                (item) =>
                                  item.tracking === 'tracked' &&
                                  item.source.kind === entry.source.kind &&
                                  item.source.url === entry.source.url &&
                                  (item.source.path || '') ===
                                    (entry.source.path || ''),
                              )}
                            />
                          ))}
                      </div>
                      {filtered.length > 50 && (
                        <div className="flex items-center justify-center gap-3">
                          <Button
                            variant="outline"
                            disabled={page === 1}
                            onClick={() => setPage(page - 1)}
                          >
                            {t('market.previous')}
                          </Button>
                          <span className="text-sm">
                            {page} / {Math.ceil(filtered.length / 50)}
                          </span>
                          <Button
                            variant="outline"
                            disabled={page * 50 >= filtered.length}
                            onClick={() => setPage(page + 1)}
                          >
                            {t('market.next')}
                          </Button>
                        </div>
                      )}
                    </>
                  )}
                </>
              )}
              {tab === 'updates' && updates.length === 0 ? (
                <Empty>
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <RefreshCw />
                    </EmptyMedia>
                    <EmptyTitle>{t('market.updatesEmpty')}</EmptyTitle>
                    <EmptyDescription>
                      {t('market.updatesHelp')}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : (
                tab !== 'discover' && (
                  <AcquiredResources
                    installations={tab === 'updates' ? updates : installations}
                    onChanged={refreshInstalled}
                    onImport={setImporting}
                    onExport={setExporting}
                    onManage={openDomain}
                    onDiscover={() => setTab('discover')}
                  />
                )
              )}
            </div>
          </div>
          {detail && (
            <MarketEntryDetail
              key={detail.id}
              detail={detail}
              acquired={installations.filter((item) => item.tracking === 'tracked' && item.source.url === detail.source.url && (item.source.path || '') === (detail.source.path || '')).length}
              onManage={() => { setDetail(undefined); setTab('acquired'); setCategory('all') }}
              onBack={() => setDetail(undefined)}
              onImport={(preview, selection) => setImporting({ preview, selection, previewOwner: 'caller' })}
            />
          )}
        </ResourceWorkspace>
      </FeaturePageShell>
      {importing && (
        <ImportDialog
          {...importing}
          onClose={() => setImporting(undefined)}
          onInstalled={() => {
            void refreshInstalled()
            setTab('acquired')
            setCategory('all')
            setDetail(undefined)
          }}
        />
      )}
      {exporting && (
        <ExportDialog
          {...exporting}
          projectID={exporting.installation?.project_id}
          onClose={() => setExporting(undefined)}
        />
      )}
    </div>
  )
}
