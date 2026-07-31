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
      user: { agent_models: { ide: { profile_id: 'fast', thinking_level: 'medium' } } },
      effective: {
        model_profiles: [
          { id: 'default', name: 'GPT 4.1', openai_model: 'gpt-4.1' },
          { id: 'fast', name: 'Turbo', openai_model: 'gpt-4.1-mini' },
        ],
        agent_models: { ide: { profile_id: 'fast', thinking_level: 'medium' } },
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

  it('uses a borderless text-and-chevron trigger with the current thinking level', async () => {
    const { container } = render(<ModelProfileSwitcher agentKey="ide" workspace="/tmp/book" />)

    const trigger = await screen.findByRole('button', { name: '切换模型，当前：Turbo 中' })
    expect(trigger).toHaveAttribute('data-current-model', 'Turbo')
    expect(trigger).toHaveAttribute('data-current-thinking-level', 'medium')
    expect(trigger).toHaveClass('border-0', 'bg-transparent')
    expect(container.querySelector('.lucide-cpu')).not.toBeInTheDocument()
    expect(container.querySelectorAll('svg')).toHaveLength(1)
    expect(container.querySelector('.lucide-chevron-down')).toBeInTheDocument()
  })

  it('switches the model from its popup list', async () => {
    const user = userEvent.setup()
    render(<ModelProfileSwitcher agentKey="ide" workspace="/tmp/book" />)

    const trigger = await screen.findByRole('button', { name: '切换模型，当前：Turbo 中' })
    expect(trigger).toHaveAttribute('data-current-model', 'Turbo')

    await user.click(trigger)
    expect(screen.getByText('模型')).toBeInTheDocument()
    expect(screen.getByText('思考强度')).toBeInTheDocument()
    expect(screen.getByRole('group', { name: '思考强度' })).toHaveClass('grid-cols-4')
    await user.click(screen.getByRole('menuitem', { name: '默认：GPT 4.1' }))

    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledWith(expect.objectContaining({
      agent_models: expect.objectContaining({ ide: expect.objectContaining({ profile_id: 'default' }) }),
    }), undefined))
    expect(await screen.findByRole('button', { name: '切换模型，当前：GPT 4.1 中' })).toBeInTheDocument()
  })

  it('supports max thinking and can return to inherited configuration', async () => {
    const user = userEvent.setup()
    render(<ModelProfileSwitcher agentKey="ide" workspace="/tmp/book" />)

    await user.click(await screen.findByRole('button', { name: '切换模型，当前：Turbo 中' }))
    await user.click(screen.getByRole('button', { name: '最大' }))

    await waitFor(() => expect(updateUserSettings).toHaveBeenLastCalledWith(expect.objectContaining({
      agent_models: expect.objectContaining({ ide: expect.objectContaining({ thinking_level: 'max' }) }),
    }), undefined))
    const maxTrigger = await screen.findByRole('button', { name: '切换模型，当前：Turbo 最大' })
    expect(maxTrigger).toHaveAttribute('data-current-thinking-level', 'max')

    await user.click(maxTrigger)
    await user.click(screen.getByRole('button', { name: '跟随配置' }))

    await waitFor(() => {
      const saved = vi.mocked(updateUserSettings).mock.calls.at(-1)?.[0]
      expect(saved).toBeDefined()
      expect(saved!.agent_models?.ide).not.toHaveProperty('thinking_level')
    })
    expect(await screen.findByRole('button', { name: '切换模型，当前：Turbo' })).toHaveAttribute('data-current-thinking-level', '')
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
    resolved_agent_tool_manifests: {},
    resolved_agent_contexts: {},
    ...patch,
  }
}
