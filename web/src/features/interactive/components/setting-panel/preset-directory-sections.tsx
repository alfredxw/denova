/** 方案预设目录的数据组装：6 类资源 → ResourceDirectorySection[]，含目录条目复合 id（`kind:itemId`）编解码。 */
import { Compass, Database, Dice5, ScrollText, SlidersHorizontal, Sparkles } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { TFunction } from 'i18next'
import type { ResourceDirectoryItem, ResourceDirectorySection } from '@/components/resource-directory/types'
import { presetResourceVisibleInMode, type PresetResourceKind, type PresetUsageMode } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../../types'
import { enabledImagePresetSlotCount, normalizedImagePresetSlots } from './ImagePresetEditor'
import { eventPackageSummaryCount, presetKindCreateLabel, presetKindDirectoryLabel, presetStatusLabel, storyDirectorSummaryCount } from './editor-shared'

const PRESET_DIRECTORY_ORDER: PresetResourceKind[] = ['director', 'teller', 'image', 'event', 'rule', 'actor-state']

const PRESET_DIRECTORY_ICONS: Record<PresetResourceKind, LucideIcon> = {
  director: Compass,
  teller: SlidersHorizontal,
  image: Sparkles,
  event: ScrollText,
  rule: Dice5,
  'actor-state': Database,
}

/** 目录条目 id 采用 `kind:itemId` 复合形式，避免跨资源 id 冲突。 */
export function presetDirectoryEntryId(kind: PresetResourceKind, itemId: string) {
  return `${kind}:${itemId}`
}

export function parsePresetDirectoryEntryId(entryId: string): { kind: PresetResourceKind; itemId: string } | null {
  const separatorIndex = entryId.indexOf(':')
  if (separatorIndex <= 0) return null
  return { kind: entryId.slice(0, separatorIndex) as PresetResourceKind, itemId: entryId.slice(separatorIndex + 1) }
}

interface PresetDirectoryLists {
  tellers: Teller[]
  storyDirectors: StoryDirector[]
  imagePresets: ImagePreset[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
}

/** 按可见模式过滤分组并组装目录 sections；onCreateKind 负责组级新建。 */
export function buildPresetDirectorySections({
  lists,
  presetUsageMode,
  presetResourceKind,
  onCreateKind,
  t,
}: {
  lists: PresetDirectoryLists
  presetUsageMode: PresetUsageMode
  presetResourceKind: PresetResourceKind
  onCreateKind: (kind: PresetResourceKind) => void
  t: TFunction
}): ResourceDirectorySection[] {
  return PRESET_DIRECTORY_ORDER
    .filter((kind) => presetResourceVisibleInMode(kind, presetUsageMode))
    .map((kind) => ({
      id: kind,
      label: presetKindDirectoryLabel(kind, t),
      icon: PRESET_DIRECTORY_ICONS[kind],
      items: presetDirectoryItemsForKind(kind, lists, t),
      onCreate: () => onCreateKind(kind),
      createLabel: presetKindCreateLabel(kind, t),
      defaultCollapsed: kind !== presetResourceKind,
    }))
}

function presetDirectoryItemsForKind(kind: PresetResourceKind, lists: PresetDirectoryLists, t: TFunction): ResourceDirectoryItem[] {
  const { tellers, storyDirectors, imagePresets, eventPackages, ruleSystems, actorStates } = lists
  if (kind === 'director') {
    return storyDirectors.map((director) => ({
      id: presetDirectoryEntryId('director', director.id),
      title: director.name,
      summary: [
        `${presetStatusLabel(director, t)} · ${t('settingPanel.storyDirector.summaryCount', { count: storyDirectorSummaryCount(director) })}`,
        director.strategy?.prompt_markdown?.trim() ? t('settingPanel.storyDirector.strategyPromptEnabled') : '',
      ].filter(Boolean).join(' · '),
      searchText: director.description || '',
    }))
  }
  if (kind === 'teller') {
    return tellers.map((teller) => ({
      id: presetDirectoryEntryId('teller', teller.id),
      title: teller.name,
      summary: `${presetStatusLabel(teller, t)} · ${t('settingPanel.enabledRules', { count: (teller.slots || []).filter((slot) => slot.enabled).length })}`,
      searchText: teller.description || '',
    }))
  }
  if (kind === 'image') {
    return imagePresets.map((preset) => ({
      id: presetDirectoryEntryId('image', preset.id),
      title: preset.name,
      summary: `${presetStatusLabel(preset, t)} · ${t('settingPanel.imagePreset.ruleCount', { count: enabledImagePresetSlotCount(preset), total: normalizedImagePresetSlots(preset).length })}`,
      searchText: preset.description || '',
    }))
  }
  if (kind === 'event') {
    return eventPackages.map((item) => ({
      id: presetDirectoryEntryId('event', item.id),
      title: item.name,
      summary: `${presetStatusLabel(item, t)} · ${t('settingPanel.eventPackage.summaryCount', { count: eventPackageSummaryCount(item) })}`,
      searchText: item.description || '',
    }))
  }
  if (kind === 'rule') {
    return ruleSystems.map((item) => ({
      id: presetDirectoryEntryId('rule', item.id),
      title: item.name,
      summary: t('settingPanel.trpgRule.directorySummary', {
        policy: t(`settingPanel.trpgRule.failurePolicy.${item.trpg_system?.rule_templates?.[0]?.failure_policy || 'fail_forward'}`),
        state: actorStates.find((state) => state.id === item.actor_state_id)?.name || t('settingPanel.trpgRule.noActorStateBinding'),
      }),
      searchText: item.description || '',
    }))
  }
  return actorStates.map((item) => ({
    id: presetDirectoryEntryId('actor-state', item.id),
    title: item.name,
    summary: t('settingPanel.actorState.directorySummary', {
      templates: item.actor_state?.templates?.length || 0,
      checks: ruleSystems.filter((rule) => rule.actor_state_id === item.id).length,
    }),
    searchText: item.description || '',
  }))
}
