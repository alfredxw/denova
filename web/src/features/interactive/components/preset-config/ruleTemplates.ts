import type { RuleCheck, RuleComputedStateChange, RuleNarrativeStateRef, RuleOutcomeStateChangeBinding, RuleStateBinding, RuleStateBindingModifier, RuleStateChangeFormula, RuleStateFormulaTerm, StoryDirectorTRPGSystem } from '../../types'

export const RULE_FAILURE_POLICY_OPTIONS = ['fail_forward', 'success_at_cost', 'blocked', 'hard_failure'] as const

const DEFAULT_RULE_TEMPLATES: RuleCheck[] = [
  {
    id: 'balanced-dice-check',
    label: 'Сбалансированная проверка d20',
    dice: '1d20',
    modifier: 0,
    failure_policy: 'fail_forward',
    difficulty_guidance: 'По умолчанию normal. Снижайте на один уровень при наличии явных способностей, подходящих инструментов или преимущества; повышайте при спешке, враждебной среде, ранениях или нехватке данных.',
    state_effect_guidance: 'При провале отдавайте приоритет обратимым последствиям: расход ресурсов, рост тревоги, ухудшение отношений или цейтнот. Не допускайте тупика в сюжете.',
    trigger: 'Используйте, когда действие сопряжено с риском и значимыми последствиями провала. Не проверяйте, если риска нет или исход очевиден.',
    must_check_examples: ['Взлом замка при приближении охраны.', 'Попытка переубедить колеблющегося NPC.', 'Опасная перебежка по рушащемуся мосту.'],
    skip_check_examples: ['Осмотр пустой безопасной комнаты.', 'Разговор с дружелюбным союзником.', 'Открытие обычной двери подходящим ключом.'],
    success_hint: 'При успехе действие достигает цели, давая ясную выгоду, улику, продвижение в локации или отношениях.',
    failure_hint: 'При провале сохраняйте ход сюжета, но четко описывайте цену, препятствие, расход ресурсов или новую опасность.',
  },
]

export function defaultRuleTemplates(): RuleCheck[] {
  return DEFAULT_RULE_TEMPLATES.map((template) => ({
    ...template,
    must_check_examples: [...(template.must_check_examples || [])],
    skip_check_examples: [...(template.skip_check_examples || [])],
  }))
}

export function normalizeRuleTemplate(item: Partial<RuleCheck>, index = 0): RuleCheck {
  const id = String(item.id || `rule-${index + 1}`).trim()
  return {
    id,
    label: item.label === undefined || item.label === null ? id : String(item.label),
    dice: '1d20',
    modifier: numberOrDefault(item.modifier, 0),
    failure_policy: optionOrDefault(RULE_FAILURE_POLICY_OPTIONS, item.failure_policy, 'fail_forward'),
    difficulty_guidance: String(item.difficulty_guidance || ''),
    state_effect_guidance: String(item.state_effect_guidance || ''),
    trigger: String(item.trigger || ''),
    must_check_examples: normalizeExampleList(item.must_check_examples),
    skip_check_examples: normalizeExampleList(item.skip_check_examples),
    success_hint: String(item.success_hint || ''),
    failure_hint: String(item.failure_hint || ''),
    state_bindings: normalizeStateBindings(item.state_bindings),
  }
}

function normalizeExampleList(value: unknown): string[] {
  const values = Array.isArray(value) ? value : []
  return Array.from(new Set(values.map((item) => String(item || '').trim()).filter(Boolean))).slice(0, 8)
}

export function normalizeTRPGSystem(value: StoryDirectorTRPGSystem | undefined): StoryDirectorTRPGSystem {
  const source = value?.rule_templates?.length ? value.rule_templates.slice(0, 1) : defaultRuleTemplates()
  return { rule_templates: source.map((item, index) => normalizeRuleTemplate(item, index)).slice(0, 1) }
}

function numberOrDefault(value: unknown, fallback: number): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function optionOrDefault<T extends readonly string[]>(options: T, value: unknown, fallback: T[number]): T[number] {
  return options.includes(String(value) as T[number]) ? String(value) as T[number] : fallback
}

function normalizeStateBindings(value: unknown): RuleStateBinding[] | undefined {
  if (!Array.isArray(value)) return undefined
  const bindings = value.slice(0, 12).map((item, index) => normalizeStateBinding(item, index)).filter((item): item is RuleStateBinding => Boolean(item))
  return bindings.length ? bindings : undefined
}

function normalizeStateBinding(value: unknown, index: number): RuleStateBinding | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const source = value as Partial<RuleStateBinding>
  const id = String(source.id || `binding-${index + 1}`).trim()
  if (!id) return null
  return {
    id,
    label: String(source.label || id),
    trigger: String(source.trigger || ''),
    actor_template_id: String(source.actor_template_id || '').trim(),
    target_template_id: String(source.target_template_id || '').trim(),
    modifiers: normalizeModifiers(source.modifiers),
    narrative_state_refs: normalizeNarrativeRefs(source.narrative_state_refs),
    outcome_state_changes: normalizeOutcomeStateChanges(source.outcome_state_changes),
  }
}

