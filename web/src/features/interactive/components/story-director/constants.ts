export const STORY_DIRECTOR_STRATEGY_PROMPT_LIMIT = 256 * 1024
export const STORY_DIRECTOR_RULE_STATE_CONSUMPTION_OPTIONS = [
  { value: 'hybrid_auto', labelKey: 'settingPanel.storyDirector.strategy.ruleState.hybridAuto', descriptionKey: 'settingPanel.storyDirector.strategy.ruleState.hybridAutoDesc' },
  { value: 'suggestions_only', labelKey: 'settingPanel.storyDirector.strategy.ruleState.suggestionsOnly', descriptionKey: 'settingPanel.storyDirector.strategy.ruleState.suggestionsOnlyDesc' },
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

export const consoleSectionClassName = 'rounded-[14px] border border-[var(--preset-line)] bg-[color-mix(in_srgb,var(--preset-surface)_90%,transparent)] shadow-[inset_0_1px_0_rgba(255,255,255,0.025)] backdrop-blur'
