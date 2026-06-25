import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

  afterEach(() => {
    vi.useRealTimers()
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
    expect(screen.queryByText('deepseek（deepseek-v3）')).not.toBeInTheDocument()

    window.dispatchEvent(new CustomEvent('nova:settings-updated'))

    await waitFor(() => {
      expect(screen.getByText('deepseek（deepseek-v3）')).toBeInTheDocument()
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

  it('keeps execute configurable on Windows runtimes', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      runtime: { goos: 'windows' },
    }))

    render(<AgentsView />)

    const title = await screen.findByText('命令执行')
    const row = title.parentElement?.parentElement
    const select = row?.querySelector('select')
    expect(screen.queryByText('Windows 暂不支持 execute')).not.toBeInTheDocument()
    expect(select).toBeTruthy()
    expect(select).not.toBeDisabled()
  })

  it('adds and edits custom SubAgents in user settings by default', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))

    render(<AgentsView />)

    await screen.findByText('SubAgents')
    await user.click(screen.getByRole('button', { name: /新增 SubAgent/ }))
    const nameInput = screen.getByDisplayValue('自定义 SubAgent')
    await user.clear(nameInput)
    await user.type(nameInput, 'Researcher')
    await user.click(screen.getByRole('button', { name: '完成' }))
    expect(screen.getByText('Researcher')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        sub_agents: [expect.objectContaining({
          id: 'subagent-1',
          name: 'Researcher',
          parents: ['ide'],
        })],
      }))
    })
  })

  it('saves Agents page edits to workspace settings after switching layers', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))

    render(<AgentsView />)

    await screen.findByText('SubAgents')
    await user.click(screen.getByRole('button', { name: '当前工作区' }))
    await user.click(screen.getByRole('button', { name: /新增 SubAgent/ }))
    const nameInput = screen.getByDisplayValue('自定义 SubAgent')
    await user.clear(nameInput)
    await user.type(nameInput, 'Workspace Researcher')
    await user.click(screen.getByRole('button', { name: '完成' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateWorkspaceSettings)).toHaveBeenCalledWith(expect.objectContaining({
        sub_agents: [expect.objectContaining({
          id: 'subagent-1',
          name: 'Workspace Researcher',
          parents: ['ide'],
        })],
      }))
    })
  })

  it('keeps the SubAgent editor open when auto-save completes', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))
    vi.mocked(updateUserSettings).mockImplementation(async (settings) => settingsSnapshot({ user: settings, effective: settings }))

    render(<AgentsView />)

    await screen.findByText('SubAgents')
    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: /新增 SubAgent/ }))
    const nameInput = screen.getByDisplayValue('自定义 SubAgent')
    fireEvent.change(nameInput, { target: { value: 'Researcher' } })

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1100)
    })

    expect(vi.mocked(updateUserSettings)).toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Researcher')).toBeInTheDocument()
  })

  it('deletes custom SubAgents from Agents page settings', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      user: {
        sub_agents: [{
          id: 'researcher',
          name: 'Researcher',
          description: 'Researches delegated context',
          system_prompt: 'Return concise findings.',
          parents: ['ide'],
          enabled: true,
        }],
      },
      effective: {
        sub_agents: [{
          id: 'researcher',
          name: 'Researcher',
          description: 'Researches delegated context',
          system_prompt: 'Return concise findings.',
          parents: ['ide'],
          enabled: true,
        }],
      },
    }))

    render(<AgentsView />)

    await screen.findByText('Researcher')
    await user.click(screen.getByRole('button', { name: '删除 SubAgent' }))
    await screen.findByText('删除 SubAgent？')
    await user.click(screen.getByRole('button', { name: '删除' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenLastCalledWith(expect.objectContaining({
        sub_agents: [],
      }))
    })
  })

  it('can disable the built-in General SubAgent for the selected parent', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      effective: {
        general_sub_agents: { ide: true },
      },
    }))

    render(<AgentsView />)

    const generalSwitch = await screen.findByLabelText('通用 SubAgent 启用状态')
    await user.selectOptions(generalSwitch, 'false')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        general_sub_agents: { ide: false },
      }))
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
