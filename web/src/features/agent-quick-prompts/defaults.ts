import type { TFunction } from 'i18next'
import type { AgentQuickPromptSettings } from '@/features/settings/types'

export type AgentQuickPromptScope =
  | 'writing'
  | 'skills'
  | 'agents'
  | 'automation'
  | 'lore'
  | 'preset-teller'
  | 'preset-event'
  | 'preset-rule'
  | 'preset-actor-state'
  | 'preset-director'
  | 'preset-image'

type PresetQuickPromptScope = Extract<AgentQuickPromptScope, `preset-${string}`>

export interface AgentQuickPromptDefaults {
  title: string
  scopeLabel: string
  prompts: AgentQuickPromptSettings[]
}

export function agentQuickPromptDefaults(
  t: TFunction,
  scope: AgentQuickPromptScope,
): AgentQuickPromptDefaults {
  // Quick prompts become user-visible drafts, so their names and content follow the user's language.
  // The selected writing Skill owns file layout and execution workflow.
  if (scope === 'writing') {
    return {
      title: t('chat.quickActions'),
      scopeLabel: t('chat.quick.scope.writing'),
      prompts: [
        ['writing-next-group', 'chat.quick.nextGroup', 'chat.quick.nextGroupPrompt'],
        ['writing-next-chapter', 'chat.quick.writeNextChapter', 'chat.quick.writeNextChapterPrompt'],
        ['writing-brainstorm', 'chat.quick.brainstorm', 'chat.quick.brainstormPrompt'],
        ['writing-humanize', 'chat.quick.humanize', 'chat.quick.humanizePrompt'],
        ['writing-align-style', 'chat.quick.alignStyle', 'chat.quick.alignStylePrompt'],
        ['writing-review', 'chat.quick.reviewChapter', 'chat.quick.reviewChapterPrompt'],
      ].map(([id, nameKey, promptKey]) => quickPrompt(id, t(nameKey), t(promptKey))),
    }
  }

  if (isPresetQuickPromptScope(scope)) {
    const resource = t(`chat.quick.scope.${scope}`)
    return {
      title: t('chat.quickConfig'),
      scopeLabel: resource,
      prompts: [
        quickPrompt(`${scope}-create`, t('chat.quick.config.createPreset', { resource }), t('chat.quick.config.createPresetPrompt', { resource })),
        quickPrompt(`${scope}-improve`, t('chat.quick.config.improvePreset', { resource }), t('chat.quick.config.improvePresetPrompt', { resource })),
        quickPrompt(`${scope}-review`, t('chat.quick.config.reviewPreset', { resource }), t('chat.quick.config.reviewPresetPrompt', { resource })),
      ],
    }
  }

  const groups: Record<Exclude<AgentQuickPromptScope, 'writing' | `preset-${string}`>, Array<[string, string, string]>> = {
    skills: [
      ['skills-create', 'chat.quick.config.skills.create', 'chat.quick.config.skills.createPrompt'],
      ['skills-improve', 'chat.quick.config.skills.improve', 'chat.quick.config.skills.improvePrompt'],
      ['skills-review', 'chat.quick.config.skills.review', 'chat.quick.config.skills.reviewPrompt'],
    ],
    agents: [
      ['agents-purpose', 'chat.quick.config.agents.purpose', 'chat.quick.config.agents.purposePrompt'],
      ['agents-capabilities', 'chat.quick.config.agents.capabilities', 'chat.quick.config.agents.capabilitiesPrompt'],
      ['agents-review', 'chat.quick.config.agents.review', 'chat.quick.config.agents.reviewPrompt'],
    ],
    automation: [
      ['automation-create', 'chat.quick.config.automation.create', 'chat.quick.config.automation.createPrompt'],
      ['automation-schedule', 'chat.quick.config.automation.schedule', 'chat.quick.config.automation.schedulePrompt'],
      ['automation-review', 'chat.quick.config.automation.review', 'chat.quick.config.automation.reviewPrompt'],
    ],
    lore: [
      ['lore-complete', 'chat.quick.config.lore.complete', 'chat.quick.config.lore.completePrompt'],
      ['lore-conflicts', 'chat.quick.config.lore.conflicts', 'chat.quick.config.lore.conflictsPrompt'],
      ['lore-organize', 'chat.quick.config.lore.organize', 'chat.quick.config.lore.organizePrompt'],
    ],
  }
  return {
    title: t('chat.quickConfig'),
    scopeLabel: t(`chat.quick.scope.${scope}`),
    prompts: groups[scope].map(([id, nameKey, promptKey]) => quickPrompt(id, t(nameKey), t(promptKey))),
  }
}

function isPresetQuickPromptScope(scope: AgentQuickPromptScope): scope is PresetQuickPromptScope {
  return scope.startsWith('preset-')
}

function quickPrompt(id: string, name: string, prompt: string): AgentQuickPromptSettings {
  return { id, name, prompt, behavior: 'fill', enabled: true }
}
