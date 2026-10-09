import { useEffect, useMemo, useState } from 'react'
import { LayoutGrid, List, Package, RefreshCw, Search, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { cn } from '@/lib/utils'
import { requestSettingsSection } from '@/features/onboarding/events'
import { matchesCategory, type MarketCategory } from './MarketSidebar'
import { MarketEntryCard } from './MarketEntryCard'
import { localized, type Catalog, type Installation, type MarketEntry } from './api'

/** Discovery only reads the index; package bytes are fetched after the user chooses Get. */
export function ResourceDiscovery({ catalog, loading, error, category, setCategory, installations, onOpen, onRefresh, registryURL }: {
  catalog?: Catalog
  registryURL?: string
  loading: boolean
  error?: string
  category: MarketCategory
  setCategory: (category: MarketCategory) => void
  installations: Installation[]
  onOpen: (entry: MarketEntry) => void
  onRefresh: () => void
}) {
  const { t, i18n } = useTranslation()
  const registry = registryURL ? new URL(registryURL) : undefined
  const registryName = registry?.pathname.split('/').filter(Boolean).at(-2) || registry?.hostname
  const [query, setQuery] = useState('')
  const [tag, setTag] = useState('all')
  const [sort, setSort] = useState('featured')
  const [view, setView] = useState('grid')
  const [page, setPage] = useState(1)
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
  return <div className="mx-auto flex max-w-[1600px] flex-col gap-4 p-4 lg:p-5">
    <div className="flex flex-wrap items-center gap-2 border-b pb-3">
      <h1 className="text-base font-semibold">{t(category === 'all' ? 'market.discover' : `market.category.${category}`)}</h1>
      <Badge variant="secondary">{t('market.results', { count: filtered.length })}</Badge>
      {registryName && <Button variant="ghost" size="sm" className="min-w-0 max-w-full shrink" title={t('market.registrySettings')} onClick={() => requestSettingsSection('market')}>
        <span className="truncate">{registryName}</span><Settings2 data-icon="inline-end" />
      </Button>}
      <Button variant="ghost" size="icon-sm" className="ml-auto" disabled={loading} aria-label={t('market.refresh')} onClick={onRefresh}>
        <RefreshCw className={loading ? 'animate-spin' : ''} />
      </Button>
    </div>
    {(error || catalog?.stale) && <p role="status" className="text-sm text-muted-foreground">
      {error ? t(error) : t('market.stale', { time: catalog?.fetched_at ? new Date(catalog.fetched_at).toLocaleString(i18n.language) : '' })}
    </p>}
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
                              onOpen={() => onOpen(entry)}
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
  </div>
}
