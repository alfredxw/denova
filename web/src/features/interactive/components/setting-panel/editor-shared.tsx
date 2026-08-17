import type { PresetResourceKind } from '../../preset-ownership'
import type { EventPackageModule, StoryDirector, TellerEventPackage } from '../../types'

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
