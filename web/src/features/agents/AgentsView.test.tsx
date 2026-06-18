import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getSkills } from '@/lib/api'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { AgentsView } from './AgentsView'

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
  updateUserSettings: vi.fn(),
  updateWorkspaceSettings: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  getSkills: vi.fn(),
}))

describe('AgentsView', () => {
  beforeEach(() => {
    vi.mocked(fetchSettings).mockReset()
    vi.mocked(updateUserSettings).mockReset()
    vi.mocked(updateWorkspaceSettings).mockReset()
    vi.mocked(getSkills).mockReset()
    vi.mocked(getSkills).mockResolvedValue({ scopes: [], skills: [] })
    vi.mocked(updateUserSettings).mockImplementation(async (settings) => settingsSnapshot({ user: settings, effective: settings }))
    vi.mocked(updateWorkspaceSettings).mockImplementation(async (settings) => settingsSnapshot({ workspace: settings, effective: settings }))
  })

  it('reloads model profiles when settings are updated elsewhere', async () => {
    vi.mocked(fetchSettings)
      .mockResolvedValueOnce(settingsSnapshot({ effective: { openai_model: 'deepseek-chat' } }))
      .mockResolvedValueOnce(settingsSnapshot({
        effective: {
          openai_model: 'deepseek-chat',
          model_profiles: [{ id: 'deepseek', name: 'DeepSeek V3', openai_model: 'deepseek-v3' }],
        },
      }))

    render(<AgentsView />)

    await screen.findByText('模型与思考')
    expect(screen.queryByText('deepseek（DeepSeek V3）')).not.toBeInTheDocument()

    window.dispatchEvent(new CustomEvent('nova:settings-updated'))

    await waitFor(() => {
      expect(screen.getByText('deepseek（DeepSeek V3）')).toBeInTheDocument()
    })
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
