import { Search, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SectionedNavigation, type SectionedNavigationGroup } from '@/components/navigation/sectioned-navigation'
import { SidebarHeader } from '@/components/ui/sidebar'
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { SETTINGS_SECTIONS, type SettingsSectionId } from './settings-sections'

const DEBUG_FIELD_KEYS = ['settings.debug.llmInputLog', 'settings.debug.traceCaptureLevel', 'settings.debug.traceExporter', 'settings.debug.traceRetentionRuns']

interface SettingsNavigationProps {
  activeId: SettingsSectionId
  search: string
  onSearchChange: (value: string) => void
  onSelect: (id: SettingsSectionId, fieldKey?: string) => void
  showDebug: boolean
  goos?: string
}

export function SettingsNavigation({ activeId, search, onSearchChange, onSelect, showDebug, goos }: SettingsNavigationProps) {
  const { t } = useTranslation()
  const query = search.trim().toLocaleLowerCase()
  const matches = (text: string) => text.toLocaleLowerCase().includes(query)
  const results = SETTINGS_SECTIONS.flatMap((section) => {
    const title = t(section.titleKey)
    if (!query || matches(title)) return [{ id: section.id as string, section, title, fieldKey: '' }]
    const keys = section.id === 'advanced' && showDebug ? [...section.fieldKeys, ...DEBUG_FIELD_KEYS] : section.fieldKeys
    return keys.filter((key) => !(goos === 'windows' && ['settings.agent.shellEnvironmentMode', 'settings.agent.shellEnvironmentShell', 'settings.agent.bashPath'].includes(key)) && matches(t(key)))
      .map((key) => ({ id: `${section.id}:${key}`, section, title: t(key), fieldKey: key }))
  })
  const groups: SectionedNavigationGroup[] = []
  for (const result of results) {
    const groupID = result.section.group
    let group = groups.find((item) => item.id === groupID)
    if (!group) {
      group = { id: groupID, title: t(`settings.group.${groupID}`), items: [] }
      groups.push(group)
    }
    group.items.push({ id: result.id, title: result.title, description: result.fieldKey ? t(result.section.titleKey) : undefined, icon: result.section.icon })
  }
  return (
    <SectionedNavigation
      groups={groups}
      activeId={activeId}
      onSelect={(id) => {
        const result = results.find((item) => item.id === id)!
        onSelect(result.section.id, result.fieldKey)
      }}
      header={(
        <SidebarHeader>
          <InputGroup>
            <InputGroupAddon><Search aria-hidden="true" /></InputGroupAddon>
            <InputGroupInput value={search} placeholder={t('settings.search.placeholder')} aria-label={t('settings.search.placeholder')}
              onChange={(event) => onSearchChange(event.target.value)} />
            {search && <InputGroupAddon align="inline-end"><InputGroupButton aria-label={t('settings.search.clear')} onClick={() => onSearchChange('')}><X /></InputGroupButton></InputGroupAddon>}
          </InputGroup>
        </SidebarHeader>
      )}
      emptyState={<Empty><EmptyHeader><EmptyTitle>{t('settings.search.empty')}</EmptyTitle></EmptyHeader></Empty>}
    />
  )
}
