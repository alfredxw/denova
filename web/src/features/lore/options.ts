import type { LoreItem } from '@/lib/api'

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

export function loreTypeLabel(
  type: LoreItem['type'],
  t: (key: string) => string,
) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.other') : label
}

export function loreImportanceLabel(
  importance: LoreItem['importance'],
  t: (key: string) => string,
) {
  const key = `lore.importance.${importance}`
  const label = t(key)
  return label === key ? t('lore.importance.important') : label
}

export function loreLoadModeLabel(
  loadMode: LoreItem['load_mode'] | undefined,
  t: (key: string) => string,
) {
  const key = `lore.loadMode.${loadMode || 'auto'}`
  const label = t(key)
  return label === key ? t('lore.loadMode.auto') : label
}

export function loadModeDescription(
  loadMode: LoreItem['load_mode'] | undefined,
  t: (key: string) => string,
) {
  if (loadMode === 'resident') return t('settingPanel.lore.residentDesc')
  if (loadMode === 'manual') return t('settingPanel.lore.manualDesc')
  if (loadMode === 'auto') return t('settingPanel.lore.autoDesc')
  return t('settingPanel.lore.indexDesc')
}
