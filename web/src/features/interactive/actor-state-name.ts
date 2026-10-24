import type { TFunction } from 'i18next'
import type { ActorStateModule } from './types'

const BUILTIN_ACTOR_STATE_IDS = new Set([
  'default',
  'xiuxian-state',
  'western-fantasy-state',
  'apocalypse-state',
  'infinite-flow-state',
])

const CHINESE_FALLBACKS: Record<string, string> = {
  '默认状态系统': 'default',
  '修仙状态系统': 'xiuxian-state',
  '西幻状态系统': 'western-fantasy-state',
  '末世状态系统': 'apocalypse-state',
  '无限流状态系统': 'infinite-flow-state',
}

/** Resolve localized actor-state preset name while preserving custom user presets. */
export function actorStateName(
  item: Pick<ActorStateModule, 'id'> & Partial<Pick<ActorStateModule, 'name' | 'custom' | 'builtin_overridden'>>,
  t: TFunction,
): string {
  if (item.custom || item.builtin_overridden) {
    return item.name || item.id
  }

  const normalizedId = item.id || (item.name ? CHINESE_FALLBACKS[item.name] : '')
  if (normalizedId && BUILTIN_ACTOR_STATE_IDS.has(normalizedId)) {
    const key = `settingPanel.actorState.builtin.${normalizedId}.name`
    const localized = t(key, { defaultValue: '' })
    if (localized) return localized
  }

  return item.name || item.id
}
