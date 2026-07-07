import type { RuleCheck, StoryDirectorTRPGSystem } from '../../types'

export const RULE_CATEGORY_OPTIONS = ['generic_action', 'combat', 'stealth', 'exploration', 'social', 'endurance', 'magic'] as const
export const RULE_DIFFICULTY_OPTIONS = ['very_easy', 'easy', 'normal', 'hard', 'very_hard'] as const
export const RULE_ROLL_MODE_OPTIONS = ['normal', 'advantage', 'disadvantage'] as const
export const RULE_FAILURE_POLICY_OPTIONS = ['fail_forward', 'success_at_cost', 'blocked', 'hard_failure'] as const
export const RULE_IMPACT_OPTIONS = ['none', 'hp_damage', 'stamina_cost', 'relationship_change', 'clue_progress', 'resource_change', 'custom'] as const

const DEFAULT_RULE_TEMPLATES: RuleCheck[] = [
  {
    id: 'high-risk-action',
    label: '高风险行动',
    category: 'generic_action',
    default_difficulty: 'normal',
    default_roll_mode: 'normal',
    failure_policy: 'fail_forward',
    impact: 'none',
    trigger: '玩家采取存在明显风险、代价或不确定结果的行动，且直接叙述裁定会削弱互动张力时使用。',
    success_hint: '成功时让行动达成核心目标，并给出清晰收益或信息推进。',
    failure_hint: '失败时继续推进局势，但引入新的代价、压力或选择。',
  },
  {
    id: 'combat-exchange',
    label: '战斗攻防',
    category: 'combat',
    default_difficulty: 'normal',
    default_roll_mode: 'normal',
    failure_policy: 'fail_forward',
    impact: 'hp_damage',
    trigger: '攻击、防御、闪避、夺取战斗位置或承受敌方压制时使用。',
    success_hint: '成功时扩大优势、造成有效压制或争取战斗节奏。',
    failure_hint: '失败时让角色承受伤害、失去位置或暴露破绽，但保留下一步行动空间。',
  },
  {
    id: 'stealth-lock',
    label: '潜行与开锁',
    category: 'stealth',
    default_difficulty: 'normal',
    default_roll_mode: 'normal',
    failure_policy: 'blocked',
    impact: 'stamina_cost',
    trigger: '潜行、开锁、拆陷阱、绕过守卫或进行精细操作时使用。',
    success_hint: '成功时悄无声息地越过阻碍，并保留行动主动权。',
    failure_hint: '失败时让操作受阻、消耗时间或体力，并提高被发现的风险。',
  },
  {
    id: 'investigation',
    label: '探索调查',
    category: 'exploration',
    default_difficulty: 'normal',
    default_roll_mode: 'normal',
    failure_policy: 'fail_forward',
    impact: 'clue_progress',
    trigger: '搜索线索、辨认异常、分析痕迹、探索未知区域或判断信息真伪时使用。',
    success_hint: '成功时给出可行动的关键线索，并连接到下一步选择。',
    failure_hint: '失败时仍给出方向，但附带误导、遗漏、额外风险或时间压力。',
  },
  {
    id: 'social-negotiation',
    label: '社交谈判',
    category: 'social',
    default_difficulty: 'normal',
    default_roll_mode: 'normal',
    failure_policy: 'success_at_cost',
    impact: 'relationship_change',
    trigger: '说服、威胁、交易、套话、安抚、挑衅或争取 NPC 支持时使用。',
    success_hint: '成功时让对方让步、透露信息或提供有限帮助。',
    failure_hint: '失败时可以达成部分目标，但提高条件、暴露意图或损害关系。',
  },
  {
    id: 'endurance-pressure',
    label: '体力/意志抗压',
    category: 'endurance',
    default_difficulty: 'hard',
    default_roll_mode: 'normal',
    failure_policy: 'success_at_cost',
    impact: 'stamina_cost',
    trigger: '长时间奔逃、忍痛、抵抗诱惑/恐惧、强撑伤势或突破极限时使用。',
    success_hint: '成功时角色稳住状态并争取关键窗口。',
    failure_hint: '失败时允许勉强撑过，但留下疲惫、伤势、心理压力或后续劣势。',
  },
]

export function defaultRuleTemplates(): RuleCheck[] {
  return DEFAULT_RULE_TEMPLATES.map((template) => ({ ...template }))
}

export function normalizeRuleTemplate(item: Partial<RuleCheck>, index = 0): RuleCheck {
  const id = String(item.id || `rule-${index + 1}`).trim()
  return {
    id,
    label: String(item.label || id).trim(),
    category: optionOrDefault(RULE_CATEGORY_OPTIONS, item.category, 'generic_action'),
    default_difficulty: optionOrDefault(RULE_DIFFICULTY_OPTIONS, item.default_difficulty, 'normal'),
    default_roll_mode: optionOrDefault(RULE_ROLL_MODE_OPTIONS, item.default_roll_mode, 'normal'),
    failure_policy: optionOrDefault(RULE_FAILURE_POLICY_OPTIONS, item.failure_policy, 'fail_forward'),
    impact: optionOrDefault(RULE_IMPACT_OPTIONS, item.impact, 'none'),
    trigger: String(item.trigger || ''),
    success_hint: String(item.success_hint || ''),
    failure_hint: String(item.failure_hint || ''),
  }
}

export function normalizeTRPGSystem(value: StoryDirectorTRPGSystem | undefined): StoryDirectorTRPGSystem {
  return { rule_templates: (value?.rule_templates || []).map((item, index) => normalizeRuleTemplate(item, index)) }
}

function optionOrDefault<T extends readonly string[]>(options: T, value: unknown, fallback: T[number]): T[number] {
  return options.includes(String(value) as T[number]) ? String(value) as T[number] : fallback
}
