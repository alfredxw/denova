import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { BookMarked, Bot, Building2, ChevronDown, ChevronsDownUp, ChevronsUpDown, Compass, Database, Dice5, FileText, Folder, Images, Library, Loader2, MapPin, Plus, ScrollText, Search, SlidersHorizontal, Sparkles, Trash2, UserRound } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { type LoreItem, workspaceAssetURL } from '@/lib/api'
import { INTERACTIVE_OPENING_PRESET_ENTRY_ID, newBookOpeningPreset, type BookOpeningPreset } from '../opening'
import { presetResourceVisibleInMode, type PresetResourceKind, type PresetUsageMode } from '../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, ImagePresetSlot, OpeningSelectorModule, RuleSystemModule, StoryDirector, StoryMemoryStructureModule, Teller, TellerEventPackage } from '../types'
import { PresetConfigSectionEditor } from './preset-config/PresetConfigSectionEditor'
import { normalizeTRPGSystem } from './preset-config/ruleTemplates'
import { ActorStateVisualEditor, EventPackageVisualEditor, MemoryStructureVisualEditor, OpeningSelectorVisualEditor, TRPGSystemVisualEditor } from './preset-config/visual-editors'
import { BooleanSwitchField } from './setting-panel/BooleanSwitchField'

const CREATOR_PATH = 'CREATOR.md'
const CREATOR_ENTRY_ID = '__creator__'
const LORE_CONFIG_AGENT_ENTRY_ID = '__config_manager_lore__'
const TELLER_CONFIG_AGENT_ENTRY_ID = '__config_manager_teller__'
const TYPE_OPTIONS = [
  { value: 'character' },
  { value: 'world' },
  { value: 'location' },
  { value: 'faction' },
  { value: 'rule' },
  { value: 'item' },
  { value: 'other' },
] as const
const IMPORTANCE_OPTIONS = [
  { value: 'major' },
  { value: 'important' },
  { value: 'minor' },
] as const
const LOAD_MODE_OPTIONS = [
  { value: 'resident' },
  { value: 'auto' },
  { value: 'manual' },
] as const
const LORE_RESIDENT_ITEM_WARNING_CHARS = 8000
const LORE_RESIDENT_TOTAL_WARNING_CHARS = 40000
const IMAGE_PRESET_PROMPT_LIMIT = 4000
const IMAGE_PRESET_TARGET_OPTIONS = [{ value: 'agent_system' }, { value: 'tool_request' }] as const
const PRESET_DIRECTORY_ORDER: PresetResourceKind[] = ['director', 'teller', 'image', 'event', 'rule', 'actor-state', 'memory-structure', 'opening']
type ImagePresetTarget = ImagePresetSlot['target']
type LoreType = LoreItem['type']
interface KnowledgeSection {
  id: string
  labelKey: string
  icon: LucideIcon
  types: LoreType[]
  createType: LoreType
  createName: string
  tag?: string
  excludeTag?: string
}
const KNOWLEDGE_SECTIONS: KnowledgeSection[] = [
  { id: 'characters', labelKey: 'lore.type.character', icon: UserRound, types: ['character'], createType: 'character', createName: '新角色' },
  { id: 'locations', labelKey: 'lore.type.location', icon: MapPin, types: ['location'], createType: 'location', createName: '新地点' },
  { id: 'factions', labelKey: 'lore.type.faction', icon: Building2, types: ['faction'], createType: 'faction', createName: '新组织' },
  { id: 'rules', labelKey: 'lore.type.rule', icon: ScrollText, types: ['world', 'rule'], createType: 'rule', createName: '新规则' },
  { id: 'templates', labelKey: 'settingPanel.section.templates', icon: FileText, types: ['other'], createType: 'other', createName: '新模板', tag: '模板' },
  { id: 'assets', labelKey: 'settingPanel.section.assets', icon: Library, types: ['item', 'other'], createType: 'item', createName: '新素材', excludeTag: '模板' },
]

const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 text-xs focus:ring-0'

