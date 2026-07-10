import type { StoryDirector, StoryDirectorActorStateSystem, StoryDirectorModuleRefs, StoryDirectorTRPGSystem, TellerEventPackage } from '../../types'
import { DIRECTOR_PLAN_REQUIRED_HEADINGS, STORY_DIRECTOR_BRANCH_PLANNING_TURNS_FALLBACK, STORY_DIRECTOR_PLANNING_TEMPLATE_LIMIT } from './constants'

export function parseDecimalInput(value: string) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

export function normalizeBranchPlanningTurns(value: string) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return STORY_DIRECTOR_BRANCH_PLANNING_TURNS_FALLBACK
  return Math.min(12, Math.max(1, Math.round(parsed)))
}

export function validateDirectorPlanningTemplate(value: string) {
  const bytes = utf8ByteLength(value || '')
  if (!String(value || '').trim()) {
    return { bytes, missingHeadings: [], valid: true }
  }
  const missingHeadings = DIRECTOR_PLAN_REQUIRED_HEADINGS.filter((heading) => !String(value || '').includes(heading))
  return {
    bytes,
    missingHeadings,
    valid: bytes <= STORY_DIRECTOR_PLANNING_TEMPLATE_LIMIT && missingHeadings.length === 0,
  }
}

export function strategyRateValue(value: number | undefined, fallbackValue: string): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallbackValue
  const clamped = Math.min(1, Math.max(0, value))
  return String(clamped)
}

export function strategyOptionText(t: (key: string, values?: Record<string, string>) => string, key: string, value: string): string {
  return t(key, { value })
}

export function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

export function normalizedStoryDirectorRefs(refs: StoryDirectorModuleRefs | undefined): StoryDirectorModuleRefs {
  const legacyEventPackageID = refs?.event_system_id || ''
  const eventPackageIDs = refs?.event_package_ids?.length
    ? refs.event_package_ids
    : legacyEventPackageID
      ? [legacyEventPackageID]
      : ['default']
  return {
    narrative_style_id: refs?.narrative_style_id || 'classic',
    narrative_style_disabled: refs?.narrative_style_disabled === true,
    event_package_ids: normalizeIDList(eventPackageIDs),
    event_packages_disabled: refs?.event_packages_disabled === true || refs?.event_system_disabled === true,
    rule_system_id: refs?.rule_system_id || 'default',
    rule_system_disabled: refs?.rule_system_disabled === true,
    actor_state_id: refs?.actor_state_id || 'default',
    actor_state_disabled: refs?.actor_state_disabled === true,
    memory_structure_id: refs?.memory_structure_id || 'default',
    memory_structure_disabled: refs?.memory_structure_disabled === true,
    image_preset_id: refs?.image_preset_id || 'game-cg',
    image_preset_disabled: refs?.image_preset_disabled === true,
  }
}

export function normalizeIDList(ids: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const raw of ids) {
    const id = raw.trim()
    if (!id || seen.has(id)) continue
    seen.add(id)
    result.push(id)
  }
  return result
}

export function directorResolvedEventPackages(director: StoryDirector): TellerEventPackage[] {
  return director.event_packages?.length
    ? director.event_packages
    : director.resolved_snapshot?.event_packages?.length
      ? director.resolved_snapshot.event_packages
      : director.resolved_snapshot?.event_system?.event_packages || []
}

export function newEmptyStoryDirectorSections(): {
  trpg_system: StoryDirectorTRPGSystem
  actor_state: StoryDirectorActorStateSystem
} {
  return {
    trpg_system: { rule_templates: [] },
    actor_state: { templates: [], trait_pools: [], initial_actors: [] },
  }
}

export function findById<T extends { id: string }>(items: T[], id: string): T | undefined {
  return items.find((item) => item.id === id)
}

export function presetStatusLabel(item: { custom?: boolean; builtin_overridden?: boolean }, t: (key: string) => string) {
  if (item.custom) return t('settingPanel.custom')
  if (item.builtin_overridden) return t('settingPanel.builtInOverridden')
  return t('settingPanel.builtIn')
}
