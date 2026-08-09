import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings } from './api'
import { modelProfilesForEditor, SettingsView, UpdatePanel } from './SettingsView'
import { MODEL_PROTOCOL_CHAT_COMPLETIONS, MODEL_PROVIDER_OPENAI, modelProfilesWithDefault } from './model-profiles'
import { terminalCommandsForEditor } from './TerminalCommandsEditor'
import type { LayeredSettings, UpdateCheckResult, UpdateInstallResult } from './types'

const { fetchSettingsMock, pingImageProfile, updateUserSettings } = vi.hoisted(() => ({
  fetchSettingsMock: vi.fn(),
  pingImageProfile: vi.fn(),
  updateUserSettings: vi.fn(),
}))

vi.mock('./api', () => {
  return {
    GLOBAL_SETTINGS_TARGET: { kind: 'global' },
    applyUpdate: vi.fn(),
    checkForUpdate: vi.fn(),
    fetchModelCatalog: vi.fn().mockResolvedValue({ providers: [], protocols: [] }),
    fetchSettings: fetchSettingsMock,
    fetchSettingsTarget: fetchSettingsMock,
    refreshSettings: fetchSettingsMock,
    refreshSettingsTarget: fetchSettingsMock,
    installUpdateStream: vi.fn(),
    pingImageProfile,
    pingModelProfile: vi.fn(),
    revokeAgentApprovalRule: vi.fn(),
    patchSettings: (_layer: string, changes: unknown, revision?: string) => revision === undefined
      ? updateUserSettings(changes)
      : updateUserSettings(changes, revision),
    patchSettingsTarget: (_target: unknown, _layer: string, changes: unknown, revision?: string) => revision === undefined
      ? updateUserSettings(changes)
      : updateUserSettings(changes, revision),
    createSettingsMergePatch: (_baseline: unknown, draft: unknown) => draft,
  }
})

vi.mock('@/features/interactive/api', () => ({
  getInteractiveTellers: vi.fn().mockResolvedValue([]),
}))

afterEach(() => {
  vi.useRealTimers()
})

describe('modelProfilesForEditor', () => {
  it('keeps a newly added blank language model profile visible before the model name is filled', () => {
    const profiles = modelProfilesForEditor({
      model_profiles: [
        { id: 'default', base_url: 'https://api.example.com/v1', model: 'gpt-4.1', context_window_tokens: 400000 },
        { context_window_tokens: 400000 },
      ],
    }, {
      openai_base_url: 'https://api.example.com/v1',
      openai_model: 'gpt-4.1',
      openai_context_window_tokens: 400000,
    })

    expect(profiles).toHaveLength(2)
    expect(profiles[1]).toEqual({ context_window_tokens: 400000 })
  })

  it('keeps an alias-only language model draft visible until it gets a model id', () => {
    const profiles = modelProfilesForEditor({
      model_profiles: [
        { id: 'default', model: 'gpt-4.1' },
        { name: 'Draft model', context_window_tokens: 400000 },
      ],
    }, {})

    expect(profiles).toHaveLength(2)
    expect(profiles[1]).toEqual({ name: 'Draft model', context_window_tokens: 400000 })
  })

  it('keeps legacy endpoints on Chat Completions but delegates explicit provider defaults to the registry', () => {
    const legacy = modelProfilesWithDefault({
      openai_base_url: 'https://api.openai.com/v1',
      openai_model: 'gpt-4.1',
    })
    expect(legacy[0]).toMatchObject({
      provider: MODEL_PROVIDER_OPENAI,
      protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
    })

    const explicit = modelProfilesWithDefault({
      openai_base_url: 'https://api.deepseek.com',
      openai_model: 'deepseek-chat',
      model_profiles: [{ id: 'default', provider: MODEL_PROVIDER_OPENAI, model: 'gpt-5' }],
    })
    expect(explicit[0]).toMatchObject({
      provider: MODEL_PROVIDER_OPENAI,
      protocol: '',
      base_url: '',
    })
  })
})

describe('terminalCommandsForEditor', () => {
  it('uses the effective presets until the user owns a registry, then preserves the user order', () => {
    const effective = {
      terminal_commands: [
        { id: 'codex', name: 'Codex CLI', command: 'codex', enabled: true },
        { id: 'claude', name: 'Claude Code', command: 'claude', enabled: true },
      ],
    }
    expect(terminalCommandsForEditor({}, effective).map((command) => command.id)).toEqual(['codex', 'claude'])

    const draft = {
      terminal_commands: [
        { id: 'aider', name: 'Aider', command: 'aider', enabled: true },
        { id: 'codex', name: 'Codex CLI', command: 'codex', enabled: false },
      ],
    }
    const commands = terminalCommandsForEditor(draft, effective)
    expect(commands.map((command) => command.id)).toEqual(['aider', 'codex'])
    commands[0].name = 'Changed locally'
    expect(draft.terminal_commands[0].name).toBe('Aider')
  })
})

