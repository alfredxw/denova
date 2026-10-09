import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Gamepad2, Puzzle, Search } from 'lucide-react'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle, EmptyDescription } from '@/components/ui/empty'
import { localized } from './api'
import type { ExtensionEntry } from './extension-directory'

export function ExtensionDirectory({ entries, onSelect }: { entries: ExtensionEntry[]; onSelect: (key: string) => void }) {
  const { t, i18n } = useTranslation()
  const [search, setSearch] = useState('')
  const words = search.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  const filtered = entries.filter(entry => {
    const text = [localized(entry.manifest?.name, i18n.language), localized(entry.manifest?.description, i18n.language), entry.installed.id].join(' ').toLocaleLowerCase()
    return words.every(word => text.includes(word))
  })
  return <>
    <InputGroup><InputGroupInput aria-label={t('platform.searchExtensions')} placeholder={t('platform.searchExtensions')} value={search} onChange={event => setSearch(event.target.value)} /><InputGroupAddon><Search /></InputGroupAddon></InputGroup>
    {words.length > 0 && !filtered.length ? <Empty><EmptyHeader><EmptyMedia variant="icon"><Search /></EmptyMedia><EmptyTitle>{t('platform.extensions.noResults')}</EmptyTitle><EmptyDescription>{t('platform.extensions.noResultsHelp')}</EmptyDescription></EmptyHeader><Button variant="outline" onClick={() => setSearch('')}>{t('platform.extensions.clearSearch')}</Button></Empty> :
      (['plugin', 'game'] as const).map(kind => {
        const items = filtered.filter(entry => entry.kind === kind)
        const Icon = kind === 'game' ? Gamepad2 : Puzzle
        return <Collapsible key={kind} defaultOpen className="rounded-lg border">
          <CollapsibleTrigger className="flex w-full items-center gap-2 p-3 text-sm font-medium [&[data-state=closed]>svg:last-child]:-rotate-90">
            <Icon className="size-4" /><span>{t('platform.type.' + kind)}</span><Badge variant="secondary">{items.length}</Badge><ChevronDown className="ml-auto size-4" />
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul className="divide-y border-t">{items.map(entry => <li key={entry.key}>
              <Button variant="ghost" className="h-auto min-h-16 w-full justify-start rounded-none p-3 text-left" onClick={() => onSelect(entry.key)}>
                <Icon data-icon="inline-start" />
                <span className="flex min-w-0 flex-1 flex-col gap-1 whitespace-normal">
                  <span className="[overflow-wrap:anywhere]">{localized(entry.manifest?.name, i18n.language) || entry.installed.id}</span>
                  <span className="text-xs font-normal text-muted-foreground [overflow-wrap:anywhere]">{localized(entry.manifest?.description, i18n.language)}</span>
                </span>
                <span className="flex shrink-0 flex-col items-end gap-1 text-xs font-normal">
                  <Badge variant="secondary">{t(entry.installed.enabled ? 'platform.extensions.enabled' : 'platform.disabled')}</Badge>
                  {entry.manifest && <span className="text-muted-foreground">v{entry.manifest.version}</span>}
                </span>
              </Button>
            </li>)}</ul>
            {!items.length && <p className="p-3 text-sm text-muted-foreground">{t('platform.directoryEmpty')}</p>}
          </CollapsibleContent>
        </Collapsible>
      })}
  </>
}
