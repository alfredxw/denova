import type { ReactNode } from 'react'
import { BookOpen, ChevronDown, CircleOff, Database, Dice5, Image as ImageIcon, ScrollText, Sparkles } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { ActorStateModule, EventPackageModule, ImagePreset, OpeningSelectorModule, RuleSystemModule, StoryDirectorModuleRefs, StoryMemoryStructureModule, Teller } from '../../types'
import { consoleSectionClassName, selectClassName } from './constants'
import { SectionTitle } from './shared'
import { normalizeIDList } from './utils'

export function DirectorModuleConsole({
  refs,
  selectedTellerName,
  selectedRuleName,
  selectedActorStateName,
  selectedMemoryStructureName,
  selectedMemoryStructureCount,
  selectedMemoryStructureTotal,
  selectedOpeningName,
  selectedImageName,
  selectedEventPackages,
  selectedEventCardCount,
  tellers,
  eventPackages,
  ruleSystems,
  actorStates,
  memoryStructures,
  openingSelectors,
  imagePresets,
  onModuleRefChange,
}: {
  refs: StoryDirectorModuleRefs
  selectedTellerName: string
  selectedRuleName: string
  selectedActorStateName: string
  selectedMemoryStructureName: string
  selectedMemoryStructureCount: number
  selectedMemoryStructureTotal: number
  selectedOpeningName: string
  selectedImageName: string
  selectedEventPackages: Array<{ id: string; name: string; invalid?: boolean; cards: number }>
  selectedEventCardCount: number
  tellers: Teller[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
  memoryStructures: StoryMemoryStructureModule[]
  openingSelectors: OpeningSelectorModule[]
  imagePresets: ImagePreset[]
  onModuleRefChange: <K extends keyof StoryDirectorModuleRefs>(key: K, value: StoryDirectorModuleRefs[K]) => void
}) {
  const { t } = useTranslation()
  const selectedEventPackageIDs = refs.event_package_ids?.length ? refs.event_package_ids : ['default']
  return (
    <section className={`${consoleSectionClassName} overflow-hidden p-4`}>
      <SectionTitle title={t('settingPanel.storyDirector.composer')} description={t('settingPanel.storyDirector.composerDesc')} badge={t('settingPanel.storyDirector.liveReference')} />
      <div className="mt-4 grid gap-2 md:grid-cols-2 xl:grid-cols-7">
        <DirectorModuleNode
          Icon={BookOpen}
          label={t('settingPanel.presetKind.teller')}
          title={selectedTellerName}
          summary={refs.narrative_style_disabled ? t('settingPanel.storyDirector.moduleDisabled') : t('settingPanel.storyDirector.moduleEnabled')}
          enabled={!refs.narrative_style_disabled}
          onEnabledChange={(enabled) => onModuleRefChange('narrative_style_disabled', !enabled)}
        >
          <ModuleSelect
            value={refs.narrative_style_id || ''}
            fallbackValue="classic"
            enabled={!refs.narrative_style_disabled}
            items={tellers}
            onChange={(value) => onModuleRefChange('narrative_style_id', value)}
          />
        </DirectorModuleNode>
        <DirectorModuleNode
          Icon={ScrollText}
          label={t('settingPanel.presetKind.event')}
          title={t('settingPanel.storyDirector.eventPackagesSummary', { packages: selectedEventPackageIDs.length, cards: selectedEventCardCount })}
          summary={selectedEventPackages.map((item) => item.name).join(' / ') || t('settingPanel.storyDirector.moduleMissing')}
          enabled={!refs.event_packages_disabled}
          onEnabledChange={(enabled) => onModuleRefChange('event_packages_disabled', !enabled)}
        >
          <EventPackagePopoverSelect
            values={selectedEventPackageIDs}
            fallbackValues={['default']}
            enabled={!refs.event_packages_disabled}
            items={eventPackages}
            onChange={(value) => onModuleRefChange('event_package_ids', value)}
          />
        </DirectorModuleNode>
        <DirectorModuleNode
          Icon={Dice5}
          label={t('settingPanel.presetKind.rule')}
          title={selectedRuleName}
          summary={refs.rule_system_disabled ? t('settingPanel.storyDirector.moduleDisabled') : t('settingPanel.storyDirector.moduleEnabled')}
          enabled={!refs.rule_system_disabled}
          onEnabledChange={(enabled) => onModuleRefChange('rule_system_disabled', !enabled)}
        >
          <ModuleSelect
            value={refs.rule_system_id || ''}
            fallbackValue="default"
            enabled={!refs.rule_system_disabled}
            items={ruleSystems}
            onChange={(value) => onModuleRefChange('rule_system_id', value)}
          />
        </DirectorModuleNode>
        <DirectorModuleNode
          Icon={Database}
          label={t('settingPanel.presetKind.actorState')}
          title={selectedActorStateName}
          summary={refs.actor_state_disabled ? t('settingPanel.storyDirector.moduleDisabled') : t('settingPanel.storyDirector.moduleEnabled')}
          enabled={!refs.actor_state_disabled}
          onEnabledChange={(enabled) => onModuleRefChange('actor_state_disabled', !enabled)}
        >
          <ModuleSelect
            value={refs.actor_state_id || ''}
            fallbackValue="default"
            enabled={!refs.actor_state_disabled}
            items={actorStates}
            onChange={(value) => onModuleRefChange('actor_state_id', value)}
          />
        </DirectorModuleNode>
        <DirectorModuleNode
          Icon={Database}
          label={t('settingPanel.presetKind.memoryStructure')}
          title={selectedMemoryStructureName}
          summary={refs.memory_structure_disabled ? t('settingPanel.storyDirector.moduleDisabled') : t('settingPanel.memoryStructure.summaryCount', { enabled: selectedMemoryStructureCount, total: selectedMemoryStructureTotal })}
          enabled={!refs.memory_structure_disabled}
          onEnabledChange={(enabled) => onModuleRefChange('memory_structure_disabled', !enabled)}
        >
          <ModuleSelect
            value={refs.memory_structure_id || ''}
            fallbackValue="default"
            enabled={!refs.memory_structure_disabled}
            items={memoryStructures}
            onChange={(value) => onModuleRefChange('memory_structure_id', value)}
          />
        </DirectorModuleNode>
        <DirectorModuleNode
          Icon={Sparkles}
          label={t('settingPanel.presetKind.opening')}
          title={selectedOpeningName}
          summary={refs.opening_selector_disabled ? t('settingPanel.storyDirector.moduleDisabled') : t('settingPanel.storyDirector.moduleEnabled')}
          enabled={!refs.opening_selector_disabled}
          onEnabledChange={(enabled) => onModuleRefChange('opening_selector_disabled', !enabled)}
        >
          <ModuleSelect
            value={refs.opening_selector_id || ''}
            fallbackValue="default"
            enabled={!refs.opening_selector_disabled}
            items={openingSelectors}
            onChange={(value) => onModuleRefChange('opening_selector_id', value)}
          />
        </DirectorModuleNode>
        <DirectorModuleNode
          Icon={ImageIcon}
          label={t('settingPanel.presetKind.image')}
          title={selectedImageName}
          summary={refs.image_preset_disabled ? t('settingPanel.storyDirector.moduleDisabled') : t('settingPanel.storyDirector.moduleEnabled')}
          enabled={!refs.image_preset_disabled}
          onEnabledChange={(enabled) => onModuleRefChange('image_preset_disabled', !enabled)}
        >
          <ModuleSelect
            value={refs.image_preset_id || ''}
            fallbackValue="game-cg"
            enabled={!refs.image_preset_disabled}
            items={imagePresets}
            onChange={(value) => onModuleRefChange('image_preset_id', value)}
          />
        </DirectorModuleNode>
      </div>
    </section>
  )
}

function DirectorModuleNode({
  Icon,
  label,
  title,
  summary,
  enabled,
  onEnabledChange,
  children,
}: {
  Icon: LucideIcon
  label: string
  title: string
  summary: string
  enabled: boolean
  onEnabledChange: (enabled: boolean) => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  const switchLabel = enabled
    ? t('settingPanel.storyDirector.disableModule', { module: label })
    : t('settingPanel.storyDirector.enableModule', { module: label })
  return (
    <div className={`group relative grid min-w-0 gap-3 rounded-[var(--nova-radius)] border p-3 transition ${enabled ? 'border-[var(--nova-border)] bg-[var(--nova-surface-2)]' : 'border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)]/60 opacity-75'}`}>
      <div className="flex min-w-0 items-start gap-2">
        <span className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border ${enabled ? 'border-[var(--nova-accent)]/30 bg-[var(--nova-accent)]/10 text-[var(--nova-accent)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]'}`}>
          {enabled ? <Icon className="h-3.5 w-3.5" /> : <CircleOff className="h-3.5 w-3.5" />}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-[11px] text-[var(--nova-text-faint)]">{label}</span>
          <span className="mt-0.5 block truncate text-xs font-medium text-[var(--nova-text)]" title={title}>{title}</span>
          <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]" title={summary}>{summary}</span>
        </span>
        <Switch checked={enabled} onCheckedChange={onEnabledChange} aria-label={switchLabel} title={switchLabel} />
      </div>
      {children}
    </div>
  )
}

function ModuleSelect<T extends { id: string; name: string; invalid?: boolean }>({
  value,
  fallbackValue,
  enabled,
  items,
  onChange,
}: {
  value: string
  fallbackValue: string
  enabled: boolean
  items: T[]
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const selectedValue = value || fallbackValue
  const hasSelected = items.some((item) => item.id === selectedValue)
  return (
    <Select value={hasSelected ? selectedValue : fallbackValue} onValueChange={onChange} disabled={!enabled}>
      <SelectTrigger size="sm" className={`${selectClassName} w-full`}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent className="nova-panel border text-[var(--nova-text)]">
        {items.length > 0 ? items.map((item) => (
          <SelectItem key={item.id} value={item.id}>
            {item.name}{item.invalid ? ` · ${t('settingPanel.invalid')}` : ''}
          </SelectItem>
        )) : (
          <SelectItem value={fallbackValue}>{fallbackValue}</SelectItem>
        )}
      </SelectContent>
    </Select>
  )
}

function EventPackagePopoverSelect<T extends { id: string; name: string; invalid?: boolean }>({
  values,
  fallbackValues,
  enabled,
  items,
  onChange,
}: {
  values: string[]
  fallbackValues: string[]
  enabled: boolean
  items: T[]
  onChange: (values: string[]) => void
}) {
  const { t } = useTranslation()
  const selectedValues = normalizeIDList(values.length ? values : fallbackValues)
  const selectedSet = new Set(selectedValues)
  const toggleValue = (id: string, checked: boolean) => {
    const next = checked
      ? normalizeIDList([...selectedValues, id])
      : selectedValues.filter((value) => value !== id)
    onChange(next.length ? next : fallbackValues)
  }
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" className={`${selectClassName} w-full justify-between px-2 text-left text-[var(--nova-text)]`} variant="outline" size="sm" disabled={!enabled}>
          <span className="min-w-0 flex-1 truncate">{t('settingPanel.storyDirector.eventPackagePickerButton', { count: selectedValues.length })}</span>
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="nova-panel w-[min(360px,calc(100vw-2rem))] border border-[var(--nova-border)] p-2 text-[var(--nova-text)]">
        <div className="grid gap-0.5">
          <div className="px-2 py-1 text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.storyDirector.eventPackagePicker')}</div>
          <div className="max-h-64 overflow-y-auto pr-1">
            {items.length > 0 ? items.map((item) => (
              <label key={item.id} className="flex min-h-8 cursor-pointer items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]">
                <input
                  type="checkbox"
                  className="h-3.5 w-3.5 shrink-0 accent-[var(--nova-accent)]"
                  checked={selectedSet.has(item.id)}
                  disabled={!enabled}
                  onChange={(event) => toggleValue(item.id, event.target.checked)}
                />
                <span className="min-w-0 flex-1 truncate">{item.name}</span>
                {item.invalid ? <span className="shrink-0 text-[10px] text-[var(--nova-danger)]">{t('settingPanel.invalid')}</span> : null}
              </label>
            )) : (
              <div className="px-2 py-2 text-xs text-[var(--nova-text-faint)]">{fallbackValues.join(', ')}</div>
            )}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