describe('UpdatePanel', () => {
  it('shows restart install action after an update is staged', () => {
    const onApply = vi.fn()
    render(
      <UpdatePanel
        status={updateStatus()}
        installResult={stagedInstallResult()}
        applyResult={null}
        installProgress={{ phase: 'staged', percent: 100 }}
        checking={false}
        installing={false}
        applying={false}
        error={null}
        onCheck={() => undefined}
        onInstall={() => undefined}
        onApply={onApply}
      />,
    )

    expect(screen.getByText('更新已暂存。点击“重启并安装”后，Denova 会退出、替换文件并自动启动新版本。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '安装更新' })).toBeDisabled()
    const applyButton = screen.getByRole('button', { name: '重启并安装' })
    expect(applyButton).toBeEnabled()
    fireEvent.click(applyButton)
    expect(onApply).toHaveBeenCalledTimes(1)
  })

  it('locks update actions while Denova is restarting to apply the update', () => {
    render(
      <UpdatePanel
        status={updateStatus()}
        installResult={stagedInstallResult()}
        applyResult={{ status: 'restarting', version: '0.2.0' }}
        installProgress={{ phase: 'staged', percent: 100 }}
        checking={false}
        installing={false}
        applying={false}
        error={null}
        onCheck={() => undefined}
        onInstall={() => undefined}
        onApply={() => undefined}
      />,
    )

    expect(screen.getByText('Denova 正在重启并应用更新。新版本可用后页面会自动刷新。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '检查更新' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '重启并安装' })).toBeDisabled()
  })
})

describe('SettingsView debug section', () => {
  beforeEach(() => {
    vi.mocked(fetchSettings).mockReset()
  })

  it('hides debug settings outside dev mode', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(layeredSettings({ devMode: false }))

    render(<SettingsView />)

    expect(await screen.findAllByText('设置')).not.toHaveLength(0)
    expect(screen.queryByText('调试')).not.toBeInTheDocument()
    expect(screen.queryByText('记录完整 LLM 输入')).not.toBeInTheDocument()
  })

  it('shows llm input log toggle in dev mode', async () => {
    vi.mocked(fetchSettings).mockResolvedValue(layeredSettings({ devMode: true }))

    render(<SettingsView />)

    expect(await screen.findAllByText('调试')).not.toHaveLength(0)
    expect(screen.getByText('记录完整 LLM 输入')).toBeInTheDocument()
  })
})

describe('SettingsView user scope', () => {
  beforeEach(() => {
    vi.mocked(fetchSettings).mockReset()
    vi.mocked(pingImageProfile).mockReset()
    vi.mocked(updateUserSettings).mockReset()
  })

  it('shows one user settings surface and persists every section to the user config', async () => {
    const settings = layeredSettings({ devMode: false })
    settings.user = { version_timed_interval_minutes: 10 }
    settings.effective = { ...settings.effective, version_timed_interval_minutes: 10 }
    vi.mocked(fetchSettings).mockResolvedValue(settings)
    vi.mocked(updateUserSettings).mockResolvedValue(settings)

    render(<SettingsView />)

    expect(await screen.findAllByText('设置')).not.toHaveLength(0)
    expect(screen.queryByRole('button', { name: '用户配置' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '当前工作区' })).not.toBeInTheDocument()
    expect(screen.queryByText('工作区配置文件')).not.toBeInTheDocument()
    expect(screen.getByText('默认叙事')).toBeInTheDocument()
    expect(screen.getByText('自动创建 Git 版本')).toBeInTheDocument()
    expect(screen.queryByText('Agent 大量输出自动保存')).not.toBeInTheDocument()
    expect(screen.getByText('故事舞台行间距')).toBeInTheDocument()
    expect(screen.getAllByText('网页访问').length).toBeGreaterThan(0)
    expect(screen.getByLabelText('SearXNG 实例地址')).toBeInTheDocument()
    expect(screen.getByLabelText('单次搜索结果上限')).toHaveAttribute('max', '20')
    expect(screen.getByLabelText('搜索服务超时（秒，0 为不限制）')).toHaveAttribute('min', '0')
    expect(screen.getByLabelText('单次正文字符上限')).toHaveAttribute('max', '262144')
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()

    vi.useFakeTimers()
    const intervalInput = screen.getByLabelText('自动版本最小间隔（分钟）')
    expect(intervalInput).toHaveAttribute('min', '1')
    fireEvent.change(intervalInput, { target: { value: '20' } })
    expect(screen.getByRole('status')).toHaveTextContent('等待自动保存')
    await act(async () => { await vi.advanceTimersByTimeAsync(1100) })

    expect(updateUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({ version_timed_interval_minutes: 20 }),
      'user-rev',
    )
  })

  it('edits preset CLI shortcuts and persists new commands through the shared registry', async () => {
    const settings = layeredSettings({ devMode: false })
    vi.mocked(fetchSettings).mockResolvedValue(settings)
    vi.mocked(updateUserSettings).mockResolvedValue(settings)

    render(<SettingsView />)

    expect(await screen.findByText('CLI 快捷命令')).toBeInTheDocument()
    expect(await screen.findByText('Codex CLI')).toBeInTheDocument()
    expect(screen.getByText('Claude Code')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '删除“Codex CLI”' })).toBeInTheDocument()

    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: '添加 CLI 快捷命令' }))
    const nameInputs = screen.getAllByLabelText('显示名称')
    const commandInputs = screen.getAllByLabelText('启动命令')
    fireEvent.change(nameInputs.at(-1)!, { target: { value: 'Aider' } })
    fireEvent.change(commandInputs.at(-1)!, { target: { value: 'aider --model sonnet' } })
    fireEvent.click(screen.getByRole('switch', { name: '启用或停用“Aider”' }))

    expect(screen.getByRole('button', { name: '删除“Aider”' })).toBeInTheDocument()
    await act(async () => { await vi.advanceTimersByTimeAsync(1100) })

    expect(updateUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        terminal_commands: expect.arrayContaining([
          expect.objectContaining({ name: 'Aider', command: 'aider --model sonnet', enabled: true }),
        ]),
      }),
      'user-rev',
    )
    vi.useRealTimers()
  })

  it('tests an image model connection with the current unsaved profile', async () => {
    const settings = layeredSettings({ devMode: false })
    settings.user = {
      image_api_profiles: [{
        id: 'flux', name: 'Flux Pro', provider: 'openai', openai_base_url: 'https://images.example.com/v1',
        openai_model: 'flux-pro', default_quality: 'high',
      }],
    }
    settings.effective = { ...settings.effective, ...settings.user }
    vi.mocked(fetchSettings).mockResolvedValue(settings)
    vi.mocked(pingImageProfile).mockResolvedValue({
      ok: true, latency_ms: 42, profile_id: 'flux', provider: 'openai',
      base_url: 'https://images.example.com/v1', model: 'flux-pro',
    })

    render(<SettingsView />)

    const fluxTitle = await screen.findByText('Flux Pro')
    const fluxCard = fluxTitle.parentElement?.parentElement?.parentElement
    expect(fluxCard).toBeTruthy()
    await act(async () => { fireEvent.click(within(fluxCard as HTMLElement).getByRole('button', { name: '测试连接' })) })

    await waitFor(() => expect(pingImageProfile).toHaveBeenCalledWith(expect.objectContaining({
      id: 'flux', openai_model: 'flux-pro', default_quality: 'high',
    }), expect.any(AbortSignal)))
    expect(await screen.findByText('连接成功 · openai · 42 ms')).toBeInTheDocument()
  })

  it('persists an explicitly empty terminal command registry after removing every preset', async () => {
    const settings = layeredSettings({ devMode: false })
    vi.mocked(fetchSettings).mockResolvedValue(settings)
    vi.mocked(updateUserSettings).mockResolvedValue(settings)

    render(<SettingsView />)

    expect(await screen.findByText('CLI 快捷命令')).toBeInTheDocument()
    const deleteCodex = await screen.findByRole('button', { name: '删除“Codex CLI”' })
    vi.useFakeTimers()
    fireEvent.click(deleteCodex)
    fireEvent.click(screen.getByRole('button', { name: '删除“Claude Code”' }))

    expect(screen.getByText('还没有 CLI 快捷命令。添加后即可从新建菜单快速启动。')).toBeInTheDocument()
    await act(async () => { await vi.advanceTimersByTimeAsync(1100) })

    expect(updateUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({ terminal_commands: [] }),
      'user-rev',
    )
  })
})

