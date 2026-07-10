import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirectorModuleRefs, StoryMemoryStructureModule, Teller } from '../../types'
import { consoleSectionClassName, selectClassName } from './constants'
import { SectionTitle } from './shared'
import { normalizeIDList } from './utils'

export function DirectorModuleConsole({
  refs,
  selectedTellerName,
  selectedRuleName,
  selectedActorStateName,
  selectedMemoryStructureCount,
  selectedMemoryStructureTotal,
  selectedImageName,
  selectedEventCardCount,
  tellers,
  eventPackages,
  ruleSystems,
  actorStates,
  memoryStructures,
  imagePresets,
  onModuleRefChange,
}: {
  refs: StoryDirectorModuleRefs
  selectedTellerName: string
  selectedRuleName: string
  selectedActorStateName: string
  selectedMemoryStructureCount: number
  selectedMemoryStructureTotal: number
  selectedImageName: string
  selectedEventCardCount: number
  tellers: Teller[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
  memoryStructures: StoryMemoryStructureModule[]
  imagePresets: ImagePreset[]
  onModuleRefChange: <K extends keyof StoryDirectorModuleRefs>(key: K, value: StoryDirectorModuleRefs[K]) => void
}) {
  const { t } = useTranslation()
  const selectedEventPackageIDs = refs.event_package_ids?.length ? refs.event_package_ids : ['default']

  return (
    <section className={`${consoleSectionClassName} p-4`}>
      <SectionTitle title={t('settingPanel.storyDirector.composer')} description={t('settingPanel.storyDirector.composerDesc')} badge={t('settingPanel.storyDirector.liveReference')} />
      <div className="mt-3 grid grid-cols-[repeat(auto-fit,minmax(min(100%,22rem),1fr))] gap-3">
        {/* 核心叙事 */}
        <ModuleGroup label={t('settingPanel.storyDirector.group.core')}>
          <ModuleRefRow
            label={t('settingPanel.presetKind.teller')}
            summary={selectedTellerName}
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
          </ModuleRefRow>
        </ModuleGroup>

        {/* 系统规则 */}
        <ModuleGroup label={t('settingPanel.storyDirector.group.rules')}>
          <ModuleRefRow
            label={t('settingPanel.presetKind.rule')}
            summary={selectedRuleName}
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
          </ModuleRefRow>
          <ModuleRefRow
            label={t('settingPanel.presetKind.actorState')}
            summary={selectedActorStateName}
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
          </ModuleRefRow>
          <ModuleRefRow
            label={t('settingPanel.presetKind.memoryStructure')}
            summary={refs.memory_structure_disabled
              ? t('settingPanel.storyDirector.moduleDisabled')
              : t('settingPanel.memoryStructure.summaryCount', { enabled: selectedMemoryStructureCount, total: selectedMemoryStructureTotal })}
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
          </ModuleRefRow>
        </ModuleGroup>

        {/* 内容生成 */}
        <ModuleGroup label={t('settingPanel.storyDirector.group.content')}>
          <ModuleRefRow
            label={t('settingPanel.presetKind.event')}
            summary={t('settingPanel.storyDirector.eventPackagesSummary', { packages: selectedEventPackageIDs.length, cards: selectedEventCardCount })}
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
          </ModuleRefRow>
          <ModuleRefRow
            label={t('settingPanel.presetKind.image')}
            summary={selectedImageName}
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
          </ModuleRefRow>
        </ModuleGroup>
      </div>
    </section>
  )
}

function ModuleGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid content-start gap-1.5 self-start">
      <div className="px-0.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">{label}</div>
      <div className="grid content-start gap-1 rounded-[11px] border border-[var(--preset-line)] bg-[var(--preset-raised)]/70 p-1.5">
        {children}
      </div>
    </div>
  )
}

function ModuleRefRow({
  label,
  summary,
  enabled,
  onEnabledChange,
  children,
}: {
  label: string
  summary: string
  enabled: boolean
  onEnabledChange: (enabled: boolean) => void
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const switchLabel = enabled
    ? t('settingPanel.storyDirector.disableModule', { module: label })
    : t('settingPanel.storyDirector.enableModule', { module: label })
  return (
    <div className={`flex min-h-12 items-center gap-2 rounded-lg px-2 py-1.5 ${enabled ? '' : 'opacity-60'}`}>
      <span className="w-24 shrink-0 text-[11px] text-[var(--nova-text-muted)]">{label}</span>
      <span className="min-w-0 flex-1">
        {children}
        {summary ? <span className="mt-0.5 block truncate text-[10px] text-[var(--nova-text-faint)]" title={summary}>{summary}</span> : null}
      </span>
      <Switch checked={enabled} onCheckedChange={onEnabledChange} aria-label={switchLabel} title={switchLabel} />
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
