import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import { agentQuickPromptDefaults, type AgentQuickPromptScope } from './defaults'

describe('agentQuickPromptDefaults', () => {
  beforeAll(() => setConfiguredLocale('zh-CN'))
  afterAll(() => setConfiguredLocale('zh-CN'))

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

  it('localizes built-in prompt names and content with the configured language', () => {
    setConfiguredLocale('zh-CN')
    const chinese = agentQuickPromptDefaults(i18n.t, 'writing', '当前章节《序章》')

    expect(chinese.prompts[3]).toMatchObject({
      name: '润色当前章',
      prompt: '请检查并润色当前章节《序章》，重点优化语句节奏、动作描写和情绪推进，不改变核心剧情。',
      behavior: 'fill',
      enabled: true,
    })

    setConfiguredLocale('en-US')
    const english = agentQuickPromptDefaults(i18n.t, 'writing', 'current chapter "Prologue"')

    expect(english.prompts[3]).toMatchObject({
      name: 'Polish Current Chapter',
      prompt: 'Review and polish current chapter "Prologue", focusing on sentence rhythm, action writing, and emotional progression without changing the core plot.',
      behavior: 'fill',
      enabled: true,
    })
  })
})
