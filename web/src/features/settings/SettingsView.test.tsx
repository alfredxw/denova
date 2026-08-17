import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings, updateUserSettings } from './api'
import { ImageAPIProfilesEditor, ModelProfilesEditor, imageAPIProfilesForEditor, modelProfilesForEditor, SettingsView, UpdatePanel, withRenamedImageAPIProfile, withRenamedLanguageModelProfile } from './SettingsView'
import type { LayeredSettings, UpdateCheckResult, UpdateInstallResult } from './types'

vi.mock('./api', () => ({
  applyUpdate: vi.fn(),
  checkForUpdate: vi.fn(),
  fetchSettings: vi.fn(),
  installUpdateStream: vi.fn(),
  updateUserSettings: vi.fn(),
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

  it('keeps an autosaved rename draft while hiding its inherited source profile', () => {
    const profiles = modelProfilesForEditor({
      model_profiles: [{ rename_from_id: 'old-model' }],
    }, {
      model_profiles: [{ id: 'old-model', openai_model: 'old-model' }],
    })

    expect(profiles.some((profile) => profile.id === 'old-model')).toBe(false)
    expect(profiles.some((profile) => profile.rename_from_id === 'old-model')).toBe(true)
  })
})

describe('imageAPIProfilesForEditor', () => {
  it('keeps an autosaved rename draft while hiding its inherited source profile', () => {
    const profiles = imageAPIProfilesForEditor({
      image_api_profiles: [{ rename_from_id: 'old-image-model' }],
    }, {
      image_api_profiles: [{ id: 'old-image-model', openai_model: 'old-image-model' }],
    })

    expect(profiles.some((profile) => profile.id === 'old-image-model')).toBe(false)
    expect(profiles.some((profile) => profile.rename_from_id === 'old-image-model')).toBe(true)
  })
})

describe('ImageAPIProfilesEditor', () => {
  it('keeps the selected default profile in sync when a model-derived id changes', () => {
    const onChange = vi.fn()
    const onDefaultProfileChange = vi.fn()
    const onProfileRename = vi.fn()
    render(
      <ImageAPIProfilesEditor
        profiles={[{ id: 'old-image-model', openai_model: 'old-image-model' }]}
        effectiveProfiles={[]}
        defaultProfileID="old-image-model"
        effectiveDefaultProfileID="old-image-model"
        onChange={onChange}
        onDefaultProfileChange={onDefaultProfileChange}
        onProfileRename={onProfileRename}
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('gpt-image-1'), { target: { value: 'new-image-model' } })

    expect(onChange).toHaveBeenCalledWith([{ id: 'new-image-model', openai_model: 'new-image-model' }])
    expect(onDefaultProfileChange).toHaveBeenCalledWith('new-image-model')
    expect(onProfileRename).toHaveBeenCalledWith('old-image-model', 'new-image-model')
  })

  it('remembers the derived id when the model is cleared before replacement', () => {
    const onChange = vi.fn()
    const onProfileRename = vi.fn()
    const view = render(
      <ImageAPIProfilesEditor
        profiles={[{ id: 'old-image-model', openai_model: 'old-image-model' }]}
        effectiveProfiles={[]}
        defaultProfileID="old-image-model"
        effectiveDefaultProfileID="old-image-model"
        onChange={onChange}
        onDefaultProfileChange={() => undefined}
        onProfileRename={onProfileRename}
      />,
    )
    const modelInput = screen.getByDisplayValue('old-image-model')
    fireEvent.change(modelInput, { target: { value: '' } })
    expect(onChange).toHaveBeenCalledWith([{
      id: '',
      rename_from_id: 'old-image-model',
      openai_model: '',
    }])
    view.unmount()
    render(
      <ImageAPIProfilesEditor
        profiles={[{ rename_from_id: 'old-image-model' }]}
        effectiveProfiles={[]}
        defaultProfileID="old-image-model"
        effectiveDefaultProfileID="old-image-model"
        onChange={() => undefined}
        onDefaultProfileChange={() => undefined}
        onProfileRename={onProfileRename}
      />,
    )
    fireEvent.change(screen.getByPlaceholderText('gpt-image-1'), { target: { value: 'new-image-model' } })

    expect(onProfileRename).toHaveBeenCalledWith('old-image-model', 'new-image-model')
  })
})

describe('ModelProfilesEditor', () => {
  it('remembers the derived id when the model is cleared before replacement', () => {
    const onChange = vi.fn()
    const onProfileRename = vi.fn()
    const view = render(
      <ModelProfilesEditor
        profiles={[{ id: 'old-model', openai_model: 'old-model' }]}
        effectiveProfiles={[]}
        onChange={onChange}
        onProfileRename={onProfileRename}
      />,
    )
    const modelInput = screen.getByDisplayValue('old-model')
    fireEvent.change(modelInput, { target: { value: '' } })
    expect(onChange).toHaveBeenCalledWith([{
      id: '',
      rename_from_id: 'old-model',
      openai_model: '',
    }])
    view.unmount()
    render(
      <ModelProfilesEditor
        profiles={[{ rename_from_id: 'old-model' }]}
        effectiveProfiles={[]}
        onChange={() => undefined}
        onProfileRename={onProfileRename}
      />,
    )
    fireEvent.change(screen.getByPlaceholderText('模型名，如 gpt-4.1'), { target: { value: 'new-model' } })

    expect(onProfileRename).toHaveBeenCalledWith('old-model', 'new-model')
  })
})

describe('model profile rename references', () => {
  it('records a language profile alias and migrates Agent and SubAgent references', () => {
    const settings = withRenamedLanguageModelProfile({
      agent_models: { ide: { profile_id: 'old-model' } },
      sub_agents: [{ id: 'reviewer', model: { profile_id: 'old-model' } }],
    }, 'old-model', 'new-model')

    expect(settings.model_profile_aliases).toEqual({ 'old-model': 'new-model' })
    expect(settings.agent_models?.ide?.profile_id).toBe('new-model')
    expect(settings.sub_agents?.[0].model?.profile_id).toBe('new-model')
  })

  it('chains image aliases and keeps the default reference on the latest id', () => {
    const first = withRenamedImageAPIProfile({ default_image_api_profile_id: 'old-image' }, 'old-image', 'next-image')
    const second = withRenamedImageAPIProfile(first, 'next-image', 'final-image')

    expect(second.image_api_profile_aliases).toEqual({
      'old-image': 'final-image',
      'next-image': 'final-image',
    })
    expect(second.default_image_api_profile_id).toBe('final-image')
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
