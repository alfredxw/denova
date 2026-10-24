import type { TFunction } from 'i18next'
import type { RuleSystemModule } from './types'

const BUILTIN_RULE_SYSTEM_IDS = new Set([
  'default',
  'dm-cinematic-heroic',
  'dm-drama-stakes',
  'dm-fail-forward',
  'dm-gritty-survival',
  'dm-mystery-clue-forward',
  'dm-osr-player-skill',
])

const CHINESE_FALLBACKS: Record<string, string> = {
  '均衡 DM 检定': 'default',
  '电影英雄型 DM：高光优先': 'dm-cinematic-heroic',
  '戏剧张力型 DM：只为重大赌注掷骰': 'dm-drama-stakes',
  '推进型 DM：失败也前进': 'dm-fail-forward',
  '硬核生存型 DM：资源与后果': 'dm-gritty-survival',
  '悬疑调查型 DM：线索不断线': 'dm-mystery-clue-forward',
  'OSR 型 DM：玩家技巧优先': 'dm-osr-player-skill',
}

/** Resolve localized rule-system preset name while preserving custom user presets. */
export function ruleSystemName(
  item: Pick<RuleSystemModule, 'id'> & Partial<Pick<RuleSystemModule, 'name' | 'custom' | 'builtin_overridden'>>,
  t: TFunction,
): string {
  if (item.custom || item.builtin_overridden) {
    return item.name || item.id
  }

  const normalizedId = item.id || (item.name ? CHINESE_FALLBACKS[item.name] : '')
  if (normalizedId && BUILTIN_RULE_SYSTEM_IDS.has(normalizedId)) {
    const key = `settingPanel.ruleSystem.builtin.${normalizedId}.name`
    const localized = t(key, { defaultValue: '' })
    if (localized) return localized
  }

  return item.name || item.id
}
