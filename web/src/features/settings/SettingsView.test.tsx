import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings } from './api'
import { modelProfilesForEditor, SettingsView, UpdatePanel } from './SettingsView'
import type { LayeredSettings, UpdateCheckResult, UpdateInstallResult } from './types'

vi.mock('./api', () => ({
  applyUpdate: vi.fn(),
  checkForUpdate: vi.fn(),
  fetchSettings: vi.fn(),
  installUpdateStream: vi.fn(),
  updateUserSettings: vi.fn(),
  updateWorkspaceSettings: vi.fn(),
}))

vi.mock('@/features/interactive/api', () => ({
  getInteractiveTellers: vi.fn().mockResolvedValue([]),
}))

describe('modelProfilesForEditor', () => {
  it('keeps a newly added blank language model profile visible before the model name is filled', () => {
    const profiles = modelProfilesForEditor({
      model_profiles: [
        { id: 'default', openai_base_url: 'https://api.example.com/v1', openai_model: 'gpt-4.1', context_window_tokens: 400000 },
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
        { id: 'default', openai_model: 'gpt-4.1' },
        { name: 'Draft model', context_window_tokens: 400000 },
      ],
    }, {})

    expect(profiles).toHaveLength(2)
    expect(profiles[1]).toEqual({ name: 'Draft model', context_window_tokens: 400000 })
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

    expect(screen.getByText('更新已暂存。点击“重启并安装”后，Nova 会退出、替换文件并自动启动新版本。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '安装更新' })).toBeDisabled()
    const applyButton = screen.getByRole('button', { name: '重启并安装' })
    expect(applyButton).toBeEnabled()
    fireEvent.click(applyButton)
    expect(onApply).toHaveBeenCalledTimes(1)
  })

  it('locks update actions while Nova is restarting to apply the update', () => {
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

    expect(screen.getByText('Nova 正在重启并应用更新。')).toBeInTheDocument()
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
  }
  return {
    default: settings,
    global: {},
    user: {},
    workspace: {},
    effective: settings,
    paths: {
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