export function LoreDirectory({
  items,
  activeId,
  query,
  saving,
  onQueryChange,
  onSelect,
  onCreate,
  onBatchGenerate,
}: {
  items: LoreItem[]
  activeId: string
  query: string
  saving: boolean
  onQueryChange: (value: string) => void
  onSelect: (id: string) => void
  onCreate: (section: KnowledgeSection) => void
  onBatchGenerate: () => void
}) {
  const { t } = useTranslation()
  const [collapsedSections, setCollapsedSections] = useState<Record<string, boolean>>({})
  const sections = KNOWLEDGE_SECTIONS
    .map((section) => ({ section, entries: sectionItems(items, section, query) }))
    .sort((a, b) => {
      if (a.entries.length === 0 && b.entries.length > 0) return 1
      if (a.entries.length > 0 && b.entries.length === 0) return -1
      return KNOWLEDGE_SECTIONS.findIndex((section) => section.id === a.section.id) - KNOWLEDGE_SECTIONS.findIndex((section) => section.id === b.section.id)
    })

  const isCollapsed = (section: KnowledgeSection, entries: LoreItem[]) => collapsedSections[section.id] ?? entries.length === 0
  const toggleSection = (section: KnowledgeSection, entries: LoreItem[]) => {
    setCollapsedSections((current) => ({
      ...current,
      [section.id]: !(current[section.id] ?? entries.length === 0),
    }))
  }

  return (
    <>
      <div className="border-b border-[var(--nova-border)] p-2">
        <div className="flex items-center gap-2">
          <div className="nova-field flex h-8 min-w-0 flex-1 items-center gap-2 rounded-[var(--nova-radius)] px-2 text-xs text-[var(--nova-text-faint)]">
            <Search className="h-3.5 w-3.5" />
            <input
              className="min-w-0 flex-1 bg-transparent text-[var(--nova-text-muted)] outline-none placeholder:text-[var(--nova-text-faint)]"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder={t('settingPanel.searchLore')}
            />
          </div>
          <Button className={iconActionClassName} variant="outline" size="icon" disabled={saving || items.length === 0} onClick={onBatchGenerate} aria-label={t('settingPanel.loreImage.batchOpen')} title={t('settingPanel.loreImage.batchOpen')}>
            <Images className="h-3.5 w-3.5" />
          </Button>
        </div>
        <button
          type="button"
          onClick={() => onSelect(CREATOR_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === CREATOR_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <BookMarked className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">{CREATOR_PATH}</span>
        </button>
        <button
          type="button"
          onClick={() => onSelect(INTERACTIVE_OPENING_PRESET_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === INTERACTIVE_OPENING_PRESET_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">{t('settingPanel.openingPreset.title')}</span>
        </button>
        <button
          type="button"
          onClick={() => onSelect(LORE_CONFIG_AGENT_ENTRY_ID)}
          className={`mt-2 flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
            activeId === LORE_CONFIG_AGENT_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
          }`}
        >
          <Bot className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
          <span className="min-w-0 flex-1 truncate">{t('settingPanel.loreAgent.title')}</span>
        </button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-2">
          {sections.map(({ section, entries }) => {
            const Icon = section.icon
            const collapsed = isCollapsed(section, entries)
            const label = t(section.labelKey)
            return (
              <section key={section.id} className={entries.length ? 'mb-2' : 'mb-1'}>
                <div className={`flex h-8 items-center gap-2 rounded px-2 text-xs ${entries.length ? 'text-[var(--nova-text-muted)]' : 'text-[var(--nova-text-faint)]'}`}>
                  <button
                    type="button"
                    className="nova-nav-item rounded p-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    onClick={() => toggleSection(section, entries)}
                    aria-label={collapsed ? `${t('chat.tool.expand')}${label}` : `${t('chat.tool.collapse')}${label}`}
                  >
                    <ChevronDown className={`h-3.5 w-3.5 transition-transform ${collapsed ? '-rotate-90' : ''}`} />
                  </button>
                  <Icon className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate font-medium">{label}</span>
                  <span className="text-[11px] text-[var(--nova-text-faint)]">{entries.length}</span>
                  <button
                    type="button"
                    className="nova-nav-item rounded p-1 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    disabled={saving}
                    onClick={() => onCreate(section)}
                    aria-label={`${t('chat.new')}${label}`}
                  >
                    <Plus className="h-3.5 w-3.5" />
                  </button>
                </div>
                {!collapsed && entries.length > 0 && (
                  <div className="ml-5 space-y-0.5 border-l border-[var(--nova-border)] pl-2">
                    {entries.map((item) => (
                      <button
                        key={item.id}
                        type="button"
                        onClick={() => onSelect(item.id)}
                        className={`flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition ${
                          activeId === item.id ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                        } ${item.enabled === false ? 'opacity-50' : ''}`}
                      >
                        <LoreItemThumb item={item} />
                        <span className="min-w-0 flex-1 truncate">{item.name}</span>
                        {item.enabled === false ? <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('settingPanel.disabled')}</span> : null}
                      </button>
                    ))}
                  </div>
                )}
              </section>
            )
          })}
        </div>
      </ScrollArea>
    </>
  )
}

function LoreItemThumb({ item }: { item: LoreItem }) {
  const imagePath = item.image?.image_path || ''
  if (!imagePath) {
    return (
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)]">
        <FileText className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
      </span>
    )
  }
  return (
    <span className="flex h-5 w-5 shrink-0 overflow-hidden rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <img src={workspaceAssetURL(imagePath)} alt="" className="h-full w-full object-cover" />
    </span>
  )
}

export function CreatorDirectory() {
  const { t } = useTranslation()
  return (
    <div className="p-2">
      <div className="flex h-8 items-center gap-2 rounded px-2 text-xs text-[var(--nova-text-muted)]">
        <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <Folder className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <span className="font-medium">{t('settingPanel.rootDirectory')}</span>
      </div>
      <div className="ml-5 border-l border-[var(--nova-border)] pl-2">
        <div className="flex h-8 items-center gap-2 rounded-[var(--nova-radius)] bg-[var(--nova-active)] px-2 text-xs text-[var(--nova-text)]">
          <BookMarked className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          <span className="truncate">{CREATOR_PATH}</span>
        </div>
      </div>
    </div>
  )
}

export function TellerDirectory({
  resourceKind,
  tellers,
  storyDirectors,
  imagePresets,
  eventPackages,
  ruleSystems,
  actorStates,
  memoryStructures,
  openingSelectors,
  activeTellerId,
  activeStoryDirectorId,
  activeImagePresetId,
  activeEventPackageId,
  activeRuleSystemId,
  activeActorStateId,
  activeMemoryStructureId,
  activeOpeningSelectorId,
  saving,
  onSelectTeller,
  onSelectStoryDirector,
  onSelectImagePreset,
  onSelectEventPackage,
  onSelectRuleSystem,
  onSelectActorState,
  onSelectMemoryStructure,
  onSelectOpeningSelector,
  onCreateTeller,
  onCreateStoryDirector,
  onCreateImagePreset,
  onCreateEventPackage,
  onCreateRuleSystem,
  onCreateActorState,
  onCreateMemoryStructure,
  onCreateOpeningSelector,
  usageMode,
}: {
  resourceKind: PresetResourceKind
  usageMode: PresetUsageMode
  tellers: Teller[]
  storyDirectors: StoryDirector[]
  imagePresets: ImagePreset[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
  memoryStructures: StoryMemoryStructureModule[]
  openingSelectors: OpeningSelectorModule[]
  activeTellerId: string
  activeStoryDirectorId: string
  activeImagePresetId: string
  activeEventPackageId: string
  activeRuleSystemId: string
  activeActorStateId: string
  activeMemoryStructureId: string
  activeOpeningSelectorId: string
  saving: boolean
  onSelectTeller: (id: string) => void
  onSelectStoryDirector: (id: string) => void
  onSelectImagePreset: (id: string) => void
  onSelectEventPackage: (id: string) => void
  onSelectRuleSystem: (id: string) => void
  onSelectActorState: (id: string) => void
  onSelectMemoryStructure: (id: string) => void
  onSelectOpeningSelector: (id: string) => void
  onCreateTeller: () => void
  onCreateStoryDirector: () => void
  onCreateImagePreset: () => void
  onCreateEventPackage: () => void
  onCreateRuleSystem: () => void
  onCreateActorState: () => void
  onCreateMemoryStructure: () => void
  onCreateOpeningSelector: () => void
}) {
  const { t } = useTranslation()
  const [collapsedSections, setCollapsedSections] = useState<Partial<Record<PresetResourceKind, boolean>>>({})
  const isConfigAgentActive = activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID
  const isVisible = (kind: PresetResourceKind) => presetResourceVisibleInMode(kind, usageMode)
  const isCollapsed = (kind: PresetResourceKind) => collapsedSections[kind] ?? defaultPresetSectionCollapsed(kind, resourceKind)
  const visibleKinds = PRESET_DIRECTORY_ORDER.filter(isVisible)
  const hasCollapsedVisibleSections = visibleKinds.some(isCollapsed)
  const DirectoryToggleIcon = hasCollapsedVisibleSections ? ChevronsUpDown : ChevronsDownUp
  const directoryToggleLabel = hasCollapsedVisibleSections ? t('settingPanel.directory.expandAll') : t('settingPanel.directory.collapseAll')
  const toggleSection = (kind: PresetResourceKind) => {
    setCollapsedSections((current) => ({
      ...current,
      [kind]: !isCollapsed(kind),
    }))
  }
  const toggleAllSections = () => {
    setCollapsedSections((current) => {
      const next = { ...current }
      for (const kind of visibleKinds) {
        next[kind] = !hasCollapsedVisibleSections
      }
      return next
    })
  }
  useEffect(() => {
    setCollapsedSections((current) => {
      if (current[resourceKind] === false) return current
      return { ...current, [resourceKind]: false }
    })
  }, [resourceKind])

  useEffect(() => {
    if (isConfigAgentActive || presetResourceVisibleInMode(resourceKind, usageMode)) return
    if (presetResourceVisibleInMode('teller', usageMode) && tellers[0]) {
      onSelectTeller(tellers[0].id)
      return
    }
    if (presetResourceVisibleInMode('image', usageMode) && imagePresets[0]) {
      onSelectImagePreset(imagePresets[0].id)
      return
    }
    if (presetResourceVisibleInMode('director', usageMode) && storyDirectors[0]) {
      onSelectStoryDirector(storyDirectors[0].id)
      return
    }
    if (presetResourceVisibleInMode('event', usageMode) && eventPackages[0]) {
      onSelectEventPackage(eventPackages[0].id)
      return
    }
    if (presetResourceVisibleInMode('rule', usageMode) && ruleSystems[0]) {
      onSelectRuleSystem(ruleSystems[0].id)
      return
    }
    if (presetResourceVisibleInMode('actor-state', usageMode) && actorStates[0]) {
      onSelectActorState(actorStates[0].id)
      return
    }
    if (presetResourceVisibleInMode('memory-structure', usageMode) && memoryStructures[0]) {
      onSelectMemoryStructure(memoryStructures[0].id)
      return
    }
    if (presetResourceVisibleInMode('opening', usageMode) && openingSelectors[0]) {
      onSelectOpeningSelector(openingSelectors[0].id)
    }
  }, [actorStates, eventPackages, imagePresets, isConfigAgentActive, memoryStructures, onSelectActorState, onSelectEventPackage, onSelectImagePreset, onSelectMemoryStructure, onSelectOpeningSelector, onSelectRuleSystem, onSelectStoryDirector, onSelectTeller, openingSelectors, resourceKind, ruleSystems, storyDirectors, tellers, usageMode])

  return (
    <>
      <div className="border-b border-[var(--nova-border)] p-2">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => onSelectTeller(TELLER_CONFIG_AGENT_ENTRY_ID)}
            className={`flex h-9 min-w-0 flex-1 items-center gap-2 rounded-md px-2 text-left text-xs transition ${
              activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
            }`}
          >
            <Bot className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
            <span className="min-w-0 flex-1 truncate">{t('settingPanel.tellerAgent.title')}</span>
          </button>
          <button
            type="button"
            onClick={toggleAllSections}
            aria-label={directoryToggleLabel}
            title={directoryToggleLabel}
            className="nova-nav-item flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          >
            <DirectoryToggleIcon className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="space-y-2 p-2">
          {isVisible('director') ? (
            <PresetDirectorySection
              kind="director"
              label={presetKindDirectoryLabel('director', t)}
              Icon={Compass}
              count={storyDirectors.length}
              createLabel={presetKindCreateLabel('director', t)}
              saving={saving}
              collapsed={isCollapsed('director')}
              onToggle={() => toggleSection('director')}
              onCreate={onCreateStoryDirector}
            >
              {storyDirectors.map((director) => (
                <PresetDirectoryItem
                  key={director.id}
                  active={!isConfigAgentActive && resourceKind === 'director' && activeStoryDirectorId === director.id}
                  Icon={Compass}
                  title={director.name}
                  summary={[
                    `${presetStatusLabel(director, t)} · ${t('settingPanel.storyDirector.summaryCount', { count: storyDirectorSummaryCount(director) })}`,
                    director.strategy?.prompt_markdown?.trim() ? t('settingPanel.storyDirector.strategyPromptEnabled') : '',
                  ].filter(Boolean).join(' · ')}
                  onSelect={() => onSelectStoryDirector(director.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}

          {isVisible('teller') ? (
            <PresetDirectorySection
              kind="teller"
              label={presetKindDirectoryLabel('teller', t)}
              Icon={SlidersHorizontal}
              count={tellers.length}
              createLabel={presetKindCreateLabel('teller', t)}
              saving={saving}
              collapsed={isCollapsed('teller')}
              onToggle={() => toggleSection('teller')}
              onCreate={onCreateTeller}
            >
              {tellers.map((teller) => (
                <PresetDirectoryItem
                  key={teller.id}
                  active={!isConfigAgentActive && resourceKind === 'teller' && activeTellerId === teller.id}
                  Icon={SlidersHorizontal}
                  title={teller.name}
                  summary={`${presetStatusLabel(teller, t)} · ${t('settingPanel.enabledRules', { count: (teller.slots || []).filter((slot) => slot.enabled).length })}`}
                  onSelect={() => onSelectTeller(teller.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}

          {isVisible('image') ? (
            <PresetDirectorySection
              kind="image"
              label={presetKindDirectoryLabel('image', t)}
              Icon={Sparkles}
              count={imagePresets.length}
              createLabel={presetKindCreateLabel('image', t)}
              saving={saving}
              collapsed={isCollapsed('image')}
              onToggle={() => toggleSection('image')}
              onCreate={onCreateImagePreset}
            >
              {imagePresets.map((preset) => (
                <PresetDirectoryItem
                  key={preset.id}
                  active={!isConfigAgentActive && resourceKind === 'image' && activeImagePresetId === preset.id}
                  Icon={Sparkles}
                  title={preset.name}
                  summary={`${presetStatusLabel(preset, t)} · ${t('settingPanel.imagePreset.ruleCount', { count: enabledImagePresetSlotCount(preset), total: normalizedImagePresetSlots(preset).length })}`}
                  onSelect={() => onSelectImagePreset(preset.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}

          {isVisible('event') ? (
            <PresetDirectorySection
              kind="event"
              label={presetKindDirectoryLabel('event', t)}
              Icon={ScrollText}
              count={eventPackages.length}
              createLabel={presetKindCreateLabel('event', t)}
              saving={saving}
              collapsed={isCollapsed('event')}
              onToggle={() => toggleSection('event')}
              onCreate={onCreateEventPackage}
            >
              {eventPackages.map((item) => (
                <PresetDirectoryItem
                  key={item.id}
                  active={!isConfigAgentActive && resourceKind === 'event' && activeEventPackageId === item.id}
                  Icon={ScrollText}
                  title={item.name}
                  summary={`${presetStatusLabel(item, t)} · ${t('settingPanel.eventPackage.summaryCount', { count: eventPackageSummaryCount(item) })}`}
                  onSelect={() => onSelectEventPackage(item.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}

          {isVisible('rule') ? (
            <PresetDirectorySection
              kind="rule"
              label={presetKindDirectoryLabel('rule', t)}
              Icon={Dice5}
              count={ruleSystems.length}
              createLabel={presetKindCreateLabel('rule', t)}
              saving={saving}
              collapsed={isCollapsed('rule')}
              onToggle={() => toggleSection('rule')}
              onCreate={onCreateRuleSystem}
            >
              {ruleSystems.map((item) => (
                <PresetDirectoryItem
                  key={item.id}
                  active={!isConfigAgentActive && resourceKind === 'rule' && activeRuleSystemId === item.id}
                  Icon={Dice5}
                  title={item.name}
                  summary={`${presetStatusLabel(item, t)} · ${t('settingPanel.ruleSystem.summaryCount', { rules: item.trpg_system?.rule_templates?.length || 0 })}`}
                  onSelect={() => onSelectRuleSystem(item.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}

          {isVisible('actor-state') ? (
            <PresetDirectorySection
              kind="actor-state"
              label={presetKindDirectoryLabel('actor-state', t)}
              Icon={Database}
              count={actorStates.length}
              createLabel={presetKindCreateLabel('actor-state', t)}
              saving={saving}
              collapsed={isCollapsed('actor-state')}
              onToggle={() => toggleSection('actor-state')}
              onCreate={onCreateActorState}
            >
              {actorStates.map((item) => (
                <PresetDirectoryItem
                  key={item.id}
                  active={!isConfigAgentActive && resourceKind === 'actor-state' && activeActorStateId === item.id}
                  Icon={Database}
                  title={item.name}
                  summary={`${presetStatusLabel(item, t)} · ${t('settingPanel.actorState.summaryCount', { templates: item.actor_state?.templates?.length || 0, actors: item.actor_state?.initial_actors?.length || 0 })}`}
                  onSelect={() => onSelectActorState(item.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}

          {isVisible('memory-structure') ? (
            <PresetDirectorySection
              kind="memory-structure"
              label={presetKindDirectoryLabel('memory-structure', t)}
              Icon={Database}
              count={memoryStructures.length}
              createLabel={presetKindCreateLabel('memory-structure', t)}
              saving={saving}
              collapsed={isCollapsed('memory-structure')}
              onToggle={() => toggleSection('memory-structure')}
              onCreate={onCreateMemoryStructure}
            >
              {memoryStructures.map((item) => (
                <PresetDirectoryItem
                  key={item.id}
                  active={!isConfigAgentActive && resourceKind === 'memory-structure' && activeMemoryStructureId === item.id}
                  Icon={Database}
                  title={item.name}
                  summary={`${presetStatusLabel(item, t)} · ${t('settingPanel.memoryStructure.summaryCount', { enabled: enabledMemoryStructureCount(item), total: item.structures?.length || 0 })}`}
                  onSelect={() => onSelectMemoryStructure(item.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}

          {isVisible('opening') ? (
            <PresetDirectorySection
              kind="opening"
              label={presetKindDirectoryLabel('opening', t)}
              Icon={Sparkles}
              count={openingSelectors.length}
              createLabel={presetKindCreateLabel('opening', t)}
              saving={saving}
              collapsed={isCollapsed('opening')}
              onToggle={() => toggleSection('opening')}
              onCreate={onCreateOpeningSelector}
            >
              {openingSelectors.map((item) => (
                <PresetDirectoryItem
                  key={item.id}
                  active={!isConfigAgentActive && resourceKind === 'opening' && activeOpeningSelectorId === item.id}
                  Icon={Sparkles}
                  title={item.name}
                  summary={`${presetStatusLabel(item, t)} · ${t('settingPanel.openingSelector.summaryCount', { pools: item.opening_selector?.trait_pools?.length || 0, ops: item.opening_selector?.initial_state_ops?.length || 0 })}`}
                  onSelect={() => onSelectOpeningSelector(item.id)}
                />
              ))}
            </PresetDirectorySection>
          ) : null}
        </div>
      </ScrollArea>
    </>
  )
}

function PresetDirectorySection({
  kind,
  label,
  Icon,
  count,
  createLabel,
  saving,
  collapsed,
  onToggle,
  onCreate,
  children,
}: {
  kind: PresetResourceKind
  label: string
  Icon: LucideIcon
  count: number
  createLabel: string
  saving: boolean
  collapsed: boolean
  onToggle: () => void
  onCreate: () => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <section data-preset-kind={kind} className="min-w-0">
      <div className={`flex h-8 min-w-0 items-center gap-2 overflow-hidden rounded px-2 text-xs ${count > 0 ? 'text-[var(--nova-text-muted)]' : 'text-[var(--nova-text-faint)]'}`}>
        <button
          type="button"
          className="nova-nav-item shrink-0 rounded p-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={onToggle}
          aria-label={collapsed ? `${t('chat.tool.expand')}${label}` : `${t('chat.tool.collapse')}${label}`}
        >
          <ChevronDown className={`h-3.5 w-3.5 transition-transform ${collapsed ? '-rotate-90' : ''}`} />
        </button>
        <Icon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
        <button type="button" className="min-w-0 flex-1 truncate text-left font-medium" onClick={onToggle}>
          {label}
        </button>
        <span className="shrink-0 tabular-nums text-[11px] text-[var(--nova-text-faint)]">{count}</span>
        <button
          type="button"
          className="nova-nav-item shrink-0 rounded p-1 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          disabled={saving}
          onClick={onCreate}
          aria-label={createLabel}
          title={createLabel}
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>
      {!collapsed && (
        <div className="ml-5 space-y-0.5 border-l border-[var(--nova-border)] pl-2">
          {children}
        </div>
      )}
    </section>
  )
}

function PresetDirectoryItem({
  active,
  Icon,
  title,
  summary,
  onSelect,
}: {
  active: boolean
  Icon: LucideIcon
  title: string
  summary: string
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex min-h-9 w-full min-w-0 items-center gap-2 overflow-hidden rounded-md px-2 py-1 text-left text-xs transition ${
        active ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
      }`}
    >
      <Icon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
      <span className="min-w-0 flex-1 overflow-hidden">
        <span className="block truncate">{title}</span>
        <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{summary}</span>
      </span>
    </button>
  )
}

function defaultPresetSectionCollapsed(kind: PresetResourceKind, resourceKind: PresetResourceKind) {
  return kind !== resourceKind && kind !== 'director' && kind !== 'teller'
}

function presetStatusLabel(item: { custom?: boolean; builtin_overridden?: boolean }, t: (key: string) => string) {
  if (item.custom) return t('settingPanel.custom')
  if (item.builtin_overridden) return t('settingPanel.builtInOverridden')
  return t('settingPanel.builtIn')
}

export function EventPackageEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
  onValidityChange,
}: {
  draft: EventPackageModule | null
  tagDraft: string
  setDraft: (draft: EventPackageModule | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noEventPackageSelected')} description={t('settingPanel.editor.noEventPackageSelectedDesc')} />
  }

  return (
    <ModuleEditorShell draft={draft} tagDraft={tagDraft} setDraft={setDraft} setTagDraft={setTagDraft}>
      <PresetConfigSectionEditor
        sectionId="event-package.events"
        resetKey={`${draft.id}:events`}
        title={t('settingPanel.presetConfig.eventCards')}
        description={t('settingPanel.editor.eventPackageEventsDesc')}
        value={draft}
        summary={t('settingPanel.eventPackage.summaryCount', { count: eventPackageSummaryCount(draft) })}
        onChange={setDraft}
        onSave={onSave}
        onValidityChange={onValidityChange}
      >
        {(props) => <EventPackageVisualEditor {...props} />}
      </PresetConfigSectionEditor>
    </ModuleEditorShell>
  )
}

export function RuleSystemEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
  onValidityChange,
}: {
  draft: RuleSystemModule | null
  tagDraft: string
  setDraft: (draft: RuleSystemModule | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const setSectionValid = usePresetSectionValidity(draft?.id || '', onValidityChange)

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noRuleSystemSelected')} description={t('settingPanel.editor.noRuleSystemSelectedDesc')} />
  }

  return (
    <ModuleEditorShell draft={draft} tagDraft={tagDraft} setDraft={setDraft} setTagDraft={setTagDraft}>
      <PresetConfigSectionEditor
        sectionId="rule-system.trpg-system"
        resetKey={`${draft.id}:trpg_system`}
        title={t('settingPanel.storyDirector.trpgSystem')}
        description={t('settingPanel.storyDirector.trpgSystemDesc')}
        value={normalizeTRPGSystem(draft.trpg_system || { rule_templates: [] })}
        summary={t('settingPanel.storyDirector.trpgSystemSummary', { count: draft.trpg_system?.rule_templates?.length || 0 })}
        onChange={(trpg_system) => setDraft({ ...draft, trpg_system: normalizeTRPGSystem(trpg_system) })}
        onSave={onSave}
        onValidityChange={(valid) => setSectionValid('trpg_system', valid)}
      >
        {(props) => <TRPGSystemVisualEditor {...props} />}
      </PresetConfigSectionEditor>
    </ModuleEditorShell>
  )
}

export function ActorStateEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
  onValidityChange,
}: {
  draft: ActorStateModule | null
  tagDraft: string
  setDraft: (draft: ActorStateModule | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noActorStateSelected')} description={t('settingPanel.editor.noActorStateSelectedDesc')} />
  }

  return (
    <ModuleEditorShell draft={draft} tagDraft={tagDraft} setDraft={setDraft} setTagDraft={setTagDraft}>
      <PresetConfigSectionEditor
        sectionId="actor-state.actor-state"
        resetKey={`${draft.id}:actor_state`}
        title={t('settingPanel.actorState.title')}
        description={t('settingPanel.actorState.description')}
        value={draft.actor_state || { templates: [], initial_actors: [] }}
        summary={t('settingPanel.actorState.summaryCount', { templates: draft.actor_state?.templates?.length || 0, actors: draft.actor_state?.initial_actors?.length || 0 })}
        onChange={(actor_state) => setDraft({ ...draft, actor_state })}
        onSave={onSave}
        onValidityChange={onValidityChange}
      >
        {(props) => <ActorStateVisualEditor {...props} />}
      </PresetConfigSectionEditor>
    </ModuleEditorShell>
  )
}

export function StoryMemoryStructureEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
  onValidityChange,
}: {
  draft: StoryMemoryStructureModule | null
  tagDraft: string
  setDraft: (draft: StoryMemoryStructureModule | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noMemoryStructureSelected')} description={t('settingPanel.editor.noMemoryStructureSelectedDesc')} />
  }

  return (
    <ModuleEditorShell draft={draft} tagDraft={tagDraft} setDraft={setDraft} setTagDraft={setTagDraft}>
      <PresetConfigSectionEditor
        sectionId="story-memory-structure.structures"
        resetKey={`${draft.id}:structures`}
        title={t('settingPanel.memoryStructure.title')}
        description={t('settingPanel.memoryStructure.description')}
        value={draft.structures || []}
        summary={t('settingPanel.memoryStructure.summaryCount', { enabled: enabledMemoryStructureCount(draft), total: draft.structures?.length || 0 })}
        onChange={(structures) => setDraft({ ...draft, structures })}
        onSave={onSave}
        onValidityChange={onValidityChange}
      >
        {(props) => <MemoryStructureVisualEditor {...props} />}
      </PresetConfigSectionEditor>
    </ModuleEditorShell>
  )
}

export function OpeningSelectorEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
  onValidityChange,
}: {
  draft: OpeningSelectorModule | null
  tagDraft: string
  setDraft: (draft: OpeningSelectorModule | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
}) {
  const { t } = useTranslation()

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noOpeningSelectorSelected')} description={t('settingPanel.editor.noOpeningSelectorSelectedDesc')} />
  }

  return (
    <ModuleEditorShell draft={draft} tagDraft={tagDraft} setDraft={setDraft} setTagDraft={setTagDraft}>
      <PresetConfigSectionEditor
        sectionId="opening-selector.opening-selector"
        resetKey={`${draft.id}:opening_selector`}
        title={t('settingPanel.storyDirector.openingSelector')}
        description={t('settingPanel.storyDirector.openingSelectorDesc')}
        value={draft.opening_selector || { enabled: true, trait_pools: [], initial_state_ops: [] }}
        summary={t('settingPanel.storyDirector.openingSelectorSummary', { pools: draft.opening_selector?.trait_pools?.length || 0, ops: draft.opening_selector?.initial_state_ops?.length || 0 })}
        onChange={(opening_selector) => setDraft({ ...draft, opening_selector })}
        onSave={onSave}
        onValidityChange={onValidityChange}
      >
        {(props) => <OpeningSelectorVisualEditor {...props} />}
      </PresetConfigSectionEditor>
    </ModuleEditorShell>
  )
}

function ModuleEditorShell<T extends { name: string; description: string; custom: boolean; builtin_overridden?: boolean }>({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  children,
}: {
  draft: T
  tagDraft: string
  setDraft: (draft: T | null) => void
  setTagDraft: (value: string) => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)_180px_120px]">
        <Field label={t('settingPanel.field.name')}>
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </Field>
        <Field label={t('settingPanel.field.description')}>
          <Input className={inputClassName} value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} placeholder={t('settingPanel.placeholder.description')} />
        </Field>
        <Field label={t('settingPanel.field.tags')}>
          <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
        </Field>
        <div className="flex items-end">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs text-[var(--nova-text-faint)]">{presetStatusLabel(draft, t)}</span>
        </div>
      </div>
      <div className="grid gap-4 p-4">
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
          {draft.custom ? t('settingPanel.storyDirector.customEditable') : t('settingPanel.storyDirector.builtInCopyHint')}
        </div>
        {children}
      </div>
    </div>
  )
}

function storyDirectorSummaryCount(director: StoryDirector) {
  return directorEventCardCount(directorResolvedEventPackages(director))
    + (director.trpg_system?.rule_templates?.length || 0)
    + (director.opening_selector?.trait_pools?.length || 0)
}

function directorResolvedEventPackages(director: StoryDirector): TellerEventPackage[] {
  return director.event_packages?.length
    ? director.event_packages
    : director.resolved_snapshot?.event_packages?.length
      ? director.resolved_snapshot.event_packages
      : director.resolved_snapshot?.event_system?.event_packages || []
}

function directorEventCardCount(eventPackages: TellerEventPackage[] | undefined) {
  return (eventPackages || []).reduce((total, pkg) => total + (pkg.events?.length || 0), 0)
}

function eventPackageSummaryCount(item: EventPackageModule) {
  return item.events?.length || 0
}

function enabledMemoryStructureCount(item: Partial<StoryMemoryStructureModule>) {
  return (item.structures || []).filter((structure) => structure.enabled !== false).length
}

function presetKindDirectoryLabel(kind: PresetResourceKind, t: (key: string) => string) {
  if (kind === 'image') return t('settingPanel.imagePresetDirectory')
  if (kind === 'director') return t('settingPanel.storyDirectorDirectory')
  if (kind === 'event') return t('settingPanel.eventPackageDirectory')
  if (kind === 'rule') return t('settingPanel.ruleSystemDirectory')
  if (kind === 'actor-state') return t('settingPanel.actorStateDirectory')
  if (kind === 'memory-structure') return t('settingPanel.memoryStructureDirectory')
  if (kind === 'opening') return t('settingPanel.openingSelectorDirectory')
  return t('settingPanel.rulePackages')
}

function presetKindCreateLabel(kind: PresetResourceKind, t: (key: string) => string) {
  if (kind === 'image') return t('settingPanel.newImagePreset')
  if (kind === 'director') return t('settingPanel.newStoryDirector')
  if (kind === 'event') return t('settingPanel.newEventPackage')
  if (kind === 'rule') return t('settingPanel.newRuleSystem')
  if (kind === 'actor-state') return t('settingPanel.newActorState')
  if (kind === 'memory-structure') return t('settingPanel.newMemoryStructure')
  if (kind === 'opening') return t('settingPanel.newOpeningSelector')
  return t('settingPanel.newTeller')
}

function usePresetSectionValidity(resetKey: string, onValidityChange?: (valid: boolean) => void) {
  const [validity, setValidity] = useState<Record<string, boolean>>({})

  useEffect(() => {
    setValidity({})
  }, [resetKey])

  useEffect(() => {
    onValidityChange?.(Object.values(validity).every((valid) => valid !== false))
  }, [onValidityChange, validity])

  return useCallback((section: string, valid: boolean) => {
    setValidity((current) => {
      if (current[section] === valid) return current
      return { ...current, [section]: valid }
    })
  }, [])
}

export function ImagePresetEditor({
  draft,
  tagDraft,
  setDraft,
  setTagDraft,
  onSave,
}: {
  draft: ImagePreset | null
  tagDraft: string
  setDraft: (draft: ImagePreset | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  const [activeSlotId, setActiveSlotId] = useState('')
  const slots = draft ? normalizedImagePresetSlots(draft) : []
  const activeSlot = slots.find((slot) => slot.id === activeSlotId) || slots[0] || null
  const slotIDs = slots.map((slot) => slot.id).join('|')

  useEffect(() => {
    setActiveSlotId((current) => {
      if (current && slots.some((slot) => slot.id === current)) return current
      return slots[0]?.id || ''
    })
  }, [draft?.id, slotIDs])

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noImagePresetSelected')} description={t('settingPanel.editor.noImagePresetSelectedDesc')} />
  }

  const setSlots = (nextSlots: ImagePresetSlot[]) => {
    setDraft({ ...draft, slots: nextSlots, prompt: imagePresetPromptForTarget(nextSlots, 'tool_request'), version: 2 })
  }

  const updateSlotById = (slotId: string, patch: Partial<ImagePresetSlot>) => {
    setSlots(slots.map((slot) => (slot.id === slotId ? { ...slot, ...patch } : slot)))
  }

  const addSlot = () => {
    const id = `slot-${Date.now()}`
    const slot: ImagePresetSlot = {
      id,
      name: t('settingPanel.imagePreset.newRuleName'),
      target: 'tool_request',
      enabled: true,
      content: '',
    }
    setSlots([...slots, slot])
    setActiveSlotId(id)
  }

  const deleteSlot = () => {
    if (!activeSlot || slots.length <= 1) return
    const nextSlots = slots.filter((slot) => slot.id !== activeSlot.id)
    setSlots(nextSlots)
    setActiveSlotId(nextSlots[0]?.id || '')
  }

  const selectedTarget = activeSlot?.target || 'tool_request'
  const contentValue = activeSlot?.content || ''

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)_180px_120px]">
        <Field label={t('settingPanel.field.name')}>
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </Field>
        <Field label={t('settingPanel.field.description')}>
          <Input className={inputClassName} value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} placeholder={t('settingPanel.placeholder.description')} />
        </Field>
        <Field label={t('settingPanel.field.tags')}>
          <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
        </Field>
        <div className="flex items-end">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs text-[var(--nova-text-faint)]">{presetStatusLabel(draft, t)}</span>
        </div>
      </div>
      <div className="grid min-h-[520px] flex-1 grid-cols-1 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="flex max-h-60 min-h-0 flex-col overflow-hidden border-b border-[var(--nova-border)] bg-[var(--nova-surface)] lg:max-h-none lg:border-b-0 lg:border-r">
          <div className="flex h-11 items-center justify-between border-b border-[var(--nova-border)] px-3">
            <div className="text-xs font-medium text-[var(--nova-text-muted)]">{t('settingPanel.imagePreset.rulesTitle')}</div>
            <Button className={iconActionClassName} variant="outline" size="icon" onClick={addSlot} aria-label={t('settingPanel.injectRules.new')}>
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </div>
          <ScrollArea className="min-h-0 flex-1">
            <div className="p-2">
              {slots.map((slot) => (
                <div key={slot.id} className={`mb-1 flex min-h-12 w-full items-center gap-2 rounded-md border px-3 py-2 text-xs transition ${activeSlot?.id === slot.id ? 'border-[var(--nova-accent)]/45 bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_3px_0_0_var(--nova-accent)]' : 'border-transparent text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}>
                  <button type="button" onClick={() => setActiveSlotId(slot.id)} className="flex min-w-0 flex-1 items-center gap-2 text-left">
                    <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{slot.name}</span>
                      <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]">
                        <span className="truncate">{imagePresetTargetLabel(slot.target, t)}</span>
                        <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${slot.enabled ? 'bg-[var(--nova-accent-green)]' : 'bg-[var(--nova-text-faint)]/35'}`} />
                        <span className="shrink-0">{slot.enabled ? t('settingPanel.enabled') : t('settingPanel.disabled')}</span>
                      </span>
                    </span>
                  </button>
                  <Switch
                    checked={slot.enabled}
                    onCheckedChange={(enabled) => updateSlotById(slot.id, { enabled })}
                    aria-label={slot.enabled ? t('settingPanel.switch.disableRule') : t('settingPanel.switch.enableRule')}
                    title={slot.enabled ? t('settingPanel.switch.disableRule') : t('settingPanel.switch.enableRule')}
                  />
                </div>
              ))}
            </div>
          </ScrollArea>
        </aside>

        {activeSlot ? (
          <section className="flex min-h-0 flex-col">
            <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
              <div className="grid gap-3 lg:grid-cols-[minmax(220px,1fr)_minmax(240px,320px)_32px]">
                <Field label={t('settingPanel.field.ruleName')}>
                  <Input className={inputClassName} value={activeSlot.name} onChange={(event) => updateSlotById(activeSlot.id, { name: event.target.value })} />
                </Field>
                <Field label={t('settingPanel.field.injectTarget')}>
                  <Select value={selectedTarget} onValueChange={(value) => updateSlotById(activeSlot.id, { target: value as ImagePresetTarget })}>
                    <SelectTrigger className={selectClassName}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {IMAGE_PRESET_TARGET_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {imagePresetTargetLabel(option.value, t)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <div className="flex items-end justify-end">
                  <Button className={iconActionClassName} variant="outline" size="icon" disabled={slots.length <= 1} onClick={deleteSlot} aria-label={t('settingPanel.injectRules.delete')}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
                <div className="lg:col-span-3">
                  <div className="min-w-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5">
                    <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
                      <span>{imagePresetTargetLabel(selectedTarget, t)}</span>
                      <span className="h-1 w-1 rounded-full bg-[var(--nova-text-faint)]/50" />
                      <span className="text-[var(--nova-text-faint)]">{imagePresetTargetSummary(selectedTarget, t)}</span>
                    </div>
                    <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{imagePresetTargetDetail(selectedTarget, t)}</div>
                  </div>
                </div>
              </div>
            </div>
            <div className="min-h-[420px] flex-1 p-4 lg:min-h-0">
              <div className="mb-2 flex min-w-0 items-center justify-between gap-3">
                <span className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.imagePreset.ruleContent')}</span>
                <span className="shrink-0 font-mono text-[10px] text-[var(--nova-text-faint)]">{contentValue.length}/{IMAGE_PRESET_PROMPT_LIMIT}</span>
              </div>
              <Textarea
                autoResize={false}
                className="nova-field h-[calc(100%-1.75rem)] min-h-[360px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
                value={contentValue}
                maxLength={IMAGE_PRESET_PROMPT_LIMIT}
                onChange={(event) => updateSlotById(activeSlot.id, { content: event.target.value.slice(0, IMAGE_PRESET_PROMPT_LIMIT) })}
                placeholder={t('settingPanel.imagePreset.promptPlaceholder')}
                onKeyDown={(event) => {
                  if (isSaveShortcut(event)) {
                    event.preventDefault()
                    event.stopPropagation()
                    onSave()
                  }
                }}
              />
            </div>
          </section>
        ) : (
          <EmptyState title={t('settingPanel.injectRules.emptyTitle')} description={t('settingPanel.imagePreset.emptyRulesDesc')} />
        )}
      </div>
    </div>
  )
}

function normalizedImagePresetSlots(preset: Partial<ImagePreset> | null | undefined): ImagePresetSlot[] {
  if (!preset) return []
  const slots = Array.isArray(preset.slots) ? preset.slots : []
  if (slots.length > 0) {
    return slots.map((slot, index) => ({
      id: sanitizeImagePresetSlotId(slot.id) || `slot-${index + 1}`,
      name: slot.name?.trim() || sanitizeImagePresetSlotId(slot.id) || `slot-${index + 1}`,
      target: isImagePresetTarget(slot.target) ? slot.target : 'tool_request',
      enabled: slot.enabled !== false,
      content: (slot.content || '').slice(0, IMAGE_PRESET_PROMPT_LIMIT),
    }))
  }
  const prompt = preset.prompt?.trim() || ''
  return [{
    id: 'tool_request',
    name: '图像请求 Prompt',
    target: 'tool_request',
    enabled: true,
    content: prompt.slice(0, IMAGE_PRESET_PROMPT_LIMIT),
  }]
}

function enabledImagePresetSlotCount(preset: Partial<ImagePreset>) {
  return normalizedImagePresetSlots(preset).filter((slot) => slot.enabled).length
}

function imagePresetPromptForTarget(slots: ImagePresetSlot[], target: ImagePresetTarget) {
  return slots
    .filter((slot) => slot.enabled && slot.target === target && slot.content.trim())
    .map((slot) => `## ${slot.name}（${slot.target}）\n\n${slot.content.trim()}`)
    .join('\n\n')
}

function sanitizeImagePresetSlotId(id: string | undefined) {
  return (id || '').replace(/[^a-zA-Z0-9_-]/g, '').trim()
}

function isImagePresetTarget(value: string | undefined): value is ImagePresetTarget {
  return value === 'agent_system' || value === 'tool_request'
}

function imagePresetTargetLabel(target: ImagePresetTarget, t: (key: string) => string) {
  return t(`settingPanel.imagePreset.target.${target}`)
}

function imagePresetTargetSummary(target: ImagePresetTarget, t: (key: string) => string) {
  return t(`settingPanel.imagePreset.targetSummary.${target}`)
}

function imagePresetTargetDetail(target: ImagePresetTarget, t: (key: string) => string) {
  return t(`settingPanel.imagePreset.targetDetail.${target}`)
}

export function LoreEditor({
  draft,
  tagDraft,
  residentTotalChars,
  imagePresets,
  imagePresetId,
  imageInstruction,
  imageGenerating,
  setDraft,
  setTagDraft,
  onImagePresetChange,
  setImageInstruction,
  onGenerateImage,
  onClearImage,
  onSave,
}: {
  draft: LoreItem | null
  tagDraft: string
  residentTotalChars: number
  imagePresets: ImagePreset[]
  imagePresetId: string
  imageInstruction: string
  imageGenerating: boolean
  setDraft: (draft: LoreItem | null) => void
  setTagDraft: (value: string) => void
  onImagePresetChange: (id: string) => void
  setImageInstruction: (value: string) => void
  onGenerateImage: () => void
  onClearImage: () => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  const [imageDialogOpen, setImageDialogOpen] = useState(false)
  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noLoreSelected')} description={t('settingPanel.editor.noLoreSelectedDesc')} />
  }

  const residentItemChars = draft.enabled !== false && draft.load_mode === 'resident' ? (draft.content || '').length : 0
  const residentWarning = draft.enabled !== false && draft.load_mode === 'resident' && (residentItemChars > LORE_RESIDENT_ITEM_WARNING_CHARS || residentTotalChars > LORE_RESIDENT_TOTAL_WARNING_CHARS)
  const imagePath = draft.image?.image_path || ''
  const imageSrc = imagePath ? workspaceAssetURL(imagePath) : ''
  const hasImage = Boolean(imageSrc)
  const validImagePresets = imagePresets.filter((preset) => !preset.invalid)
  const selectedImagePresetId = imagePresetId || validImagePresets[0]?.id || 'game-cg'
  const openGenerateLabel = imagePath ? t('settingPanel.loreImage.openRegenerate') : t('settingPanel.loreImage.openGenerate')
  const topGridClassName = `grid shrink-0 items-stretch gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 ${
    hasImage ? 'lg:grid-cols-[15rem_minmax(0,1fr)] 2xl:grid-cols-[18rem_minmax(0,1fr)]' : 'lg:grid-cols-[12rem_minmax(0,1fr)] 2xl:grid-cols-[14rem_minmax(0,1fr)]'
  }`
  const imageColumnClassName = hasImage ? 'grid min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-1.5' : 'grid content-start gap-1.5'

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:overflow-hidden">
      <div className={topGridClassName}>
        <div className={imageColumnClassName}>
          <div className="flex min-w-0 items-center justify-between gap-2">
            <span className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.current')}</span>
            <Button className={iconActionClassName} variant="outline" size="icon-sm" disabled={imageGenerating} onClick={() => setImageDialogOpen(true)} aria-label={openGenerateLabel} title={openGenerateLabel}>
              {imageGenerating ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Sparkles className="h-3.5 w-3.5" />}
            </Button>
          </div>
          <LoreImageCompactControl
            imageSrc={imageSrc}
            title={draft.name || t('settingPanel.loreImage.current')}
            alt={draft.image?.alt_text || draft.name}
          />
        </div>
        <div className="grid min-w-0 gap-3">
          <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-[minmax(220px,1fr)_120px_150px_150px_170px]">
            <Field label={t('settingPanel.field.name')}>
              <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
            </Field>
            <BooleanSwitchField label={t('settingPanel.field.enabled')} checked={draft.enabled ?? true} onCheckedChange={(enabled) => setDraft({ ...draft, enabled })} />
            <Field label={t('settingPanel.field.type')}>
              <Select value={draft.type} onValueChange={(value) => setDraft({ ...draft, type: value as LoreItem['type'] })}>
                <SelectTrigger size="sm" className={selectClassName}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  {TYPE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>{loreTypeLabel(option.value, t)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('settingPanel.field.importance')}>
              <Select value={draft.importance} onValueChange={(value) => setDraft({ ...draft, importance: value as LoreItem['importance'] })}>
                <SelectTrigger size="sm" className={selectClassName}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  {IMPORTANCE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>{loreImportanceLabel(option.value, t)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('settingPanel.field.loadMode')}>
              <Select value={draft.load_mode || 'auto'} onValueChange={(value) => setDraft({ ...draft, load_mode: value as LoreItem['load_mode'] })}>
                <SelectTrigger size="sm" className={selectClassName}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  {LOAD_MODE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>{loreLoadModeLabel(option.value, t)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </div>
          <Field label={t('settingPanel.field.tags')}>
            <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
          </Field>
          <Field label={t('settingPanel.field.brief')}>
            <Textarea
              autoResize
              className="nova-field min-h-[4.5rem] resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
              value={draft.brief_description || ''}
              onChange={(event) => setDraft({ ...draft, brief_description: event.target.value })}
              placeholder={t('settingPanel.placeholder.brief')}
            />
          </Field>
          <div className="text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {draft.load_mode === 'resident' ? t('settingPanel.lore.residentDesc') : loadModeDescription(draft.load_mode, t)}
            {residentWarning ? <span className="ml-2 text-[var(--nova-danger)]">{t('settingPanel.lore.residentWarning')}</span> : null}
          </div>
        </div>
      </div>
      <div className="min-h-[420px] flex-1 p-4 md:min-h-0">
        <Textarea
          autoResize={false}
          className="nova-field h-full min-h-[360px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
          value={draft.content || ''}
          onChange={(event) => setDraft({ ...draft, content: event.target.value })}
          onKeyDown={(event) => {
            if (isSaveShortcut(event)) {
              event.preventDefault()
              event.stopPropagation()
              onSave()
            }
          }}
        />
      </div>
      <LoreImageGenerateDialog
        open={imageDialogOpen}
        itemName={draft.name || t('settingPanel.loreImage.current')}
        imagePath={imagePath}
        imagePresets={validImagePresets}
        imagePresetId={selectedImagePresetId}
        imageInstruction={imageInstruction}
        imageGenerating={imageGenerating}
        onOpenChange={setImageDialogOpen}
        onImagePresetChange={onImagePresetChange}
        setImageInstruction={setImageInstruction}
        onGenerateImage={onGenerateImage}
        onClearImage={onClearImage}
      />
    </div>
  )
}

function LoreImageCompactControl({
  imageSrc,
  title,
  alt,
}: {
  imageSrc: string
  title: string
  alt: string
}) {
  const { t } = useTranslation()

  if (!imageSrc) {
    return (
      <div className="flex min-h-14 min-w-0 items-center justify-center rounded-lg border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-faint)]">
        {t('settingPanel.loreImage.empty')}
      </div>
    )
  }

  return (
    <div className="flex h-full min-h-48 min-w-0 overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <ImagePreviewDialog src={imageSrc} title={title} alt={alt}>
        <button type="button" className="group h-full w-full overflow-hidden bg-[var(--nova-surface)]" aria-label={t('settingPanel.loreImage.openPreview')} title={t('settingPanel.loreImage.openPreview')}>
          <img src={imageSrc} alt={alt} className="h-full w-full object-cover transition group-hover:scale-[1.03]" />
        </button>
      </ImagePreviewDialog>
    </div>
  )
}

function LoreImageGenerateDialog({
  open,
  itemName,
  imagePath,
  imagePresets,
  imagePresetId,
  imageInstruction,
  imageGenerating,
  onOpenChange,
  onImagePresetChange,
  setImageInstruction,
  onGenerateImage,
  onClearImage,
}: {
  open: boolean
  itemName: string
  imagePath: string
  imagePresets: ImagePreset[]
  imagePresetId: string
  imageInstruction: string
  imageGenerating: boolean
  onOpenChange: (open: boolean) => void
  onImagePresetChange: (id: string) => void
  setImageInstruction: (value: string) => void
  onGenerateImage: () => void
  onClearImage: () => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[min(calc(100vw-2rem),560px)] gap-3 border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <DialogHeader>
          <DialogTitle>{imagePath ? t('settingPanel.loreImage.regenerate') : t('settingPanel.loreImage.generate')}</DialogTitle>
          <DialogDescription>{t('settingPanel.loreImage.dialogDesc', { name: itemName })}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-3">
          <Field label={t('settingPanel.loreImage.preset')}>
            <Select value={imagePresetId} onValueChange={onImagePresetChange} disabled={imageGenerating}>
              <SelectTrigger size="sm" className={selectClassName}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="nova-panel border text-[var(--nova-text)]">
                {imagePresets.length > 0 ? imagePresets.map((preset) => (
                  <SelectItem key={preset.id} value={preset.id}>{preset.name}</SelectItem>
                )) : (
                  <SelectItem value="game-cg">{t('settingPanel.editor.defaultImagePreset')}</SelectItem>
                )}
              </SelectContent>
            </Select>
          </Field>
          <Field label={t('settingPanel.loreImage.instruction')}>
            <Textarea
              className="nova-field min-h-28 resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
              value={imageInstruction}
              onChange={(event) => setImageInstruction(event.target.value)}
              placeholder={t('settingPanel.loreImage.instructionPlaceholder')}
              disabled={imageGenerating}
            />
          </Field>
        </div>

        <DialogFooter className="border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
          <Button className={actionButtonClassName} variant="outline" size="sm" disabled={!imagePath || imageGenerating} onClick={onClearImage}>
            <Trash2 className="h-4 w-4" />
            {t('settingPanel.loreImage.clear')}
          </Button>
          <Button className={actionButtonClassName} variant="outline" size="sm" disabled={imageGenerating} onClick={onGenerateImage}>
            {imageGenerating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
            {imagePath ? t('settingPanel.loreImage.regenerate') : t('settingPanel.loreImage.generate')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function CreatorEditor({
  content,
  setContent,
  onSave,
}: {
  content: string
  setContent: (value: string) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4">
      <Textarea
        autoResize={false}
        className="nova-field h-full min-h-[520px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
        value={content}
        onChange={(event) => setContent(event.target.value)}
        placeholder={t('settingPanel.placeholder.creator')}
        onKeyDown={(event) => {
          if (isSaveShortcut(event)) {
            event.preventDefault()
            event.stopPropagation()
            onSave()
          }
        }}
      />
    </div>
  )
}

export function OpeningPresetEditor({
  presets,
  activeId,
  setActiveId,
  setPresets,
  onSave,
}: {
  presets: BookOpeningPreset[]
  activeId: string
  setActiveId: (id: string) => void
  setPresets: (presets: BookOpeningPreset[]) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  const activePreset = presets.find((preset) => preset.id === activeId) || presets[0] || null
  const updateActivePreset = (patch: Partial<BookOpeningPreset>) => {
    if (!activePreset) return
    setPresets(presets.map((preset) => (preset.id === activePreset.id ? { ...preset, ...patch } : preset)))
  }
  const addPreset = () => {
    const preset = newBookOpeningPreset(t('settingPanel.openingPreset.defaultName', { number: presets.length + 1 }))
    setPresets([...presets, preset])
    setActiveId(preset.id)
  }
  const deleteActivePreset = () => {
    if (!activePreset) return
    const nextPresets = presets.filter((preset) => preset.id !== activePreset.id)
    setPresets(nextPresets)
    setActiveId(nextPresets[0]?.id || '')
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:overflow-hidden">
      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.openingPreset.title')}</div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.openingPreset.description')}</div>
          </div>
          <Button className={iconActionClassName} variant="outline" size="sm" onClick={addPreset}>
            <Plus className="h-3.5 w-3.5" />
            {t('settingPanel.openingPreset.add')}
          </Button>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <aside className="max-h-48 shrink-0 overflow-y-auto border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-2 md:max-h-none md:w-56 md:border-b-0 md:border-r">
          {presets.length === 0 ? (
            <div className="px-2 py-3 text-xs leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.openingPreset.empty')}</div>
          ) : (
            <div className="space-y-1">
              {presets.map((preset) => (
                <button
                  key={preset.id}
                  type="button"
                  onClick={() => setActiveId(preset.id)}
                  className={`flex min-h-8 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition ${
                    activePreset?.id === preset.id ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
                  }`}
                >
                  <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate">{preset.title || t('settingPanel.openingPreset.untitled')}</span>
                </button>
              ))}
            </div>
          )}
        </aside>
        <div className="min-h-[420px] flex-1 p-4 md:min-h-0">
          {activePreset ? (
            <div className="flex h-full min-h-0 flex-col gap-3">
              <div className="flex items-end gap-3">
                <Field className="min-w-0 flex-1" label={t('settingPanel.openingPreset.name')}>
                  <Input className={inputClassName} value={activePreset.title} onChange={(event) => updateActivePreset({ title: event.target.value })} placeholder={t('settingPanel.openingPreset.untitled')} />
                </Field>
                <Button className={iconActionClassName} variant="outline" size="icon" onClick={deleteActivePreset} aria-label={t('settingPanel.openingPreset.delete')}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
              <Textarea
                autoResize={false}
                className="nova-field min-h-0 flex-1 resize-none text-sm leading-7 shadow-none focus-visible:ring-0"
                value={activePreset.content}
                onChange={(event) => updateActivePreset({ content: event.target.value })}
                placeholder={t('settingPanel.openingPreset.placeholder')}
                onKeyDown={(event) => {
                  if (isSaveShortcut(event)) {
                    event.preventDefault()
                    event.stopPropagation()
                    onSave()
                  }
                }}
              />
            </div>
          ) : (
            <EmptyState title={t('settingPanel.openingPreset.emptyTitle')} description={t('settingPanel.openingPreset.emptyDesc')} />
          )}
        </div>
      </div>
    </div>
  )
}

function sectionItems(items: LoreItem[], section: KnowledgeSection, query = '') {
  const normalizedQuery = query.trim().toLowerCase()
  return items.filter((item) => {
    if (!section.types.includes(item.type)) return false
    const tags = item.tags || []
    if (section.tag && !tags.includes(section.tag)) return false
    if (section.excludeTag && tags.includes(section.excludeTag)) return false
    if (normalizedQuery) {
      const haystack = [item.name, item.brief_description || '', item.content || '', tags.join('\n')].join('\n').toLowerCase()
      if (!haystack.includes(normalizedQuery)) return false
    }
    return true
  })
}

function loreTypeLabel(type: LoreItem['type'], t: (key: string) => string) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.other') : label
}

function loreImportanceLabel(importance: LoreItem['importance'], t: (key: string) => string) {
  const key = `lore.importance.${importance}`
  const label = t(key)
  return label === key ? t('lore.importance.important') : label
}

function loreLoadModeLabel(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  const key = `lore.loadMode.${loadMode || 'auto'}`
  const label = t(key)
  return label === key ? t('lore.loadMode.auto') : label
}

function loadModeDescription(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  if (loadMode === 'resident') return t('settingPanel.lore.residentDesc')
  if (loadMode === 'manual') return t('settingPanel.lore.manualDesc')
  if (loadMode === 'auto') return t('settingPanel.lore.autoDesc')
  return t('settingPanel.lore.indexDesc')
}

function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return (
    <label className={`grid gap-1.5 ${className}`}>
      <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
        <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
        <div className="mt-1 text-xs text-[var(--nova-text-faint)]">{description}</div>
      </div>
    </div>
  )
}
