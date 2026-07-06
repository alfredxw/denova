import type { DirectorPlanDocs } from '../../types'

export const STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT = 64 * 1024
export const STORY_DIRECTOR_PLANNING_TEMPLATE_LIMIT = 64 * 1024
export const STORY_DIRECTOR_BRANCH_PLANNING_TURNS_FALLBACK = 5
export const EMPTY_DIRECTOR_PLANNING_TEMPLATES: DirectorPlanDocs = { plan: '' }
export const DIRECTOR_PLAN_REQUIRED_HEADINGS = [
  '## 正文Agent可读',
  '## 后台导演私密',
  '### 阶段钩子与阅读欲望',
  '### 资料库锚点',
  '### 核心角色与关系张力',
  '### 重要势力与阶段阻力',
  '### 当前场景与行动空间',
  '### 信息揭示与线索密度',
  '### 遭遇、检定与代价',
  '### 爽点、危机与反转',
  '### 状态连续性',
  '### 最近分支安排',
  '### 伏笔与回收',
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
export const STORY_DIRECTOR_RANDOM_RATE_OPTIONS = [
  { value: '0', rate: 0, labelKey: 'settingPanel.storyDirector.strategy.random.off', descriptionKey: 'settingPanel.storyDirector.strategy.random.offDesc' },
  { value: '0.08', rate: 0.08, labelKey: 'settingPanel.storyDirector.strategy.random.low', descriptionKey: 'settingPanel.storyDirector.strategy.random.lowDesc' },
  { value: '0.15', rate: 0.15, labelKey: 'settingPanel.storyDirector.strategy.random.medium', descriptionKey: 'settingPanel.storyDirector.strategy.random.mediumDesc' },
  { value: '0.3', rate: 0.3, labelKey: 'settingPanel.storyDirector.strategy.random.high', descriptionKey: 'settingPanel.storyDirector.strategy.random.highDesc' },
] as const
export const STORY_DIRECTOR_AGENT_MODE_OPTIONS = [
  { value: 'triggered', labelKey: 'settingPanel.storyDirector.strategy.agentMode.triggered', descriptionKey: 'settingPanel.storyDirector.strategy.agentMode.triggeredDesc' },
  { value: 'every_turn', labelKey: 'settingPanel.storyDirector.strategy.agentMode.everyTurn', descriptionKey: 'settingPanel.storyDirector.strategy.agentMode.everyTurnDesc' },
  { value: 'off', labelKey: 'settingPanel.storyDirector.strategy.agentMode.off', descriptionKey: 'settingPanel.storyDirector.strategy.agentMode.offDesc' },
] as const
export const EDITOR_TABS = ['stats', 'trpg', 'opening', 'events'] as const

export type StoryDirectorEditorTab = typeof EDITOR_TABS[number]
export type StrategySelectOption = {
  value: string
  labelKey: string
  descriptionKey: string
}

export const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
export const selectClassName = 'nova-field h-8 text-xs focus:ring-0'
export const consoleSectionClassName = 'rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--nova-surface)_88%,transparent)] shadow-[inset_0_1px_0_rgba(255,255,255,0.025)] backdrop-blur'
