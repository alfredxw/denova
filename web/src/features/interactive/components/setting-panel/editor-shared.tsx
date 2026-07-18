import type { LoreItem } from '@/lib/api'
import type { PresetResourceKind } from '../../preset-ownership'
import type { EventPackageModule, StoryDirector, TellerEventPackage } from '../../types'

export const TYPE_OPTIONS = [
  { value: 'character' },
  { value: 'world' },
  { value: 'location' },
  { value: 'faction' },
  { value: 'rule' },
  { value: 'item' },
  { value: 'other' },
] as const
export const IMPORTANCE_OPTIONS = [
  { value: 'major' },
  { value: 'important' },
  { value: 'minor' },
] as const
export const LOAD_MODE_OPTIONS = [
  { value: 'resident' },
  { value: 'auto' },
  { value: 'manual' },
] as const
export const LORE_RESIDENT_TOTAL_WARNING_BYTES = 32 * 1024

export function loreTypeLabel(type: LoreItem['type'], t: (key: string) => string) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.other') : label
}

export function loreImportanceLabel(importance: LoreItem['importance'], t: (key: string) => string) {
  const key = `lore.importance.${importance}`
  const label = t(key)
  return label === key ? t('lore.importance.important') : label
}

export function loreLoadModeLabel(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  const key = `lore.loadMode.${loadMode || 'auto'}`
  const label = t(key)
  return label === key ? t('lore.loadMode.auto') : label
}

export function loadModeDescription(loadMode: LoreItem['load_mode'] | undefined, t: (key: string) => string) {
  if (loadMode === 'resident') return t('settingPanel.lore.residentDesc')
  if (loadMode === 'manual') return t('settingPanel.lore.manualDesc')
  if (loadMode === 'auto') return t('settingPanel.lore.autoDesc')
  return t('settingPanel.lore.indexDesc')
}

export function storyDirectorSummaryCount(director: StoryDirector) {
  return directorEventCardCount(directorResolvedEventPackages(director))
    + (director.trpg_system?.rule_templates?.length || 0)
}

function directorResolvedEventPackages(director: StoryDirector): TellerEventPackage[] {
  return director.event_packages?.length
    ? director.event_packages
    : director.resolved_snapshot?.event_packages?.length
      ? director.resolved_snapshot.event_packages
      : []
}

function directorEventCardCount(eventPackages: TellerEventPackage[] | undefined) {
  return (eventPackages || []).reduce((total, pkg) => total + (pkg.events?.length || 0), 0)
}

export function eventPackageSummaryCount(item: EventPackageModule) {
  return item.events?.length || 0
}

export function presetKindDirectoryLabel(kind: PresetResourceKind, t: (key: string) => string) {
  if (kind === 'image') return t('settingPanel.imagePresetDirectory')
  if (kind === 'director') return t('settingPanel.storyDirectorDirectory')
  if (kind === 'event') return t('settingPanel.eventPackageDirectory')
  if (kind === 'rule') return t('settingPanel.ruleSystemDirectory')
  if (kind === 'actor-state') return t('settingPanel.actorStateDirectory')
  return t('settingPanel.rulePackages')
}

export function presetKindCreateLabel(kind: PresetResourceKind, t: (key: string) => string) {
  if (kind === 'image') return t('settingPanel.newImagePreset')
  if (kind === 'director') return t('settingPanel.newStoryDirector')
  if (kind === 'event') return t('settingPanel.newEventPackage')
  if (kind === 'rule') return t('settingPanel.newRuleSystem')
  if (kind === 'actor-state') return t('settingPanel.newActorState')
  return t('settingPanel.newTeller')
}
