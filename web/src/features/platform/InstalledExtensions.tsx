import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Puzzle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle, EmptyDescription } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { AGENT_CHAT_PROJECT_UPDATED_EVENT } from '@/features/agent-chat/api'
import type { Installation } from '@/features/market/api'
import { openResourceDestination, useResourceNavigation } from '@/features/market/resource-navigation'
import { management, platformError, type CatalogEntry, type DevelopmentSource, type RuntimeSnapshot } from './api'
import { ExtensionDirectory } from './ExtensionDirectory'
import { ExtensionDetails } from './ExtensionDetails'
import { extensionEntries } from './extension-directory'

/** Installed editors survive navigation while dirty; runtime polling only runs on this destination. */
export function InstalledExtensions({ visible, catalog, error, installations, dirtyEntries, onDirtyChange, onRefresh, onUpdate, blockedReason }: {
  visible: boolean
  catalog?: CatalogEntry[]
  error: unknown
  installations: Installation[]
  dirtyEntries: ReadonlySet<string>
  onDirtyChange: (key: string, dirty: boolean) => void
  onRefresh: () => Promise<void>
  onUpdate: (installation: Installation) => Promise<void>
  blockedReason?: string
}) {
  const { t } = useTranslation()
  const destination = useResourceNavigation(state => state.destination)
  const selected = destination.section === 'extensions' ? destination.extensionKey : undefined
  const sources = useQuery({ queryKey: ['platform', 'development'], queryFn: () => management<DevelopmentSource[]>('/development'), enabled: visible })
  const runtimes = useQuery({ queryKey: ['platform', 'runtimes'], queryFn: () => management<RuntimeSnapshot[]>('/runtimes'), enabled: visible, refetchInterval: visible ? 3000 : false })
  const entries = extensionEntries(sources.data ?? [], catalog ?? [])
  const current = entries.find(entry => entry.key === selected)
  useEffect(() => {
    if (!visible) return
    const refresh = () => { void onRefresh() }
    window.addEventListener(AGENT_CHAT_PROJECT_UPDATED_EVENT, refresh)
    return () => window.removeEventListener(AGENT_CHAT_PROJECT_UPDATED_EVENT, refresh)
  }, [visible, onRefresh])
  useEffect(() => {
    if (!catalog) return
    for (const key of dirtyEntries) {
      if (!catalog.some(item => !item.removed && `installed:${item.kind}:${item.id}` === key)) onDirtyChange(key, false)
    }
  }, [catalog, dirtyEntries, onDirtyChange])
  return <>
    {error ? <p role="alert" className="p-4 text-sm text-destructive">{platformError(error)}</p> : null}
    {!catalog ? !error && <div role="status" aria-label={t('common.loading')} className="flex flex-col gap-4 p-4"><Skeleton className="h-20" /><Skeleton className="h-40" /></div> : <>
      {!current && <div className="mx-auto flex max-w-6xl flex-col gap-4 p-4 lg:p-5">
        <h1 className="border-b pb-3 text-base font-semibold">{t('market.extensions')}</h1>
        {entries.length ? <ExtensionDirectory entries={entries} onSelect={extensionKey => openResourceDestination({ section: 'extensions', extensionKey })} /> :
          <Empty><EmptyHeader><EmptyMedia variant="icon"><Puzzle /></EmptyMedia><EmptyTitle>{t('platform.extensions.empty')}</EmptyTitle><EmptyDescription>{t('platform.extensions.emptyDescription')}</EmptyDescription></EmptyHeader><Button onClick={() => openResourceDestination({ section: 'discover' })}>{t('market.discover')}</Button></Empty>}
      </div>}
      {current && <Button variant="ghost" className="ml-4 mt-3" onClick={() => openResourceDestination({ section: 'extensions' })}><ArrowLeft data-icon="inline-start" />{t('market.back')}</Button>}
      {entries.filter(entry => entry.key === current?.key || dirtyEntries.has(entry.key)).map(entry => <div key={entry.key} hidden={entry.key !== current?.key}>
        <ExtensionDetails entry={entry} runtimes={runtimes.data ?? []} active={visible && entry.key === current?.key}
          dirty={dirtyEntries.has(entry.key)} onDirtyChange={dirty => onDirtyChange(entry.key, dirty)} onRefresh={onRefresh}
          owner={installations.find(item => item.tracking === 'tracked' && item.bindings.some(binding => binding.ownership === 'owned' && binding.local.kind === `extension.${entry.kind}` && binding.local.id === entry.installed.id))}
          onUpdate={onUpdate} blockedReason={blockedReason} />
      </div>)}
    </>}
  </>
}