function normalizeModifiers(value: unknown): RuleStateBindingModifier[] | undefined {
  if (!Array.isArray(value)) return undefined
  const items = value.slice(0, 24).map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return null
    const source = item as Partial<RuleStateBindingModifier>
    const fieldId = String(source.field_id || '').normalize('NFKC').trim()
    if (!fieldId) return null
    return {
      source: stringOption(source.source, ['actor', 'target'], 'actor'),
      field_id: fieldId,
      value_path: normalizeValuePath(source.value_path),
      effect: stringOption(source.effect, ['advantage', 'resistance'], 'advantage'),
      scale: numberOrDefault(source.scale, 1),
      offset: numberOrDefault(source.offset, 0),
      min: optionalNumber(source.min),
      max: optionalNumber(source.max),
      rounding: stringOption(source.rounding, ['none', 'floor', 'ceil', 'nearest'], 'nearest'),
      required: source.required === false ? false : true,
    }
  }).filter(Boolean) as RuleStateBindingModifier[]
  return items.length ? items : undefined
}

function normalizeNarrativeRefs(value: unknown): RuleNarrativeStateRef[] | undefined {
  if (!Array.isArray(value)) return undefined
  const items = value.slice(0, 24).map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return null
    const source = item as Partial<RuleNarrativeStateRef>
    return {
      source: stringOption(source.source, ['actor', 'target', 'scene'], 'actor'),
      field_id: String(source.field_id || '').normalize('NFKC').trim(),
      usage: stringOption(source.usage, ['check_decision', 'difficulty', 'outcome_design', 'prose'], 'outcome_design'),
      guidance: String(source.guidance || ''),
    }
  }).filter(Boolean) as RuleNarrativeStateRef[]
  return items.length ? items : undefined
}

function normalizeOutcomeStateChanges(value: unknown): RuleOutcomeStateChangeBinding[] | undefined {
  if (!Array.isArray(value)) return undefined
  const items = value.slice(0, 24).map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return null
    const source = item as Partial<RuleOutcomeStateChangeBinding>
    const changes = normalizeComputedStateChanges(source.state_changes)
    if (!changes?.length) return null
    return {
      outcome: stringOption(source.outcome, ['critical_success', 'success', 'failure', 'critical_failure'], 'success'),
      state_changes: changes,
    }
  }).filter(Boolean) as RuleOutcomeStateChangeBinding[]
  return items.length ? items : undefined
}

function normalizeComputedStateChanges(value: unknown): RuleComputedStateChange[] | undefined {
  if (!Array.isArray(value)) return undefined
  const items = value.slice(0, 24).map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return null
    const source = item as Partial<RuleComputedStateChange>
    const fieldId = String(source.field_id || '').normalize('NFKC').trim()
    if (!fieldId) return null
    return {
      source: stringOption(source.source, ['actor', 'target'], 'actor'),
      field_id: fieldId,
      change_formula: normalizeFormula(source.change_formula),
      reason: String(source.reason || ''),
    }
  }).filter(Boolean) as RuleComputedStateChange[]
  return items.length ? items : undefined
}

function normalizeFormula(value: unknown): RuleStateChangeFormula {
  const source = value && typeof value === 'object' && !Array.isArray(value) ? value as Partial<RuleStateChangeFormula> : {}
  return {
    base: numberOrDefault(source.base, 0),
    terms: normalizeFormulaTerms(source.terms),
    min: optionalNumber(source.min),
    max: optionalNumber(source.max),
    rounding: stringOption(source.rounding, ['none', 'floor', 'ceil', 'nearest'], 'nearest'),
  }
}

function normalizeFormulaTerms(value: unknown): RuleStateFormulaTerm[] | undefined {
  if (!Array.isArray(value)) return undefined
  const items = value.slice(0, 24).map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return null
    const source = item as Partial<RuleStateFormulaTerm>
    const fieldId = String(source.field_id || '').normalize('NFKC').trim()
    if (!fieldId) return null
    return {
      source: stringOption(source.source, ['actor', 'target'], 'actor'),
      field_id: fieldId,
      value_path: normalizeValuePath(source.value_path),
      scale: numberOrDefault(source.scale, 1),
      offset: numberOrDefault(source.offset, 0),
    }
  }).filter(Boolean) as RuleStateFormulaTerm[]
  return items.length ? items : undefined
}

function optionalNumber(value: unknown): number | undefined {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

function normalizeValuePath(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  const path = value
    .slice(0, 24)
    .map((segment) => String(segment || '').normalize('NFKC').trim())
    .filter(Boolean)
  return path.length ? path : undefined
}

function stringOption<T extends string>(value: unknown, options: readonly T[], fallback: T): T {
  const text = String(value || '').trim()
  return options.includes(text as T) ? text as T : fallback
}
