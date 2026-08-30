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
  writingTarget: string,
): AgentQuickPromptDefaults {
  if (scope === 'writing') {
    return {
      title: t('chat.quickActions'),
      scopeLabel: t('chat.quick.scope.writing'),
      prompts: [
        quickPrompt('writing-next-group', t('chat.quick.nextGroup'), modelPrompt(t, 'chat.quick.nextGroupPrompt')),
        quickPrompt('writing-next-chapter', t('chat.quick.writeNextChapter'), modelPrompt(t, 'chat.quick.writeNextChapterPrompt')),
        quickPrompt('writing-continue', t('chat.quick.continueParagraph'), modelPrompt(t, 'chat.quick.continueParagraphPrompt')),
        quickPrompt('writing-polish', t('chat.quick.polishChapter'), modelPrompt(t, 'chat.quick.polishChapterPrompt', { target: writingTarget })),
        quickPrompt('writing-sync-state', t('chat.quick.finalizeState'), modelPrompt(t, 'chat.quick.finalizeStatePrompt', { target: writingTarget })),
        quickPrompt('writing-consistency', t('chat.quick.consistencyCheck'), modelPrompt(t, 'chat.quick.consistencyCheckPrompt', { target: writingTarget })),
      ],
    }
  }

  if (isPresetQuickPromptScope(scope)) {
    const resource = t(`chat.quick.scope.${scope}`)
    const modelResource = t(`chat.quick.scope.${scope}`, { lng: 'en-US' })
    return {
      title: t('chat.quickConfig'),
      scopeLabel: resource,
      prompts: [
        quickPrompt(`${scope}-create`, t('chat.quick.config.createPreset', { resource }), modelPrompt(t, 'chat.quick.config.createPresetPrompt', { resource: modelResource })),
        quickPrompt(`${scope}-improve`, t('chat.quick.config.improvePreset', { resource }), modelPrompt(t, 'chat.quick.config.improvePresetPrompt', { resource: modelResource })),
        quickPrompt(`${scope}-review`, t('chat.quick.config.reviewPreset', { resource }), modelPrompt(t, 'chat.quick.config.reviewPresetPrompt', { resource: modelResource })),
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
    prompts: groups[scope].map(([id, nameKey, promptKey]) => quickPrompt(id, t(nameKey), modelPrompt(t, promptKey))),
  }
}

function modelPrompt(t: TFunction, key: string, values?: Record<string, string>): string {
  return t(key, { ...values, lng: 'en-US' })
}

function isPresetQuickPromptScope(scope: AgentQuickPromptScope): scope is PresetQuickPromptScope {
  return scope.startsWith('preset-')
}

function quickPrompt(id: string, name: string, prompt: string): AgentQuickPromptSettings {
  return { id, name, prompt, behavior: 'fill', enabled: true }
}