function updateStatus(): UpdateCheckResult {
  return {
    current_version: '0.1.0',
    latest_version: '0.2.0',
    update_available: true,
    can_install: true,
    platform: 'darwin-arm64',
    release_url: 'https://example.com/release',
    published_at: new Date().toISOString(),
  }
}

function stagedInstallResult(): UpdateInstallResult {
  return {
    previous_version: '0.1.0',
    installed_version: '0.2.0',
    status: 'staged',
    installed: false,
    staged: true,
    apply_ready: true,
    restart_required: true,
    staged_path: '/tmp/nova/.nova-updates/pending-0.2.0/nova',
  }
}

function layeredSettings({ devMode }: { devMode: boolean }): LayeredSettings {
  const settings = {
    language: 'zh-CN',
    theme: 'dark',
    update_check_enabled: false,
    llm_input_log_enabled: false,
    terminal_enabled: true,
    terminal_commands: [
      { id: 'codex', name: 'Codex CLI', command: 'codex', enabled: true },
      { id: 'claude', name: 'Claude Code', command: 'claude', enabled: true },
    ],
  }
  return {
    default: settings,
    global: {},
    user: {},
    workspace: {},
    effective: settings,
    resolved_agent_tool_manifests: {},
    resolved_agent_contexts: {},
    paths: {
      denova_dir: '/tmp/denova',
      nova_dir: '/tmp/nova',
      user_config: '/tmp/nova/config.toml',
      workspace_config: '/tmp/book/.nova/config.toml',
    },
    runtime: {
      goos: 'darwin',
      dev_mode: devMode,
    },
    revisions: {
      user: 'user-rev',
      workspace: 'workspace-rev',
    },
  }
}
