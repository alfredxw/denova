import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Gamepad2, PackagePlus, Plus, Puzzle, Search } from 'lucide-react'
import { EmbeddedSidebar } from '@/components/navigation/embedded-sidebar'
import { SidebarContent, SidebarGroup, SidebarGroupAction, SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle, EmptyDescription } from '@/components/ui/empty'
import { cn } from '@/lib/utils'
import { localized, type PackageKind } from './api'
import type { ExtensionEntry } from './extension-directory'

export function ExtensionDirectory({ entries, selected, onSelect, onCreate, onInstall }: {
  entries: ExtensionEntry[]
  selected: string
  onSelect: (key: string) => void
  onCreate: (kind: PackageKind) => void
  onInstall: () => void
}) {
  const { t, i18n } = useTranslation()
  const [search, setSearch] = useState('')
  const words = search.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  const filtered = entries.filter(entry => {
    const text = [localized(entry.manifest?.name, i18n.language), localized(entry.manifest?.description, i18n.language), entry.installed.id].join(' ').toLocaleLowerCase()
    return words.every(word => text.includes(word))
  })
  return <EmbeddedSidebar>
    <SidebarHeader className="gap-3 border-b p-3">
      <div className="flex items-center gap-2 px-1">
        <div className="min-w-0 flex-1"><h3 className="text-sm font-medium">{t('platform.extensions.directory')}</h3><p className="mt-1 text-xs text-muted-foreground">{t('platform.extensions.directoryHelp')}</p></div>
        <Badge variant="secondary">{entries.length}</Badge>
      </div>
      <InputGroup><InputGroupInput aria-label={t('platform.searchExtensions')} placeholder={t('platform.searchExtensions')} value={search} onChange={event => setSearch(event.target.value)} /><InputGroupAddon><Search /></InputGroupAddon></InputGroup>
      {/* Package actions belong to the directory, including empty and mobile views. */}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" className="flex-1" onClick={onInstall}><PackagePlus data-icon="inline-start" />{t('platform.install')}</Button>
        <Button variant="outline" size="sm" className="flex-1" onClick={() => onCreate('plugin')}><Plus data-icon="inline-start" />{t('platform.createExtension')}</Button>
      </div>
    </SidebarHeader>
    <SidebarContent>
      {words.length > 0 && filtered.length === 0 ? <Empty className="flex-none px-4 py-10"><EmptyHeader><EmptyMedia variant="icon"><Search /></EmptyMedia><EmptyTitle>{t('platform.extensions.noResults')}</EmptyTitle><EmptyDescription>{t('platform.extensions.noResultsHelp')}</EmptyDescription></EmptyHeader><Button variant="outline" size="sm" onClick={() => setSearch('')}>{t('platform.extensions.clearSearch')}</Button></Empty> : (['plugin', 'game'] as const).map(kind => {
        const items = filtered.filter(entry => entry.kind === kind)
        const Icon = kind === 'game' ? Gamepad2 : Puzzle
        return <SidebarGroup key={kind}>
          <Collapsible defaultOpen>
            <CollapsibleTrigger className="flex w-full items-center gap-2 rounded-md py-2 pl-2 pr-10 text-sm font-medium hover:bg-sidebar-accent [&[data-state=closed]>svg:last-child]:-rotate-90">
              <Icon className="size-4" /><span className="flex-1 text-left">{t('platform.type.' + kind)}</span><span className="text-xs text-muted-foreground">{items.length}</span><ChevronDown className="size-4" />
            </CollapsibleTrigger>
            <SidebarGroupAction type="button" className="top-3 size-7" aria-label={t('platform.create.' + kind)} title={t('platform.create.' + kind)} onClick={() => onCreate(kind)}>
              <Plus aria-hidden="true" />
            </SidebarGroupAction>
            <CollapsibleContent>
              <SidebarMenu>
                {items.map(entry => <SidebarMenuItem key={entry.key}>
                  <SidebarMenuButton isActive={entry.key === selected} title={localized(entry.manifest?.name, i18n.language) || entry.installed.id} className="h-auto min-h-12 items-start gap-2.5 py-2" onClick={() => onSelect(entry.key)}>
                    <Icon className="mt-0.5 text-muted-foreground" />
                    <div className="flex min-w-0 flex-1 flex-col gap-1">
                      <span className="line-clamp-2 break-words [overflow-wrap:anywhere]">{localized(entry.manifest?.name, i18n.language) || entry.installed.id}</span>
                      <span className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                        <span className={cn('size-1.5 rounded-full', entry.installed.enabled ? 'bg-[var(--nova-success)]' : 'bg-muted-foreground')} aria-hidden="true" />
                        <span>{t(entry.installed.enabled ? 'platform.extensions.enabled' : 'platform.disabled')}</span>
                        {entry.manifest && <><span aria-hidden="true">·</span><span>{entry.manifest.version}</span></>}
                      </span>
                    </div>
                  </SidebarMenuButton>
                </SidebarMenuItem>)}
              </SidebarMenu>
              {items.length === 0 && <p className="px-2 py-3 text-xs text-muted-foreground">{t('platform.directoryEmpty')}</p>}
            </CollapsibleContent>
          </Collapsible>
        </SidebarGroup>
      })}
    </SidebarContent>
  </EmbeddedSidebar>
}
