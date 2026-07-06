export type PresetResourceKind = 'teller' | 'event' | 'rule' | 'actor-state' | 'memory-structure' | 'opening' | 'director' | 'image'

export type PresetModuleOwnership = 'shared' | 'gameOnly' | 'writingOnly'
export type PresetUsageMode = 'writing' | 'game'

export const PRESET_RESOURCE_OWNERSHIP: Record<PresetResourceKind, PresetModuleOwnership> = {
  teller: 'shared',
  image: 'shared',
  director: 'gameOnly',
  event: 'gameOnly',
  rule: 'gameOnly',
  'actor-state': 'gameOnly',
  'memory-structure': 'gameOnly',
  opening: 'gameOnly',
}

export const SHARED_PRESET_RESOURCE_KINDS: PresetResourceKind[] = ['teller', 'image']
export const GAME_ONLY_PRESET_RESOURCE_KINDS: PresetResourceKind[] = ['director', 'event', 'rule', 'actor-state', 'memory-structure', 'opening']
export const WRITING_ONLY_PRESET_RESOURCE_KINDS: PresetResourceKind[] = []

export function presetModuleOwnership(kind: PresetResourceKind): PresetModuleOwnership {
  return PRESET_RESOURCE_OWNERSHIP[kind]
}

export function presetResourceVisibleInMode(kind: PresetResourceKind, mode: PresetUsageMode): boolean {
  const ownership = presetModuleOwnership(kind)
  if (ownership === 'shared') return true
  if (mode === 'writing') return ownership === 'writingOnly'
  return ownership === 'gameOnly'
}
