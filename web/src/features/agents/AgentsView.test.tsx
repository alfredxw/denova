import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getSkills } from '@/lib/api'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { AgentsView } from './AgentsView'

const { configManagerChatProps } = vi.hoisted(() => ({
  configManagerChatProps: [] as Array<{
    origin?: string
    resourceId?: string
    context?: Record<string, string>
    onMutated?: () => void
  }>,
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
  updateUserSettings: vi.fn(),
  updateWorkspaceSettings: vi.fn(),
}))

vi.mock('@/components/Chat/ConfigManagerChat', () => ({
  ConfigManagerChat: (props: {
    origin?: string
    resourceId?: string
    context?: Record<string, string>
    onMutated?: () => void
  }) => {
    configManagerChatProps.push(props)
    return (
      <div data-testid="config-manager-chat">
        <button type="button" onClick={() => props.onMutated?.()}>mock mutation</button>
      </div>
    )
  },
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
    configManagerChatProps.length = 0
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
    const toggle = row ? within(row).getByRole('switch', { name: '命令执行' }) : null
    expect(screen.queryByText('Windows 暂不支持 execute')).not.toBeInTheDocument()
    expect(toggle).toBeTruthy()
    expect(toggle).not.toBeDisabled()
  })

  it('shows inherited empty thinking as the default state', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))

    render(<AgentsView />)

    await screen.findByText('模型与思考')
    const thinkingSwitch = screen.getByRole('switch', { name: '思考开关' })
    expect(thinkingSwitch).toBeChecked()
    expect(thinkingSwitch).toHaveAttribute('title', '思考开关: 默认')
    expect(thinkingSwitch.parentElement?.querySelector('[aria-hidden="true"]')).toBeTruthy()
  })

  it('shows SubAgent thinking as inherited from the parent model', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      effective: {
        agent_models: {
          default: { enable_thinking: true },
        },
        sub_agents: [{
          id: 'reviewer',
          name: 'Reviewer',
          description: 'Reviews drafts.',
          system_prompt: 'Review only.',
          parents: ['ide'],
          enabled: true,
          model: {},
        }],
      },
    }))

    render(<AgentsView />)

    const reviewer = await screen.findByText('Reviewer')
    const row = reviewer.closest('div.rounded-\\[var\\(--nova-radius\\)\\]')
    expect(row).toBeTruthy()
    await user.click(within(row as HTMLElement).getByRole('button', { name: '编辑 SubAgent' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('switch', { name: '思考开关' })).toBeChecked()
    expect(within(dialog).getAllByText('继承').length).toBeGreaterThan(0)
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

  it('can disable inherited default SubAgents from the active settings layer', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      effective: {
        sub_agents: [{
          id: 'reviewer',
          name: 'Reviewer',
          description: 'Reviews drafts.',
          system_prompt: 'Review only.',
          parents: ['ide'],
          enabled: true,
        }],
      },
    }))

    render(<AgentsView />)

    const reviewer = await screen.findByText('Reviewer')
    const row = reviewer.closest('div.rounded-\\[var\\(--nova-radius\\)\\]')
    expect(row).toBeTruthy()
    await user.click(within(row as HTMLElement).getByRole('switch', { name: '启用状态' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        sub_agents: [expect.objectContaining({
          id: 'reviewer',
          enabled: false,
        })],
      }))
    })
  })

  it('deletes inherited SubAgents without re-enabling them on the next render', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      effective: {
        sub_agents: [{
          id: 'reviewer',
          name: 'Reviewer',
          description: 'Reviews drafts.',
          system_prompt: 'Review only.',
          parents: ['ide'],
          enabled: true,
        }],
      },
    }))

    render(<AgentsView />)

    await screen.findByText('Reviewer')
    await user.click(screen.getByRole('button', { name: '删除 SubAgent' }))
    await screen.findByText('删除 SubAgent？')
    await user.click(screen.getByRole('button', { name: '删除' }))

    await waitFor(() => {
      expect(screen.queryByText('Reviewer')).not.toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        sub_agents: [expect.objectContaining({
          id: 'reviewer',
          enabled: false,
          parents: ['interactive_story', 'config_manager', 'automation'],
        })],
      }))
    })
  })

  it('shows inherited SubAgents only on matching parent agents', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      effective: {
        sub_agents: [{
          id: 'reviewer',
          name: 'Reviewer',
          description: 'Reviews drafts.',
          system_prompt: 'Review only.',
          parents: ['ide'],
          enabled: true,
        }],
      },
    }))

    render(<AgentsView />)

    await screen.findByText('Reviewer')
    await user.click(screen.getByRole('button', { name: '配置管理 Agent资料库、叙事编排、Skills、自动化与故事记忆管理' }))

    await waitFor(() => {
      expect(screen.queryByText('Reviewer')).not.toBeInTheDocument()
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

  it('keeps SubAgent dialog edits local until Done', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))
    vi.mocked(updateUserSettings).mockImplementation(async (settings) => settingsSnapshot({ user: settings, effective: settings }))

    render(<AgentsView />)

    await screen.findByText('SubAgents')
    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: /新增 SubAgent/ }))
    const dialog = screen.getByRole('dialog')
    const nameInput = within(dialog).getByDisplayValue('自定义 SubAgent')
    fireEvent.change(nameInput, { target: { value: 'Researcher' } })
    fireEvent.click(within(dialog).getByLabelText('写作'))

    expect(within(dialog).getByText('当前父 Agent 未启用这个 SubAgent。')).toBeInTheDocument()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1100)
    })

    expect(vi.mocked(updateUserSettings)).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Researcher')).toBeInTheDocument()

    fireEvent.click(within(dialog).getByRole('button', { name: '完成' }))

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1100)
    })

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByText('Researcher')).not.toBeInTheDocument()
    expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
      sub_agents: [expect.objectContaining({
        id: 'subagent-1',
        name: 'Researcher',
        parents: ['interactive_story', 'config_manager', 'automation'],
      })],
    }))
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
    await user.click(generalSwitch)
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        general_sub_agents: { ide: false },
      }))
    })
  })

  it('opens Config Manager chat from Agents page with current Agent context', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      paths: {
        nova_dir: '/nova',
        user_config: '/nova/config.toml',
        workspace_config: '/books/demo/.nova/config.toml',
      },
    }))

    render(<AgentsView />)

    await screen.findByText('模型与思考')
    await user.click(screen.getByRole('button', { name: '用配置管理 Agent 调整' }))

    expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    expect(configManagerChatProps.at(-1)).toMatchObject({
      origin: 'agents',
      resourceId: 'user:ide',
      context: expect.objectContaining({
        active_settings_layer: 'user',
        active_agent: 'ide',
        write_scope_required: 'true',
      }),
    })

    await user.click(screen.getByRole('button', { name: 'mock mutation' }))
    await waitFor(() => {
      expect(vi.mocked(fetchSettings).mock.calls.length).toBeGreaterThan(1)
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
