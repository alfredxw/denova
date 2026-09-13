import { useCallback, useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Puzzle } from 'lucide-react'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle, EmptyDescription } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { ResourceWorkspace } from '@/components/layout/resource-workspace'
import { closeMobilePanes } from '@/components/layout/mobile-pane-events'
import { SidebarVisibilityToggle } from '@/components/layout/sidebar-visibility-toggle'
import { CreateProjectDirectoryDialog } from '@/features/agent-chat/CreateProjectDirectoryDialog'
import { AGENT_CHAT_PROJECT_UPDATED_EVENT } from '@/features/agent-chat/api'
import { management, platformError, type CatalogEntry, type Candidate, type DevelopmentSource, type PackageKind, type RuntimeSnapshot } from './api'
import { InstallDialog } from './InstallDialog'
import { ExtensionDirectory } from './ExtensionDirectory'
import { ExtensionDetails } from './ExtensionDetails'
import { extensionEntries } from './extension-directory'
import { useExtensionNavigation } from './extension-navigation'

/** Consumption owns installed packages. Source links always navigate to the workbench. */
export function ExtensionsView({ visible = true }: { visible?: boolean }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const selected = useExtensionNavigation(state => state.selected)
  const [sidebarVisible, setSidebarVisible] = useState(true)
  const [installOpen, setInstallOpen] = useState(false)
  const [creationKind, setCreationKind] = useState<PackageKind>()
  const [candidate, setCandidate] = useState<Candidate | null>(null)
  const [updateGrants, setUpdateGrants] = useState<string[] | undefined>()
  const [dirtyEntries, setDirtyEntries] = useState<ReadonlySet<string>>(() => new Set())
  const updateDirty = useCallback((key: string, dirty: boolean) => setDirtyEntries(previous => {
    if (previous.has(key) === dirty) return previous
    const next = new Set(previous)
    if (dirty) next.add(key)
    else next.delete(key)
    return next
  }), [])
  const catalog = useQuery({ queryKey: ['platform', 'catalog'], queryFn: () => management<CatalogEntry[]>('/catalog'), enabled: visible })
  const sources = useQuery({ queryKey: ['platform', 'development'], queryFn: () => management<DevelopmentSource[]>('/development'), enabled: visible })
  const runtimes = useQuery({ queryKey: ['platform', 'runtimes'], queryFn: () => management<RuntimeSnapshot[]>('/runtimes'), enabled: visible, refetchInterval: visible ? 3000 : false })
  const refresh = useCallback(() => {
    // Settings save their own query result. Catalog changes must not refetch an
    // uninstalled package's editor before React has removed it from the page.
    void client.invalidateQueries({ queryKey: ['platform'], predicate: query => query.queryKey[1] !== 'settings' })
  }, [client])
  useEffect(() => {
    if (!visible) return
    refresh()
    window.addEventListener(AGENT_CHAT_PROJECT_UPDATED_EVENT, refresh)
    return () => window.removeEventListener(AGENT_CHAT_PROJECT_UPDATED_EVENT, refresh)
  }, [refresh, visible])
  // Source discovery is optional and cannot hide installed packages or their controls.
  const entries = extensionEntries(sources.data ?? [], catalog.data ?? [])
  const current = entries.find(entry => entry.key === selected) ?? entries[0]
  // Retain only editors with drafts; clean inactive extensions can be unmounted.
  useEffect(() => {
    if (!catalog.data) return
    const available = new Set(catalog.data.filter(item => !item.removed).map(item => `installed:${item.kind}:${item.id}`))
    setDirtyEntries(previous => [...previous].every(key => available.has(key)) ? previous : new Set([...previous].filter(key => available.has(key))))
  }, [catalog.data])
  useEffect(() => {
    if (!dirtyEntries.size) return
    const preventDraftLoss = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', preventDraftLoss)
    return () => window.removeEventListener('beforeunload', preventDraftLoss)
  }, [dirtyEntries.size])
  const directory = <ExtensionDirectory entries={entries} selected={current?.key ?? ''}
    onSelect={selected => { useExtensionNavigation.setState({ selected }); closeMobilePanes() }}
    onCreate={kind => { closeMobilePanes(); setCreationKind(kind) }}
    onInstall={() => { closeMobilePanes(); setUpdateGrants(undefined); setInstallOpen(true) }} />
  return <>
    <ResourceWorkspace title={t('platform.extensions.title')}
      left={{ id: 'extension-directory', title: t('platform.extensions.title'), side: 'left', content: directory, desktopVisible: sidebarVisible, desktopClassName: 'min-h-0 border-r', mobileClassName: 'w-[min(88vw,340px)]' }}
      leftResize={{ layoutKey: 'nova-extensions-directory-layout', label: t('layout.resize.sidebar'), defaultSize: '260px', minSize: '180px', maxSize: '35%' }}
      collapseAt={760} className="flex-1" mainClassName="min-h-0 min-w-0 bg-[var(--nova-surface)]">
      <FeaturePageShell title={t('platform.extensions.title')} subtitle={current ? '/ ' + t('platform.type.' + current.kind) : t('platform.extensions.subtitle')} icon={Puzzle} mobileHeader="hidden"
        leadingContent={<SidebarVisibilityToggle visible={sidebarVisible} onToggle={() => setSidebarVisible(value => !value)} />}
        error={catalog.error ? platformError(catalog.error) : null}>
        <div className="@container min-h-0 flex-1 overflow-y-auto">
          {catalog.isPending ? <div role="status" aria-label={t('common.loading')} className="mx-auto flex max-w-5xl flex-col gap-6 p-6"><Skeleton className="h-20 w-full" /><Skeleton className="h-40 w-full" /><Skeleton className="h-32 w-full" /></div> : current
            ? entries.filter(entry => entry.key === current.key || dirtyEntries.has(entry.key)).map(entry => <div key={entry.key} hidden={entry.key !== current.key}>
              <ExtensionDetails entry={entry} runtimes={runtimes.data ?? []} active={visible && entry.key === current.key} dirty={dirtyEntries.has(entry.key)} onDirtyChange={dirty => updateDirty(entry.key, dirty)} onRefresh={refresh} onUpdate={candidate => {
                setCandidate(candidate); setUpdateGrants(entry.installed.grants); setInstallOpen(true)
              }} />
            </div>)
            : !catalog.error && <Empty className="h-full"><EmptyHeader><EmptyMedia variant="icon"><Puzzle /></EmptyMedia><EmptyTitle>{t('platform.extensions.empty')}</EmptyTitle><EmptyDescription>{t('platform.extensions.emptyDescription')}</EmptyDescription></EmptyHeader></Empty>}
        </div>
      </FeaturePageShell>
    </ResourceWorkspace>
    <InstallDialog key={candidate?.candidateId ?? 'import'} open={installOpen} candidate={candidate} updateGrants={updateGrants} onCandidate={setCandidate} onOpenChange={setInstallOpen} onInstalled={refresh} />
    {creationKind && <CreateProjectDirectoryDialog extension initialKind={creationKind} open onOpenChange={open => { if (!open) setCreationKind(undefined) }} />}
  </>
}
