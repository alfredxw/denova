import { describe, expect, it } from 'vitest'
import type { LayeredSettings } from '@/features/settings/types'
import { buildModelProfileOptions, resolveCurrentProfileID } from './ModelProfileSwitcher'

describe('ModelProfileSwitcher profile options', () => {
  it('uses default as the stable id when the first model has no alias', () => {
    const options = buildModelProfileOptions(settingsSnapshot({
      effective: {
        model_profiles: [{ id: 'default', openai_model: 'deepseek-v4-pro' }],
      },
      workspace: {
        model_profiles: [{ id: 'default', openai_model: 'deepseek-v4-pro' }],
      },
    }), t)

    expect(options.map((option) => option.id)).toEqual(['default'])
    expect(options[0].label).toBe('default:deepseek-v4-pro')
  })

  it('falls back to default instead of exposing a missing profile id', () => {
    const settings = settingsSnapshot({
      effective: {
        model_profiles: [{ id: 'default', openai_model: 'deepseek-v4-pro' }],
        agent_models: { default: { profile_id: 'DeepSeek 写作' } },
      },
    })
    const options = buildModelProfileOptions(settings, t)

    expect(options.some((option) => option.id === 'DeepSeek 写作')).toBe(false)
    expect(resolveCurrentProfileID(settings.effective, 'ide', options)).toBe('default')
  })
})

function t(key: string, options?: Record<string, unknown>) {
  if (key === 'chat.modelProfile.defaultProfile') return `default:${options?.label ?? ''}`
  if (key === 'chat.modelProfile.profile') return `${options?.id ?? ''}:${options?.label ?? ''}`
  if (key === 'chat.modelProfile.defaultModel') return 'Default model'
  return key
}

function settingsSnapshot(patch: Partial<LayeredSettings>): LayeredSettings {
  return {
    default: {},
    global: {},
    user: {},
    workspace: {},
    effective: {},
    paths: {
      nova_dir: '/nova',
      user_config: '/nova/config.toml',
      workspace_config: '/books/demo/.nova/config.toml',
    },
    builtin_agent_prompts: {},
    builtin_agent_prompt_blocks: {},
    builtin_agent_prompt_sources: {},
    ...patch,
  }
}
