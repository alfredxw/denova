import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

  it('shows context compaction prompt and target ratio settings', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      effective: {
        agent_context: {
          context_compaction: {
            compaction_recent_turns: 4,
            compaction_target_min_ratio: 0.09,
            compaction_target_max_ratio: 0.31,
          },
        },
      },
      builtin_agent_prompt_sources: {
        context_compaction: {
          sources: [
            { id: 'flow', title: '流程规则', source: 'Nova built-in', content: '压缩流程', editable: true, field: 'flow_prompt' },
            { id: 'custom', title: '用户自定义', source: 'user/workspace config', editable: true, field: 'system_prompt' },
          ],
        },
      },
    }))

    render(<AgentsView />)

    await user.click(await screen.findByRole('button', { name: /上下文压缩 Agent/ }))

    expect(screen.getByText('压缩目标下限 (%)')).toBeInTheDocument()
    expect(screen.getByText('压缩目标上限 (%)')).toBeInTheDocument()
    expect(screen.getByText('压缩后保留回合')).toBeInTheDocument()
    expect(screen.getByText('流程规则')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('12')).not.toBeInTheDocument()
    expect(screen.getByDisplayValue('4')).toBeInTheDocument()
    expect(screen.getByDisplayValue('9')).toBeInTheDocument()
    expect(screen.getByDisplayValue('31')).toBeInTheDocument()
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
