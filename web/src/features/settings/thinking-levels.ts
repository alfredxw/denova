export const THINKING_LEVELS = [
  'default',
  'off',
  'low',
  'medium',
  'high',
  'xhigh',
  'max',
] as const

export type ThinkingLevel = (typeof THINKING_LEVELS)[number]
export type ThinkingLevelSelection = '' | ThinkingLevel

export const THINKING_LEVEL_SELECTIONS: readonly ThinkingLevelSelection[] = ['', ...THINKING_LEVELS]

export function normalizeThinkingLevel(value: string | null | undefined): ThinkingLevel | null {
  let normalized = value?.trim().toLowerCase().replaceAll('_', '-').replaceAll(' ', '-') ?? ''
  while (normalized.includes('--')) normalized = normalized.replaceAll('--', '-')
  if (!normalized) return null
  if (['auto', 'model-default', 'provider-default'].includes(normalized)) normalized = 'default'
  if (['none', 'disabled'].includes(normalized)) normalized = 'off'
  if (['minimal', 'light'].includes(normalized)) normalized = 'low'
  if (['extra-high', 'extra-high-effort'].includes(normalized)) normalized = 'xhigh'
  if (normalized === 'maximum') normalized = 'max'
  return THINKING_LEVELS.includes(normalized as ThinkingLevel) ? normalized as ThinkingLevel : null
}
