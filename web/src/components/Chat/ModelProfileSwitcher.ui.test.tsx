import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings, updateUserSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { ModelProfileSwitcher } from './ModelProfileSwitcher'

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}))

let latestSettings: LayeredSettings

describe('ModelProfileSwitcher quick control', () => {
  beforeEach(() => {
    latestSettings = settingsSnapshot({
      user: { agent_models: { ide: { profile_id: 'fast' } } },
      effective: {
        model_profiles: [
          { id: 'default', name: 'GPT 4.1', openai_model: 'gpt-4.1' },
          { id: 'fast', name: 'Turbo', openai_model: 'gpt-4.1-mini' },
        ],
        agent_models: { ide: { profile_id: 'fast' } },
      },
    })
    vi.mocked(fetchSettings).mockReset()
    vi.mocked(fetchSettings).mockImplementation(async () => latestSettings)
    vi.mocked(updateUserSettings).mockReset()
    vi.mocked(updateUserSettings).mockImplementation(async (userSettings) => {
      latestSettings = settingsSnapshot({
        user: userSettings,
        effective: {
          ...latestSettings.effective,
          agent_models: userSettings.agent_models,
        },
      })
      return latestSettings
    })
  })

  it('shows the current model on the trigger and switches from its popup list', async () => {
    const user = userEvent.setup()
    render(<ModelProfileSwitcher agentKey="ide" workspace="/tmp/book" />)

    const trigger = await screen.findByRole('button', { name: '切换模型，当前：Turbo' })
    expect(trigger).toHaveAttribute('data-current-model', 'Turbo')

    await user.click(trigger)
    expect(screen.getByRole('menu', { name: '切换模型，当前：Turbo' })).toBeInTheDocument()
    await user.click(screen.getByRole('menuitem', { name: '默认：GPT 4.1' }))

    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledWith(expect.objectContaining({
      agent_models: expect.objectContaining({ ide: expect.objectContaining({ profile_id: 'default' }) }),
    }), undefined))
    expect(await screen.findByRole('button', { name: '切换模型，当前：GPT 4.1' })).toBeInTheDocument()
  })
})

function settingsSnapshot(patch: Partial<LayeredSettings>): LayeredSettings {
  return {
    default: {},
    global: {},
    user: {},
    workspace: {},
    effective: {},
    paths: {
      denova_dir: '/denova',
      nova_dir: '/nova',
      user_config: '/nova/config.toml',
      workspace_config: '/tmp/book/.nova/config.toml',
    },
    builtin_agent_prompts: {},
    builtin_agent_prompt_blocks: {},
    builtin_agent_prompt_sources: {},
    ...patch,
  }
}
