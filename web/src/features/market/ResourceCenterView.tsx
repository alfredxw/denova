import { useCallback, useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Gamepad2, MoreHorizontal, Package, Puzzle, RefreshCw, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { ResourceWorkspace } from '@/components/layout/resource-workspace'
import { SidebarVisibilityToggle } from '@/components/layout/sidebar-visibility-toggle'
import { CreateProjectDirectoryDialog } from '@/features/agent-chat/CreateProjectDirectoryDialog'
import { GLOBAL_SETTINGS_TARGET, settingsQueryOptions } from '@/features/settings/query'
import { InstalledExtensions } from '@/features/platform/InstalledExtensions'
import { management, type CatalogEntry, type PackageKind } from '@/features/platform/api'
import { openInstalledExtension } from '@/features/platform/extension-navigation'
import { MarketSidebar, type MarketCategory } from './MarketSidebar'
import { ResourceDiscovery } from './ResourceDiscovery'
import { MarketEntryDetail } from './MarketEntryDetail'
import { MarketInstallationUpdates } from './MarketInstallationUpdates'
import { AcquiredResources } from './AcquiredResources'
import { ImportDialog, type ImportDialogProps } from './ImportDialog'
import { ExportDialog } from './ExportDialog'
import { openResourceDestination, useResourceNavigation } from './resource-navigation'
import { discardPreview, exchange, getCatalog, type Installation, type MarketEntry, type Preview } from './api'

/** One acquisition workspace; installation ownership and extension runtime state stay in their domain stores. */
export function ResourceCenterView({ projectID, visible }: { projectID?: string; visible: boolean }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const destination = useResourceNavigation(state => state.destination)
  const section = destination.section
  const settings = useQuery({ ...settingsQueryOptions(GLOBAL_SETTINGS_TARGET), enabled: visible })
  const registryURL = settings.data?.effective.market?.registry_url
  const catalogKey = ['resource-market', registryURL]
  const catalog = useQuery({ queryKey: catalogKey, queryFn: () => getCatalog(), enabled: visible && settings.isSuccess, retry: false })
  const installed = useQuery({ queryKey: ['resources', 'installations'], queryFn: () => exchange<Installation[]>('/installations'), enabled: visible })
  const extensions = useQuery({ queryKey: ['platform', 'catalog'], queryFn: () => management<CatalogEntry[]>('/catalog'), enabled: visible })
  const installations = installed.data ?? []
  const updates = installations.filter(item => item.tracking === 'tracked' && item.remote_state === 'update_available')
  const [sidebarVisible, setSidebarVisible] = useState(true)
  const [category, setCategory] = useState<MarketCategory>('all')
  const [detail, setDetail] = useState<MarketEntry>()
  const [importing, setImporting] = useState<Omit<ImportDialogProps, 'onClose' | 'onInstalled'>>()
  const [exporting, setExporting] = useState<{ installation?: Installation }>()
  const [creationKind, setCreationKind] = useState<PackageKind>()
  const [checking, setChecking] = useState(false)
  const [dirtyExtensions, setDirtyExtensions] = useState<ReadonlySet<string>>(() => new Set())
  const blockedReason = dirtyExtensions.size ? t('market.unsavedExtensions') : undefined
  const updateDirty = useCallback((key: string, dirty: boolean) => setDirtyExtensions(previous => {
    if (previous.has(key) === dirty) return previous
    const next = new Set(previous)
    if (dirty) next.add(key)
    else next.delete(key)
    return next
  }), [])
  const refreshResources = useCallback(async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['resources', 'installations'] }),
      // Do not refetch settings before an uninstalled editor has been removed.
      client.invalidateQueries({ queryKey: ['platform'], predicate: query => query.queryKey[1] !== 'settings' }),
    ])
  }, [client])
  useEffect(() => { if (visible) void refreshResources() }, [visible, refreshResources])
  useEffect(() => { setDetail(undefined) }, [destination, registryURL])
  useEffect(() => {
    if (!dirtyExtensions.size) return
    const preventDraftLoss = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', preventDraftLoss)
    return () => window.removeEventListener('beforeunload', preventDraftLoss)
  }, [dirtyExtensions.size])
  const openImport = (props: Omit<ImportDialogProps, 'onClose' | 'onInstalled'>) => {
    if (blockedReason) { toast.info(blockedReason); return }
    setImporting(props)
  }
  const checkUpdate = async (installation: Installation) => {
    if (blockedReason) { toast.info(blockedReason); return }
    if (installation.source.kind === 'file') {
      setImporting({ installation })
      return
    }
    const preview = await exchange<Preview>(`/installations/${installation.installation_id}/check`, {})
    await refreshResources()
    setImporting({ preview, installation })
  }
  const checkAllUpdates = async () => {
    setChecking(true)
    try {
      const results = await Promise.allSettled(installations.filter(item => item.tracking === 'tracked' && item.source.kind !== 'file').map(async item => {
        const preview = await exchange<Preview>(`/installations/${item.installation_id}/check`, {})
        discardPreview(preview)
      }))
      await refreshResources()
      const failures = results.filter(result => result.status === 'rejected')
      if (failures.length) {
        console.error('[resources] checking installed sources failed', { failures })
        toast.error(t('market.checkFailedCount', { count: failures.length }))
      }
    } finally { setChecking(false) }
  }
  const matchingInstallations = detail ? installations.filter(item => item.tracking === 'tracked' && item.package.id === detail.id
    && item.source.kind === detail.source.kind && item.source.url === detail.source.url
    && (item.source.ref || '') === (detail.source.ref || '') && (item.source.path || '') === (detail.source.path || '')) : []
  return <div className="flex h-full min-h-0 flex-col" data-testid="resource-market">
    <FeaturePageShell icon={Package} title={t('market.title')} mobileHeader="toolbar"
      leadingContent={<SidebarVisibilityToggle visible={sidebarVisible} onToggle={() => setSidebarVisible(value => !value)} />}
      actions={<>
        <Button size="sm" onClick={() => openImport({})} disabled={!!blockedReason}><Download data-icon="inline-start" />{t('market.import.title')}</Button>
        <Button size="sm" variant="outline" onClick={() => setExporting({})}><Upload data-icon="inline-start" />{t('market.export.title')}</Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" aria-label={t('market.development')}><MoreHorizontal /></Button></DropdownMenuTrigger>
          <DropdownMenuContent align="end"><DropdownMenuGroup>
            <DropdownMenuItem onSelect={() => setCreationKind('plugin')}><Puzzle />{t('platform.create.plugin')}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setCreationKind('game')}><Gamepad2 />{t('platform.create.game')}</DropdownMenuItem>
          </DropdownMenuGroup></DropdownMenuContent>
        </DropdownMenu>
      </>}
    >
      {blockedReason && <div role="status" className="flex flex-wrap items-center gap-2 border-b px-4 py-2 text-sm">
        <span>{blockedReason}</span><Button size="sm" variant="link" onClick={() => openResourceDestination({ section: 'extensions', extensionKey: [...dirtyExtensions][0] })}>{t('market.reviewSettings')}</Button>
      </div>}
      <ResourceWorkspace title={t('market.title')} className="flex-1" mainClassName="min-h-0 min-w-0"
        left={{ id: 'market-navigation', title: t('market.navigation'), side: 'left', icon: <Package />, desktopVisible: sidebarVisible, desktopClassName: 'min-h-0 border-r', mobileClassName: 'w-[min(90vw,320px)]', content:
          <MarketSidebar section={section} category={category} entries={catalog.data?.entries ?? []}
            acquiredCount={installations.length} extensionCount={extensions.data?.filter(item => !item.removed).length ?? 0} updateCount={updates.length}
            onSelect={(section, selected) => { setCategory(selected); openResourceDestination({ section }) }}
            officialRegistry={!!settings.data && registryURL === settings.data.default.market?.registry_url} /> }}
        leftResize={{ layoutKey: 'nova-market-navigation-layout', label: t('layout.resize.sidebar'), defaultSize: '216px', minSize: '184px', maxSize: '30%' }}
      >
        <div hidden={section !== 'discover' || !!detail} className="h-full overflow-auto">
          <ResourceDiscovery registryURL={registryURL} key={registryURL} category={category} setCategory={setCategory} catalog={catalog.data} loading={catalog.isFetching}
            error={catalog.error ? 'market.errors.catalogUnavailable' : undefined} installations={installations} onOpen={setDetail}
            onRefresh={() => { void client.fetchQuery({ queryKey: catalogKey, queryFn: () => getCatalog(true), staleTime: 0 }).catch(error => console.error('[resources] catalog refresh failed', error)) }} />
        </div>
        {section === 'discover' && detail && <MarketEntryDetail detail={detail} acquired={matchingInstallations.length}
          onBack={() => setDetail(undefined)} onImport={source => openImport({ source })}
          onManage={() => openResourceDestination({ section: 'acquired', installationID: matchingInstallations[0]?.installation_id })}
          updates={<MarketInstallationUpdates installations={matchingInstallations} projectID={projectID} onImport={openImport} onChanged={refreshResources} blockedReason={blockedReason} />} />}
        {(section === 'acquired' || section === 'updates') && <div className="h-full overflow-auto">
          <div className="mx-auto flex max-w-[1600px] flex-col gap-4 p-4 lg:p-5">
            <div className="flex flex-wrap items-center gap-2 border-b pb-3">
              <h1 className="text-base font-semibold">{t(`market.${section}`)}</h1>
              <Badge variant="secondary">{t('market.results', { count: section === 'updates' ? updates.length : installations.length })}</Badge>
              <Button variant="outline" size="sm" className="ml-auto" disabled={checking || installed.isPending || !!blockedReason} onClick={() => void checkAllUpdates()}>
                <RefreshCw className={checking ? 'animate-spin' : ''} data-icon="inline-start" />{t('market.checkUpdates')}
              </Button>
            </div>
            {installed.error && <p role="alert" className="text-sm text-destructive">{t('market.errors.operationFailed')}</p>}
            {installed.isPending ? <p role="status">{t('common.loading')}</p> : section === 'updates' && updates.length === 0 ? <Empty><EmptyHeader><EmptyMedia variant="icon"><RefreshCw /></EmptyMedia><EmptyTitle>{t('market.updatesEmpty')}</EmptyTitle><EmptyDescription>{t('market.updatesHelp')}</EmptyDescription></EmptyHeader></Empty> :
              <AcquiredResources installations={section === 'updates' ? updates : installations} requested={destination.section === 'acquired' ? destination.installationID : undefined}
                extensions={extensions.data ?? []} blockedReason={blockedReason} onChanged={refreshResources} onImport={openImport} onExport={setExporting} onDiscover={() => openResourceDestination({ section: 'discover' })} />}
          </div>
        </div>}
        <div hidden={section !== 'extensions'} className="h-full overflow-auto">
          <InstalledExtensions visible={visible && section === 'extensions'} catalog={extensions.data} error={extensions.error} installations={installations}
            dirtyEntries={dirtyExtensions} onDirtyChange={updateDirty} onRefresh={refreshResources} onUpdate={checkUpdate} blockedReason={blockedReason} />
        </div>
      </ResourceWorkspace>
    </FeaturePageShell>
    {importing && <ImportDialog {...importing} installedExtensions={extensions.data ?? []} onClose={() => setImporting(undefined)} onInstalled={async installation => {
      await refreshResources()
      const binding = installation.bindings.length === 1 ? installation.bindings[0] : undefined
      if (binding?.local.kind === 'extension.plugin' || binding?.local.kind === 'extension.game') openInstalledExtension(binding.local.kind === 'extension.game' ? 'game' : 'plugin', binding.local.id)
      else openResourceDestination({ section: 'acquired', installationID: installation.installation_id })
    }} />}
    {exporting && <ExportDialog {...exporting} projectID={exporting.installation?.project_id ?? projectID} onClose={() => setExporting(undefined)} />}
    {creationKind && <CreateProjectDirectoryDialog extension initialKind={creationKind} open onOpenChange={open => { if (!open) setCreationKind(undefined) }} />}
  </div>
}
