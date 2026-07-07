import type { RuleCheck, StoryDirectorTRPGSystem } from '../../types'

export const RULE_DICE_OPTIONS = ['1d20', '1d100'] as const
export const RULE_FAILURE_POLICY_OPTIONS = ['fail_forward', 'success_at_cost', 'blocked', 'hard_failure'] as const

const DEFAULT_RULE_TEMPLATES: RuleCheck[] = [
  {
    id: 'high-risk-action',
    label: '高风险行动',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'fail_forward',
    difficulty_guidance: '以普通目标为基准。角色有合适工具、明确计划、熟悉环境或相关属性优势时降一档；时间紧迫、信息不足、受伤、被压制或环境危险时升一档；多项不利同时存在时可升到困难或极难。',
    state_effect_guidance: '默认不强制改状态。若失败代价已经在剧情中成立，可给体力、警戒度、位置暴露、临时劣势或资源消耗 1-2 点变化；代价成功时优先选择轻量消耗而不是阻断行动。',
    trigger: '玩家采取存在明显风险、代价或不确定结果的行动，且直接叙述裁定会削弱互动张力时使用。',
    success_hint: '成功时让行动达成核心目标，并给出清晰收益或信息推进。',
    failure_hint: '失败时继续推进局势，但引入新的代价、压力或选择。',
  },
  {
    id: 'combat-exchange',
    label: '战斗攻防',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'fail_forward',
    difficulty_guidance: '以普通目标为基准。武器克制、占据高地、队友牵制或敌人露出破绽时降一档；敌人护甲厚重、数量压制、主角受伤、视野差或被控制时升一档。面对精英敌人或致命处境时使用困难以上。',
    state_effect_guidance: '根据伤害规模调整生命或护盾：擦伤约 -1，普通命中约 -2~-4，重击或大失败约 -5 以上；也可写入流血、缴械、失位、破甲等状态。成功可给敌方压制、破绽或位置优势。',
    trigger: '攻击、防御、闪避、夺取战斗位置或承受敌方压制时使用。',
    success_hint: '成功时扩大优势、造成有效压制或争取战斗节奏。',
    failure_hint: '失败时让角色承受伤害、失去位置或暴露破绽，但保留下一步行动空间。',
  },
  {
    id: 'stealth-lock',
    label: '潜行与开锁',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'blocked',
    difficulty_guidance: '以普通目标为基准。光线昏暗、守卫分心、工具合适、路线熟悉时降一档；锁具复杂、时间受限、附近有人、地面易响或角色负重受伤时升一档。魔法锁、警戒阵或多人巡逻可使用困难以上。',
    state_effect_guidance: '失败优先造成时间、体力或警戒度变化：体力 -1~-2、警戒度 +1、工具耐久下降或陷阱倒计时推进；大失败才直接暴露或触发陷阱。成功可保留隐蔽、取得位置优势或降低后续难度。',
    trigger: '潜行、开锁、拆陷阱、绕过守卫或进行精细操作时使用。',
    success_hint: '成功时悄无声息地越过阻碍，并保留行动主动权。',
    failure_hint: '失败时让操作受阻、消耗时间或体力，并提高被发现的风险。',
  },
  {
    id: 'investigation',
    label: '探索调查',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'fail_forward',
    difficulty_guidance: '以普通目标为基准。线索新鲜、现场完整、角色有专业知识或工具时降一档；线索被伪装、信息残缺、时间久远、干扰严重或存在误导时升一档。隐藏真相、复杂机关或跨领域推理可用困难以上。',
    state_effect_guidance: '成功应推进线索进度、解锁事实或减少迷雾；部分成功可给线索进度 +1 但增加时间压力；失败也给方向，但可增加误导标记、危险接近、调查资源 -1 或错过一条次要线索。',
    trigger: '搜索线索、辨认异常、分析痕迹、探索未知区域或判断信息真伪时使用。',
    success_hint: '成功时给出可行动的关键线索，并连接到下一步选择。',
    failure_hint: '失败时仍给出方向，但附带误导、遗漏、额外风险或时间压力。',
  },
  {
    id: 'social-negotiation',
    label: '社交谈判',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'success_at_cost',
    difficulty_guidance: '以普通目标为基准。关系友好、筹码充分、诉求合理、抓住对方动机时降一档；立场敌对、利益冲突、证据不足、身份劣势或公开场合丢脸风险时升一档。说服强敌、逼迫重大让步或识破谎言可用困难以上。',
    state_effect_guidance: '根据谈判结果调整关系、信任、声望或债务：小幅态度变化约 +/-1，明确让步或冒犯约 +/-2~3；代价成功可附加欠人情、提高价格、暴露意图或留下后续条件。',
    trigger: '说服、威胁、交易、套话、安抚、挑衅或争取 NPC 支持时使用。',
    success_hint: '成功时让对方让步、透露信息或提供有限帮助。',
    failure_hint: '失败时可以达成部分目标，但提高条件、暴露意图或损害关系。',
  },
  {
    id: 'endurance-pressure',
    label: '体力/意志抗压',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'success_at_cost',
    difficulty_guidance: '以普通目标为基准。休息充分、目标强烈、同伴支援、装备防护或相关抗性时降一档；连续消耗、伤病、恐惧、饥饿、极端天气或精神压迫时升一档。突破身体极限或抵抗强力控制时用困难以上。',
    state_effect_guidance: '优先调整体力、压力、疲劳、伤势或意志标记：轻度消耗 -1，持续压迫 -2~-3，大失败可增加负面状态或后续劣势。成功可移除轻度恐惧、稳定伤势或获得短暂抗性。',
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
    dice: optionOrDefault(RULE_DICE_OPTIONS, item.dice, '1d20'),
    modifier: numberOrDefault(item.modifier, 0),
    failure_policy: optionOrDefault(RULE_FAILURE_POLICY_OPTIONS, item.failure_policy, 'fail_forward'),
    difficulty_guidance: String(item.difficulty_guidance || ''),
    state_effect_guidance: String(item.state_effect_guidance || ''),
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

function numberOrDefault(value: unknown, fallback: number): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}
