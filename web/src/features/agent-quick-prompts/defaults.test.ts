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
      agentQuickPromptDefaults(i18n.t, scope).prompts[0]?.id
    ))

    expect(new Set(firstPromptIDs).size).toBe(scopes.length)
  })

  it.each<AgentQuickPromptScope>([
    'writing', 'skills', 'agents', 'automation', 'lore',
    'preset-teller', 'preset-event', 'preset-rule', 'preset-actor-state', 'preset-director', 'preset-image',
  ])('localizes both names and inserted instructions for %s', (scope) => {
    setConfiguredLocale('zh-CN')
    const chinese = agentQuickPromptDefaults(i18n.t, scope)
    setConfiguredLocale('en-US')
    const english = agentQuickPromptDefaults(i18n.t, scope)

    expect(chinese.prompts.map(({ id, behavior, enabled }) => ({ id, behavior, enabled })))
      .toEqual(english.prompts.map(({ id, behavior, enabled }) => ({ id, behavior, enabled })))
    for (const [index, translated] of chinese.prompts.entries()) {
      expect(translated.name).toMatch(/[\u4e00-\u9fff]/)
      expect(translated.prompt).toMatch(/[\u4e00-\u9fff]/)
      expect(english.prompts[index].name).not.toMatch(/[\u4e00-\u9fff]/)
      expect(english.prompts[index].prompt).not.toMatch(/[\u4e00-\u9fff]/)
      expect(translated.prompt).not.toBe(english.prompts[index].prompt)
      expect(translated.prompt).not.toMatch(/\{\{resource\}\}/)
      expect(english.prompts[index].prompt).not.toMatch(/\{\{resource\}\}/)
    }
  })
})
