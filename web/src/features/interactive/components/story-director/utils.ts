import type { StoryDirector, StoryDirectorModuleRefs, TellerEventPackage } from '../../types'

export function strategyOptionText(t: (key: string, values?: Record<string, string>) => string, key: string, value: string): string {
  return t(key, { value })
}

export function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

export function normalizedStoryDirectorRefs(refs: StoryDirectorModuleRefs | undefined): StoryDirectorModuleRefs {
  return {
    narrative_style_id: refs?.narrative_style_id || 'rhythm',
    narrative_style_disabled: refs?.narrative_style_disabled === true,
    event_package_ids: normalizeIDList(refs?.event_package_ids?.length ? refs.event_package_ids : ['default']),
    event_packages_disabled: refs?.event_packages_disabled === true,
    rule_system_id: refs?.rule_system_id || 'default',
    rule_system_disabled: refs?.rule_system_disabled === true,
    actor_state_id: refs?.actor_state_id || 'default',
    actor_state_disabled: refs?.actor_state_disabled === true,
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
      : []
}

export function findById<T extends { id: string }>(items: T[], id: string): T | undefined {
  return items.find((item) => item.id === id)
}
