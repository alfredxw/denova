import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getSkills } from '@/lib/api'
import { fetchSettings } from '@/features/settings/api'
import type { AgentToolCapability, LayeredSettings, ResolvedAgentContextSettings, ResolvedAgentToolCapability } from '@/features/settings/types'
import { AgentsView } from './AgentsView'

const { configManagerChatProps } = vi.hoisted(() => ({
  configManagerChatProps: [] as Array<{
    origin?: string
    resourceId?: string
    context?: Record<string, string>
    onMutated?: () => void
  }>,
}))
const { updateUserSettings, updateWorkspaceSettings } = vi.hoisted(() => ({
  updateUserSettings: vi.fn(),
  updateWorkspaceSettings: vi.fn(),
}))

vi.mock('@/features/settings/api', () => {
  const fetchSettings = vi.fn()
  return {
    fetchSettings,
    createSettingsMergePatch: (_baseline: unknown, draft: unknown) => draft,
    patchSettings: (layer: string, changes: unknown, revision?: string) => layer === 'workspace'
      ? revision === undefined ? updateWorkspaceSettings(changes) : updateWorkspaceSettings(changes, revision)
      : revision === undefined ? updateUserSettings(changes) : updateUserSettings(changes, revision),
  }
})

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
    const user = userEvent.setup()
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

    await waitFor(() => expect(vi.mocked(fetchSettings)).toHaveBeenCalledTimes(2))
    await user.click(screen.getAllByRole('combobox')[0])
    expect(await screen.findByText('deepseek（DeepSeek V3）')).toBeInTheDocument()
  })

  it('shows context compaction prompt while keeping mechanics backend-managed', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
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

    expect(screen.queryByText('压缩目标下限 (%)')).not.toBeInTheDocument()
    expect(screen.queryByText('压缩目标上限 (%)')).not.toBeInTheDocument()
    expect(screen.queryByText('压缩后保留回合')).not.toBeInTheDocument()
    expect(screen.getByText('流程规则')).toBeInTheDocument()
    expect(screen.getByText('后端管理的缓存安全边界')).toBeInTheDocument()
    expect(screen.getByRole('spinbutton', { name: '单片段上限 (KB)' })).toHaveValue(256)
    expect(screen.getByRole('spinbutton', { name: '本轮注入总上限 (KB)' })).toHaveValue(1024)
    expect(screen.getByRole('spinbutton', { name: '本轮片段数量上限' })).toHaveValue(256)
    expect(screen.getByRole('spinbutton', { name: '来源元数据上限 (KB)' })).toHaveValue(4)
  })

  it('shows backend-resolved context intent and saves only the edited override', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))

    render(<AgentsView />)

    expect(await screen.findByRole('spinbutton', { name: '触发阈值 (%)' })).toHaveValue(85)
    expect(screen.getByRole('switch', { name: '工具结果上下文' })).toBeChecked()
    expect(screen.queryByRole('spinbutton', { name: '工具结果清理阈值 (%)' })).not.toBeInTheDocument()

    fireEvent.change(screen.getByRole('spinbutton', { name: '触发阈值 (%)' }), { target: { value: '72' } })
    flushAgentsAutosave()

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        agent_context: expect.objectContaining({
          ide: expect.objectContaining({ compaction_threshold: 0.72 }),
        }),
      }))
    })
  })

  it('keeps the shell capability configurable on Windows runtimes', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      runtime: { goos: 'windows' },
      resolved_agent_tool_manifests: {
        ide: [resolvedTool('shell', 'agents.tool.shell.title', ['pwsh'])],
      },
    }))

    render(<AgentsView />)

    const title = await screen.findByText('Shell 命令')
    const row = title.closest<HTMLElement>('.min-h-16')
    const toggle = row ? within(row).getByRole('switch', { name: 'Shell 命令' }) : null
    expect(toggle).toBeTruthy()
    expect(toggle).not.toBeDisabled()
    expect(row ? within(row).getByText('pwsh') : null).toBeTruthy()
    expect(row ? within(row).queryByText('bash') : null).toBeNull()
  })

  it('uses backend manifest order and resolved allowed values without a frontend fallback matrix', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      resolved_agent_tool_manifests: {
        ide: [
          resolvedTool('todo', 'agents.tool.todo.title', ['todo']),
          resolvedTool('shell', 'agents.tool.shell.title', ['bash'], false),
        ],
      },
    }))

    render(<AgentsView />)

    const todo = await screen.findByText('任务清单')
    const shell = screen.getByText('Shell 命令')
    expect(todo.compareDocumentPosition(shell) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0)
    expect(screen.getByRole('switch', { name: 'Shell 命令' })).not.toBeChecked()
  })

  it('shows runtime-check and unavailable manifest states without disabling policy configuration', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      resolved_agent_tool_manifests: {
        ide: [
          resolvedTool('web_search', 'agents.tool.webSearch.title', ['web_search'], true, true, 'runtime_check'),
          resolvedTool('shell', 'agents.tool.shell.title', ['bash'], false, true, 'unavailable'),
        ],
      },
    }))

    render(<AgentsView />)

    expect(await screen.findByText('运行时检查')).toBeInTheDocument()
    expect(screen.getByText('不可用')).toBeInTheDocument()
    expect(screen.getByText('当前 Agent 的工具策略已禁用此能力。')).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: '网页搜索' })).not.toBeDisabled()
    expect(screen.getByRole('switch', { name: 'Shell 命令' })).not.toBeDisabled()
  })

  it('renders the Director-only event read capability from the backend manifest', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      resolved_agent_tool_manifests: {
        interactive_director: [
          resolvedTool('event_read', 'agents.tool.eventRead.title', ['read'], true, false, 'runtime_check'),
        ],
      },
    }))

    render(<AgentsView />)

    await user.click(await screen.findByRole('button', { name: /后台导演 Agent/ }))
    expect(await screen.findByText('读取事件卡')).toBeInTheDocument()
    expect(screen.getByText('read')).toBeInTheDocument()
  })

  it('deletes a reset Agent tool override before persisting it', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      user: { agent_tools: { ide: { shell: false } } },
      effective: { agent_tools: { ide: { shell: false } } },
      resolved_agent_tool_manifests: {
        ide: [resolvedTool('shell', 'agents.tool.shell.title', ['bash'])],
      },
    }))

    render(<AgentsView />)

    const title = await screen.findByText('Shell 命令')
    const row = title.closest<HTMLElement>('.min-h-16')
    expect(row).toBeTruthy()
    expect(within(row as HTMLElement).getByRole('switch', { name: 'Shell 命令' })).not.toBeChecked()
    await user.click(within(row as HTMLElement).getByRole('button', { name: '覆盖' }))
    expect(within(row as HTMLElement).getByText('继承')).toBeInTheDocument()
    flushAgentsAutosave()

    await waitFor(() => expect(vi.mocked(updateUserSettings)).toHaveBeenCalled())
    const submitted = vi.mocked(updateUserSettings).mock.calls.at(-1)?.[0]
    expect(submitted?.agent_tools?.ide).toEqual({})
    expect(JSON.stringify(submitted)).not.toContain('"shell":null')
  })

  it('keeps the tool section empty until the backend manifest is loaded', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      resolved_agent_tool_manifests: undefined,
    }))

    render(<AgentsView />)

    await screen.findByText('模型与思考')
    expect(screen.queryByText('Shell 命令')).not.toBeInTheDocument()
    expect(screen.queryByText('任务清单')).not.toBeInTheDocument()
  })

  it('shows inherited empty thinking as the model default level', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))

    render(<AgentsView />)

    await screen.findByText('模型与思考')
    const thinkingLevel = screen.getByRole('combobox', { name: '思考强度' })
    expect(thinkingLevel).toHaveTextContent('模型默认')
  })

  it('shows SubAgent thinking level as inherited from the parent model', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      effective: {
        agent_models: {
          default: { thinking_level: 'medium' },
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
    expect(within(dialog).getByRole('combobox', { name: '思考强度' })).toHaveTextContent('继承')
  })

  it('shows SubAgent tools capped by the parent manifest and counts only explicit restrictions', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      resolved_agent_tool_manifests: {
        ide: [
          resolvedTool('workspace_read', 'agents.tool.workspaceRead.title', ['read', 'glob', 'grep'], false),
          resolvedTool('shell', 'agents.tool.shell.title', ['bash']),
        ],
      },
      effective: {
        sub_agents: [{
          id: 'reviewer',
          name: 'Reviewer',
          description: 'Reviews drafts.',
          system_prompt: 'Review only.',
          parents: ['ide'],
          enabled: true,
          tools: {
            workspace_read: true,
            shell: false,
          },
        }],
      },
    }))

    render(<AgentsView />)

    const reviewer = await screen.findByText('Reviewer')
    const row = reviewer.closest('[data-subagent-id]')
    expect(row).toBeTruthy()
    expect(within(row as HTMLElement).getByText('已限制 1 项工具')).toBeInTheDocument()
    await user.click(within(row as HTMLElement).getByRole('button', { name: '编辑 SubAgent' }))

    const dialog = screen.getByRole('dialog')
    const parentDenied = within(dialog).getByRole('switch', { name: '读取与搜索工作区' })
    expect(parentDenied).not.toBeChecked()
    expect(parentDenied).toBeDisabled()
    const explicitRestriction = within(dialog).getByRole('switch', { name: 'Shell 命令' })
    expect(explicitRestriction).not.toBeChecked()
    expect(explicitRestriction).not.toBeDisabled()
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
    flushAgentsAutosave()

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
    flushAgentsAutosave()

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        sub_agents: [expect.objectContaining({
          id: 'reviewer',
          enabled: true,
          parents: [],
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
    await user.click(screen.getByRole('button', { name: '仅从当前父 Agent 移除' }))

    await waitFor(() => {
      expect(screen.queryByText('Reviewer')).not.toBeInTheDocument()
    })

    flushAgentsAutosave()

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
        sub_agents: [expect.objectContaining({
          id: 'reviewer',
          enabled: true,
          parents: [],
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
    await user.click(screen.getByRole('button', { name: '配置管理 Agent资料库、方案预设、Skills 与自动化管理' }))

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
    flushAgentsAutosave()

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

  it('inherits and saves tool parallelism in user and workspace layers', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({
      default: { agent_tool_parallelism: 8 },
      user: { agent_tool_parallelism: 4 },
      workspace: {},
      effective: { agent_tool_parallelism: 4 },
    }))

    render(<AgentsView />)

    const userInput = await screen.findByRole('spinbutton', { name: '只读工具并发数（1–64）' })
    expect(userInput).toHaveValue(4)
    await user.click(screen.getByRole('button', { name: '当前工作区' }))
    const workspaceInput = screen.getByRole('spinbutton', { name: '只读工具并发数（1–64）' })
    expect(workspaceInput).toHaveValue(null)
    expect(workspaceInput).toHaveAttribute('placeholder', '4')
    fireEvent.change(workspaceInput, { target: { value: '12' } })
    flushAgentsAutosave()

    await waitFor(() => {
      expect(vi.mocked(updateWorkspaceSettings)).toHaveBeenCalledWith(expect.objectContaining({
        agent_tool_parallelism: 12,
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
    const doneButton = within(dialog).getByRole('button', { name: '完成' })
    expect(doneButton.parentElement).toHaveClass('mx-0', 'mb-0')
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

    fireEvent.click(doneButton)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1100)
    })

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByText('Researcher')).not.toBeInTheDocument()
    expect(vi.mocked(updateUserSettings)).toHaveBeenCalledWith(expect.objectContaining({
      sub_agents: [expect.objectContaining({
        id: 'subagent-1',
        name: 'Researcher',
        parents: [],
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
    await user.click(screen.getByRole('button', { name: '全部删除' }))
    flushAgentsAutosave()

    await waitFor(() => {
      expect(vi.mocked(updateUserSettings)).toHaveBeenLastCalledWith(expect.objectContaining({
        sub_agents: [],
      }))
    })
  })

  it('defaults General SubAgent to writing and automation only', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchSettings).mockResolvedValue(settingsSnapshot({}))

    render(<AgentsView />)

    expect(await screen.findByLabelText('通用 SubAgent 启用状态')).toBeChecked()

    await user.click(screen.getByRole('button', { name: /游戏叙事 Agent/ }))
    await waitFor(() => {
      expect(screen.getByLabelText('通用 SubAgent 启用状态')).not.toBeChecked()
    })

    await user.click(screen.getByRole('button', { name: /自动化Agent/ }))
    await waitFor(() => {
      expect(screen.getByLabelText('通用 SubAgent 启用状态')).toBeChecked()
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
    expect(generalSwitch).toBeChecked()
    await user.click(generalSwitch)
    expect(generalSwitch).not.toBeChecked()
    flushAgentsAutosave()

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
        denova_dir: '/denova',
        nova_dir: '/nova',
        user_config: '/nova/config.toml',
        workspace_config: '/books/demo/.nova/config.toml',
      },
    }))

    render(<AgentsView />)

    await screen.findByText('模型与思考')
    expect(screen.getByRole('separator', { name: '调整侧边栏宽度' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: '用配置管理 Agent 调整' }))

    expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    expect(screen.getByRole('separator', { name: '调整右侧面板宽度' })).toBeVisible()
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

function flushAgentsAutosave() {
  fireEvent.keyDown(screen.getByRole('heading', { level: 2, name: 'Agents' }), { key: 's', ctrlKey: true })
}

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
      workspace_config: '/books/demo/.nova/config.toml',
    },
    builtin_agent_prompts: {},
    builtin_agent_prompt_blocks: {},
    builtin_agent_prompt_sources: {},
    resolved_agent_tool_manifests: defaultToolManifests(),
    resolved_agent_contexts: defaultAgentContexts(),
    ...patch,
  }
}

function defaultAgentContexts(): NonNullable<LayeredSettings['resolved_agent_contexts']> {
  const base: ResolvedAgentContextSettings = {
    compaction_enabled: true,
    compaction_threshold: 0.85,
    tool_result_context_enabled: false,
    max_fragment_bytes: 256 * 1024,
    max_total_injected_bytes: 1024 * 1024,
    max_fragments: 256,
    max_metadata_field_bytes: 4 * 1024,
    max_provider_input_bytes: 4 * 1024 * 1024,
  }
  return {
    ide: { ...base, tool_result_context_enabled: true },
    interactive_story: { ...base, tool_result_context_enabled: true },
    config_manager: { ...base },
    interactive_director: { ...base },
    version_summary: { ...base },
    tool_agent: { ...base },
    image: { ...base },
    automation: { ...base },
    context_compaction: { ...base },
  }
}

function defaultToolManifests(): NonNullable<LayeredSettings['resolved_agent_tool_manifests']> {
  return {
    ide: [
      resolvedTool('workspace_read', 'agents.tool.workspaceRead.title', ['read', 'glob', 'grep']),
      resolvedTool('shell', 'agents.tool.shell.title', ['bash']),
      resolvedTool('todo', 'agents.tool.todo.title', ['todo'], true, false),
      resolvedTool('skills', 'agents.tool.skills.title', ['skill', 'read']),
    ],
    interactive_story: [
      resolvedTool('workspace_read', 'agents.tool.workspaceRead.title', ['read', 'glob', 'grep']),
      resolvedTool('skills', 'agents.tool.skills.title', ['skill', 'read']),
    ],
    interactive_director: [
      resolvedTool('event_read', 'agents.tool.eventRead.title', ['read'], true, false, 'runtime_check'),
    ],
    config_manager: [
      resolvedTool('workspace_read', 'agents.tool.workspaceRead.title', ['read', 'glob', 'grep']),
      resolvedTool('skills', 'agents.tool.skills.title', ['skill', 'read']),
    ],
    automation: [
      resolvedTool('workspace_read', 'agents.tool.workspaceRead.title', ['read', 'glob', 'grep']),
      resolvedTool('skills', 'agents.tool.skills.title', ['skill', 'read']),
    ],
  }
}

function resolvedTool(
  capability: AgentToolCapability,
  titleKey: string,
  toolNames: string[],
  allowed = true,
  availableToSubAgents = true,
  availability: ResolvedAgentToolCapability['availability'] = allowed ? 'available' : 'unavailable',
): ResolvedAgentToolCapability {
  return {
    capability,
    title_key: titleKey,
    description_key: `${titleKey.replace(/\.title$/, '')}.subtitle`,
    tool_names: toolNames,
    allowed,
    availability,
    unavailable_reason_key: availability === 'unavailable' ? 'agents.tool.unavailable.disabledByPolicy' : undefined,
    available_to_subagents: availableToSubAgents,
    descriptor: {
      execution: 'parallel_read',
      mutation_scope: 'none',
      post_check: 'none',
      recovery: 'read_only',
      result_projection: 'bounded_model_context',
      result_retention: 'receipt',
      steering: 'finish_current',
    },
  }
}
