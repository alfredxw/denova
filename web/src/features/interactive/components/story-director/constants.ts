import type { DirectorPlanningTemplates } from '../../types'

export const STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT = 64 * 1024
export const STORY_DIRECTOR_PLANNING_TEMPLATE_LIMIT = 64 * 1024
export const STORY_DIRECTOR_BRANCH_PLANNING_TURNS_FALLBACK = 5
export const EMPTY_DIRECTOR_PLANNING_TEMPLATES: DirectorPlanningTemplates = { plan: '', agent_brief: '' }
export const DIRECTOR_PRIVATE_PLAN_REQUIRED_HEADINGS = [
  '## 阶段目标与隐藏钩子',
  '## 资料库锚点',
  '## 选角覆盖',
  '## 核心角色与关系张力',
  '## 重要势力与阶段阻力',
  '## 当前场景幕后信息',
  '## 信息揭示与线索密度',
  '## 遭遇、检定与代价',
  '## 爽点、危机与反转',
  '## 状态连续性',
  '## 最近分支安排',
  '## 伏笔与回收',
] as const
export const DIRECTOR_AGENT_BRIEF_REQUIRED_HEADINGS = [
  '## 当前目标与可见钩子',
  '## 当前场景与行动空间',
  '## 当前角色与可见关系',
  '## 已公开信息与可发现线索',
  '## 遭遇、检定与可见代价',
  '## 状态连续性',
  '## 最近分支承接',
] as const
export const STORY_DIRECTOR_MAINLINE_OPTIONS = [
  { value: 'soft_guidance', labelKey: 'settingPanel.storyDirector.strategy.mainline.softGuidance', descriptionKey: 'settingPanel.storyDirector.strategy.mainline.softGuidanceDesc' },
  { value: 'balanced', labelKey: 'settingPanel.storyDirector.strategy.mainline.balanced', descriptionKey: 'settingPanel.storyDirector.strategy.mainline.balancedDesc' },
  { value: 'strong_arc', labelKey: 'settingPanel.storyDirector.strategy.mainline.strongArc', descriptionKey: 'settingPanel.storyDirector.strategy.mainline.strongArcDesc' },
] as const
export const STORY_DIRECTOR_FAILURE_OPTIONS = [
  { value: 'reversible', labelKey: 'settingPanel.storyDirector.strategy.failure.reversible', descriptionKey: 'settingPanel.storyDirector.strategy.failure.reversibleDesc' },
  { value: 'consequence', labelKey: 'settingPanel.storyDirector.strategy.failure.consequence', descriptionKey: 'settingPanel.storyDirector.strategy.failure.consequenceDesc' },
  { value: 'fail_forward', labelKey: 'settingPanel.storyDirector.strategy.failure.failForward', descriptionKey: 'settingPanel.storyDirector.strategy.failure.failForwardDesc' },
] as const
export const STORY_DIRECTOR_PACING_OPTIONS = [
  { value: 'progressive', labelKey: 'settingPanel.storyDirector.strategy.pacing.progressive', descriptionKey: 'settingPanel.storyDirector.strategy.pacing.progressiveDesc' },
  { value: 'wave', labelKey: 'settingPanel.storyDirector.strategy.pacing.wave', descriptionKey: 'settingPanel.storyDirector.strategy.pacing.waveDesc' },
  { value: 'goal-pressure-payoff', labelKey: 'settingPanel.storyDirector.strategy.pacing.goalPressurePayoff', descriptionKey: 'settingPanel.storyDirector.strategy.pacing.goalPressurePayoffDesc' },
] as const
export const STORY_DIRECTOR_EVENT_FREQUENCY_OPTIONS = [
	{ value: 'off', labelKey: 'settingPanel.storyDirector.strategy.eventFrequency.off', descriptionKey: 'settingPanel.storyDirector.strategy.eventFrequency.offDesc' },
	{ value: 'sparse', labelKey: 'settingPanel.storyDirector.strategy.eventFrequency.sparse', descriptionKey: 'settingPanel.storyDirector.strategy.eventFrequency.sparseDesc' },
	{ value: 'balanced', labelKey: 'settingPanel.storyDirector.strategy.eventFrequency.balanced', descriptionKey: 'settingPanel.storyDirector.strategy.eventFrequency.balancedDesc' },
	{ value: 'frequent', labelKey: 'settingPanel.storyDirector.strategy.eventFrequency.frequent', descriptionKey: 'settingPanel.storyDirector.strategy.eventFrequency.frequentDesc' },
] as const
export const STORY_DIRECTOR_AGENT_MODE_OPTIONS = [
  { value: 'triggered', labelKey: 'settingPanel.storyDirector.strategy.agentMode.triggered', descriptionKey: 'settingPanel.storyDirector.strategy.agentMode.triggeredDesc' },
  { value: 'every_turn', labelKey: 'settingPanel.storyDirector.strategy.agentMode.everyTurn', descriptionKey: 'settingPanel.storyDirector.strategy.agentMode.everyTurnDesc' },
  { value: 'off', labelKey: 'settingPanel.storyDirector.strategy.agentMode.off', descriptionKey: 'settingPanel.storyDirector.strategy.agentMode.offDesc' },
] as const
export const STORY_DIRECTOR_STATE_SCHEMA_ADAPTATION_OPTIONS = [
  { value: 'after_opening', labelKey: 'settingPanel.storyDirector.strategy.stateSchemaAdaptation.afterOpening', descriptionKey: 'settingPanel.storyDirector.strategy.stateSchemaAdaptation.afterOpeningDesc' },
  { value: 'off', labelKey: 'settingPanel.storyDirector.strategy.stateSchemaAdaptation.off', descriptionKey: 'settingPanel.storyDirector.strategy.stateSchemaAdaptation.offDesc' },
] as const
export const STORY_DIRECTOR_RULE_STATE_CONSUMPTION_OPTIONS = [
  { value: 'hybrid_auto', labelKey: 'settingPanel.storyDirector.strategy.ruleState.hybridAuto', descriptionKey: 'settingPanel.storyDirector.strategy.ruleState.hybridAutoDesc' },
  { value: 'director_only', labelKey: 'settingPanel.storyDirector.strategy.ruleState.directorOnly', descriptionKey: 'settingPanel.storyDirector.strategy.ruleState.directorOnlyDesc' },
] as const
export const STORY_DIRECTOR_RULE_VISIBILITY_OPTIONS = [
  { value: 'audit_only', labelKey: 'settingPanel.storyDirector.strategy.ruleVisibility.auditOnly', descriptionKey: 'settingPanel.storyDirector.strategy.ruleVisibility.auditOnlyDesc' },
  { value: 'public_roll', labelKey: 'settingPanel.storyDirector.strategy.ruleVisibility.publicRoll', descriptionKey: 'settingPanel.storyDirector.strategy.ruleVisibility.publicRollDesc' },
] as const
export type StrategySelectOption = {
  value: string
  labelKey: string
  descriptionKey: string
}

export const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
export const selectClassName = 'nova-field h-8 w-full text-xs focus:ring-0'
export const consoleSectionClassName = 'rounded-[14px] border border-[var(--preset-line)] bg-[color-mix(in_srgb,var(--preset-surface)_90%,transparent)] shadow-[inset_0_1px_0_rgba(255,255,255,0.025)] backdrop-blur'
