import { beforeAll, describe, expect, it } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import { agentQuickPromptDefaults, type AgentQuickPromptScope } from './defaults'

describe('agentQuickPromptDefaults', () => {
  beforeAll(() => setConfiguredLocale('zh-CN'))

  it('uses distinct defaults for each configuration page', () => {
    const scopes: AgentQuickPromptScope[] = [
      'skills',
      'agents',
      'automation',
      'lore',
      'preset-teller',
      'preset-event',
      'preset-rule',
      'preset-actor-state',
      'preset-director',
      'preset-image',
    ]
    const firstPromptIDs = scopes.map((scope) => (
      agentQuickPromptDefaults(i18n.t, scope, 'current work').prompts[0]?.id
    ))

    expect(new Set(firstPromptIDs).size).toBe(scopes.length)
  })

  it('localizes labels while keeping built-in model prompts in English', () => {
    const defaults = agentQuickPromptDefaults(i18n.t, 'skills', 'current work')

    expect(defaults.prompts[0]).toMatchObject({
      name: '创建 Skill',
      prompt: 'Create a new Skill for the following goal:',
      behavior: 'fill',
      enabled: true,
    })
  })
})
